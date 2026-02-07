package approval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/audit"
)

func TestNewServer(t *testing.T) {
	queue := NewQueue()
	server := NewServer(queue, nil)

	if server == nil {
		t.Fatal("NewServer returned nil")
	}
	if server.Addr != ":9999" {
		t.Errorf("expected addr :9999, got %s", server.Addr)
	}
	if server.Queue != queue {
		t.Error("Queue should be set")
	}
}

func TestServer_StartStop(t *testing.T) {
	queue := NewQueue()
	server := NewServer(queue, nil)
	server.Addr = "127.0.0.1:0" // Use random port

	// Start should succeed
	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify we got a listening address
	addr := server.ListenAddr()
	if addr == "" {
		t.Error("ListenAddr should return non-empty after Start")
	}

	// Start again should fail
	if err := server.Start(); err == nil {
		t.Error("second Start should fail")
	}

	// Stop should succeed
	if err := server.Stop(context.Background()); err != nil {
		t.Errorf("Stop failed: %v", err)
	}

	// Stop again should be idempotent
	if err := server.Stop(context.Background()); err != nil {
		t.Errorf("second Stop failed: %v", err)
	}
}

func TestServer_ListenAddrBeforeStart(t *testing.T) {
	queue := NewQueue()
	server := NewServer(queue, nil)

	addr := server.ListenAddr()
	if addr != "" {
		t.Errorf("expected empty addr before Start, got %q", addr)
	}
}

func TestServer_HandleIndex(t *testing.T) {
	queue := NewQueue()
	server := NewServer(queue, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	server.handleIndex(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	contentType := rr.Header().Get("Content-Type")
	if contentType != "text/html; charset=utf-8" {
		t.Errorf("expected Content-Type text/html; charset=utf-8, got %s", contentType)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "Cloister Approval") {
		t.Error("expected body to contain 'Cloister Approval'")
	}
}

func TestServer_HandlePending_Empty(t *testing.T) {
	queue := NewQueue()
	server := NewServer(queue, nil)

	req := httptest.NewRequest(http.MethodGet, "/pending", nil)
	rr := httptest.NewRecorder()

	server.handlePending(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	var resp pendingResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Requests) != 0 {
		t.Errorf("expected 0 requests, got %d", len(resp.Requests))
	}
}

func TestServer_HandlePending_WithRequests(t *testing.T) {
	queue := NewQueue()

	// Add a test request
	respChan := make(chan Response, 1)
	req := &PendingRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Branch:    "main",
		Agent:     "claude",
		Cmd:       "docker compose up -d",
		Timestamp: time.Date(2024, 1, 15, 14, 32, 5, 0, time.UTC),
		Response:  respChan,
	}
	id, err := queue.Add(req)
	if err != nil {
		t.Fatalf("failed to add request: %v", err)
	}

	server := NewServer(queue, nil)

	httpReq := httptest.NewRequest(http.MethodGet, "/pending", nil)
	rr := httptest.NewRecorder()

	server.handlePending(rr, httpReq)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp pendingResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(resp.Requests))
	}

	r := resp.Requests[0]
	if r.ID != id {
		t.Errorf("expected ID %s, got %s", id, r.ID)
	}
	if r.Cloister != "test-cloister" {
		t.Errorf("expected cloister 'test-cloister', got %q", r.Cloister)
	}
	if r.Project != "test-project" {
		t.Errorf("expected project 'test-project', got %q", r.Project)
	}
	if r.Branch != "main" {
		t.Errorf("expected branch 'main', got %q", r.Branch)
	}
	if r.Agent != "claude" {
		t.Errorf("expected agent 'claude', got %q", r.Agent)
	}
	if r.Cmd != "docker compose up -d" {
		t.Errorf("expected cmd 'docker compose up -d', got %q", r.Cmd)
	}
	if r.Timestamp != "2024-01-15T14:32:05Z" {
		t.Errorf("expected timestamp '2024-01-15T14:32:05Z', got %q", r.Timestamp)
	}
}

func TestServer_HandleApprove_Success(t *testing.T) {
	queue := NewQueue()

	// Add a test request with a response channel
	respChan := make(chan Response, 1)
	req := &PendingRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Cmd:       "docker compose up -d",
		Timestamp: time.Now(),
		Response:  respChan,
	}
	id, err := queue.Add(req)
	if err != nil {
		t.Fatalf("failed to add request: %v", err)
	}

	server := NewServer(queue, nil)

	httpReq := httptest.NewRequest(http.MethodPost, "/approve/"+id, nil)
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	server.handleApprove(rr, httpReq)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp approveResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "approved" {
		t.Errorf("expected status 'approved', got %q", resp.Status)
	}
	if resp.ID != id {
		t.Errorf("expected ID %s, got %s", id, resp.ID)
	}

	// Verify the response was sent on the channel
	select {
	case approvalResp := <-respChan:
		if approvalResp.Status != "approved" {
			t.Errorf("expected approval response status 'approved', got %q", approvalResp.Status)
		}
	default:
		t.Error("expected approval response on channel")
	}

	// Verify request was removed from queue
	if queue.Len() != 0 {
		t.Errorf("expected queue to be empty, got %d", queue.Len())
	}
}

func TestServer_HandleApprove_NotFound(t *testing.T) {
	queue := NewQueue()
	server := NewServer(queue, nil)

	req := httptest.NewRequest(http.MethodPost, "/approve/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	rr := httptest.NewRecorder()

	server.handleApprove(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}

	var resp errorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error != "request not found" {
		t.Errorf("expected error 'request not found', got %q", resp.Error)
	}
}

func TestServer_HandleDeny_Success(t *testing.T) {
	queue := NewQueue()

	// Add a test request with a response channel
	respChan := make(chan Response, 1)
	req := &PendingRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Cmd:       "rm -rf /",
		Timestamp: time.Now(),
		Response:  respChan,
	}
	id, err := queue.Add(req)
	if err != nil {
		t.Fatalf("failed to add request: %v", err)
	}

	server := NewServer(queue, nil)

	httpReq := httptest.NewRequest(http.MethodPost, "/deny/"+id, nil)
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	server.handleDeny(rr, httpReq)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp denyResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "denied" {
		t.Errorf("expected status 'denied', got %q", resp.Status)
	}
	if resp.ID != id {
		t.Errorf("expected ID %s, got %s", id, resp.ID)
	}

	// Verify the response was sent on the channel with default reason
	select {
	case denyResp := <-respChan:
		if denyResp.Status != "denied" {
			t.Errorf("expected denial response status 'denied', got %q", denyResp.Status)
		}
		if !strings.Contains(denyResp.Reason, "Denied by") {
			t.Errorf("expected default reason to start with 'Denied by', got %q", denyResp.Reason)
		}
	default:
		t.Error("expected denial response on channel")
	}

	// Verify request was removed from queue
	if queue.Len() != 0 {
		t.Errorf("expected queue to be empty, got %d", queue.Len())
	}
}

func TestServer_HandleDeny_WithReason(t *testing.T) {
	queue := NewQueue()

	// Add a test request with a response channel
	respChan := make(chan Response, 1)
	req := &PendingRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Cmd:       "rm -rf /",
		Timestamp: time.Now(),
		Response:  respChan,
	}
	id, err := queue.Add(req)
	if err != nil {
		t.Fatalf("failed to add request: %v", err)
	}

	server := NewServer(queue, nil)

	body := `{"reason": "Command looks dangerous"}`
	httpReq := httptest.NewRequest(http.MethodPost, "/deny/"+id, bytes.NewBufferString(body))
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	server.handleDeny(rr, httpReq)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	// Verify the response was sent with custom reason
	select {
	case denyResp := <-respChan:
		if denyResp.Status != "denied" {
			t.Errorf("expected denial response status 'denied', got %q", denyResp.Status)
		}
		if denyResp.Reason != "Command looks dangerous" {
			t.Errorf("expected reason 'Command looks dangerous', got %q", denyResp.Reason)
		}
	default:
		t.Error("expected denial response on channel")
	}
}

