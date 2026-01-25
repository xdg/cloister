// Package guardian implements the HTTP CONNECT proxy and related services
// that mediate network access for cloister containers.
package guardian

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DefaultProxyPort is the standard port for HTTP CONNECT proxies.
const DefaultProxyPort = 3128

// Timeout constants for tunnel connections.
const (
	// dialTimeout is the maximum time to establish a connection to the upstream server.
	dialTimeout = 10 * time.Second
	// idleTimeout is the maximum time a tunnel connection can be idle before being closed.
	idleTimeout = 5 * time.Minute
)

// TokenValidator validates authentication tokens for proxy requests.
// Implementations should be thread-safe.
type TokenValidator interface {
	// Validate checks if a token is valid and returns true if so.
	Validate(token string) bool
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

	// Logger is used to log authentication failures. If nil, the default log package is used.
	Logger *log.Logger

	server   *http.Server
	listener net.Listener
	mu       sync.Mutex
	running  bool
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

	return nil
}

// Stop gracefully shuts down the proxy server.
func (p *ProxyServer) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	p.running = false
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
	msg := fmt.Sprintf("proxy auth failure from %s: %s", sourceIP, reason)
	if p.Logger != nil {
		p.Logger.Println(msg)
	} else {
		log.Println(msg)
	}
}

// handleConnect processes CONNECT requests.
// It checks the allowlist and establishes a bidirectional tunnel to the upstream server.
// Returns 403 Forbidden for non-allowed domains, 502 Bad Gateway for connection failures.
func (p *ProxyServer) handleConnect(w http.ResponseWriter, r *http.Request) {
	// r.Host contains the target host:port for CONNECT requests
	host := r.Host

	// Check allowlist
	if p.Allowlist == nil || !p.Allowlist.IsAllowed(host) {
		http.Error(w, "Forbidden - domain not in allowlist", http.StatusForbidden)
		return
	}

	// Establish connection to upstream server.
	// We use net.Dial (not TLS) because the client will perform TLS handshake
	// through the tunnel - this is how HTTP CONNECT proxies work.
	dialer := &net.Dialer{
		Timeout: dialTimeout,
	}
	upstreamConn, err := dialer.DialContext(r.Context(), "tcp", host)
	if err != nil {
		http.Error(w, fmt.Sprintf("Bad Gateway - failed to connect to upstream: %v", err), http.StatusBadGateway)
		return
	}
	defer upstreamConn.Close()

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
	defer clientConn.Close()

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
