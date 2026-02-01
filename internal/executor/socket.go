// Package executor provides the interface and types for host command execution.
package executor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
)

// DefaultSocketPath is the default path for the hostexec Unix socket.
// Deprecated: Use TCP mode instead for cross-platform compatibility.
var DefaultSocketPath = filepath.Join(os.Getenv("HOME"), ".local", "share", "cloister", "hostexec.sock")

// SocketRequest is the JSON request sent over the Unix socket.
// It wraps ExecuteRequest with authentication fields.
type SocketRequest struct {
	Secret  string         `json:"secret"`
	Request ExecuteRequest `json:"request"`
}

// SocketResponse is the JSON response sent over the Unix socket.
// It wraps ExecuteResponse with additional error information.
type SocketResponse struct {
	Success  bool            `json:"success"`
	Error    string          `json:"error,omitempty"`
	Response ExecuteResponse `json:"response,omitempty"`
}

// SocketServer listens on a Unix socket or TCP port and executes commands via an Executor.
type SocketServer struct {
	socketPath string
	tcpAddr    string // If set, use TCP instead of Unix socket
	secret     string
	executor   Executor

	listener net.Listener
	wg       sync.WaitGroup
	shutdown chan struct{}
	mu       sync.Mutex // protects listener and shutdown state
}

// SocketServerOption configures a SocketServer.
type SocketServerOption func(*SocketServer)

// WithSocketPath sets a custom socket path.
func WithSocketPath(path string) SocketServerOption {
	return func(s *SocketServer) {
		s.socketPath = path
	}
}

// WithTCPAddr configures the server to listen on a TCP address instead of Unix socket.
// The address should be in the form "host:port" or ":port".
// Use ":0" to let the OS choose a random available port.
func WithTCPAddr(addr string) SocketServerOption {
	return func(s *SocketServer) {
		s.tcpAddr = addr
	}
}

// NewSocketServer creates a new SocketServer.
// The secret is used to authenticate requests from the guardian.
// Token validation is handled by the guardian before forwarding to the executor.
func NewSocketServer(secret string, executor Executor, opts ...SocketServerOption) *SocketServer {
	s := &SocketServer{
		socketPath: DefaultSocketPath,
		secret:     secret,
		executor:   executor,
		shutdown:   make(chan struct{}),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start begins listening on the configured address.
// For TCP mode (WithTCPAddr), it listens on the specified TCP address.
// For Unix socket mode (default), it creates the parent directory if needed
// and sets socket permissions to 0600.
// Returns an error if the listener cannot be created.
func (s *SocketServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var listener net.Listener
	var err error

	if s.tcpAddr != "" {
		// TCP mode
		listener, err = net.Listen("tcp", s.tcpAddr)
		if err != nil {
			return fmt.Errorf("failed to listen on TCP %s: %w", s.tcpAddr, err)
		}
	} else {
		// Unix socket mode
		// Create parent directory if needed
		dir := filepath.Dir(s.socketPath)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}

		// Remove existing socket if present
		if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
			return err
		}

		// Listen on Unix socket
		listener, err = net.Listen("unix", s.socketPath)
		if err != nil {
			return err
		}

		// Set socket permissions to 0600 (owner read/write only)
		if err := os.Chmod(s.socketPath, 0600); err != nil {
			listener.Close()
			return err
		}
	}

	s.listener = listener

	// Start accepting connections in a goroutine
	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// Stop gracefully shuts down the socket server.
// It stops accepting new connections and waits for existing connections to complete.
func (s *SocketServer) Stop() error {
	s.mu.Lock()
	if s.listener == nil {
		s.mu.Unlock()
		return nil
	}

	// Signal shutdown
	close(s.shutdown)

	// Close listener to stop accepting new connections
	err := s.listener.Close()
	isTCP := s.tcpAddr != ""
	s.mu.Unlock()

	// Wait for all connections to complete
	s.wg.Wait()

	// Remove socket file (only for Unix socket mode)
	if !isTCP {
		os.Remove(s.socketPath)
	}

	return err
}

// SocketPath returns the path to the Unix socket.
// Returns empty string if using TCP mode.
func (s *SocketServer) SocketPath() string {
	if s.tcpAddr != "" {
		return ""
	}
	return s.socketPath
}

// ListenAddr returns the actual address the server is listening on.
// For TCP mode, this is the bound TCP address (useful when using :0).
// For Unix socket mode, this returns the socket path.
func (s *SocketServer) ListenAddr() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// IsTCP returns true if the server is configured for TCP mode.
func (s *SocketServer) IsTCP() bool {
	return s.tcpAddr != ""
}

// acceptLoop accepts connections until shutdown.
func (s *SocketServer) acceptLoop() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			// Check if we're shutting down
			select {
			case <-s.shutdown:
				return
			default:
				// Log error and continue (could be transient)
				continue
			}
		}

		// Handle connection in a goroutine
		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

// handleConnection processes a single client connection.
// It reads a newline-delimited JSON request, validates it, executes the command,
// and writes a JSON response.
func (s *SocketServer) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	// Read request (newline-delimited JSON)
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		s.writeError(conn, "failed to read request: "+err.Error())
		return
	}

	// Parse request
	var req SocketRequest
	if err := json.Unmarshal(line, &req); err != nil {
		s.writeError(conn, "invalid JSON: "+err.Error())
		return
	}

	// Validate shared secret (guardian-executor authentication)
	// Token validation is handled by the guardian before forwarding requests
	if req.Secret != s.secret {
		s.writeError(conn, "invalid secret")
		return
	}

	// Execute command
	ctx := context.Background()
	select {
	case <-s.shutdown:
		s.writeError(conn, "server shutting down")
		return
	default:
	}

	execResp := s.executor.Execute(ctx, req.Request)

	// Write response
	resp := SocketResponse{
		Success:  true,
		Response: execResp,
	}
	s.writeResponse(conn, resp)
}

// writeError writes an error response to the connection.
func (s *SocketServer) writeError(conn net.Conn, errMsg string) {
	resp := SocketResponse{
		Success: false,
		Error:   errMsg,
	}
	s.writeResponse(conn, resp)
}

// writeResponse writes a JSON response to the connection.
func (s *SocketServer) writeResponse(conn net.Conn, resp SocketResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		// Last resort: write a minimal error (ignore write error)
		_, _ = conn.Write([]byte(`{"success":false,"error":"failed to marshal response"}` + "\n"))
		return
	}
	data = append(data, '\n')
	_, _ = conn.Write(data) // Ignore write error; connection may be closed
}