func TestServer_HandleDeny_NotFound(t *testing.T) {
	queue := NewQueue()
	server := NewServer(queue, nil)

	req := httptest.NewRequest(http.MethodPost, "/deny/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	rr := httptest.NewRecorder()

	server.handleDeny(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}

	var resp errorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error != "request not found" {
		t.Errorf("expected error 'request not found', got %q", resp.Error)
	}
}

func TestServer_ViaHTTP(t *testing.T) {
	queue := NewQueue()
	server := NewServer(queue, nil)
	server.Addr = "127.0.0.1:0" // Use random port

	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = server.Stop(context.Background()) }()

	baseURL := "http://" + server.ListenAddr()

	// Test GET /
	resp, err := http.Get(baseURL + "/")
	if err != nil {
		t.Fatalf("request to / failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET / expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	// Test GET /pending
	resp, err = http.Get(baseURL + "/pending")
	if err != nil {
		t.Fatalf("request to /pending failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /pending expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	// Test POST /approve/{id} (not found)
	resp, err = http.Post(baseURL+"/approve/nonexistent", "application/json", nil)
	if err != nil {
		t.Fatalf("request to /approve/nonexistent failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("POST /approve/nonexistent expected status %d, got %d", http.StatusNotFound, resp.StatusCode)
	}

	// Test POST /deny/{id} (not found)
	resp, err = http.Post(baseURL+"/deny/nonexistent", "application/json", nil)
	if err != nil {
		t.Fatalf("request to /deny/nonexistent failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("POST /deny/nonexistent expected status %d, got %d", http.StatusNotFound, resp.StatusCode)
	}
}

func TestServer_HandleApprove_MissingID(t *testing.T) {
	queue := NewQueue()
	server := NewServer(queue, nil)

	// Create request without path value set
	req := httptest.NewRequest(http.MethodPost, "/approve/", nil)
	// Don't set path value to simulate missing ID
	rr := httptest.NewRecorder()

	server.handleApprove(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestServer_HandleDeny_MissingID(t *testing.T) {
	queue := NewQueue()
	server := NewServer(queue, nil)

	// Create request without path value set
	req := httptest.NewRequest(http.MethodPost, "/deny/", nil)
	// Don't set path value to simulate missing ID
	rr := httptest.NewRecorder()

	server.handleDeny(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestTemplates_ParseWithoutError(t *testing.T) {
	// Verify templates are parsed and available (init runs at package load)
	if templates == nil {
		t.Fatal("templates should be parsed at init")
	}

	// Verify index.html template exists
	indexTmpl := templates.Lookup("index.html")
	if indexTmpl == nil {
		t.Error("index.html template not found")
	}

	// Verify request template exists
	requestTmpl := templates.Lookup("request")
	if requestTmpl == nil {
		t.Error("request template not found")
	}
}

func TestTemplates_ExecuteWithData(t *testing.T) {
	// Test that templates execute without error with sample data
	data := indexData{
		Requests: []templateRequest{
			{
				ID:        "abc123",
				Cloister:  "test-cloister",
				Project:   "test-project",
				Branch:    "main",
				Agent:     "claude",
				Cmd:       "docker compose up -d",
				Timestamp: "2024-01-15T14:32:05Z",
			},
		},
	}

	var buf bytes.Buffer
	err := templates.ExecuteTemplate(&buf, "index.html", data)
	if err != nil {
		t.Fatalf("failed to execute index.html template: %v", err)
	}

	output := buf.String()

	// Verify key content is present
	if !strings.Contains(output, "Cloister Approval Queue") {
		t.Error("expected output to contain 'Cloister Approval Queue'")
	}
	if !strings.Contains(output, "test-cloister") {
		t.Error("expected output to contain 'test-cloister'")
	}
	if !strings.Contains(output, "docker compose up -d") {
		t.Error("expected output to contain 'docker compose up -d'")
	}
	if !strings.Contains(output, "abc123") {
		t.Error("expected output to contain request ID 'abc123'")
	}
}

func TestTemplates_ExecuteEmpty(t *testing.T) {
	// Test that templates handle empty request list
	data := indexData{
		Requests: []templateRequest{},
	}

	var buf bytes.Buffer
	err := templates.ExecuteTemplate(&buf, "index.html", data)
	if err != nil {
		t.Fatalf("failed to execute index.html template with empty data: %v", err)
	}

	output := buf.String()

	// Verify empty state message is shown
	if !strings.Contains(output, "No pending requests") {
		t.Error("expected output to contain 'No pending requests'")
	}
}

func TestTemplates_RequestPartial(t *testing.T) {
	// Test that request partial executes independently
	data := templateRequest{
		ID:        "def456",
		Cloister:  "my-cloister",
		Project:   "my-project",
		Branch:    "feature-x",
		Agent:     "codex",
		Cmd:       "git push origin feature-x",
		Timestamp: "2024-01-15T15:00:00Z",
	}

	var buf bytes.Buffer
	err := templates.ExecuteTemplate(&buf, "request", data)
	if err != nil {
		t.Fatalf("failed to execute request template: %v", err)
	}

	output := buf.String()

	// Verify key content is present
	if !strings.Contains(output, "my-cloister") {
		t.Error("expected output to contain 'my-cloister'")
	}
	if !strings.Contains(output, "git push origin feature-x") {
		t.Error("expected output to contain 'git push origin feature-x'")
	}
	if !strings.Contains(output, "def456") {
		t.Error("expected output to contain request ID 'def456'")
	}
	if !strings.Contains(output, "Approve") {
		t.Error("expected output to contain 'Approve' button")
	}
	if !strings.Contains(output, "Deny") {
		t.Error("expected output to contain 'Deny' button")
	}
}

func TestTemplates_ParseFS(t *testing.T) {
	// Verify that templates can be re-parsed from the embedded filesystem
	// This tests the embed.FS is valid
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		t.Fatalf("failed to parse templates from embed.FS: %v", err)
	}

	// Verify both templates are present
	if tmpl.Lookup("index.html") == nil {
		t.Error("index.html not found in parsed templates")
	}
	if tmpl.Lookup("request") == nil {
		t.Error("request template not found in parsed templates")
	}
}

func TestTemplates_DomainRequestPartial(t *testing.T) {
	// Test that domain_request template executes without error
	data := domainTemplateRequest{
		ID:        "domain123",
		Domain:    "example.com",
		Cloister:  "test-cloister",
		Project:   "test-project",
		Timestamp: "2024-01-15T14:32:05Z",
	}

	var buf bytes.Buffer
	err := templates.ExecuteTemplate(&buf, "domain_request", data)
	if err != nil {
		t.Fatalf("failed to execute domain_request template: %v", err)
	}

	output := buf.String()

	// Verify key content is present
	if !strings.Contains(output, "example.com") {
		t.Error("expected output to contain 'example.com'")
	}
	if !strings.Contains(output, "test-cloister") {
		t.Error("expected output to contain 'test-cloister'")
	}
	if !strings.Contains(output, "test-project") {
		t.Error("expected output to contain 'test-project'")
	}
	if !strings.Contains(output, "domain123") {
		t.Error("expected output to contain request ID 'domain123'")
	}
	if !strings.Contains(output, "Allow (Session)") {
		t.Error("expected output to contain 'Allow (Session)' button")
	}
	if !strings.Contains(output, "Save to Project") {
		t.Error("expected output to contain 'Save to Project' button")
	}
	if !strings.Contains(output, "Save to Global") {
		t.Error("expected output to contain 'Save to Global' button")
	}
	if !strings.Contains(output, "Deny") {
		t.Error("expected output to contain 'Deny' button")
	}
}

func TestTemplates_DomainResultApproved(t *testing.T) {
	// Test that domain_result template renders for approved state
	data := domainResultData{
		ID:        "domain456",
		Domain:    "trusted.com",
		Status:    "approved",
		Scope:     "project",
		IsPattern: false,
	}

	var buf bytes.Buffer
	err := templates.ExecuteTemplate(&buf, "domain_result", data)
	if err != nil {
		t.Fatalf("failed to execute domain_result template with approved status: %v", err)
	}

	output := buf.String()

	// Verify key content is present
	if !strings.Contains(output, "domain456") {
		t.Error("expected output to contain request ID 'domain456'")
	}
	if !strings.Contains(output, "trusted.com") {
		t.Error("expected output to contain 'trusted.com'")
	}
	if !strings.Contains(output, "Approved") {
		t.Error("expected output to contain 'Approved'")
	}
	if !strings.Contains(output, "project") {
		t.Error("expected output to contain scope 'project'")
	}
	// Should NOT contain warning class when there's no persistence error
	if strings.Contains(output, "request-warning") {
		t.Error("expected output NOT to contain 'request-warning' class when no persistence error")
	}
}

func TestTemplates_DomainResultDenied(t *testing.T) {
	// Test that domain_result template renders for denied state
	data := domainResultData{
		ID:        "domain789",
		Domain:    "suspicious.com",
		Status:    "denied",
		Reason:    "Suspicious domain",
		IsPattern: false,
	}

	var buf bytes.Buffer
	err := templates.ExecuteTemplate(&buf, "domain_result", data)
	if err != nil {
		t.Fatalf("failed to execute domain_result template with denied status: %v", err)
	}

	output := buf.String()

	// Verify key content is present
	if !strings.Contains(output, "domain789") {
		t.Error("expected output to contain request ID 'domain789'")
	}
	if !strings.Contains(output, "suspicious.com") {
		t.Error("expected output to contain 'suspicious.com'")
	}
	if !strings.Contains(output, "Denied") {
		t.Error("expected output to contain 'Denied'")
	}
	if !strings.Contains(output, "Suspicious domain") {
		t.Error("expected output to contain reason 'Suspicious domain'")
	}
}


// Tests for audit logging integration

func TestServer_HandleApprove_AuditLogging(t *testing.T) {
	queue := NewQueue()

	// Add a test request with a response channel
	respChan := make(chan Response, 1)
	req := &PendingRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Branch:    "main",
		Cmd:       "docker compose up -d",
		Timestamp: time.Now(),
		Response:  respChan,
	}
	id, err := queue.Add(req)
	if err != nil {
		t.Fatalf("failed to add request: %v", err)
	}

	// Create a buffer to capture audit logs
	var auditBuf bytes.Buffer
	auditLogger := audit.NewLogger(&auditBuf)

	server := NewServer(queue, auditLogger)

	httpReq := httptest.NewRequest(http.MethodPost, "/approve/"+id, nil)
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	server.handleApprove(rr, httpReq)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	// Verify audit logs
	auditOutput := auditBuf.String()

	// Should have APPROVE event
	if !strings.Contains(auditOutput, "HOSTEXEC APPROVE") {
		t.Errorf("expected audit log to contain APPROVE event, got: %s", auditOutput)
	}

	// Verify project and cloister are in the logs
	if !strings.Contains(auditOutput, "project=test-project") {
		t.Errorf("expected audit log to contain project=test-project, got: %s", auditOutput)
	}
	if !strings.Contains(auditOutput, "cloister=test-cloister") {
		t.Errorf("expected audit log to contain cloister=test-cloister, got: %s", auditOutput)
	}
	if !strings.Contains(auditOutput, "branch=main") {
		t.Errorf("expected audit log to contain branch=main, got: %s", auditOutput)
	}
	if !strings.Contains(auditOutput, `user=`) {
		t.Errorf("expected audit log to contain user field, got: %s", auditOutput)
	}

	// Drain the response channel
	<-respChan
}

func TestServer_HandleDeny_AuditLogging(t *testing.T) {
	queue := NewQueue()

	// Add a test request with a response channel
	respChan := make(chan Response, 1)
	req := &PendingRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Branch:    "feature",
		Cmd:       "rm -rf /",
		Timestamp: time.Now(),
		Response:  respChan,
	}
	id, err := queue.Add(req)
	if err != nil {
		t.Fatalf("failed to add request: %v", err)
	}

	// Create a buffer to capture audit logs
	var auditBuf bytes.Buffer
	auditLogger := audit.NewLogger(&auditBuf)

	server := NewServer(queue, auditLogger)

	body := `{"reason": "Command looks dangerous"}`
	httpReq := httptest.NewRequest(http.MethodPost, "/deny/"+id, bytes.NewBufferString(body))
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	server.handleDeny(rr, httpReq)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	// Verify audit logs
	auditOutput := auditBuf.String()

	// Should have DENY event
	if !strings.Contains(auditOutput, "HOSTEXEC DENY") {
		t.Errorf("expected audit log to contain DENY event, got: %s", auditOutput)
	}

	// Verify project and cloister are in the logs
	if !strings.Contains(auditOutput, "project=test-project") {
		t.Errorf("expected audit log to contain project=test-project, got: %s", auditOutput)
	}
	if !strings.Contains(auditOutput, "cloister=test-cloister") {
		t.Errorf("expected audit log to contain cloister=test-cloister, got: %s", auditOutput)
	}

	// Verify the denial reason is in the logs
	if !strings.Contains(auditOutput, "Command looks dangerous") {
		t.Errorf("expected audit log to contain denial reason, got: %s", auditOutput)
	}

	// Drain the response channel
	<-respChan
}

func TestServer_HandleDeny_AuditLogging_DefaultReason(t *testing.T) {
	queue := NewQueue()

	// Add a test request with a response channel
	respChan := make(chan Response, 1)
	req := &PendingRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Cmd:       "rm -rf /",
		Timestamp: time.Now(),
		Response:  respChan,
	}
	id, err := queue.Add(req)
	if err != nil {
		t.Fatalf("failed to add request: %v", err)
	}

	// Create a buffer to capture audit logs
	var auditBuf bytes.Buffer
	auditLogger := audit.NewLogger(&auditBuf)

	server := NewServer(queue, auditLogger)

	// Deny without providing a reason
	httpReq := httptest.NewRequest(http.MethodPost, "/deny/"+id, nil)
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	server.handleDeny(rr, httpReq)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	// Verify audit logs include default reason
	auditOutput := auditBuf.String()

	if !strings.Contains(auditOutput, "HOSTEXEC DENY") {
		t.Errorf("expected audit log to contain DENY event, got: %s", auditOutput)
	}

	// Default reason should start with "Denied by"
	if !strings.Contains(auditOutput, "Denied by") {
		t.Errorf("expected audit log to contain default reason starting with 'Denied by', got: %s", auditOutput)
	}

	// Drain the response channel
	<-respChan
}

func TestServer_HandleApprove_NilAuditLogger(t *testing.T) {
	queue := NewQueue()

	// Add a test request with a response channel
	respChan := make(chan Response, 1)
	req := &PendingRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Cmd:       "docker compose up -d",
		Timestamp: time.Now(),
		Response:  respChan,
	}
	id, err := queue.Add(req)
	if err != nil {
		t.Fatalf("failed to add request: %v", err)
	}

	// Create server with nil audit logger - should not panic
	server := NewServer(queue, nil)

	httpReq := httptest.NewRequest(http.MethodPost, "/approve/"+id, nil)
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	// Should not panic with nil logger
	server.handleApprove(rr, httpReq)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp approveResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "approved" {
		t.Errorf("expected status 'approved', got %q", resp.Status)
	}

	// Drain the response channel
	<-respChan
}

// Tests for domain approval handlers

func TestServer_HandlePendingDomains_Empty(t *testing.T) {
	queue := NewQueue()
	domainQueue := NewDomainQueue()
	server := NewServer(queue, nil)
	server.DomainQueue = domainQueue

	req := httptest.NewRequest(http.MethodGet, "/pending-domains", nil)
	rr := httptest.NewRecorder()

	server.handlePendingDomains(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	var resp pendingDomainsResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Requests) != 0 {
		t.Errorf("expected 0 requests, got %d", len(resp.Requests))
	}
}

func TestServer_HandlePendingDomains_WithRequests(t *testing.T) {
	queue := NewQueue()
	domainQueue := NewDomainQueue()

	// Add a test domain request
	respChan := make(chan DomainResponse, 1)
	domainReq := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Date(2024, 1, 15, 14, 32, 5, 0, time.UTC),
		Responses: []chan<- DomainResponse{respChan},
	}
	id, err := domainQueue.Add(domainReq)
	if err != nil {
		t.Fatalf("failed to add domain request: %v", err)
	}

	server := NewServer(queue, nil)
	server.DomainQueue = domainQueue

	httpReq := httptest.NewRequest(http.MethodGet, "/pending-domains", nil)
	rr := httptest.NewRecorder()

	server.handlePendingDomains(rr, httpReq)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp pendingDomainsResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(resp.Requests))
	}

	r := resp.Requests[0]
	if r.ID != id {
		t.Errorf("expected ID %s, got %s", id, r.ID)
	}
	if r.Cloister != "test-cloister" {
		t.Errorf("expected cloister 'test-cloister', got %q", r.Cloister)
	}
	if r.Project != "test-project" {
		t.Errorf("expected project 'test-project', got %q", r.Project)
	}
	if r.Domain != "example.com" {
		t.Errorf("expected domain 'example.com', got %q", r.Domain)
	}
	if r.Timestamp != "2024-01-15T14:32:05Z" {
		t.Errorf("expected timestamp '2024-01-15T14:32:05Z', got %q", r.Timestamp)
	}
}

func TestServer_HandlePendingDomains_NilDomainQueue(t *testing.T) {
	queue := NewQueue()
	server := NewServer(queue, nil)
	// DomainQueue is nil by default

	req := httptest.NewRequest(http.MethodGet, "/pending-domains", nil)
	rr := httptest.NewRecorder()

	server.handlePendingDomains(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}

	var resp errorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error != "domain queue not initialized" {
		t.Errorf("expected error 'domain queue not initialized', got %q", resp.Error)
	}
}

// mockConfigPersister is a test double for ConfigPersister.
type mockConfigPersister struct {
	addDomainToProjectCalls []struct {
		project string
		domain  string
	}
	addDomainToGlobalCalls []struct {
		domain string
	}
	addPatternToProjectCalls []struct {
		project string
		pattern string
	}
	addPatternToGlobalCalls []struct {
		pattern string
	}
	addDomainToProjectErr  error
	addDomainToGlobalErr   error
	addPatternToProjectErr error
	addPatternToGlobalErr  error
}

func (m *mockConfigPersister) AddDomainToProject(project, domain string) error {
	m.addDomainToProjectCalls = append(m.addDomainToProjectCalls, struct {
		project string
		domain  string
	}{project, domain})
	return m.addDomainToProjectErr
}

func (m *mockConfigPersister) AddDomainToGlobal(domain string) error {
	m.addDomainToGlobalCalls = append(m.addDomainToGlobalCalls, struct {
		domain string
	}{domain})
	return m.addDomainToGlobalErr
}

func (m *mockConfigPersister) AddPatternToProject(project, pattern string) error {
	m.addPatternToProjectCalls = append(m.addPatternToProjectCalls, struct {
		project string
		pattern string
	}{project, pattern})
	return m.addPatternToProjectErr
}

func (m *mockConfigPersister) AddPatternToGlobal(pattern string) error {
	m.addPatternToGlobalCalls = append(m.addPatternToGlobalCalls, struct {
		pattern string
	}{pattern})
	return m.addPatternToGlobalErr
}

func TestServer_HandleApproveDomain_SessionScope(t *testing.T) {
	queue := NewQueue()
	domainQueue := NewDomainQueue()

	// Add a test domain request with a response channel
	respChan := make(chan DomainResponse, 1)
	domainReq := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}
	id, err := domainQueue.Add(domainReq)
	if err != nil {
		t.Fatalf("failed to add domain request: %v", err)
	}

	server := NewServer(queue, nil)
	server.DomainQueue = domainQueue
	// ConfigPersister is nil (not needed for session scope)

	body := `{"scope": "session"}`
	httpReq := httptest.NewRequest(http.MethodPost, "/approve-domain/"+id, bytes.NewBufferString(body))
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	server.handleApproveDomain(rr, httpReq)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp approveDomainResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "approved" {
		t.Errorf("expected status 'approved', got %q", resp.Status)
	}
	if resp.ID != id {
		t.Errorf("expected ID %s, got %s", id, resp.ID)
	}
	if resp.Scope != "session" {
		t.Errorf("expected scope 'session', got %q", resp.Scope)
	}

	// Verify the response was sent on the channel
	select {
	case approvalResp := <-respChan:
		if approvalResp.Status != "approved" {
			t.Errorf("expected approval response status 'approved', got %q", approvalResp.Status)
		}
		if approvalResp.Scope != "session" {
			t.Errorf("expected approval response scope 'session', got %q", approvalResp.Scope)
		}
	default:
		t.Error("expected approval response on channel")
	}

	// Verify request was removed from queue
	if domainQueue.Len() != 0 {
		t.Errorf("expected domain queue to be empty, got %d", domainQueue.Len())
	}
}

func TestServer_HandleApproveDomain_ProjectScope(t *testing.T) {
	queue := NewQueue()
	domainQueue := NewDomainQueue()

	// Add a test domain request with a response channel
	respChan := make(chan DomainResponse, 1)
	domainReq := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}
	id, err := domainQueue.Add(domainReq)
	if err != nil {
		t.Fatalf("failed to add domain request: %v", err)
	}

	mockPersister := &mockConfigPersister{}

	server := NewServer(queue, nil)
	server.DomainQueue = domainQueue
	server.ConfigPersister = mockPersister

	body := `{"scope": "project"}`
	httpReq := httptest.NewRequest(http.MethodPost, "/approve-domain/"+id, bytes.NewBufferString(body))
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	server.handleApproveDomain(rr, httpReq)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp approveDomainResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "approved" {
		t.Errorf("expected status 'approved', got %q", resp.Status)
	}
	if resp.Scope != "project" {
		t.Errorf("expected scope 'project', got %q", resp.Scope)
	}

	// Verify ConfigPersister.AddDomainToProject was called
	if len(mockPersister.addDomainToProjectCalls) != 1 {
		t.Fatalf("expected AddDomainToProject to be called once, got %d calls", len(mockPersister.addDomainToProjectCalls))
	}
	call := mockPersister.addDomainToProjectCalls[0]
	if call.project != "test-project" {
		t.Errorf("expected project 'test-project', got %q", call.project)
	}
	if call.domain != "example.com" {
		t.Errorf("expected domain 'example.com', got %q", call.domain)
	}

	// Verify the response was sent on the channel
	select {
	case approvalResp := <-respChan:
		if approvalResp.Status != "approved" {
			t.Errorf("expected approval response status 'approved', got %q", approvalResp.Status)
		}
		if approvalResp.Scope != "project" {
			t.Errorf("expected approval response scope 'project', got %q", approvalResp.Scope)
		}
	default:
		t.Error("expected approval response on channel")
	}

	// Verify request was removed from queue
	if domainQueue.Len() != 0 {
		t.Errorf("expected domain queue to be empty, got %d", domainQueue.Len())
	}
}

