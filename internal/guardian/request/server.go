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

	"github.com/xdg/cloister/internal/guardian/approval"
	"github.com/xdg/cloister/internal/guardian/patterns"
)

// DefaultRequestPort is the port for the request server.
// This server receives hostexec command requests from cloister containers.
const DefaultRequestPort = 9998

// PatternMatcher matches commands against approval patterns.
// Uses the patterns.Matcher interface from the patterns package.
type PatternMatcher interface {
	// Match checks a command string against configured patterns.
	// Returns MatchResult indicating the action to take.
	Match(cmd string) patterns.MatchResult
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

	// Queue holds pending requests awaiting human approval.
	// If nil, ManualApprove commands will be denied.
	Queue *approval.Queue

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

	// If no pattern matcher is configured, deny all commands
	if s.PatternMatcher == nil {
		s.writeJSON(w, http.StatusOK, CommandResponse{
			Status: "denied",
			Reason: "no approval patterns configured",
		})
		return
	}

	// Match command against configured patterns
	result := s.PatternMatcher.Match(req.Cmd)

	switch result.Action {
	case patterns.AutoApprove:
		// Auto-approved: proceed to execution
		// Phase 4.4/4.5 will add actual command execution via the Executor.
		// For now, return a placeholder success response.
		s.writeJSON(w, http.StatusOK, CommandResponse{
			Status:   "auto_approved",
			Pattern:  result.Pattern,
			ExitCode: 0,
			Stdout:   "[placeholder: command execution not yet implemented]",
		})

	case patterns.ManualApprove:
		// Manual approval required: queue for human review
		if s.Queue == nil {
			s.writeJSON(w, http.StatusOK, CommandResponse{
				Status: "denied",
				Reason: "manual approval required but approval queue not configured",
			})
			return
		}

		// Create a buffered response channel to avoid blocking the approval sender
		respChan := make(chan approval.Response, 1)

		// Create pending request with metadata from token lookup
		pendingReq := &approval.PendingRequest{
			Cloister:  info.CloisterName,
			Project:   info.ProjectName,
			Branch:    "", // Not available from token; may be added later
			Agent:     "", // Not available from token; may be added later
			Cmd:       req.Cmd,
			Timestamp: time.Now(),
			Response:  respChan,
		}

		// Add to queue (this starts the timeout goroutine)
		_, err := s.Queue.Add(pendingReq)
		if err != nil {
			s.writeJSON(w, http.StatusInternalServerError, CommandResponse{
				Status: "error",
				Reason: "failed to queue request for approval",
			})
			return
		}

		// Block waiting for approval, denial, or timeout
		approvalResp := <-respChan
		// Convert approval.Response to CommandResponse (same structure)
		s.writeJSON(w, http.StatusOK, CommandResponse{
			Status:   approvalResp.Status,
			Pattern:  approvalResp.Pattern,
			Reason:   approvalResp.Reason,
			ExitCode: approvalResp.ExitCode,
			Stdout:   approvalResp.Stdout,
			Stderr:   approvalResp.Stderr,
		})

	case patterns.Deny:
		// No pattern matched: deny the command
		s.writeJSON(w, http.StatusOK, CommandResponse{
			Status: "denied",
			Reason: "command does not match any approval pattern",
		})

	default:
		// Unexpected action - should never happen
		s.writeJSON(w, http.StatusInternalServerError, CommandResponse{
			Status: "error",
			Reason: "internal error: unknown pattern action",
		})
	}
}

// writeJSON writes a JSON response with the given status code.
func (s *Server) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
