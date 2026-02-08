// Package guardian implements the HTTP CONNECT proxy and related services
// that mediate network access for cloister containers.
package guardian

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
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

// ConfigReloader is a function that returns a new Allowlist based on reloaded configuration.
type ConfigReloader func() (*Allowlist, error)

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

// SessionAllowlist tracks ephemeral session-approved domains per token.
// Token-based isolation ensures each cloister session has an independent
// domain cache, even when multiple cloisters belong to the same project.
type SessionAllowlist interface {
	IsAllowed(token, domain string) bool
	Add(token, domain string) error
	Clear(token string) // Called when token is revoked to clean up session domains
}

// ProxyServer is an HTTP CONNECT proxy that enforces domain allowlists
// for cloister containers.
type ProxyServer struct {
	// Addr is the address to listen on (e.g., ":3128").
	Addr string

	// Allowlist controls which domains are permitted. If nil, all domains are blocked.
	Allowlist *Allowlist

	// TokenValidator validates authentication tokens. If nil, all requests are allowed
	// (useful for testing). When set, requests must include a valid Proxy-Authorization
	// header with Basic auth where the password is the token.
	TokenValidator TokenValidator

	// AllowlistCache provides per-project allowlist lookups. If nil, the global
	// Allowlist is used for all requests.
	AllowlistCache *AllowlistCache

	// TokenLookup provides token-to-project mapping for per-project allowlists.
	// If nil, the global Allowlist is used for all requests.
	TokenLookup TokenLookupFunc

	// DomainApprover requests human approval for unlisted domains. If nil, unlisted
	// domains are immediately rejected with 403 (preserving current behavior).
	DomainApprover DomainApprover

	// SessionAllowlist tracks domains approved with "session" scope (ephemeral).
	// If nil, session allowlist checks are skipped.
	SessionAllowlist SessionAllowlist

	server         *http.Server
	listener       net.Listener
	mu             sync.Mutex
	running        bool
	configReloader ConfigReloader
	sighupChan     chan os.Signal
	stopSighup     chan struct{}
}

// NewProxyServer creates a new proxy server listening on the specified address.
// If addr is empty, it defaults to ":3128".
// The server is created with the default allowlist.
func NewProxyServer(addr string) *ProxyServer {
	if addr == "" {
		addr = fmt.Sprintf(":%d", DefaultProxyPort)
	}
	return &ProxyServer{
		Addr:      addr,
		Allowlist: NewDefaultAllowlist(),
	}
}

// NewProxyServerWithConfig creates a new proxy server with the specified allowlist.
// If addr is empty, it defaults to ":3128".
// If allowlist is nil, the default allowlist is used.
func NewProxyServerWithConfig(addr string, allowlist *Allowlist) *ProxyServer {
	if addr == "" {
		addr = fmt.Sprintf(":%d", DefaultProxyPort)
	}
	if allowlist == nil {
		allowlist = NewDefaultAllowlist()
	}
	return &ProxyServer{
		Addr:      addr,
		Allowlist: allowlist,
	}
}

// SetAllowlist replaces the proxy's allowlist.
func (p *ProxyServer) SetAllowlist(a *Allowlist) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Allowlist = a
}

// SetConfigReloader sets a function that will be called on SIGHUP to reload config.
// The reloader should return a new Allowlist based on the reloaded configuration.
func (p *ProxyServer) SetConfigReloader(reload ConfigReloader) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.configReloader = reload
}

// Start begins accepting connections on the proxy server.
// It returns an error if the server is already running or fails to start.
func (p *ProxyServer) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return errors.New("proxy server already running")
	}

	listener, err := net.Listen("tcp", p.Addr)
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
		// Serve blocks until the server is shut down.
		// ErrServerClosed is expected on graceful shutdown and is not an error.
		// Other errors are silently ignored as there's no good way to report
		// them from a background goroutine - the caller should monitor via
		// health checks or observe connection failures.
		_ = p.server.Serve(listener)
	}()

	// Start SIGHUP handler if configReloader is set
	if p.configReloader != nil {
		p.startSighupHandler()
	}

	return nil
}

// startSighupHandler sets up signal handling for SIGHUP to reload configuration.
func (p *ProxyServer) startSighupHandler() {
	p.sighupChan = make(chan os.Signal, 1)
	p.stopSighup = make(chan struct{})
	signal.Notify(p.sighupChan, syscall.SIGHUP)

	go func() {
		for {
			select {
			case <-p.sighupChan:
				p.handleSighup()
			case <-p.stopSighup:
				signal.Stop(p.sighupChan)
				return
			}
		}
	}()
}