func TestServer_HandleApproveDomain_GlobalScope(t *testing.T) {
	queue := NewQueue()
	domainQueue := NewDomainQueue()

	// Add a test domain request with a response channel
	respChan := make(chan DomainResponse, 1)
	domainReq := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}
	id, err := domainQueue.Add(domainReq)
	if err != nil {
		t.Fatalf("failed to add domain request: %v", err)
	}

	mockPersister := &mockConfigPersister{}

	server := NewServer(queue, nil)
	server.DomainQueue = domainQueue
	server.ConfigPersister = mockPersister

	body := `{"scope": "global"}`
	httpReq := httptest.NewRequest(http.MethodPost, "/approve-domain/"+id, bytes.NewBufferString(body))
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	server.handleApproveDomain(rr, httpReq)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp approveDomainResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "approved" {
		t.Errorf("expected status 'approved', got %q", resp.Status)
	}
	if resp.Scope != "global" {
		t.Errorf("expected scope 'global', got %q", resp.Scope)
	}

	// Verify ConfigPersister.AddDomainToGlobal was called
	if len(mockPersister.addDomainToGlobalCalls) != 1 {
		t.Fatalf("expected AddDomainToGlobal to be called once, got %d calls", len(mockPersister.addDomainToGlobalCalls))
	}
	call := mockPersister.addDomainToGlobalCalls[0]
	if call.domain != "example.com" {
		t.Errorf("expected domain 'example.com', got %q", call.domain)
	}

	// Verify the response was sent on the channel
	select {
	case approvalResp := <-respChan:
		if approvalResp.Status != "approved" {
			t.Errorf("expected approval response status 'approved', got %q", approvalResp.Status)
		}
		if approvalResp.Scope != "global" {
			t.Errorf("expected approval response scope 'global', got %q", approvalResp.Scope)
		}
	default:
		t.Error("expected approval response on channel")
	}

	// Verify request was removed from queue
	if domainQueue.Len() != 0 {
		t.Errorf("expected domain queue to be empty, got %d", domainQueue.Len())
	}
}

