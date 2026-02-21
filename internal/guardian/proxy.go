// Package guardian implements the HTTP CONNECT proxy and related services
// that mediate network access for cloister containers.
package guardian

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/xdg/cloister/internal/clog"
)

// DefaultProxyPort is the standard port for HTTP CONNECT proxies.
const DefaultProxyPort = 3128

// Timeout constants for tunnel connections.
const (
	// dialTimeout is the maximum time to establish a connection to the upstream server.
	dialTimeout = 30 * time.Second
	// idleTimeout is the maximum time a tunnel connection can be idle before being closed.
	idleTimeout = 5 * time.Minute
)

// TokenValidator validates authentication tokens for proxy requests.
// Implementations should be thread-safe.
type TokenValidator interface {
	// Validate checks if a token is valid and returns true if so.
	Validate(token string) bool
}

// DomainApprovalResult represents the result of a domain approval request.
type DomainApprovalResult struct {
	Approved bool
	Scope    string // "session", "project", or "global"
}

// DomainApprover requests human approval for unlisted domains.
// Implementations should block until approval/denial/timeout (typically 60s).
// The token parameter is used for session allowlist updates (token-based isolation),
// while project is used for the approval queue/UI display.
type DomainApprover interface {
	RequestApproval(project, cloister, domain, token string) (DomainApprovalResult, error)
}

// TunnelHandler handles the upstream connection after the proxy decides
// to allow a CONNECT request. The proxy calls ServeTunnel only when the
// domain passes all deny/allow/approval checks.
type TunnelHandler interface {
	ServeTunnel(w http.ResponseWriter, r *http.Request, targetHostPort string)
}

// ProxyServer is an HTTP CONNECT proxy that enforces domain allowlists
// for cloister containers.
type ProxyServer struct {
	// Addr is the address to listen on (e.g., ":3128").
	Addr string

	// PolicyEngine handles all domain access control. It evaluates domain access
	// using a deny-first, then allow, then fallback-to-AskHuman strategy.
	PolicyEngine PolicyChecker

	// TokenValidator validates authentication tokens. If nil, all requests are allowed
	// (useful for testing). When set, requests must include a valid Proxy-Authorization
	// header with Basic auth where the password is the token.
	TokenValidator TokenValidator

	// TokenLookup provides token-to-project mapping for per-project allowlists.
	// If nil, the global Allowlist is used for all requests.
	TokenLookup TokenLookupFunc

	// DomainApprover requests human approval for unlisted domains. If nil, unlisted
	// domains are immediately rejected with 403 (preserving current behavior).
	DomainApprover DomainApprover

	// TunnelHandler optionally handles tunnel establishment after domain approval.
	// If nil, the proxy uses its built-in dialAndTunnel method.
	TunnelHandler TunnelHandler

	// OnReload is an optional callback invoked after a successful SIGHUP-triggered
	// PolicyEngine reload. Use this to clear caches (e.g. PatternCache) that should
	// be invalidated when config changes. Called only when PolicyEngine is set.
	OnReload func()

	// OnTokenReload is an optional callback invoked during SIGHUP to reconcile
	// the in-memory token registry with on-disk token storage. This handles
	// tokens that were manually added or removed from disk.
	OnTokenReload func()

	// Transport is the HTTP transport used for forwarding plain HTTP requests.
	// If nil, a default transport with dialTimeout is created on first use.
	Transport *http.Transport

	server        *http.Server
	listener      net.Listener
	mu            sync.Mutex
	running       bool
	sighupChan    chan os.Signal
	stopSighup    chan struct{}
	transportOnce sync.Once
}

// NewProxyServer creates a new proxy server listening on the specified address.
// If addr is empty, it defaults to ":3128".
func NewProxyServer(addr string) *ProxyServer {
	if addr == "" {
		addr = fmt.Sprintf(":%d", DefaultProxyPort)
	}
	return &ProxyServer{
		Addr: addr,
	}
}

