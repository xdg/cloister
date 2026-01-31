// Package executor provides the interface and types for host command execution.
package executor

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
)

// DefaultSocketPath is the default path for the hostexec Unix socket.
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

// TokenValidator validates a token and returns the associated worktree path.
// Returns an error if the token is invalid.
type TokenValidator func(token string) (worktreePath string, err error)

// WorkdirValidator validates that the requested workdir matches the token's registered worktree.
// Returns an error if the workdir does not match.
type WorkdirValidator func(requestedWorkdir, registeredWorktree string) error

// SocketServer listens on a Unix socket and executes commands via an Executor.
type SocketServer struct {
	socketPath       string
	secret           string
	executor         Executor
	tokenValidator   TokenValidator
	workdirValidator WorkdirValidator

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

// WithTokenValidator sets a custom token validator.
func WithTokenValidator(v TokenValidator) SocketServerOption {
	return func(s *SocketServer) {
		s.tokenValidator = v
	}
}

// WithWorkdirValidator sets a custom workdir validator.
func WithWorkdirValidator(v WorkdirValidator) SocketServerOption {
	return func(s *SocketServer) {
		s.workdirValidator = v
	}
}

// NewSocketServer creates a new SocketServer.
// The secret is used to authenticate requests.
// The executor is used to execute commands.
func NewSocketServer(secret string, executor Executor, opts ...SocketServerOption) *SocketServer {
	s := &SocketServer{
		socketPath:       DefaultSocketPath,
		secret:           secret,
		executor:         executor,
		tokenValidator:   defaultTokenValidator,
		workdirValidator: defaultWorkdirValidator,
		shutdown:         make(chan struct{}),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// defaultTokenValidator is a placeholder that accepts all tokens.
// It will be replaced in Phase 4.4.4 with real validation.
func defaultTokenValidator(token string) (string, error) {
	// Placeholder: accept all tokens, return empty worktree path
	return "", nil
}

// defaultWorkdirValidator is a placeholder that accepts all workdirs.
// It will be replaced in Phase 4.4.5 with real validation.
func defaultWorkdirValidator(requestedWorkdir, registeredWorktree string) error {
	// Placeholder: accept all workdirs
	return nil
}

// Start begins listening on the Unix socket.
// It creates the parent directory if needed and sets socket permissions to 0600.
// Returns an error if the socket cannot be created.
func (s *SocketServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

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
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return err
	}

	// Set socket permissions to 0600 (owner read/write only)
	if err := os.Chmod(s.socketPath, 0600); err != nil {
		listener.Close()
		return err
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
	s.mu.Unlock()

	// Wait for all connections to complete
	s.wg.Wait()

	// Remove socket file
	os.Remove(s.socketPath)

	return err
}

// SocketPath returns the path to the Unix socket.
func (s *SocketServer) SocketPath() string {
	return s.socketPath
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

	// Validate shared secret
	if req.Secret != s.secret {
		s.writeError(conn, "invalid secret")
		return
	}

	// Validate token
	registeredWorktree, err := s.tokenValidator(req.Request.Token)
	if err != nil {
		s.writeError(conn, "invalid token: "+err.Error())
		return
	}

	// Validate workdir
	if err := s.workdirValidator(req.Request.Workdir, registeredWorktree); err != nil {
		s.writeError(conn, "workdir mismatch: "+err.Error())
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

// ErrInvalidToken is returned when a token is not found in the registry.
var ErrInvalidToken = errors.New("token not found")

// ErrWorkdirMismatch is returned when the requested workdir doesn't match the registered worktree.
var ErrWorkdirMismatch = errors.New("workdir does not match registered worktree")