func TestServer_HandleApproveDomain_PatternProjectScope(t *testing.T) {
	queue := NewQueue()
	domainQueue := NewDomainQueue()

	// Add a test domain request with a response channel
	respChan := make(chan DomainResponse, 1)
	domainReq := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "api.example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}
	id, err := domainQueue.Add(domainReq)
	if err != nil {
		t.Fatalf("failed to add domain request: %v", err)
	}

	mockPersister := &mockConfigPersister{}

	server := NewServer(queue, nil)
	server.DomainQueue = domainQueue
	server.ConfigPersister = mockPersister

	// Approve with a wildcard pattern instead of exact domain
	body := `{"scope": "project", "pattern": "*.example.com"}`
	httpReq := httptest.NewRequest(http.MethodPost, "/approve-domain/"+id, bytes.NewBufferString(body))
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	server.handleApproveDomain(rr, httpReq)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp approveDomainResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "approved" {
		t.Errorf("expected status 'approved', got %q", resp.Status)
	}
	if resp.Scope != "project" {
		t.Errorf("expected scope 'project', got %q", resp.Scope)
	}

	// Verify AddPatternToProject was called instead of AddDomainToProject
	if len(mockPersister.addDomainToProjectCalls) != 0 {
		t.Errorf("expected AddDomainToProject NOT to be called, got %d calls", len(mockPersister.addDomainToProjectCalls))
	}
	if len(mockPersister.addPatternToProjectCalls) != 1 {
		t.Fatalf("expected AddPatternToProject to be called once, got %d calls", len(mockPersister.addPatternToProjectCalls))
	}
	call := mockPersister.addPatternToProjectCalls[0]
	if call.project != "test-project" {
		t.Errorf("expected project 'test-project', got %q", call.project)
	}
	if call.pattern != "*.example.com" {
		t.Errorf("expected pattern '*.example.com', got %q", call.pattern)
	}

	// Verify the response was sent on the channel with pattern info
	select {
	case approvalResp := <-respChan:
		if approvalResp.Status != "approved" {
			t.Errorf("expected approval response status 'approved', got %q", approvalResp.Status)
		}
		if approvalResp.Scope != "project" {
			t.Errorf("expected approval response scope 'project', got %q", approvalResp.Scope)
		}
		if approvalResp.Pattern != "*.example.com" {
			t.Errorf("expected approval response pattern '*.example.com', got %q", approvalResp.Pattern)
		}
	default:
		t.Error("expected approval response on channel")
	}

	// Verify request was removed from queue
	if domainQueue.Len() != 0 {
		t.Errorf("expected domain queue to be empty, got %d", domainQueue.Len())
	}
}