// Start begins accepting connections on the proxy server.
// It returns an error if the server is already running or fails to start.
func (p *ProxyServer) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return errors.New("proxy server already running")
	}

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", p.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", p.Addr, err)
	}

	p.listener = listener
	p.server = &http.Server{
		Handler:           http.HandlerFunc(p.handleRequest),
		ReadHeaderTimeout: 30 * time.Second,
	}
	p.running = true

	go func() {
		if err := p.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			clog.Warn("proxy server error: %v", err)
		}
	}()

	// Start SIGHUP handler if PolicyEngine is available.
	if p.PolicyEngine != nil {
		p.startSighupHandler()
	}

	return nil
}

// startSighupHandler sets up signal handling for SIGHUP to reload configuration.
func (p *ProxyServer) startSighupHandler() {
	p.sighupChan = make(chan os.Signal, 1)
	p.stopSighup = make(chan struct{})
	signal.Notify(p.sighupChan, syscall.SIGHUP)

	// Capture channels locally to avoid racing with Stop() which nils the fields.
	sighupChan := p.sighupChan
	stopChan := p.stopSighup

	go func() {
		for {
			select {
			case <-sighupChan:
				p.handleSighup()
			case <-stopChan:
				signal.Stop(sighupChan)
				return
			}
		}
	}()
}

// handleSighup is called when SIGHUP is received to reload configuration.
func (p *ProxyServer) handleSighup() {
	// Reconcile tokens with disk before reloading policies, so that
	// revoked tokens are removed and new tokens are picked up.
	if p.OnTokenReload != nil {
		p.OnTokenReload()
	}

	// PolicyEngine path: reload all policies from disk.
	if p.PolicyEngine != nil {
		if pe, ok := p.PolicyEngine.(*PolicyEngine); ok {
			if err := pe.ReloadAll(); err != nil {
				p.log("SIGHUP PolicyEngine reload failed: %v", err)
				return
			}
			p.log("SIGHUP PolicyEngine reloaded successfully")
			if p.OnReload != nil {
				p.OnReload()
			}
		} else {
			p.log("SIGHUP skipped: PolicyChecker does not support reload")
		}
		return
	}
}

// Stop gracefully shuts down the proxy server.
func (p *ProxyServer) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	p.running = false

	// Stop SIGHUP handler if running
	if p.stopSighup != nil {
		close(p.stopSighup)
	}

	if err := p.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown proxy server: %w", err)
	}
	return nil
}

// ListenAddr returns the actual address the server is listening on.
// This is useful when the server was started with port 0 (random port).
// Returns empty string if the server is not running.
func (p *ProxyServer) ListenAddr() string {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.listener == nil {
		return ""
	}
	return p.listener.Addr().String()
}

// handleRequest processes incoming HTTP requests.
// CONNECT requests are handled as TLS tunnels. Plain HTTP requests with
// absolute URIs (e.g. "GET http://example.com/ HTTP/1.1") are dispatched
// to the forward proxy handler. All other request forms return 405.
// Authentication is checked before processing if TokenValidator is set.
func (p *ProxyServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		// Check authentication if TokenValidator is set
		if p.TokenValidator != nil {
			if !p.authenticate(w, r) {
				return
			}
		}
		p.handleConnect(w, r)
		return
	}

	// Plain HTTP forward proxy request: absolute URI form
	if r.URL.Scheme == "http" && r.URL.Host != "" {
		p.handlePlainHTTP(w, r)
		return
	}

	http.Error(w, "Method Not Allowed - only CONNECT and plain HTTP forward proxy are supported", http.StatusMethodNotAllowed)
}

// handlePlainHTTP processes non-CONNECT requests sent in absolute-URI form
// (e.g. "GET http://example.com/path HTTP/1.1"). It applies the same
// authentication and domain policy checks as CONNECT, then forwards the
// request to the upstream server.
func (p *ProxyServer) handlePlainHTTP(w http.ResponseWriter, r *http.Request) {
	// Authenticate (same as CONNECT path)
	if p.TokenValidator != nil {
		if !p.authenticate(w, r) {
			return
		}
	}

	resolved := p.resolveRequest(r)
	domain := strings.ToLower(stripPort(r.URL.Host))

	if err := p.checkDomainAccess(domain, resolved); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	p.forwardHTTP(w, r)
}

