// Package approval provides an in-memory queue for pending command execution
// requests that require human review before proceeding.
package approval

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"os/user"
	"sync"
	"time"

	"github.com/xdg/cloister/internal/audit"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// templates holds the parsed HTML templates for the approval UI.
var templates *template.Template

func init() {
	var err error
	templates, err = template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		panic(fmt.Sprintf("failed to parse approval templates: %v", err))
	}
}

// DefaultApprovalPort is the port for the approval server.
// This server is bound to localhost only for security (host-accessible only).
const DefaultApprovalPort = 9999

// ConfigPersister is the interface for persisting approved domains to config files.
type ConfigPersister interface {
	AddDomainToProject(project, domain string) error
	AddDomainToGlobal(domain string) error
	AddPatternToProject(project, pattern string) error
	AddPatternToGlobal(pattern string) error
}

// Server handles approval requests from the host via a web UI.
// It provides endpoints for viewing pending requests and approving/denying them.
type Server struct {
	// Addr is the address to listen on (e.g., "127.0.0.1:9999").
	Addr string

	// Queue holds pending requests awaiting human approval.
	Queue *Queue

	// DomainQueue holds pending domain approval requests awaiting human decision.
	DomainQueue *DomainQueue

	// ConfigPersister persists approved domains to config files.
	ConfigPersister ConfigPersister

	// Events is the event hub for SSE connections.
	Events *EventHub

	// AuditLogger logs hostexec events. If nil, no audit logging is performed.
	AuditLogger *audit.Logger

	server   *http.Server
	listener net.Listener
	mu       sync.Mutex
	running  bool
}

// NewServer creates a new approval server.
// The queue is required for managing pending requests.
// The auditLogger is optional; if nil, no audit logging is performed.
// The server binds to 0.0.0.0 inside the container (for Docker port publishing)
// but is only exposed to host localhost via -p 127.0.0.1:9999:9999.
func NewServer(queue *Queue, auditLogger *audit.Logger) *Server {
	events := NewEventHub()
	// Wire the event hub to the queue so it can broadcast SSE events
	queue.SetEventHub(events)
	return &Server{
		Addr:        fmt.Sprintf(":%d", DefaultApprovalPort),
		Queue:       queue,
		Events:      events,
		AuditLogger: auditLogger,
	}
}

// getUserIdentity returns the current user identity for audit logging.
// It attempts to get the OS username, falling back to "host-operator" if unavailable.
// This runs inside the guardian container, so it returns the container user unless
// we can detect the host user through other means.
func getUserIdentity() string {
	currentUser, err := user.Current()
	if err == nil && currentUser.Username != "" {
		return currentUser.Username
	}
	// Fallback to a descriptive placeholder if we can't determine the user
	return "host-operator"
}

// SetDomainQueue sets the domain queue and wires its event hub connection.
func (s *Server) SetDomainQueue(dq *DomainQueue) {
	s.DomainQueue = dq
	if dq != nil && s.Events != nil {
		dq.SetEventHub(s.Events)
	}
}

// Start begins accepting connections on the approval server.
// The server is bound to localhost only for security.
// Returns an error if the server is already running or fails to start.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return errors.New("approval server already running")
	}

	listener, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.Addr, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /pending", s.handlePending)
	mux.HandleFunc("GET /events", s.handleEvents)
	mux.HandleFunc("POST /approve/{id}", s.handleApprove)
	mux.HandleFunc("POST /deny/{id}", s.handleDeny)
	mux.HandleFunc("GET /pending-domains", s.handlePendingDomains)
	mux.HandleFunc("POST /approve-domain/{id}", s.handleApproveDomain)
	mux.HandleFunc("POST /deny-domain/{id}", s.handleDenyDomain)
	staticSubFS, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSubFS))))

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

// Stop gracefully shuts down the approval server.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false

	// Close the event hub to disconnect all SSE clients
	if s.Events != nil {
		s.Events.Close()
	}

	// Use a background context if nil is provided
	if ctx == nil {
		ctx = context.Background()
	}

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

// indexData holds the data passed to the index.html template.
type indexData struct {
	Requests       []templateRequest
	DomainRequests []domainTemplateRequest
}

