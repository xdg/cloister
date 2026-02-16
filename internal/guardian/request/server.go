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
	"github.com/xdg/cloister/internal/clog"
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

// PatternLookup returns the PatternMatcher for a given project name.
// The returned matcher should reflect merged global + project patterns.
// If nil is returned, all commands for that project require manual approval.
type PatternLookup func(projectName string) PatternMatcher

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

	// PatternLookup returns the pattern matcher for a given project.
	// If nil, all commands require manual approval.
	PatternLookup PatternLookup

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
// The tokenLookup is required; patternLookup, commandExecutor, and auditLogger may be nil.
// (commandExecutor is optional during initial setup; auditLogger enables audit logging if provided).
func NewServer(tokenLookup TokenLookup, patternLookup PatternLookup, commandExecutor CommandExecutor, auditLogger *audit.Logger) *Server {
	return &Server{
		Addr:            fmt.Sprintf(":%d", DefaultRequestPort),
		TokenLookup:     tokenLookup,
		PatternLookup:   patternLookup,
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

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.Addr, err)
	}

	mux := http.NewServeMux()

	// Apply auth middleware to the request handler and route manually by method
	requestHandler := AuthMiddleware(s.TokenLookup)(http.HandlerFunc(s.handleRequestRouter))
	mux.Handle("/request", requestHandler)

	s.listener = listener
	s.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
	}
	s.running = true

	go func() {
		if err := s.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			clog.Warn("request server error: %v", err)
		}
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
	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown request server: %w", err)
	}
	return nil
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

// handleRequestRouter routes /request requests based on method.
func (s *Server) handleRequestRouter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	s.handleRequest(w, r)
}

// validatedRequest holds a parsed and validated command request.
type validatedRequest struct {
	args []string
	cmd  string
	info TokenInfo
}

// parseAndValidateRequest parses the JSON body, validates args, and extracts cloister info.
// Returns nil and writes an error response if validation fails.
func (s *Server) parseAndValidateRequest(w http.ResponseWriter, r *http.Request) *validatedRequest {
	var req CommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSON(w, http.StatusBadRequest, CommandResponse{Status: "error", Reason: "invalid JSON body"})
		return nil
	}

	if len(req.Args) == 0 {
		s.writeJSON(w, http.StatusBadRequest, CommandResponse{Status: "error", Reason: "args is required"})
		return nil
	}

	for _, arg := range req.Args {
		if containsNUL(arg) {
			s.writeJSON(w, http.StatusBadRequest, CommandResponse{Status: "error", Reason: "arguments cannot contain NUL bytes"})
			return nil
		}
	}

	info, ok := CloisterInfo(r.Context())
	if !ok {
		s.writeJSON(w, http.StatusInternalServerError, CommandResponse{Status: "error", Reason: "internal error: missing cloister info"})
		return nil
	}

	return &validatedRequest{args: req.Args, cmd: canonicalCmd(req.Args), info: info}
}

// logAudit logs an audit event if the logger is configured.
func (s *Server) logAudit(logFn func() error) {
	if s.AuditLogger == nil {
		return
	}
	if err := logFn(); err != nil {
		clog.Warn("failed to log audit event: %v", err)
	}
}

// executeAndLog runs a command and logs the completion event.
func (s *Server) executeAndLog(w http.ResponseWriter, vr *validatedRequest, status, pattern string) {
	startTime := time.Now()
	resp := s.executeCommand(vr.args, status, pattern)
	s.logAudit(func() error {
		return s.AuditLogger.LogComplete(vr.info.ProjectName, vr.info.CloisterName, vr.cmd, resp.ExitCode, time.Since(startTime))
	})
	s.writeJSON(w, http.StatusOK, resp)
}