// transport returns the HTTP transport for forwarding plain HTTP requests,
// lazily initializing it with proxy-appropriate timeouts if not already set.
func (p *ProxyServer) transport() *http.Transport {
	p.transportOnce.Do(func() {
		if p.Transport == nil {
			p.Transport = &http.Transport{
				DialContext: (&net.Dialer{
					Timeout: dialTimeout,
				}).DialContext,
				ResponseHeaderTimeout: 60 * time.Second,
			}
		}
	})
	return p.Transport
}

// forwardHTTP forwards a plain HTTP request to the upstream server and copies
// the response back to the client. It strips hop-by-hop headers, does not
// follow redirects, and does not set X-Forwarded-For.
func (p *ProxyServer) forwardHTTP(w http.ResponseWriter, r *http.Request) {
	// Clone the request for the outbound call
	outReq := r.Clone(r.Context())
	outReq.RequestURI = "" // Must be empty for http.Client/Transport

	// Hop-by-hop headers that must not be forwarded
	outReq.Header.Del("Proxy-Authorization")
	outReq.Header.Del("Proxy-Connection")

	// Remove headers listed in the Connection header, then Connection itself
	if connHeaders := outReq.Header.Get("Connection"); connHeaders != "" {
		for _, h := range strings.Split(connHeaders, ",") {
			outReq.Header.Del(strings.TrimSpace(h))
		}
		outReq.Header.Del("Connection")
	}

	outReq.Header.Del("TE")
	outReq.Header.Del("Transfer-Encoding")
	outReq.Header.Del("Keep-Alive")
	outReq.Header.Del("Trailer")
	outReq.Header.Del("Upgrade")

	// Execute the request using RoundTrip directly to avoid following redirects
	resp, err := p.transport().RoundTrip(outReq)
	if err != nil {
		clog.Warn("forwardHTTP: upstream request to %s failed: %v", outReq.URL.Host, err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	//nolint:errcheck // Best-effort copy to response writer; nothing useful to do on error
	io.Copy(w, resp.Body)
}

// authenticate checks the Proxy-Authorization header and validates the token.
// Returns true if authentication succeeds, false otherwise.
// On failure, it writes the appropriate 407 response.
func (p *ProxyServer) authenticate(w http.ResponseWriter, r *http.Request) bool {
	authHeader := r.Header.Get("Proxy-Authorization")
	if authHeader == "" {
		p.logAuthFailure(r, "missing Proxy-Authorization header")
		p.writeAuthRequired(w)
		return false
	}

	// Parse Basic auth: "Basic base64(username:password)"
	token, ok := p.parseBasicAuth(authHeader)
	if !ok {
		p.logAuthFailure(r, "invalid Proxy-Authorization header format")
		p.writeAuthRequired(w)
		return false
	}

	if !p.TokenValidator.Validate(token) {
		p.logAuthFailure(r, "invalid token")
		p.writeAuthRequired(w)
		return false
	}

	return true
}

// parseBasicAuth extracts the token (password) from a Basic auth header.
// The username is ignored as the token is passed as the password.
// Returns the token and true if parsing succeeded, empty string and false otherwise.
func (p *ProxyServer) parseBasicAuth(authHeader string) (string, bool) {
	const prefix = "Basic "
	if !strings.HasPrefix(authHeader, prefix) {
		return "", false
	}

	encoded := authHeader[len(prefix):]
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", false
	}

	// Format is "username:password", we only care about password (token)
	credentials := string(decoded)
	colonIdx := strings.Index(credentials, ":")
	if colonIdx < 0 {
		return "", false
	}

	// Password (token) is everything after the first colon
	token := credentials[colonIdx+1:]
	return token, true
}

// writeAuthRequired writes a 407 Proxy Authentication Required response.
func (p *ProxyServer) writeAuthRequired(w http.ResponseWriter) {
	w.Header().Set("Proxy-Authenticate", `Basic realm="cloister"`)
	http.Error(w, "Proxy Authentication Required", http.StatusProxyAuthRequired)
}

// logAuthFailure logs an authentication failure with the source IP.
func (p *ProxyServer) logAuthFailure(r *http.Request, reason string) {
	sourceIP := r.RemoteAddr
	clog.Warn("proxy auth failure from %s: %s", sourceIP, reason)
}

// log writes a formatted message to the proxy's logger.
func (p *ProxyServer) log(format string, args ...interface{}) {
	clog.Debug(format, args...)
}

// isTimeoutError checks if an error is a timeout error.
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	// Check for net.Error timeout
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}