// handleSighup is called when SIGHUP is received to reload configuration.
func (p *ProxyServer) handleSighup() {
	p.mu.Lock()
	reloader := p.configReloader
	p.mu.Unlock()

	if reloader == nil {
		return
	}

	newAllowlist, err := reloader()
	if err != nil {
		p.log("SIGHUP config reload failed: %v", err)
		return
	}

	p.mu.Lock()
	p.Allowlist = newAllowlist
	p.mu.Unlock()
	p.log("SIGHUP config reloaded successfully")
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
		p.stopSighup = nil
	}

	return p.server.Shutdown(ctx)
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
// Only CONNECT method is allowed; all other methods return 405.
// Authentication is checked before processing if TokenValidator is set.
func (p *ProxyServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect {
		http.Error(w, "Method Not Allowed - only CONNECT is supported", http.StatusMethodNotAllowed)
		return
	}

	// Check authentication if TokenValidator is set
	if p.TokenValidator != nil {
		if !p.authenticate(w, r) {
			return
		}
	}

	p.handleConnect(w, r)
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

// handleConnect processes CONNECT requests.
// It checks the allowlist and establishes a bidirectional tunnel to the upstream server.
// Returns 403 Forbidden for non-allowed domains, 502 Bad Gateway for connection failures.
func (p *ProxyServer) handleConnect(w http.ResponseWriter, r *http.Request) {
	// r.Host contains the target host:port for CONNECT requests (e.g., "example.com:443")
	// We strip the port early so all subsequent processing uses domain-only.
	// This ensures consistent behavior across allowlist matching, session cache,
	// approval queue display, and config persistence.
	targetHostPort := r.Host
	domain := stripPort(targetHostPort)

	// Resolve allowlist, project, cloister, and token from the request in a single lookup
	allowlist, projectName, cloisterName, token := p.resolveRequest(r)

	// Check static allowlist FIRST, before session or approval logic
	staticAllowed := allowlist != nil && allowlist.IsAllowed(domain)
	clog.Debug("handleConnect: host=%s, domain=%s, allowlist=%v, staticAllowed=%v",
		targetHostPort, domain, allowlist != nil, staticAllowed)

	// If NOT in static allowlist, check session allowlist and domain approver
	if !staticAllowed {
		sessionAllowed := false
		if p.SessionAllowlist != nil && token != "" {
			sessionAllowed = p.SessionAllowlist.IsAllowed(token, domain)
		}

		if !sessionAllowed {
			if p.DomainApprover == nil {
				// No approver - reject immediately (backward compatible)
				http.Error(w, "Forbidden - domain not allowed", http.StatusForbidden)
				return
			}

			// Validate domain format before queueing for approval
			if err := ValidateDomain(domain); err != nil {
				http.Error(w, fmt.Sprintf("Forbidden - invalid domain: %v", err), http.StatusForbidden)
				return
			}

			// Request approval (blocks until response)
			// Pass domain-only (no port) so approval UI shows clean domain names
			// and duplicates are properly detected across different ports
			result, err := p.DomainApprover.RequestApproval(projectName, cloisterName, domain, token)
			if err != nil || !result.Approved {
				http.Error(w, "Forbidden - domain not approved", http.StatusForbidden)
				return
			}
		}
	}
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
	defer func() { _ = upstreamConn.Close() }()

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
	defer func() { _ = clientConn.Close() }()

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
			_ = tcpConn.CloseWrite()
		}
	}()

	// Copy from upstream to client
	go func() {
		defer wg.Done()
		copyWithIdleTimeout(clientConn, upstreamConn, idleTimeout)
		// When upstream closes or times out, close client write side
		if tcpConn, ok := clientConn.(*net.TCPConn); ok {
			_ = tcpConn.CloseWrite()
		}
	}()

	wg.Wait()
}

// copyWithIdleTimeout copies from src to dst, resetting the deadline on each read.
// This implements an idle timeout - the connection is closed if no data is transferred
// for the specified duration.
func copyWithIdleTimeout(dst net.Conn, src net.Conn, idleTimeout time.Duration) {
	buf := make([]byte, 32*1024) // 32KB buffer, same as io.Copy default
	for {
		// Set read deadline for idle timeout
		_ = src.SetReadDeadline(time.Now().Add(idleTimeout))

		n, err := src.Read(buf)
		if n > 0 {
			// Reset write deadline and write data
			_ = dst.SetWriteDeadline(time.Now().Add(idleTimeout))
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

// resolveRequest determines the allowlist, project name, cloister name, and token for a request.
// It extracts the token from the Proxy-Authorization header and performs a single lookup.
// Falls back to the global allowlist with empty project/cloister if lookup is unavailable.
func (p *ProxyServer) resolveRequest(r *http.Request) (allowlist *Allowlist, projectName string, cloisterName string, token string) {
	clog.Debug("resolveRequest: AllowlistCache=%v, TokenLookup=%v", p.AllowlistCache != nil, p.TokenLookup != nil)

	// Extract token from request header (needed for session allowlist regardless)
	authHeader := r.Header.Get("Proxy-Authorization")
	if authHeader != "" {
		if t, ok := p.parseBasicAuth(authHeader); ok {
			token = t
		}
	}

	// If no per-project support is configured, use global allowlist
	if p.AllowlistCache == nil || p.TokenLookup == nil {
		clog.Debug("resolveRequest: returning p.Allowlist (global)")
		return p.Allowlist, "", "", token
	}

	if token == "" {
		return p.Allowlist, "", "", ""
	}

	// Single token lookup for project and cloister
	result, valid := p.TokenLookup(token)
	if !valid || result.ProjectName == "" {
		return p.Allowlist, "", "", token
	}

	// Get project-specific allowlist
	return p.AllowlistCache.GetProject(result.ProjectName), result.ProjectName, result.CloisterName, token
}