func TestServer_HandleApproveDomain_PatternGlobalScope(t *testing.T) {
	queue := NewQueue()
	domainQueue := NewDomainQueue()

	// Add a test domain request with a response channel
	respChan := make(chan DomainResponse, 1)
	domainReq := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "cdn.example.org",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}
	id, err := domainQueue.Add(domainReq)
	if err != nil {
		t.Fatalf("failed to add domain request: %v", err)
	}

	mockPersister := &mockConfigPersister{}

	server := NewServer(queue, nil)
	server.DomainQueue = domainQueue
	server.ConfigPersister = mockPersister

	// Approve with a wildcard pattern at global scope
	body := `{"scope": "global", "pattern": "*.example.org"}`
	httpReq := httptest.NewRequest(http.MethodPost, "/approve-domain/"+id, bytes.NewBufferString(body))
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	server.handleApproveDomain(rr, httpReq)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp approveDomainResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "approved" {
		t.Errorf("expected status 'approved', got %q", resp.Status)
	}
	if resp.Scope != "global" {
		t.Errorf("expected scope 'global', got %q", resp.Scope)
	}

	// Verify AddPatternToGlobal was called instead of AddDomainToGlobal
	if len(mockPersister.addDomainToGlobalCalls) != 0 {
		t.Errorf("expected AddDomainToGlobal NOT to be called, got %d calls", len(mockPersister.addDomainToGlobalCalls))
	}
	if len(mockPersister.addPatternToGlobalCalls) != 1 {
		t.Fatalf("expected AddPatternToGlobal to be called once, got %d calls", len(mockPersister.addPatternToGlobalCalls))
	}
	call := mockPersister.addPatternToGlobalCalls[0]
	if call.pattern != "*.example.org" {
		t.Errorf("expected pattern '*.example.org', got %q", call.pattern)
	}

	// Verify the response was sent on the channel with pattern info
	select {
	case approvalResp := <-respChan:
		if approvalResp.Status != "approved" {
			t.Errorf("expected approval response status 'approved', got %q", approvalResp.Status)
		}
		if approvalResp.Scope != "global" {
			t.Errorf("expected approval response scope 'global', got %q", approvalResp.Scope)
		}
		if approvalResp.Pattern != "*.example.org" {
			t.Errorf("expected approval response pattern '*.example.org', got %q", approvalResp.Pattern)
		}
	default:
		t.Error("expected approval response on channel")
	}

	// Verify request was removed from queue
	if domainQueue.Len() != 0 {
		t.Errorf("expected domain queue to be empty, got %d", domainQueue.Len())
	}
}

func TestServer_HandleDenyDomain_Success(t *testing.T) {
	queue := NewQueue()
	domainQueue := NewDomainQueue()

	// Add a test domain request with a response channel
	respChan := make(chan DomainResponse, 1)
	domainReq := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "suspicious.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}
	id, err := domainQueue.Add(domainReq)
	if err != nil {
		t.Fatalf("failed to add domain request: %v", err)
	}

	server := NewServer(queue, nil)
	server.DomainQueue = domainQueue

	httpReq := httptest.NewRequest(http.MethodPost, "/deny-domain/"+id, nil)
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	server.handleDenyDomain(rr, httpReq)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp denyDomainResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "denied" {
		t.Errorf("expected status 'denied', got %q", resp.Status)
	}
	if resp.ID != id {
		t.Errorf("expected ID %s, got %s", id, resp.ID)
	}

	// Verify the response was sent on the channel with default reason
	select {
	case denyResp := <-respChan:
		if denyResp.Status != "denied" {
			t.Errorf("expected denial response status 'denied', got %q", denyResp.Status)
		}
		if !strings.Contains(denyResp.Reason, "Denied by") {
			t.Errorf("expected default reason to start with 'Denied by', got %q", denyResp.Reason)
		}
	default:
		t.Error("expected denial response on channel")
	}

	// Verify request was removed from queue
	if domainQueue.Len() != 0 {
		t.Errorf("expected domain queue to be empty, got %d", domainQueue.Len())
	}
}