// handleConnect processes CONNECT requests by evaluating the domain against
// deny/allow rules in strict precedence order, then tunneling if permitted.
//
// Evaluation order (via PolicyEngine.Check):
//  1. Deny pass: global -> project -> token (session)
//  2. Allow pass: global -> project -> token (session)
//  3. Fallback to AskHuman (human approval via web UI)
//
// Returns 403 Forbidden for denied/non-allowed domains, 502 Bad Gateway for
// upstream connection failures, 504 Gateway Timeout for upstream dial timeouts.
func (p *ProxyServer) handleConnect(w http.ResponseWriter, r *http.Request) {
	targetHostPort := r.Host
	domain := strings.ToLower(stripPort(targetHostPort))
	resolved := p.resolveRequest(r)

	clog.Debug("handleConnect: host=%s, domain=%s, project=%s, policyEngine=%v",
		targetHostPort, domain, resolved.ProjectName, p.PolicyEngine != nil)

	if err := p.checkDomainAccess(domain, resolved); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if p.TunnelHandler != nil {
		p.TunnelHandler.ServeTunnel(w, r, targetHostPort)
	} else {
		p.dialAndTunnel(w, r, targetHostPort)
	}
}

// checkDomainAccess evaluates deny/allow rules via PolicyEngine.
// Returns nil if the domain is allowed, or an error message if denied.
func (p *ProxyServer) checkDomainAccess(domain string, resolved resolvedRequest) error {
	if p.PolicyEngine != nil {
		decision := p.PolicyEngine.Check(resolved.Token, resolved.ProjectName, domain)
		switch decision {
		case Allow:
			return nil
		case Deny:
			return fmt.Errorf("forbidden - domain denied")
		case AskHuman:
			return p.requestDomainApproval(domain, resolved)
		default:
			return fmt.Errorf("forbidden - unknown policy decision")
		}
	}

	// No PolicyEngine configured: reject everything not approved.
	return p.requestDomainApproval(domain, resolved)
}

// requestDomainApproval queues a domain for human approval or rejects immediately.
func (p *ProxyServer) requestDomainApproval(domain string, resolved resolvedRequest) error {
	if p.DomainApprover == nil {
		return fmt.Errorf("forbidden - domain not allowed")
	}
	if err := ValidateDomain(domain); err != nil {
		return fmt.Errorf("forbidden - invalid domain: %w", err)
	}
	result, err := p.DomainApprover.RequestApproval(resolved.ProjectName, resolved.CloisterName, domain, resolved.Token)
	if err != nil || !result.Approved {
		return fmt.Errorf("forbidden - domain not approved")
	}
	return nil
}