// templateRequest holds request data for template rendering.
type templateRequest struct {
	ID        string
	Cloister  string
	Project   string
	Branch    string
	Agent     string
	Cmd       string
	Timestamp string
}

// domainTemplateRequest holds domain request data for template rendering.
type domainTemplateRequest struct {
	ID        string
	Domain    string
	Cloister  string
	Project   string
	Timestamp string
	Wildcard  string // Suggested wildcard pattern like "*.example.com" (empty if not applicable)
}

// resultData holds the data passed to the result.html template.
type resultData struct {
	ID     string
	Status string
	Cmd    string
}

// domainResultData holds the data passed to the domain_result.html template.
type domainResultData struct {
	ID               string
	Status           string
	Domain           string
	Scope            string
	Reason           string
	IsPattern        bool
	PersistenceError string // Error message if config persistence failed (domain still approved for session)
}

// handleIndex serves the HTML UI for the approval interface.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	pending := s.Queue.List()

	data := indexData{
		Requests: make([]templateRequest, len(pending)),
	}
	for i, req := range pending {
		data.Requests[i] = templateRequest{
			ID:        req.ID,
			Cloister:  req.Cloister,
			Project:   req.Project,
			Branch:    req.Branch,
			Agent:     req.Agent,
			Cmd:       req.Cmd,
			Timestamp: req.Timestamp.Format(time.RFC3339),
		}
	}

	// Add domain requests if DomainQueue is available
	if s.DomainQueue != nil {
		pendingDomains := s.DomainQueue.List()
		data.DomainRequests = make([]domainTemplateRequest, len(pendingDomains))
		for i, req := range pendingDomains {
			data.DomainRequests[i] = domainTemplateRequest{
				ID:        req.ID,
				Domain:    req.Domain,
				Cloister:  req.Cloister,
				Project:   req.Project,
				Timestamp: req.Timestamp.Format(time.RFC3339),
				Wildcard:  domainToWildcard(req.Domain),
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, "failed to render template", http.StatusInternalServerError)
	}
}

// pendingRequestJSON represents a pending request in JSON format for the API.
type pendingRequestJSON struct {
	ID        string `json:"id"`
	Cloister  string `json:"cloister"`
	Project   string `json:"project"`
	Branch    string `json:"branch"`
	Agent     string `json:"agent"`
	Cmd       string `json:"cmd"`
	Timestamp string `json:"timestamp"`
}

// pendingResponse is the response body for GET /pending.
type pendingResponse struct {
	Requests []pendingRequestJSON `json:"requests"`
}

// handlePending returns a JSON array of pending requests.
func (s *Server) handlePending(w http.ResponseWriter, r *http.Request) {
	pending := s.Queue.List()

	response := pendingResponse{
		Requests: make([]pendingRequestJSON, len(pending)),
	}

	for i, req := range pending {
		response.Requests[i] = pendingRequestJSON{
			ID:        req.ID,
			Cloister:  req.Cloister,
			Project:   req.Project,
			Branch:    req.Branch,
			Agent:     req.Agent,
			Cmd:       req.Cmd,
			Timestamp: req.Timestamp.Format(time.RFC3339),
		}
	}

	s.writeJSON(w, http.StatusOK, response)
}

// HeartbeatInterval is the interval between heartbeat events sent to SSE clients.
// This keeps connections alive through proxies and load balancers that may
// close idle connections.
const HeartbeatInterval = 30 * time.Second