func TestServer_HandleDenyDomain_WithReason(t *testing.T) {
	queue := NewQueue()
	domainQueue := NewDomainQueue()

	// Add a test domain request with a response channel
	respChan := make(chan DomainResponse, 1)
	domainReq := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "suspicious.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}
	id, err := domainQueue.Add(domainReq)
	if err != nil {
		t.Fatalf("failed to add domain request: %v", err)
	}

	server := NewServer(queue, nil)
	server.DomainQueue = domainQueue

	body := `{"reason": "Domain is known malware"}`
	httpReq := httptest.NewRequest(http.MethodPost, "/deny-domain/"+id, bytes.NewBufferString(body))
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	server.handleDenyDomain(rr, httpReq)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	// Verify the response was sent with custom reason
	select {
	case denyResp := <-respChan:
		if denyResp.Status != "denied" {
			t.Errorf("expected denial response status 'denied', got %q", denyResp.Status)
		}
		if denyResp.Reason != "Domain is known malware" {
			t.Errorf("expected reason 'Domain is known malware', got %q", denyResp.Reason)
		}
	default:
		t.Error("expected denial response on channel")
	}
}

func TestServer_HandleApproveDomain_NotFound(t *testing.T) {
	queue := NewQueue()
	domainQueue := NewDomainQueue()

	server := NewServer(queue, nil)
	server.DomainQueue = domainQueue

	body := `{"scope": "session"}`
	req := httptest.NewRequest(http.MethodPost, "/approve-domain/nonexistent", bytes.NewBufferString(body))
	req.SetPathValue("id", "nonexistent")
	rr := httptest.NewRecorder()

	server.handleApproveDomain(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}

	var resp errorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error != "request not found" {
		t.Errorf("expected error 'request not found', got %q", resp.Error)
	}
}

func TestServer_HandleDenyDomain_NotFound(t *testing.T) {
	queue := NewQueue()
	domainQueue := NewDomainQueue()

	server := NewServer(queue, nil)
	server.DomainQueue = domainQueue

	req := httptest.NewRequest(http.MethodPost, "/deny-domain/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	rr := httptest.NewRecorder()

	server.handleDenyDomain(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}

	var resp errorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error != "request not found" {
		t.Errorf("expected error 'request not found', got %q", resp.Error)
	}
}

func TestServer_HandleApproveDomain_InvalidScope(t *testing.T) {
	queue := NewQueue()
	domainQueue := NewDomainQueue()

	// Add a test domain request
	respChan := make(chan DomainResponse, 1)
	domainReq := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}
	id, err := domainQueue.Add(domainReq)
	if err != nil {
		t.Fatalf("failed to add domain request: %v", err)
	}

	server := NewServer(queue, nil)
	server.DomainQueue = domainQueue

	body := `{"scope": "invalid"}`
	httpReq := httptest.NewRequest(http.MethodPost, "/approve-domain/"+id, bytes.NewBufferString(body))
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	server.handleApproveDomain(rr, httpReq)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}

	var resp errorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error != "scope must be session, project, or global" {
		t.Errorf("expected error 'scope must be session, project, or global', got %q", resp.Error)
	}
}

func TestServer_HandleApproveDomain_MissingConfigPersister(t *testing.T) {
	queue := NewQueue()
	domainQueue := NewDomainQueue()

	// Add a test domain request
	respChan := make(chan DomainResponse, 1)
	domainReq := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}
	id, err := domainQueue.Add(domainReq)
	if err != nil {
		t.Fatalf("failed to add domain request: %v", err)
	}

	server := NewServer(queue, nil)
	server.DomainQueue = domainQueue
	// ConfigPersister is nil

	body := `{"scope": "project"}`
	httpReq := httptest.NewRequest(http.MethodPost, "/approve-domain/"+id, bytes.NewBufferString(body))
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	server.handleApproveDomain(rr, httpReq)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}

	var resp errorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error != "config persistence not available" {
		t.Errorf("expected error 'config persistence not available', got %q", resp.Error)
	}
}