// dialAndTunnel establishes a TCP connection to the upstream server, hijacks
// the client connection, and performs bidirectional copy until either side
// closes or the idle timeout is reached.
func (p *ProxyServer) dialAndTunnel(w http.ResponseWriter, r *http.Request, targetHostPort string) {
	// Establish connection to upstream server.
	// We use net.Dial (not TLS) because the client will perform TLS handshake
	// through the tunnel - this is how HTTP CONNECT proxies work.
	// Use the original targetHostPort (with port) for the actual connection.
	dialer := &net.Dialer{
		Timeout: dialTimeout,
	}
	upstreamConn, err := dialer.DialContext(r.Context(), "tcp", targetHostPort)
	if err != nil {
		// Log timeout errors with specific message for debugging
		if isTimeoutError(err) {
			p.log("proxy connection timeout to %s after %v: %v", targetHostPort, dialTimeout, err)
			http.Error(w, fmt.Sprintf("Gateway Timeout - connection to upstream timed out after %v", dialTimeout), http.StatusGatewayTimeout)
			return
		}
		p.log("proxy connection failed to %s: %v", targetHostPort, err)
		http.Error(w, fmt.Sprintf("Bad Gateway - failed to connect to upstream: %v", err), http.StatusBadGateway)
		return
	}
	defer func() {
		if err := upstreamConn.Close(); err != nil {
			clog.Warn("failed to close upstream connection: %v", err)
		}
	}()

	// Hijack the client connection to get raw TCP access.
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Internal Server Error - connection hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, fmt.Sprintf("Internal Server Error - failed to hijack connection: %v", err), http.StatusInternalServerError)
		return
	}
	defer func() {
		if err := clientConn.Close(); err != nil {
			clog.Warn("failed to close client connection: %v", err)
		}
	}()

	// Send 200 Connection Established to the client.
	// This tells the client the tunnel is ready and it can begin TLS handshake.
	_, err = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if err != nil {
		// Client connection failed, nothing more we can do
		return
	}

	// Set up bidirectional copy with idle timeout.
	// We use a WaitGroup to ensure both directions complete before returning.
	var wg sync.WaitGroup
	wg.Add(2)

	// Copy from client to upstream
	go func() {
		defer wg.Done()
		copyWithIdleTimeout(upstreamConn, clientConn, idleTimeout)
		// When client closes or times out, close upstream write side
		if tcpConn, ok := upstreamConn.(*net.TCPConn); ok {
			if err := tcpConn.CloseWrite(); err != nil {
				clog.Warn("failed to close-write upstream connection: %v", err)
			}
		}
	}()

	// Copy from upstream to client
	go func() {
		defer wg.Done()
		copyWithIdleTimeout(clientConn, upstreamConn, idleTimeout)
		// When upstream closes or times out, close client write side
		if tcpConn, ok := clientConn.(*net.TCPConn); ok {
			if err := tcpConn.CloseWrite(); err != nil {
				clog.Warn("failed to close-write client connection: %v", err)
			}
		}
	}()

	wg.Wait()
}

// copyWithIdleTimeout copies from src to dst, resetting the deadline on each read.
// This implements an idle timeout - the connection is closed if no data is transferred
// for the specified duration.
func copyWithIdleTimeout(dst, src net.Conn, idleTimeout time.Duration) {
	buf := make([]byte, 32*1024) // 32KB buffer, same as io.Copy default
	for {
		// Set read deadline for idle timeout
		if err := src.SetReadDeadline(time.Now().Add(idleTimeout)); err != nil {
			clog.Warn("failed to set read deadline: %v", err)
		}

		n, err := src.Read(buf)
		if n > 0 {
			// Reset write deadline and write data
			if err := dst.SetWriteDeadline(time.Now().Add(idleTimeout)); err != nil {
				clog.Warn("failed to set write deadline: %v", err)
			}
			_, writeErr := dst.Write(buf[:n])
			if writeErr != nil {
				return
			}
		}
		if err != nil {
			// EOF or timeout or other error - stop copying
			return
		}
	}
}

// resolvedRequest holds the result of resolving a proxy request's identity.
type resolvedRequest struct {
	ProjectName  string
	CloisterName string
	Token        string
}

// resolveRequest determines the project name, cloister name, and token for a request
// using the TokenLookup function.
func (p *ProxyServer) resolveRequest(r *http.Request) resolvedRequest {
	var token string

	// Extract token from request header.
	authHeader := r.Header.Get("Proxy-Authorization")
	if authHeader != "" {
		if t, ok := p.parseBasicAuth(authHeader); ok {
			token = t
		}
	}

	if p.TokenLookup == nil || token == "" {
		return resolvedRequest{Token: token}
	}
	result, valid := p.TokenLookup(token)
	if !valid || result.ProjectName == "" {
		return resolvedRequest{Token: token}
	}
	return resolvedRequest{
		ProjectName:  result.ProjectName,
		CloisterName: result.CloisterName,
		Token:        token,
	}
}