// handleEvents serves the SSE endpoint for real-time updates.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Check if the client supports flushing
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Subscribe to events
	eventCh := s.Events.Subscribe()
	if eventCh == nil {
		http.Error(w, "server shutting down", http.StatusServiceUnavailable)
		return
	}
	defer s.Events.Unsubscribe(eventCh)

	// Flush headers immediately so client knows connection is established
	flusher.Flush()

	// Start heartbeat ticker to keep connection alive
	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()

	// Stream events to client
	for {
		select {
		case <-r.Context().Done():
			// Client disconnected
			return
		case <-ticker.C:
			// Send heartbeat to keep connection alive
			heartbeat := Event{Type: EventHeartbeat, Data: ""}
			_, err := fmt.Fprint(w, FormatSSE(heartbeat))
			if err != nil {
				return
			}
			flusher.Flush()
		case event, ok := <-eventCh:
			if !ok {
				// Event hub closed
				return
			}
			// Write the SSE formatted event
			_, err := fmt.Fprint(w, FormatSSE(event))
			if err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// approveResponse is the response body for POST /approve/{id}.
type approveResponse struct {
	Status string `json:"status"`
	ID     string `json:"id"`
}

// handleApprove approves a pending request by ID.
func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	req, ok := s.Queue.Get(id)
	if !ok {
		s.writeError(w, http.StatusNotFound, "request not found")
		return
	}

	// Capture request info before removing from queue
	cmd := req.Cmd
	project := req.Project
	branch := req.Branch
	cloister := req.Cloister

	// Log APPROVE event
	if s.AuditLogger != nil {
		_ = s.AuditLogger.LogApprove(project, branch, cloister, cmd, getUserIdentity())
	}

	// Send approved response on the request's channel.
	// The request handler is blocked waiting on this channel and will
	// proceed to execute the command via the executor client.
	if req.Response != nil {
		req.Response <- Response{
			Status: "approved",
		}
	}

	// Remove from queue (also cancels timeout)
	s.Queue.Remove(id)

	// Broadcast removal event to SSE clients
	s.Events.BroadcastRequestRemoved(id)

	// Check if this is an htmx request
	if r.Header.Get("HX-Request") == "true" {
		s.writeResultHTML(w, id, "approved", cmd)
		return
	}

	s.writeJSON(w, http.StatusOK, approveResponse{
		Status: "approved",
		ID:     id,
	})
}

// denyRequest is the optional request body for POST /deny/{id}.
type denyRequest struct {
	Reason string `json:"reason,omitempty"`
}

// denyResponse is the response body for POST /deny/{id}.
type denyResponse struct {
	Status string `json:"status"`
	ID     string `json:"id"`
}

// handleDeny denies a pending request by ID with an optional reason.
func (s *Server) handleDeny(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	req, ok := s.Queue.Get(id)
	if !ok {
		s.writeError(w, http.StatusNotFound, "request not found")
		return
	}

	// Capture request info before removing from queue
	cmd := req.Cmd
	project := req.Project
	branch := req.Branch
	cloister := req.Cloister

	// Parse optional reason from request body
	var denyReq denyRequest
	// Ignore decode errors - reason is optional
	_ = json.NewDecoder(r.Body).Decode(&denyReq)

	reason := denyReq.Reason
	if reason == "" {
		reason = fmt.Sprintf("Denied by %s", getUserIdentity())
	}

	// Log DENY event
	if s.AuditLogger != nil {
		_ = s.AuditLogger.LogDeny(project, branch, cloister, cmd, reason)
	}

	// Send denied response on the request's channel
	if req.Response != nil {
		req.Response <- Response{
			Status: "denied",
			Reason: reason,
		}
	}

	// Remove from queue (also cancels timeout)
	s.Queue.Remove(id)

	// Broadcast removal event to SSE clients
	s.Events.BroadcastRequestRemoved(id)

	// Check if this is an htmx request
	if r.Header.Get("HX-Request") == "true" {
		s.writeResultHTML(w, id, "denied", cmd)
		return
	}

	s.writeJSON(w, http.StatusOK, denyResponse{
		Status: "denied",
		ID:     id,
	})
}

// errorResponse is an error response.
type errorResponse struct {
	Error string `json:"error"`
}

// writeJSON writes a JSON response with the given status code.
func (s *Server) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes an error response with the given status code.
func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, errorResponse{Error: message})
}

