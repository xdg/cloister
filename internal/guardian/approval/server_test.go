package approval

import (
	"bytes"
	"context"
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	queue := NewQueue()
	server := NewServer(queue)

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
	server := NewServer(queue)
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
	server := NewServer(queue)

	addr := server.ListenAddr()
	if addr != "" {
		t.Errorf("expected empty addr before Start, got %q", addr)
	}
}

func TestServer_HandleIndex(t *testing.T) {
	queue := NewQueue()
	server := NewServer(queue)

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
	server := NewServer(queue)

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

	server := NewServer(queue)

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

	server := NewServer(queue)

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
	server := NewServer(queue)

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

	server := NewServer(queue)

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
		if denyResp.Reason != "Denied by user" {
			t.Errorf("expected default reason 'Denied by user', got %q", denyResp.Reason)
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

	server := NewServer(queue)

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
	server := NewServer(queue)

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
	server := NewServer(queue)
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
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET / expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	// Test GET /pending
	resp, err = http.Get(baseURL + "/pending")
	if err != nil {
		t.Fatalf("request to /pending failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /pending expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	// Test POST /approve/{id} (not found)
	resp, err = http.Post(baseURL+"/approve/nonexistent", "application/json", nil)
	if err != nil {
		t.Fatalf("request to /approve/nonexistent failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("POST /approve/nonexistent expected status %d, got %d", http.StatusNotFound, resp.StatusCode)
	}

	// Test POST /deny/{id} (not found)
	resp, err = http.Post(baseURL+"/deny/nonexistent", "application/json", nil)
	if err != nil {
		t.Fatalf("request to /deny/nonexistent failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("POST /deny/nonexistent expected status %d, got %d", http.StatusNotFound, resp.StatusCode)
	}
}

func TestServer_HandleApprove_MissingID(t *testing.T) {
	queue := NewQueue()
	server := NewServer(queue)

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
	server := NewServer(queue)

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

func TestServer_StaticHtmxServed(t *testing.T) {
	queue := NewQueue()
	server := NewServer(queue)
	server.Addr = "127.0.0.1:0" // Use random port

	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = server.Stop(context.Background()) }()

	baseURL := "http://" + server.ListenAddr()

	// Test GET /static/htmx.min.js
	resp, err := http.Get(baseURL + "/static/htmx.min.js")
	if err != nil {
		t.Fatalf("request to /static/htmx.min.js failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /static/htmx.min.js expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	// Verify Content-Type is application/javascript or text/javascript
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "javascript") {
		t.Errorf("expected Content-Type to contain 'javascript', got %q", contentType)
	}

	// Verify the response body contains htmx code
	body := make([]byte, 100)
	n, _ := resp.Body.Read(body)
	if n == 0 {
		t.Error("expected non-empty response body for htmx.min.js")
	}
	if !strings.Contains(string(body[:n]), "htmx") {
		t.Error("expected response body to contain 'htmx'")
	}
}

func TestServer_IndexIncludesHtmxScript(t *testing.T) {
	queue := NewQueue()
	server := NewServer(queue)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	server.handleIndex(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, `<script src="/static/htmx.min.js"></script>`) {
		t.Error("expected index.html to include htmx script tag")
	}
}