func TestServer_SetDomainQueue(t *testing.T) {
	queue := NewQueue()
	server := NewServer(queue, nil)

	// Verify server has an EventHub
	if server.Events == nil {
		t.Fatal("expected server to have EventHub")
	}

	// Create a domain queue
	domainQueue := NewDomainQueue()

	// Initially the domain queue should not have an event hub
	if domainQueue.events != nil {
		t.Error("expected new domain queue to have nil event hub")
	}

	// Set the domain queue on the server
	server.SetDomainQueue(domainQueue)

	// Verify the domain queue was set
	if server.DomainQueue != domainQueue {
		t.Error("expected server.DomainQueue to be set")
	}

	// Verify the event hub was wired to the domain queue
	if domainQueue.events == nil {
		t.Error("expected domain queue event hub to be wired after SetDomainQueue")
	}

	// Verify it's the same event hub
	if domainQueue.events != server.Events {
		t.Error("expected domain queue to use server's event hub")
	}

	// Verify events are actually broadcast
	ch := server.Events.Subscribe()
	defer server.Events.Unsubscribe(ch)

	domainReq := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{make(chan DomainResponse, 1)},
	}

	_, err := domainQueue.Add(domainReq)
	if err != nil {
		t.Fatalf("failed to add domain request: %v", err)
	}

	// Should receive the broadcast event
	select {
	case event := <-ch:
		if event.Type != EventDomainRequestAdded {
			t.Errorf("expected event type %s, got %s", EventDomainRequestAdded, event.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for domain request added event")
	}
}

func TestServer_SetDomainQueue_Nil(t *testing.T) {
	queue := NewQueue()
	server := NewServer(queue, nil)

	// Setting nil domain queue should not panic
	server.SetDomainQueue(nil)

	if server.DomainQueue != nil {
		t.Error("expected server.DomainQueue to be nil")
	}
}

// Domain audit logging tests

func TestServer_HandleApproveDomain_AuditLogging(t *testing.T) {
	queue := NewQueue()
	domainQueue := NewDomainQueue()

	// Add a test domain request with a response channel
	respChan := make(chan DomainResponse, 1)
	req := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "api.example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}
	id, err := domainQueue.Add(req)
	if err != nil {
		t.Fatalf("failed to add domain request: %v", err)
	}

	// Create a buffer to capture audit logs
	var auditBuf bytes.Buffer
	auditLogger := audit.NewLogger(&auditBuf)

	server := NewServer(queue, auditLogger)
	server.SetDomainQueue(domainQueue)

	// Approve the domain request with project scope
	body := `{"scope": "project"}`
	httpReq := httptest.NewRequest(http.MethodPost, "/approve-domain/"+id, bytes.NewBufferString(body))
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	// Mock config persister
	server.ConfigPersister = &mockConfigPersister{}

	server.handleApproveDomain(rr, httpReq)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	// Verify audit logs
	auditOutput := auditBuf.String()

	// Should have DOMAIN_APPROVE event
	if !strings.Contains(auditOutput, "DOMAIN DOMAIN_APPROVE") {
		t.Errorf("expected audit log to contain DOMAIN_APPROVE event, got: %s", auditOutput)
	}

	// Should contain project and cloister
	if !strings.Contains(auditOutput, "project=test-project") {
		t.Errorf("expected audit log to contain project=test-project, got: %s", auditOutput)
	}

	if !strings.Contains(auditOutput, "cloister=test-cloister") {
		t.Errorf("expected audit log to contain cloister=test-cloister, got: %s", auditOutput)
	}

	// Should contain domain
	if !strings.Contains(auditOutput, `domain="api.example.com"`) {
		t.Errorf("expected audit log to contain domain, got: %s", auditOutput)
	}

	// Should contain scope
	if !strings.Contains(auditOutput, `scope="project"`) {
		t.Errorf("expected audit log to contain scope=project, got: %s", auditOutput)
	}

	// Should contain user
	if !strings.Contains(auditOutput, `user=`) {
		t.Errorf("expected audit log to contain user field, got: %s", auditOutput)
	}
}

func TestServer_HandleApproveDomain_AuditLogging_SessionScope(t *testing.T) {
	queue := NewQueue()
	domainQueue := NewDomainQueue()

	// Add a test domain request with a response channel
	respChan := make(chan DomainResponse, 1)
	req := &DomainRequest{
		Cloister:  "my-api-main",
		Project:   "my-api",
		Domain:    "cdn.example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}
	id, err := domainQueue.Add(req)
	if err != nil {
		t.Fatalf("failed to add domain request: %v", err)
	}

	// Create a buffer to capture audit logs
	var auditBuf bytes.Buffer
	auditLogger := audit.NewLogger(&auditBuf)

	server := NewServer(queue, auditLogger)
	server.SetDomainQueue(domainQueue)

	// Approve with session scope (no config persister needed)
	body := `{"scope": "session"}`
	httpReq := httptest.NewRequest(http.MethodPost, "/approve-domain/"+id, bytes.NewBufferString(body))
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	server.handleApproveDomain(rr, httpReq)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	// Verify audit logs
	auditOutput := auditBuf.String()

	// Should have DOMAIN_APPROVE event
	if !strings.Contains(auditOutput, "DOMAIN DOMAIN_APPROVE") {
		t.Errorf("expected audit log to contain DOMAIN_APPROVE event, got: %s", auditOutput)
	}

	// Should contain session scope
	if !strings.Contains(auditOutput, `scope="session"`) {
		t.Errorf("expected audit log to contain scope=session, got: %s", auditOutput)
	}
}

func TestServer_HandleDenyDomain_AuditLogging(t *testing.T) {
	queue := NewQueue()
	domainQueue := NewDomainQueue()

	// Add a test domain request with a response channel
	respChan := make(chan DomainResponse, 1)
	req := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "malicious.example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}
	id, err := domainQueue.Add(req)
	if err != nil {
		t.Fatalf("failed to add domain request: %v", err)
	}

	// Create a buffer to capture audit logs
	var auditBuf bytes.Buffer
	auditLogger := audit.NewLogger(&auditBuf)

	server := NewServer(queue, auditLogger)
	server.SetDomainQueue(domainQueue)

	// Deny with reason
	body := `{"reason": "Domain looks suspicious"}`
	httpReq := httptest.NewRequest(http.MethodPost, "/deny-domain/"+id, bytes.NewBufferString(body))
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	server.handleDenyDomain(rr, httpReq)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	// Verify audit logs
	auditOutput := auditBuf.String()

	// Should have DOMAIN_DENY event
	if !strings.Contains(auditOutput, "DOMAIN DOMAIN_DENY") {
		t.Errorf("expected audit log to contain DOMAIN_DENY event, got: %s", auditOutput)
	}

	// Should contain project and cloister
	if !strings.Contains(auditOutput, "project=test-project") {
		t.Errorf("expected audit log to contain project=test-project, got: %s", auditOutput)
	}

	if !strings.Contains(auditOutput, "cloister=test-cloister") {
		t.Errorf("expected audit log to contain cloister=test-cloister, got: %s", auditOutput)
	}

	// Should contain domain
	if !strings.Contains(auditOutput, `domain="malicious.example.com"`) {
		t.Errorf("expected audit log to contain domain, got: %s", auditOutput)
	}

	// Should contain reason
	if !strings.Contains(auditOutput, `reason="Domain looks suspicious"`) {
		t.Errorf("expected audit log to contain reason, got: %s", auditOutput)
	}
}

func TestServer_HandleDenyDomain_AuditLogging_DefaultReason(t *testing.T) {
	queue := NewQueue()
	domainQueue := NewDomainQueue()

	// Add a test domain request with a response channel
	respChan := make(chan DomainResponse, 1)
	req := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "blocked.example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}
	id, err := domainQueue.Add(req)
	if err != nil {
		t.Fatalf("failed to add domain request: %v", err)
	}

	// Create a buffer to capture audit logs
	var auditBuf bytes.Buffer
	auditLogger := audit.NewLogger(&auditBuf)

	server := NewServer(queue, auditLogger)
	server.SetDomainQueue(domainQueue)

	// Deny without providing a reason
	httpReq := httptest.NewRequest(http.MethodPost, "/deny-domain/"+id, nil)
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	server.handleDenyDomain(rr, httpReq)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	// Verify audit logs include default reason
	auditOutput := auditBuf.String()

	if !strings.Contains(auditOutput, "DOMAIN DOMAIN_DENY") {
		t.Errorf("expected audit log to contain DOMAIN_DENY event, got: %s", auditOutput)
	}

	if !strings.Contains(auditOutput, `reason="Denied by`) {
		t.Errorf("expected audit log to contain default reason starting with 'Denied by', got: %s", auditOutput)
	}
}

func TestDomainQueue_AuditLogging_Request(t *testing.T) {
	// Create a buffer to capture audit logs
	var auditBuf bytes.Buffer
	auditLogger := audit.NewLogger(&auditBuf)

	domainQueue := NewDomainQueue()
	domainQueue.SetAuditLogger(auditLogger)

	// Add a domain request
	respChan := make(chan DomainResponse, 1)
	req := &DomainRequest{
		Cloister:  "my-api-main",
		Project:   "my-api",
		Domain:    "api.example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}
	_, err := domainQueue.Add(req)
	if err != nil {
		t.Fatalf("failed to add domain request: %v", err)
	}

	// Verify audit logs
	auditOutput := auditBuf.String()

	// Should have DOMAIN_REQUEST event
	if !strings.Contains(auditOutput, "DOMAIN DOMAIN_REQUEST") {
		t.Errorf("expected audit log to contain DOMAIN_REQUEST event, got: %s", auditOutput)
	}

	// Should contain project and cloister
	if !strings.Contains(auditOutput, "project=my-api") {
		t.Errorf("expected audit log to contain project=my-api, got: %s", auditOutput)
	}

	if !strings.Contains(auditOutput, "cloister=my-api-main") {
		t.Errorf("expected audit log to contain cloister=my-api-main, got: %s", auditOutput)
	}

	// Should contain domain
	if !strings.Contains(auditOutput, `domain="api.example.com"`) {
		t.Errorf("expected audit log to contain domain, got: %s", auditOutput)
	}
}

func TestDomainQueue_AuditLogging_Timeout(t *testing.T) {
	// Create a buffer to capture audit logs
	var auditBuf bytes.Buffer
	auditLogger := audit.NewLogger(&auditBuf)

	// Create queue with very short timeout
	domainQueue := NewDomainQueueWithTimeout(10 * time.Millisecond)
	domainQueue.SetAuditLogger(auditLogger)

	// Add a domain request
	respChan := make(chan DomainResponse, 1)
	req := &DomainRequest{
		Cloister:  "my-api-main",
		Project:   "my-api",
		Domain:    "slow.example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}
	_, err := domainQueue.Add(req)
	if err != nil {
		t.Fatalf("failed to add domain request: %v", err)
	}

	// Wait for the timeout response — this synchronizes with the timeout goroutine
	// which writes audit logs before sending on respChan, so reading here guarantees
	// the audit writes have completed (no race on auditBuf).
	select {
	case <-respChan:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for domain timeout response")
	}

	// Verify audit logs
	auditOutput := auditBuf.String()

	// Should have both DOMAIN_REQUEST and DOMAIN_TIMEOUT events
	if !strings.Contains(auditOutput, "DOMAIN DOMAIN_REQUEST") {
		t.Errorf("expected audit log to contain DOMAIN_REQUEST event, got: %s", auditOutput)
	}

	if !strings.Contains(auditOutput, "DOMAIN DOMAIN_TIMEOUT") {
		t.Errorf("expected audit log to contain DOMAIN_TIMEOUT event, got: %s", auditOutput)
	}

	// DOMAIN_TIMEOUT should contain the same domain
	if strings.Count(auditOutput, `domain="slow.example.com"`) != 2 {
		t.Errorf("expected domain to appear in both REQUEST and TIMEOUT events, got: %s", auditOutput)
	}
}

func TestServer_HandleApproveDomain_NilAuditLogger(t *testing.T) {
	queue := NewQueue()
	domainQueue := NewDomainQueue()

	// Add a test domain request
	respChan := make(chan DomainResponse, 1)
	req := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "api.example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}
	id, err := domainQueue.Add(req)
	if err != nil {
		t.Fatalf("failed to add domain request: %v", err)
	}

	// Create server with nil audit logger
	server := NewServer(queue, nil)
	server.SetDomainQueue(domainQueue)

	// Approve the domain request (should not panic with nil logger)
	body := `{"scope": "session"}`
	httpReq := httptest.NewRequest(http.MethodPost, "/approve-domain/"+id, bytes.NewBufferString(body))
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	server.handleApproveDomain(rr, httpReq)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestServer_HandleApproveDomain_PersistenceError_FallsBackToSession(t *testing.T) {
	queue := NewQueue()
	domainQueue := NewDomainQueue()

	// Add a test domain request with a response channel
	respChan := make(chan DomainResponse, 1)
	domainReq := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}
	id, err := domainQueue.Add(domainReq)
	if err != nil {
		t.Fatalf("failed to add domain request: %v", err)
	}

	// Create a mock persister that returns an error
	mockPersister := &mockConfigPersister{
		addDomainToProjectErr: errors.New("permission denied"),
	}

	server := NewServer(queue, nil)
	server.DomainQueue = domainQueue
	server.ConfigPersister = mockPersister

	body := `{"scope": "project"}`
	httpReq := httptest.NewRequest(http.MethodPost, "/approve-domain/"+id, bytes.NewBufferString(body))
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	server.handleApproveDomain(rr, httpReq)

	// Should still succeed (not return an error)
	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp approveDomainResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should be approved
	if resp.Status != "approved" {
		t.Errorf("expected status 'approved', got %q", resp.Status)
	}

	// Scope should fall back to session
	if resp.Scope != "session" {
		t.Errorf("expected scope 'session' (fallback), got %q", resp.Scope)
	}

	// Verify the response was sent on the channel
	select {
	case approvalResp := <-respChan:
		if approvalResp.Status != "approved" {
			t.Errorf("expected approval response status 'approved', got %q", approvalResp.Status)
		}
		if approvalResp.Scope != "session" {
			t.Errorf("expected approval response scope 'session' (fallback), got %q", approvalResp.Scope)
		}
		// Should include persistence error message
		if approvalResp.PersistenceError == "" {
			t.Error("expected persistence error to be set")
		}
		if !strings.Contains(approvalResp.PersistenceError, "permission denied") {
			t.Errorf("expected persistence error to contain 'permission denied', got %q", approvalResp.PersistenceError)
		}
	default:
		t.Error("expected approval response on channel")
	}

	// Verify request was removed from queue
	if domainQueue.Len() != 0 {
		t.Errorf("expected domain queue to be empty, got %d", domainQueue.Len())
	}
}

func TestServer_HandleApproveDomain_PersistenceError_GlobalFallsBackToSession(t *testing.T) {
	queue := NewQueue()
	domainQueue := NewDomainQueue()

	// Add a test domain request with a response channel
	respChan := make(chan DomainResponse, 1)
	domainReq := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}
	id, err := domainQueue.Add(domainReq)
	if err != nil {
		t.Fatalf("failed to add domain request: %v", err)
	}

	// Create a mock persister that returns an error for global config
	mockPersister := &mockConfigPersister{
		addDomainToGlobalErr: errors.New("read-only filesystem"),
	}

	server := NewServer(queue, nil)
	server.DomainQueue = domainQueue
	server.ConfigPersister = mockPersister

	body := `{"scope": "global"}`
	httpReq := httptest.NewRequest(http.MethodPost, "/approve-domain/"+id, bytes.NewBufferString(body))
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	server.handleApproveDomain(rr, httpReq)

	// Should still succeed (not return an error)
	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp approveDomainResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should be approved
	if resp.Status != "approved" {
		t.Errorf("expected status 'approved', got %q", resp.Status)
	}

	// Scope should fall back to session
	if resp.Scope != "session" {
		t.Errorf("expected scope 'session' (fallback), got %q", resp.Scope)
	}

	// Verify the response was sent on the channel with error info
	select {
	case approvalResp := <-respChan:
		if approvalResp.Status != "approved" {
			t.Errorf("expected approval response status 'approved', got %q", approvalResp.Status)
		}
		if approvalResp.Scope != "session" {
			t.Errorf("expected approval response scope 'session' (fallback), got %q", approvalResp.Scope)
		}
		// Should include persistence error message
		if !strings.Contains(approvalResp.PersistenceError, "read-only filesystem") {
			t.Errorf("expected persistence error to contain 'read-only filesystem', got %q", approvalResp.PersistenceError)
		}
	default:
		t.Error("expected approval response on channel")
	}
}

func TestServer_HandleApproveDomain_PersistenceError_PatternFallsBackToSession(t *testing.T) {
	queue := NewQueue()
	domainQueue := NewDomainQueue()

	// Add a test domain request with a response channel
	respChan := make(chan DomainResponse, 1)
	domainReq := &DomainRequest{
		Cloister:  "test-cloister",
		Project:   "test-project",
		Domain:    "api.example.com",
		Timestamp: time.Now(),
		Responses: []chan<- DomainResponse{respChan},
	}
	id, err := domainQueue.Add(domainReq)
	if err != nil {
		t.Fatalf("failed to add domain request: %v", err)
	}

	// Create a mock persister that returns an error for patterns
	mockPersister := &mockConfigPersister{
		addPatternToProjectErr: errors.New("disk full"),
	}

	server := NewServer(queue, nil)
	server.DomainQueue = domainQueue
	server.ConfigPersister = mockPersister

	// Approve with wildcard pattern
	body := `{"scope": "project", "pattern": "*.example.com"}`
	httpReq := httptest.NewRequest(http.MethodPost, "/approve-domain/"+id, bytes.NewBufferString(body))
	httpReq.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	server.handleApproveDomain(rr, httpReq)

	// Should still succeed
	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp approveDomainResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should be approved with fallback to session
	if resp.Status != "approved" {
		t.Errorf("expected status 'approved', got %q", resp.Status)
	}
	if resp.Scope != "session" {
		t.Errorf("expected scope 'session' (fallback), got %q", resp.Scope)
	}

	// Verify the response was sent on the channel
	select {
	case approvalResp := <-respChan:
		if approvalResp.Status != "approved" {
			t.Errorf("expected approval response status 'approved', got %q", approvalResp.Status)
		}
		if approvalResp.Scope != "session" {
			t.Errorf("expected approval response scope 'session' (fallback), got %q", approvalResp.Scope)
		}
		// Pattern should be cleared since we fell back to session
		if approvalResp.Pattern != "" {
			t.Errorf("expected pattern to be cleared on fallback, got %q", approvalResp.Pattern)
		}
		// Should include persistence error message
		if !strings.Contains(approvalResp.PersistenceError, "disk full") {
			t.Errorf("expected persistence error to contain 'disk full', got %q", approvalResp.PersistenceError)
		}
	default:
		t.Error("expected approval response on channel")
	}
}

func TestTemplates_DomainResultWithPersistenceError(t *testing.T) {
	// Test that domain_result template renders persistence warning
	data := domainResultData{
		ID:               "domain123",
		Domain:           "example.com",
		Status:           "approved",
		Scope:            "session",
		IsPattern:        false,
		PersistenceError: "failed to persist to project config: permission denied",
	}

	var buf bytes.Buffer
	err := templates.ExecuteTemplate(&buf, "domain_result", data)
	if err != nil {
		t.Fatalf("failed to execute domain_result template with persistence error: %v", err)
	}

	output := buf.String()

	// Verify key content is present
	if !strings.Contains(output, "domain123") {
		t.Error("expected output to contain request ID 'domain123'")
	}
	if !strings.Contains(output, "example.com") {
		t.Error("expected output to contain 'example.com'")
	}
	if !strings.Contains(output, "Approved") {
		t.Error("expected output to contain 'Approved'")
	}
	if !strings.Contains(output, "session") {
		t.Error("expected output to contain scope 'session'")
	}
	// Should contain the warning class
	if !strings.Contains(output, "request-warning") {
		t.Error("expected output to contain 'request-warning' class")
	}
	// Should contain the persistence warning
	if !strings.Contains(output, "persistence-warning") {
		t.Error("expected output to contain 'persistence-warning' div")
	}
	// Should contain the warning text
	if !strings.Contains(output, "Approved for session only") {
		t.Error("expected output to contain 'Approved for session only'")
	}
	if !strings.Contains(output, "permission denied") {
		t.Error("expected output to contain the error message")
	}
}