// writeResultHTML renders the result template for htmx responses.
func (s *Server) writeResultHTML(w http.ResponseWriter, id, status, cmd string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := resultData{
		ID:     id,
		Status: status,
		Cmd:    cmd,
	}
	if err := templates.ExecuteTemplate(w, "result", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// writeDomainResultHTML renders the domain_result template for htmx responses.
func (s *Server) writeDomainResultHTML(w http.ResponseWriter, id, status, domain, scope, reason string, isPattern bool, persistenceError string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := domainResultData{
		ID:               id,
		Status:           status,
		Domain:           domain,
		Scope:            scope,
		Reason:           reason,
		IsPattern:        isPattern,
		PersistenceError: persistenceError,
	}
	if err := templates.ExecuteTemplate(w, "domain_result", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// pendingDomainRequestJSON represents a pending domain request in JSON format for the API.
type pendingDomainRequestJSON struct {
	ID        string `json:"id"`
	Cloister  string `json:"cloister"`
	Project   string `json:"project"`
	Domain    string `json:"domain"`
	Timestamp string `json:"timestamp"`
}

// pendingDomainsResponse is the response body for GET /pending-domains.
type pendingDomainsResponse struct {
	Requests []pendingDomainRequestJSON `json:"requests"`
}

// handlePendingDomains returns a JSON array of pending domain requests.
func (s *Server) handlePendingDomains(w http.ResponseWriter, r *http.Request) {
	if s.DomainQueue == nil {
		s.writeError(w, http.StatusInternalServerError, "domain queue not initialized")
		return
	}

	pending := s.DomainQueue.List()

	response := pendingDomainsResponse{
		Requests: make([]pendingDomainRequestJSON, len(pending)),
	}

	for i, req := range pending {
		response.Requests[i] = pendingDomainRequestJSON{
			ID:        req.ID,
			Cloister:  req.Cloister,
			Project:   req.Project,
			Domain:    req.Domain,
			Timestamp: req.Timestamp.Format(time.RFC3339),
		}
	}

	s.writeJSON(w, http.StatusOK, response)
}

// approveDomainRequest is the request body for POST /approve-domain/{id}.
type approveDomainRequest struct {
	Scope   string `json:"scope"`   // "session", "project", or "global"
	Pattern string `json:"pattern"` // optional wildcard pattern like "*.example.com"
}

// approveDomainResponse is the response body for POST /approve-domain/{id}.
type approveDomainResponse struct {
	Status string `json:"status"`
	ID     string `json:"id"`
	Scope  string `json:"scope"`
}

// handleApproveDomain approves a pending domain request by ID.
func (s *Server) handleApproveDomain(w http.ResponseWriter, r *http.Request) {
	if s.DomainQueue == nil {
		s.writeError(w, http.StatusInternalServerError, "domain queue not initialized")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	// Parse scope from request body
	var approveReq approveDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&approveReq); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	scope := approveReq.Scope
	if scope != "session" && scope != "project" && scope != "global" {
		s.writeError(w, http.StatusBadRequest, "scope must be session, project, or global")
		return
	}

	// Check if ConfigPersister is available for project/global scopes
	if (scope == "project" || scope == "global") && s.ConfigPersister == nil {
		s.writeError(w, http.StatusInternalServerError, "config persistence not available")
		return
	}

	req, ok := s.DomainQueue.Get(id)
	if !ok {
		s.writeError(w, http.StatusNotFound, "request not found")
		return
	}

	// Capture request info before removing from queue
	domain := req.Domain
	project := req.Project
	cloister := req.Cloister
	pattern := approveReq.Pattern
	isPattern := pattern != ""

	// Determine what to persist: exact domain or wildcard pattern
	persistValue := domain
	if isPattern {
		persistValue = pattern
	}

	// Track persistence error - if config save fails, we fall back to session scope
	// but still approve the domain for immediate use
	var persistenceError string
	requestedScope := scope

	// Persist to config if needed
	if scope == "project" {
		var err error
		if isPattern {
			err = s.ConfigPersister.AddPatternToProject(req.Project, pattern)
		} else {
			err = s.ConfigPersister.AddDomainToProject(req.Project, domain)
		}
		if err != nil {
			// Log the error but don't fail - fall back to session scope
			persistenceError = fmt.Sprintf("failed to persist to project config: %v", err)
			scope = "session"
			isPattern = false // Session scope doesn't persist patterns
		}
	} else if scope == "global" {
		var err error
		if isPattern {
			err = s.ConfigPersister.AddPatternToGlobal(pattern)
		} else {
			err = s.ConfigPersister.AddDomainToGlobal(domain)
		}
		if err != nil {
			// Log the error but don't fail - fall back to session scope
			persistenceError = fmt.Sprintf("failed to persist to global config: %v", err)
			scope = "session"
			isPattern = false // Session scope doesn't persist patterns
		}
	}

	// Log DOMAIN_APPROVE event (with actual scope used, not requested scope)
	if s.AuditLogger != nil {
		_ = s.AuditLogger.LogDomainApprove(project, cloister, persistValue, scope, getUserIdentity())
	}

	// Send approved response on the request's channels BEFORE removing from queue
	// to prevent race conditions. Broadcasts to all waiting callers (coalesced requests).
	resp := DomainResponse{
		Status:           "approved",
		Scope:            scope,
		Pattern:          pattern,
		PersistenceError: persistenceError,
	}
	// Clear pattern if we fell back to session scope
	if scope == "session" && requestedScope != "session" {
		resp.Pattern = ""
	}
	broadcastDomainResponse(req, resp)

	// Remove from queue (also cancels timeout)
	s.DomainQueue.Remove(id)

	// Broadcast removal event to SSE clients
	s.Events.BroadcastDomainRequestRemoved(id)

	// Check if this is an htmx request
	if r.Header.Get("HX-Request") == "true" {
		displayValue := domain
		if isPattern && persistenceError == "" {
			displayValue = pattern
		}
		s.writeDomainResultHTML(w, id, "approved", displayValue, scope, "", isPattern && persistenceError == "", persistenceError)
		return
	}

	s.writeJSON(w, http.StatusOK, approveDomainResponse{
		Status: "approved",
		ID:     id,
		Scope:  scope,
	})
}

// denyDomainRequest is the optional request body for POST /deny-domain/{id}.
type denyDomainRequest struct {
	Reason string `json:"reason,omitempty"`
}

// denyDomainResponse is the response body for POST /deny-domain/{id}.
type denyDomainResponse struct {
	Status string `json:"status"`
	ID     string `json:"id"`
}

// handleDenyDomain denies a pending domain request by ID.
func (s *Server) handleDenyDomain(w http.ResponseWriter, r *http.Request) {
	if s.DomainQueue == nil {
		s.writeError(w, http.StatusInternalServerError, "domain queue not initialized")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	req, ok := s.DomainQueue.Get(id)
	if !ok {
		s.writeError(w, http.StatusNotFound, "request not found")
		return
	}

	// Capture request info before removing from queue
	domain := req.Domain
	project := req.Project
	cloister := req.Cloister

	// Parse optional reason from request body
	var denyReq denyDomainRequest
	// Ignore decode errors - reason is optional
	_ = json.NewDecoder(r.Body).Decode(&denyReq)

	reason := denyReq.Reason
	if reason == "" {
		reason = fmt.Sprintf("Denied by %s", getUserIdentity())
	}

	// Log DOMAIN_DENY event
	if s.AuditLogger != nil {
		_ = s.AuditLogger.LogDomainDeny(project, cloister, domain, reason)
	}

	// Send denied response on the request's channels. Broadcasts to all waiting callers.
	broadcastDomainResponse(req, DomainResponse{
		Status: "denied",
		Reason: reason,
	})

	// Remove from queue (also cancels timeout)
	s.DomainQueue.Remove(id)

	// Broadcast removal event to SSE clients
	s.Events.BroadcastDomainRequestRemoved(id)

	// Check if this is an htmx request
	if r.Header.Get("HX-Request") == "true" {
		s.writeDomainResultHTML(w, id, "denied", req.Domain, "", reason, false, "")
		return
	}

	s.writeJSON(w, http.StatusOK, denyDomainResponse{
		Status: "denied",
		ID:     id,
	})
}

// broadcastDomainResponse sends a response to all waiting channels in a DomainRequest.
// This is used for coalesced requests where multiple callers are waiting for the same approval.
func broadcastDomainResponse(req *DomainRequest, resp DomainResponse) {
	for _, ch := range req.Responses {
		if ch != nil {
			// Non-blocking send to prevent goroutine leaks
			select {
			case ch <- resp:
			default:
			}
		}
	}
}
