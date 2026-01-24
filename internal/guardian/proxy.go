// Package guardian implements the HTTP CONNECT proxy and related services
// that mediate network access for cloister containers.
package guardian

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// DefaultProxyPort is the standard port for HTTP CONNECT proxies.
const DefaultProxyPort = 3128

// ProxyServer is an HTTP CONNECT proxy that enforces domain allowlists
// for cloister containers.
type ProxyServer struct {
	// Addr is the address to listen on (e.g., ":3128").
	Addr string

	server   *http.Server
	listener net.Listener
	mu       sync.Mutex
	running  bool
}

// NewProxyServer creates a new proxy server listening on the specified address.
// If addr is empty, it defaults to ":3128".
func NewProxyServer(addr string) *ProxyServer {
	if addr == "" {
		addr = fmt.Sprintf(":%d", DefaultProxyPort)
	}
	return &ProxyServer{Addr: addr}
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
func (p *ProxyServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect {
		http.Error(w, "Method Not Allowed - only CONNECT is supported", http.StatusMethodNotAllowed)
		return
	}

	p.handleConnect(w, r)
}

// handleConnect processes CONNECT requests.
// This is a stub implementation that accepts the connection but does not
// perform actual tunneling - that will be implemented in phase 1.3.3.
func (p *ProxyServer) handleConnect(w http.ResponseWriter, _ *http.Request) {
	// Stub implementation: accept the request and close
	// Actual tunneling will be implemented in phase 1.3.3
	w.WriteHeader(http.StatusOK)
}
