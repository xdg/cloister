// Package request defines types and middleware for hostexec command requests
// between cloister containers and the guardian request server.
package request

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// DefaultRequestPort is the port for the request server.
// This server receives hostexec command requests from cloister containers.
const DefaultRequestPort = 9998

// PatternMatcher matches commands against approval patterns.
// This interface will be implemented in Phase 4.2.
type PatternMatcher interface {
	// Match returns the action to take for the given command.
	// Returns true if the command should be auto-approved.
	Match(cmd string) (autoApprove bool, pattern string)
}

// Executor executes approved commands on the host.
// This interface will be implemented in Phase 4.4.
type Executor interface {
	// Execute runs the command and returns the result.
	Execute(ctx context.Context, cmd string) (stdout, stderr string, exitCode int, err error)
}

// Server handles hostexec command requests from cloister containers.
// It validates tokens, matches commands against patterns, and coordinates
// with the approval queue and executor for command execution.
type Server struct {
	// Addr is the address to listen on (e.g., ":9998").
	Addr string

	// TokenLookup validates tokens and returns associated info.
	TokenLookup TokenLookup

	// PatternMatcher matches commands against approval patterns.
	// If nil, all commands require manual approval.
	PatternMatcher PatternMatcher

	// Executor executes approved commands.
	// If nil, commands will return a "not implemented" response.
	Executor Executor

	server   *http.Server
	listener net.Listener
	mu       sync.Mutex
	running  bool
}

// NewServer creates a new request server.
// The tokenLookup is required; patternMatcher and executor may be nil
// (they will be implemented in later phases).
func NewServer(tokenLookup TokenLookup, patternMatcher PatternMatcher, executor Executor) *Server {
	return &Server{
		Addr:           fmt.Sprintf(":%d", DefaultRequestPort),
		TokenLookup:    tokenLookup,
		PatternMatcher: patternMatcher,
		Executor:       executor,
	}
}

// Start begins accepting connections on the request server.
// Returns an error if the server is already running or fails to start.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return errors.New("request server already running")
	}

	listener, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.Addr, err)
	}

	mux := http.NewServeMux()

	// Apply auth middleware to the request handler
	requestHandler := AuthMiddleware(s.TokenLookup)(http.HandlerFunc(s.handleRequest))
	mux.Handle("POST /request", requestHandler)

	s.listener = listener
	s.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
	}
	s.running = true

	go func() {
		_ = s.server.Serve(listener)
	}()

	return nil
}

// Stop gracefully shuts down the request server.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false
	return s.server.Shutdown(ctx)
}

// ListenAddr returns the actual address the server is listening on.
// This is useful when the server was started with port 0 (random port).
// Returns empty string if the server is not running.
func (s *Server) ListenAddr() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// handleRequest processes POST /request from cloister containers.
// The auth middleware has already validated the token and attached
// TokenInfo to the context.
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Parse the command request
	var req CommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSON(w, http.StatusBadRequest, CommandResponse{
			Status: "error",
			Reason: "invalid JSON body",
		})
		return
	}

	if req.Cmd == "" {
		s.writeJSON(w, http.StatusBadRequest, CommandResponse{
			Status: "error",
			Reason: "cmd is required",
		})
		return
	}

	// Get cloister info from context (set by auth middleware)
	info, ok := CloisterInfo(r.Context())
	if !ok {
		// This shouldn't happen if auth middleware is working correctly
		s.writeJSON(w, http.StatusInternalServerError, CommandResponse{
			Status: "error",
			Reason: "internal error: missing cloister info",
		})
		return
	}

	// For now, return a placeholder response.
	// Phase 4.2 will add pattern matching for auto-approve.
	// Phase 4.3 will add the approval queue for manual approval.
	// Phase 4.4/4.5 will add actual command execution.
	_ = info // Will be used for logging and approval queue in later phases

	s.writeJSON(w, http.StatusNotImplemented, CommandResponse{
		Status: "error",
		Reason: "command execution not yet implemented",
	})
}

// writeJSON writes a JSON response with the given status code.
func (s *Server) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
