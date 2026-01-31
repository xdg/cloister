package request

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/guardian/approval"
	"github.com/xdg/cloister/internal/guardian/patterns"
)

func TestNewServer(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"test-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	server := NewServer(lookup, nil, nil)

	if server == nil {
		t.Fatal("NewServer returned nil")
	}
	if server.Addr != ":9998" {
		t.Errorf("expected addr :9998, got %s", server.Addr)
	}
	if server.TokenLookup == nil {
		t.Error("TokenLookup should be set")
	}
}

func TestServer_StartStop(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"test-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	server := NewServer(lookup, nil, nil)
	server.Addr = ":0" // Use random port

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

	// ListenAddr should still return the old address (listener is closed but not nil)
	// This is expected behavior - we don't nil out the listener

	// Stop again should be idempotent
	if err := server.Stop(context.Background()); err != nil {
		t.Errorf("second Stop failed: %v", err)
	}
}

func TestServer_HandleRequest_MissingToken(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	server := NewServer(lookup, nil, nil)

	// Create a test request handler by manually wrapping with auth middleware
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	body := `{"cmd": "echo hello"}`
	req := httptest.NewRequest(http.MethodPost, "/request", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// Note: no X-Cloister-Token header

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestServer_HandleRequest_InvalidToken(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	server := NewServer(lookup, nil, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	body := `{"cmd": "echo hello"}`
	req := httptest.NewRequest(http.MethodPost, "/request", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, "invalid-token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestServer_HandleRequest_InvalidJSON(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	server := NewServer(lookup, nil, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	req := httptest.NewRequest(http.MethodPost, "/request", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, "valid-token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}

	var resp CommandResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "error" {
		t.Errorf("expected status 'error', got %q", resp.Status)
	}
}

func TestServer_HandleRequest_EmptyCmd(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	server := NewServer(lookup, nil, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	body := `{"cmd": ""}`
	req := httptest.NewRequest(http.MethodPost, "/request", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, "valid-token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}

	var resp CommandResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "error" {
		t.Errorf("expected status 'error', got %q", resp.Status)
	}
	if !strings.Contains(resp.Reason, "cmd is required") {
		t.Errorf("expected reason to contain 'cmd is required', got %q", resp.Reason)
	}
}

func TestServer_HandleRequest_ValidRequest_NilMatcher(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	server := NewServer(lookup, nil, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Cmd: "echo hello"}
	body, _ := json.Marshal(cmdReq)
	req := httptest.NewRequest(http.MethodPost, "/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, "valid-token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// With nil PatternMatcher, commands are denied
	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp CommandResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "denied" {
		t.Errorf("expected status 'denied', got %q", resp.Status)
	}
	if !strings.Contains(resp.Reason, "no approval patterns configured") {
		t.Errorf("expected reason to contain 'no approval patterns configured', got %q", resp.Reason)
	}
}

func TestServer_HandleRequest_ViaHTTPServer(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	matcher := &mockPatternMatcher{
		results: map[string]patterns.MatchResult{
			"echo hello": {Action: patterns.AutoApprove, Pattern: "^echo .*$"},
		},
	}

	server := NewServer(lookup, matcher, nil)
	server.Addr = ":0" // Use random port

	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = server.Stop(context.Background()) }()

	// Make a real HTTP request to the running server
	url := "http://" + server.ListenAddr() + "/request"
	cmdReq := CommandRequest{Cmd: "echo hello"}
	body, _ := json.Marshal(cmdReq)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, "valid-token")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	var cmdResp CommandResponse
	if err := json.NewDecoder(resp.Body).Decode(&cmdResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if cmdResp.Status != "auto_approved" {
		t.Errorf("expected status 'auto_approved', got %q", cmdResp.Status)
	}
}

func TestServer_ListenAddrBeforeStart(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{})
	server := NewServer(lookup, nil, nil)

	addr := server.ListenAddr()
	if addr != "" {
		t.Errorf("expected empty addr before Start, got %q", addr)
	}
}

// mockPatternMatcher implements PatternMatcher for testing
type mockPatternMatcher struct {
	results map[string]patterns.MatchResult
}

func (m *mockPatternMatcher) Match(cmd string) patterns.MatchResult {
	if result, ok := m.results[cmd]; ok {
		return result
	}
	// Default to deny if not in results map
	return patterns.MatchResult{Action: patterns.Deny}
}

func TestServer_HandleRequest_AutoApprove(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	matcher := &mockPatternMatcher{
		results: map[string]patterns.MatchResult{
			"docker compose ps": {Action: patterns.AutoApprove, Pattern: "^docker compose ps$"},
		},
	}

	server := NewServer(lookup, matcher, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Cmd: "docker compose ps"}
	body, _ := json.Marshal(cmdReq)
	req := httptest.NewRequest(http.MethodPost, "/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, "valid-token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp CommandResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "auto_approved" {
		t.Errorf("expected status 'auto_approved', got %q", resp.Status)
	}
	if resp.Pattern != "^docker compose ps$" {
		t.Errorf("expected pattern '^docker compose ps$', got %q", resp.Pattern)
	}
	// ExitCode should be 0 (placeholder success)
	if resp.ExitCode != 0 {
		t.Errorf("expected exit_code 0, got %d", resp.ExitCode)
	}
	// Stdout should contain placeholder message
	if resp.Stdout == "" {
		t.Error("expected stdout to contain placeholder message")
	}
}

func TestServer_HandleRequest_ManualApprove_NilQueue(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	matcher := &mockPatternMatcher{
		results: map[string]patterns.MatchResult{
			"docker compose up -d": {Action: patterns.ManualApprove, Pattern: "^docker compose (up|down).*$"},
		},
	}

	// Server with nil queue - should deny manual approval requests
	server := NewServer(lookup, matcher, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Cmd: "docker compose up -d"}
	body, _ := json.Marshal(cmdReq)
	req := httptest.NewRequest(http.MethodPost, "/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, "valid-token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp CommandResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "denied" {
		t.Errorf("expected status 'denied', got %q", resp.Status)
	}
	if !strings.Contains(resp.Reason, "approval queue not configured") {
		t.Errorf("expected reason to contain 'approval queue not configured', got %q", resp.Reason)
	}
}

func TestServer_HandleRequest_Deny(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	// Matcher with no patterns that match "rm -rf /"
	matcher := &mockPatternMatcher{
		results: map[string]patterns.MatchResult{
			// Only safe commands are configured
			"docker compose ps": {Action: patterns.AutoApprove, Pattern: "^docker compose ps$"},
		},
	}

	server := NewServer(lookup, matcher, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Cmd: "rm -rf /"}
	body, _ := json.Marshal(cmdReq)
	req := httptest.NewRequest(http.MethodPost, "/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, "valid-token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp CommandResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "denied" {
		t.Errorf("expected status 'denied', got %q", resp.Status)
	}
	if !strings.Contains(resp.Reason, "does not match") {
		t.Errorf("expected reason to contain 'does not match', got %q", resp.Reason)
	}
}

func TestServer_HandleRequest_NoPatternMatcher(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	// Server with nil pattern matcher
	server := NewServer(lookup, nil, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Cmd: "echo hello"}
	body, _ := json.Marshal(cmdReq)
	req := httptest.NewRequest(http.MethodPost, "/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, "valid-token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp CommandResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "denied" {
		t.Errorf("expected status 'denied', got %q", resp.Status)
	}
	if !strings.Contains(resp.Reason, "no approval patterns configured") {
		t.Errorf("expected reason to contain 'no approval patterns configured', got %q", resp.Reason)
	}
}

func TestServer_HandleRequest_ManualApprove_BlocksUntilApproval(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	matcher := &mockPatternMatcher{
		results: map[string]patterns.MatchResult{
			"docker compose up -d": {Action: patterns.ManualApprove, Pattern: "^docker compose (up|down).*$"},
		},
	}

	// Create server with an approval queue
	queue := approval.NewQueue()
	server := NewServer(lookup, matcher, nil)
	server.Queue = queue
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Cmd: "docker compose up -d"}
	body, _ := json.Marshal(cmdReq)
	req := httptest.NewRequest(http.MethodPost, "/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, "valid-token")

	// Run the request handler in a goroutine since it will block
	done := make(chan struct{})
	rr := httptest.NewRecorder()

	go func() {
		handler.ServeHTTP(rr, req)
		close(done)
	}()

	// Wait a bit to ensure the request is queued
	time.Sleep(50 * time.Millisecond)

	// Verify request is in the queue
	pending := queue.List()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending request, got %d", len(pending))
	}

	if pending[0].Cloister != "test-cloister" {
		t.Errorf("expected cloister 'test-cloister', got %q", pending[0].Cloister)
	}
	if pending[0].Project != "test-project" {
		t.Errorf("expected project 'test-project', got %q", pending[0].Project)
	}
	if pending[0].Cmd != "docker compose up -d" {
		t.Errorf("expected cmd 'docker compose up -d', got %q", pending[0].Cmd)
	}

	// Get the actual request to send approval response
	actualReq, ok := queue.Get(pending[0].ID)
	if !ok {
		t.Fatal("failed to get pending request by ID")
	}

	// Send approval response
	actualReq.Response <- approval.Response{
		Status:   "approved",
		ExitCode: 0,
		Stdout:   "containers started",
	}
	queue.Remove(pending[0].ID)

	// Wait for handler to complete
	select {
	case <-done:
		// Handler completed
	case <-time.After(1 * time.Second):
		t.Fatal("handler did not complete after approval")
	}

	// Verify response
	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp CommandResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "approved" {
		t.Errorf("expected status 'approved', got %q", resp.Status)
	}
	if resp.ExitCode != 0 {
		t.Errorf("expected exit_code 0, got %d", resp.ExitCode)
	}
	if resp.Stdout != "containers started" {
		t.Errorf("expected stdout 'containers started', got %q", resp.Stdout)
	}
}

func TestServer_HandleRequest_ManualApprove_Timeout(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	matcher := &mockPatternMatcher{
		results: map[string]patterns.MatchResult{
			"docker compose up -d": {Action: patterns.ManualApprove, Pattern: "^docker compose (up|down).*$"},
		},
	}

	// Create server with a very short timeout queue
	queue := approval.NewQueueWithTimeout(100 * time.Millisecond)
	server := NewServer(lookup, matcher, nil)
	server.Queue = queue
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Cmd: "docker compose up -d"}
	body, _ := json.Marshal(cmdReq)
	req := httptest.NewRequest(http.MethodPost, "/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, "valid-token")

	// Run the request handler in a goroutine
	done := make(chan struct{})
	rr := httptest.NewRecorder()

	go func() {
		handler.ServeHTTP(rr, req)
		close(done)
	}()

	// Wait for timeout (should be ~100ms + some margin)
	select {
	case <-done:
		// Handler completed due to timeout
	case <-time.After(500 * time.Millisecond):
		t.Fatal("handler did not complete after timeout")
	}

	// Verify timeout response
	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp CommandResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "timeout" {
		t.Errorf("expected status 'timeout', got %q", resp.Status)
	}
	if resp.Reason == "" {
		t.Error("expected non-empty reason for timeout")
	}

	// Queue should be empty after timeout
	if queue.Len() != 0 {
		t.Errorf("queue should be empty after timeout, got len=%d", queue.Len())
	}
}

func TestServer_HandleRequest_ManualApprove_Denied(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	matcher := &mockPatternMatcher{
		results: map[string]patterns.MatchResult{
			"docker compose up -d": {Action: patterns.ManualApprove, Pattern: "^docker compose (up|down).*$"},
		},
	}

	// Create server with an approval queue
	queue := approval.NewQueue()
	server := NewServer(lookup, matcher, nil)
	server.Queue = queue
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Cmd: "docker compose up -d"}
	body, _ := json.Marshal(cmdReq)
	req := httptest.NewRequest(http.MethodPost, "/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, "valid-token")

	// Run the request handler in a goroutine
	done := make(chan struct{})
	rr := httptest.NewRecorder()

	go func() {
		handler.ServeHTTP(rr, req)
		close(done)
	}()

	// Wait a bit to ensure the request is queued
	time.Sleep(50 * time.Millisecond)

	// Verify request is in the queue
	pending := queue.List()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending request, got %d", len(pending))
	}

	// Get the actual request to send denial response
	actualReq, ok := queue.Get(pending[0].ID)
	if !ok {
		t.Fatal("failed to get pending request by ID")
	}

	// Send denial response
	actualReq.Response <- approval.Response{
		Status: "denied",
		Reason: "Denied by user: not safe",
	}
	queue.Remove(pending[0].ID)

	// Wait for handler to complete
	select {
	case <-done:
		// Handler completed
	case <-time.After(1 * time.Second):
		t.Fatal("handler did not complete after denial")
	}

	// Verify denial response
	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp CommandResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "denied" {
		t.Errorf("expected status 'denied', got %q", resp.Status)
	}
	if resp.Reason != "Denied by user: not safe" {
		t.Errorf("expected reason 'Denied by user: not safe', got %q", resp.Reason)
	}
}

func TestServer_HandleRequest_GETReturns405(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	server := NewServer(lookup, nil, nil)
	server.Addr = ":0" // Use random port

	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = server.Stop(context.Background()) }()

	// Make a GET request (should be rejected - POST only)
	url := "http://" + server.ListenAddr() + "/request"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set(TokenHeader, "valid-token")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Go's net/http returns 405 Method Not Allowed for unmatched methods
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, resp.StatusCode)
	}
}