// handleRequest processes POST /request from cloister containers.
// The auth middleware has already validated the token and attached
// TokenInfo to the context.
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	vr := s.parseAndValidateRequest(w, r)
	if vr == nil {
		return
	}

	s.logAudit(func() error {
		return s.AuditLogger.LogRequest(vr.info.ProjectName, vr.info.CloisterName, vr.cmd)
	})

	matcher := s.lookupMatcher(vr.info.ProjectName)
	if matcher == nil {
		s.logAudit(func() error {
			return s.AuditLogger.LogDeny(vr.info.ProjectName, vr.info.CloisterName, vr.cmd, "no approval patterns configured")
		})
		s.writeJSON(w, http.StatusOK, CommandResponse{Status: "denied", Reason: "no approval patterns configured"})
		return
	}

	result := matcher.Match(vr.cmd)
	s.dispatchByAction(w, vr, result)
}

// lookupMatcher returns the pattern matcher for a project, or nil.
func (s *Server) lookupMatcher(projectName string) PatternMatcher {
	if s.PatternLookup == nil {
		return nil
	}
	return s.PatternLookup(projectName)
}

// dispatchByAction handles the command based on the pattern match result.
func (s *Server) dispatchByAction(w http.ResponseWriter, vr *validatedRequest, result patterns.MatchResult) {
	switch result.Action {
	case patterns.AutoApprove:
		s.logAudit(func() error {
			return s.AuditLogger.LogAutoApprove(vr.info.ProjectName, vr.info.CloisterName, vr.cmd, result.Pattern)
		})
		s.executeAndLog(w, vr, "auto_approved", result.Pattern)

	case patterns.ManualApprove:
		s.handleManualApprove(w, vr)

	case patterns.Deny:
		s.logAudit(func() error {
			return s.AuditLogger.LogDeny(vr.info.ProjectName, vr.info.CloisterName, vr.cmd, "command does not match any approval pattern")
		})
		s.writeJSON(w, http.StatusOK, CommandResponse{Status: "denied", Reason: "command does not match any approval pattern"})

	default:
		s.writeJSON(w, http.StatusInternalServerError, CommandResponse{Status: "error", Reason: "internal error: unknown pattern action"})
	}
}

// handleManualApprove queues a request for human approval and blocks until resolved.
func (s *Server) handleManualApprove(w http.ResponseWriter, vr *validatedRequest) {
	if s.Queue == nil {
		s.logAudit(func() error {
			return s.AuditLogger.LogDeny(vr.info.ProjectName, vr.info.CloisterName, vr.cmd, "manual approval required but approval queue not configured")
		})
		s.writeJSON(w, http.StatusOK, CommandResponse{Status: "denied", Reason: "manual approval required but approval queue not configured"})
		return
	}

	respChan := make(chan approval.Response, 1)
	pendingReq := &approval.PendingRequest{
		Cloister:  vr.info.CloisterName,
		Project:   vr.info.ProjectName,
		Cmd:       vr.cmd,
		Timestamp: time.Now(),
		Response:  respChan,
	}

	if _, err := s.Queue.Add(pendingReq); err != nil {
		s.writeJSON(w, http.StatusInternalServerError, CommandResponse{Status: "error", Reason: "failed to queue request for approval"})
		return
	}

	approvalResp := <-respChan

	if approvalResp.Status == "approved" {
		s.executeAndLog(w, vr, "approved", "")
		return
	}

	if approvalResp.Status == "timeout" {
		s.logAudit(func() error {
			return s.AuditLogger.LogTimeout(vr.info.ProjectName, vr.info.CloisterName, vr.cmd)
		})
	}

	s.writeJSON(w, http.StatusOK, CommandResponse{
		Status:   approvalResp.Status,
		Pattern:  approvalResp.Pattern,
		Reason:   approvalResp.Reason,
		ExitCode: approvalResp.ExitCode,
		Stdout:   approvalResp.Stdout,
		Stderr:   approvalResp.Stderr,
	})
}

// writeJSON writes a JSON response with the given status code.
func (s *Server) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		clog.Warn("failed to encode JSON response: %v", err)
	}
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
