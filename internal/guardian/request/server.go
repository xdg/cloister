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

	"github.com/xdg/cloister/internal/audit"
	"github.com/xdg/cloister/internal/executor"
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

// CommandExecutor executes approved commands on the host via the executor socket.
// This interface wraps the executor client for testability.
type CommandExecutor interface {
	// Execute sends an execution request to the host executor and returns the response.
	Execute(req executor.ExecuteRequest) (*executor.ExecuteResponse, error)
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

	// CommandExecutor executes approved commands via the host executor socket.
	// If nil, commands will return a "not implemented" response.
	CommandExecutor CommandExecutor

	// Queue holds pending requests awaiting human approval.
	// If nil, ManualApprove commands will be denied.
	Queue *approval.Queue

	// AuditLogger logs hostexec events. If nil, no audit logging is performed.
	AuditLogger *audit.Logger

	server   *http.Server
	listener net.Listener
	mu       sync.Mutex
	running  bool
}

// NewServer creates a new request server.
// The tokenLookup is required; patternMatcher, commandExecutor, and auditLogger may be nil.
// (commandExecutor is optional during initial setup; auditLogger enables audit logging if provided).
func NewServer(tokenLookup TokenLookup, patternMatcher PatternMatcher, commandExecutor CommandExecutor, auditLogger *audit.Logger) *Server {
	return &Server{
		Addr:            fmt.Sprintf(":%d", DefaultRequestPort),
		TokenLookup:     tokenLookup,
		PatternMatcher:  patternMatcher,
		CommandExecutor: commandExecutor,
		AuditLogger:     auditLogger,
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

	if len(req.Args) == 0 {
		s.writeJSON(w, http.StatusBadRequest, CommandResponse{
			Status: "error",
			Reason: "args is required",
		})
		return
	}

	// Reject arguments containing NUL bytes, which cannot be safely embedded
	// in shell arguments and could cause divergent behavior.
	for _, arg := range req.Args {
		if containsNUL(arg) {
			s.writeJSON(w, http.StatusBadRequest, CommandResponse{
				Status: "error",
				Reason: "arguments cannot contain NUL bytes",
			})
			return
		}
	}

	// Reconstruct canonical command from args for pattern matching and logging.
	// This is the authoritative representation - cmd is ignored (deprecated).
	cmd := canonicalCmd(req.Args)

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

	// Log REQUEST event
	_ = s.AuditLogger.LogRequest(info.ProjectName, "", info.CloisterName, cmd)

	// If no pattern matcher is configured, deny all commands
	if s.PatternMatcher == nil {
		_ = s.AuditLogger.LogDeny(info.ProjectName, "", info.CloisterName, cmd, "no approval patterns configured")
		s.writeJSON(w, http.StatusOK, CommandResponse{
			Status: "denied",
			Reason: "no approval patterns configured",
		})
		return
	}

	// Match command against configured patterns
	result := s.PatternMatcher.Match(cmd)

	switch result.Action {
	case patterns.AutoApprove:
		// Log AUTO_APPROVE event
		_ = s.AuditLogger.LogAutoApprove(info.ProjectName, "", info.CloisterName, cmd, result.Pattern)
		// Auto-approved: proceed to execution
		startTime := time.Now()
		resp := s.executeCommand(req.Args, "auto_approved", result.Pattern)
		// Log COMPLETE event with duration
		_ = s.AuditLogger.LogComplete(info.ProjectName, "", info.CloisterName, cmd, resp.ExitCode, time.Since(startTime))
		s.writeJSON(w, http.StatusOK, resp)

	case patterns.ManualApprove:
		// Manual approval required: queue for human review
		if s.Queue == nil {
			_ = s.AuditLogger.LogDeny(info.ProjectName, "", info.CloisterName, cmd, "manual approval required but approval queue not configured")
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
			Cmd:       cmd,
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

		// If approved, execute the command
		if approvalResp.Status == "approved" {
			// Note: APPROVE event is logged by approval server
			startTime := time.Now()
			resp := s.executeCommand(req.Args, "approved", "")
			// Log COMPLETE event with duration
			_ = s.AuditLogger.LogComplete(info.ProjectName, "", info.CloisterName, cmd, resp.ExitCode, time.Since(startTime))
			s.writeJSON(w, http.StatusOK, resp)
			return
		}

		// Handle timeout specifically
		if approvalResp.Status == "timeout" {
			_ = s.AuditLogger.LogTimeout(info.ProjectName, "", info.CloisterName, cmd)
		}
		// Note: DENY events for manual approval are logged by approval server

		// For denied/timeout/error, pass through the response
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
		_ = s.AuditLogger.LogDeny(info.ProjectName, "", info.CloisterName, cmd, "command does not match any approval pattern")
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

// executeCommand runs the command through the executor and returns a CommandResponse.
// The args parameter is the tokenized argument array (args[0] is the command).
// The status parameter is used for the response status (e.g., "approved" or "auto_approved").
// The pattern parameter is included in the response for auto_approved commands.
func (s *Server) executeCommand(args []string, status, pattern string) CommandResponse {
	if s.CommandExecutor == nil {
		return CommandResponse{
			Status: "error",
			Reason: "command execution not configured",
		}
	}

	if len(args) == 0 {
		return CommandResponse{
			Status: "error",
			Reason: "empty args array",
		}
	}

	// Build the executor request
	// args[0] is the command, args[1:] are the arguments
	// Using pre-tokenized args prevents shell injection
	execReq := executor.ExecuteRequest{
		Command: args[0],
		Args:    args[1:],
		// Workdir, Env, and TimeoutMs can be extended later
	}

	execResp, err := s.CommandExecutor.Execute(execReq)
	if err != nil {
		return CommandResponse{
			Status: "error",
			Reason: err.Error(),
		}
	}

	// Map executor response to command response
	return mapExecutorResponse(execResp, status, pattern)
}

// mapExecutorResponse converts an executor.ExecuteResponse to a CommandResponse.
// The status parameter overrides the response status for approved commands.
// The pattern is included for auto_approved commands.
func mapExecutorResponse(execResp *executor.ExecuteResponse, status, pattern string) CommandResponse {
	resp := CommandResponse{
		Pattern:  pattern,
		ExitCode: execResp.ExitCode,
		Stdout:   execResp.Stdout,
		Stderr:   execResp.Stderr,
	}

	// Map executor status to command response status
	switch execResp.Status {
	case executor.StatusCompleted:
		resp.Status = status // Use the provided status (approved/auto_approved)
	case executor.StatusTimeout:
		resp.Status = "timeout"
		resp.Reason = "command execution timed out"
		if execResp.Error != "" {
			resp.Reason = execResp.Error
		}
	case executor.StatusError:
		resp.Status = "error"
		resp.Reason = execResp.Error
	default:
		resp.Status = "error"
		resp.Reason = "unknown executor status: " + execResp.Status
	}

	return resp
}
