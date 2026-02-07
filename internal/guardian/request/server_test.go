package request

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/audit"
	"github.com/xdg/cloister/internal/executor"
	"github.com/xdg/cloister/internal/guardian/approval"
	"github.com/xdg/cloister/internal/guardian/patterns"
	"github.com/xdg/cloister/internal/testutil"
)

// noProxyClient returns an HTTP client that doesn't use any proxy.
// Delegates to testutil.NoProxyClient for the canonical implementation.
func noProxyClient() *http.Client {
	return testutil.NoProxyClient()
}

func TestNewServer(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"test-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	server := NewServer(lookup, nil, nil, nil)

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

	server := NewServer(lookup, nil, nil, nil)
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

	server := NewServer(lookup, nil, nil, nil)

	// Create a test request handler by manually wrapping with auth middleware
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	body := `{"args": ["echo", "hello"]}`
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

	server := NewServer(lookup, nil, nil, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	body := `{"args": ["echo", "hello"]}`
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

	server := NewServer(lookup, nil, nil, nil)
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

func TestServer_HandleRequest_EmptyArgs(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	server := NewServer(lookup, nil, nil, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	body := `{"args": []}`
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
	if !strings.Contains(resp.Reason, "args is required") {
		t.Errorf("expected reason to contain 'args is required', got %q", resp.Reason)
	}
}

func TestServer_HandleRequest_ValidRequest_NilMatcher(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	server := NewServer(lookup, nil, nil, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Args: []string{"echo", "hello"}}
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

	mockExec := &mockCommandExecutor{}

	server := NewServer(lookup, matcher, mockExec, nil)
	server.Addr = ":0" // Use random port

	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = server.Stop(context.Background()) }()

	// Make a real HTTP request to the running server
	url := "http://" + server.ListenAddr() + "/request"
	cmdReq := CommandRequest{Args: []string{"echo", "hello"}}
	body, _ := json.Marshal(cmdReq)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, "valid-token")

	client := noProxyClient()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

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
	server := NewServer(lookup, nil, nil, nil)

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

// mockCommandExecutor implements CommandExecutor for testing
type mockCommandExecutor struct {
	responses map[string]*executor.ExecuteResponse
	err       error
}

func (m *mockCommandExecutor) Execute(req executor.ExecuteRequest) (*executor.ExecuteResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	if resp, ok := m.responses[req.Command]; ok {
		return resp, nil
	}
	// Default to a successful response
	return &executor.ExecuteResponse{
		Status:   executor.StatusCompleted,
		ExitCode: 0,
		Stdout:   "mock output for: " + req.Command,
	}, nil
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

	mockExec := &mockCommandExecutor{
		responses: map[string]*executor.ExecuteResponse{
			"docker": {
				Status:   executor.StatusCompleted,
				ExitCode: 0,
				Stdout:   "NAME    STATUS\nweb     running",
			},
		},
	}

	server := NewServer(lookup, matcher, mockExec, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Args: []string{"docker", "compose", "ps"}}
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
	// ExitCode should be 0 (from mock executor)
	if resp.ExitCode != 0 {
		t.Errorf("expected exit_code 0, got %d", resp.ExitCode)
	}
	// Stdout should contain the mock output
	if !strings.Contains(resp.Stdout, "NAME") {
		t.Errorf("expected stdout to contain output, got %q", resp.Stdout)
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
	server := NewServer(lookup, matcher, nil, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Args: []string{"docker", "compose", "up", "-d"}}
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

	server := NewServer(lookup, matcher, nil, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Args: []string{"rm", "-rf", "/"}}
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
	server := NewServer(lookup, nil, nil, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Args: []string{"echo", "hello"}}
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

	// Mock executor that returns expected output
	mockExec := &mockCommandExecutor{
		responses: map[string]*executor.ExecuteResponse{
			"docker": {
				Status:   executor.StatusCompleted,
				ExitCode: 0,
				Stdout:   "containers started",
			},
		},
	}

	// Create server with an approval queue and executor
	queue := approval.NewQueue()
	server := NewServer(lookup, matcher, mockExec, nil)
	server.Queue = queue
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Args: []string{"docker", "compose", "up", "-d"}}
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

	// Send approval response (executor provides the output)
	actualReq.Response <- approval.Response{
		Status: "approved",
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
	server := NewServer(lookup, matcher, nil, nil)
	server.Queue = queue
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Args: []string{"docker", "compose", "up", "-d"}}
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
	server := NewServer(lookup, matcher, nil, nil)
	server.Queue = queue
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Args: []string{"docker", "compose", "up", "-d"}}
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

	server := NewServer(lookup, nil, nil, nil)
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

	client := noProxyClient()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Go's net/http returns 405 Method Not Allowed for unmatched methods
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, resp.StatusCode)
	}
}

// Tests for Phase 4.5.2: Executor wiring

func TestServer_HandleRequest_AutoApprove_ExecutorCalled(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"test-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	matcher := &mockPatternMatcher{
		results: map[string]patterns.MatchResult{
			"echo hello": {Action: patterns.AutoApprove, Pattern: "^echo .*$"},
		},
	}

	mockExec := &mockCommandExecutor{
		responses: map[string]*executor.ExecuteResponse{
			"echo": {
				Status:   executor.StatusCompleted,
				ExitCode: 0,
				Stdout:   "hello\n",
			},
		},
	}

	server := NewServer(lookup, matcher, mockExec, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Args: []string{"echo", "hello"}}
	body, _ := json.Marshal(cmdReq)
	req := httptest.NewRequest(http.MethodPost, "/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, "test-token")

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
	if resp.Pattern != "^echo .*$" {
		t.Errorf("expected pattern '^echo .*$', got %q", resp.Pattern)
	}
	if resp.ExitCode != 0 {
		t.Errorf("expected exit_code 0, got %d", resp.ExitCode)
	}
	if resp.Stdout != "hello\n" {
		t.Errorf("expected stdout 'hello\\n', got %q", resp.Stdout)
	}
}

func TestServer_HandleRequest_AutoApprove_NilExecutor(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"test-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	matcher := &mockPatternMatcher{
		results: map[string]patterns.MatchResult{
			"echo hello": {Action: patterns.AutoApprove, Pattern: "^echo .*$"},
		},
	}

	// Server with nil executor should return error
	server := NewServer(lookup, matcher, nil, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Args: []string{"echo", "hello"}}
	body, _ := json.Marshal(cmdReq)
	req := httptest.NewRequest(http.MethodPost, "/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, "test-token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp CommandResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "error" {
		t.Errorf("expected status 'error', got %q", resp.Status)
	}
	if !strings.Contains(resp.Reason, "not configured") {
		t.Errorf("expected reason to contain 'not configured', got %q", resp.Reason)
	}
}

func TestServer_HandleRequest_AutoApprove_ExecutorError(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"test-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	matcher := &mockPatternMatcher{
		results: map[string]patterns.MatchResult{
			"echo hello": {Action: patterns.AutoApprove, Pattern: "^echo .*$"},
		},
	}

	mockExec := &mockCommandExecutor{
		err: errors.New("connection refused"),
	}

	server := NewServer(lookup, matcher, mockExec, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Args: []string{"echo", "hello"}}
	body, _ := json.Marshal(cmdReq)
	req := httptest.NewRequest(http.MethodPost, "/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, "test-token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp CommandResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "error" {
		t.Errorf("expected status 'error', got %q", resp.Status)
	}
	if !strings.Contains(resp.Reason, "connection refused") {
		t.Errorf("expected reason to contain 'connection refused', got %q", resp.Reason)
	}
}

func TestServer_HandleRequest_AutoApprove_ExecutorTimeout(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"test-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	matcher := &mockPatternMatcher{
		results: map[string]patterns.MatchResult{
			"slow command": {Action: patterns.AutoApprove, Pattern: "^slow .*$"},
		},
	}

	mockExec := &mockCommandExecutor{
		responses: map[string]*executor.ExecuteResponse{
			"slow": {
				Status:   executor.StatusTimeout,
				ExitCode: -1,
				Stdout:   "partial output",
				Error:    "command timed out after 30s",
			},
		},
	}

	server := NewServer(lookup, matcher, mockExec, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Args: []string{"slow", "command"}}
	body, _ := json.Marshal(cmdReq)
	req := httptest.NewRequest(http.MethodPost, "/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, "test-token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

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
	if !strings.Contains(resp.Reason, "timed out") {
		t.Errorf("expected reason to contain 'timed out', got %q", resp.Reason)
	}
	// Should still include partial output
	if resp.Stdout != "partial output" {
		t.Errorf("expected partial stdout, got %q", resp.Stdout)
	}
}

func TestServer_HandleRequest_AutoApprove_CommandFailed(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"test-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	matcher := &mockPatternMatcher{
		results: map[string]patterns.MatchResult{
			"failing command": {Action: patterns.AutoApprove, Pattern: "^failing .*$"},
		},
	}

	mockExec := &mockCommandExecutor{
		responses: map[string]*executor.ExecuteResponse{
			"failing": {
				Status:   executor.StatusCompleted,
				ExitCode: 1,
				Stderr:   "error: command failed",
			},
		},
	}

	server := NewServer(lookup, matcher, mockExec, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Args: []string{"failing", "command"}}
	body, _ := json.Marshal(cmdReq)
	req := httptest.NewRequest(http.MethodPost, "/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, "test-token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp CommandResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Even with non-zero exit code, status should be auto_approved (command completed)
	if resp.Status != "auto_approved" {
		t.Errorf("expected status 'auto_approved', got %q", resp.Status)
	}
	if resp.ExitCode != 1 {
		t.Errorf("expected exit_code 1, got %d", resp.ExitCode)
	}
	if resp.Stderr != "error: command failed" {
		t.Errorf("expected stderr 'error: command failed', got %q", resp.Stderr)
	}
}

func TestMapExecutorResponse(t *testing.T) {
	tests := []struct {
		name     string
		execResp *executor.ExecuteResponse
		status   string
		pattern  string
		want     CommandResponse
	}{
		{
			name: "completed with approved status",
			execResp: &executor.ExecuteResponse{
				Status:   executor.StatusCompleted,
				ExitCode: 0,
				Stdout:   "output",
			},
			status:  "approved",
			pattern: "",
			want: CommandResponse{
				Status:   "approved",
				ExitCode: 0,
				Stdout:   "output",
			},
		},
		{
			name: "completed with auto_approved status and pattern",
			execResp: &executor.ExecuteResponse{
				Status:   executor.StatusCompleted,
				ExitCode: 0,
				Stdout:   "output",
			},
			status:  "auto_approved",
			pattern: "^test$",
			want: CommandResponse{
				Status:   "auto_approved",
				Pattern:  "^test$",
				ExitCode: 0,
				Stdout:   "output",
			},
		},
		{
			name: "timeout with error message",
			execResp: &executor.ExecuteResponse{
				Status: executor.StatusTimeout,
				Error:  "custom timeout message",
				Stdout: "partial",
			},
			status:  "approved",
			pattern: "",
			want: CommandResponse{
				Status: "timeout",
				Reason: "custom timeout message",
				Stdout: "partial",
			},
		},
		{
			name: "timeout without error message",
			execResp: &executor.ExecuteResponse{
				Status: executor.StatusTimeout,
			},
			status:  "approved",
			pattern: "",
			want: CommandResponse{
				Status: "timeout",
				Reason: "command execution timed out",
			},
		},
		{
			name: "error status",
			execResp: &executor.ExecuteResponse{
				Status: executor.StatusError,
				Error:  "command not found",
			},
			status:  "approved",
			pattern: "",
			want: CommandResponse{
				Status: "error",
				Reason: "command not found",
			},
		},
		{
			name: "unknown status",
			execResp: &executor.ExecuteResponse{
				Status: "unknown",
			},
			status:  "approved",
			pattern: "",
			want: CommandResponse{
				Status: "error",
				Reason: "unknown executor status: unknown",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapExecutorResponse(tt.execResp, tt.status, tt.pattern)
			if got.Status != tt.want.Status {
				t.Errorf("Status = %q, want %q", got.Status, tt.want.Status)
			}
			if got.Pattern != tt.want.Pattern {
				t.Errorf("Pattern = %q, want %q", got.Pattern, tt.want.Pattern)
			}
			if got.Reason != tt.want.Reason {
				t.Errorf("Reason = %q, want %q", got.Reason, tt.want.Reason)
			}
			if got.ExitCode != tt.want.ExitCode {
				t.Errorf("ExitCode = %d, want %d", got.ExitCode, tt.want.ExitCode)
			}
			if got.Stdout != tt.want.Stdout {
				t.Errorf("Stdout = %q, want %q", got.Stdout, tt.want.Stdout)
			}
			if got.Stderr != tt.want.Stderr {
				t.Errorf("Stderr = %q, want %q", got.Stderr, tt.want.Stderr)
			}
		})
	}
}

// Tests for audit logging integration

func TestServer_HandleRequest_AuditLogging_AutoApprove(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	matcher := &mockPatternMatcher{
		results: map[string]patterns.MatchResult{
			"echo hello": {Action: patterns.AutoApprove, Pattern: "^echo .*$"},
		},
	}

	mockExec := &mockCommandExecutor{
		responses: map[string]*executor.ExecuteResponse{
			"echo": {
				Status:   executor.StatusCompleted,
				ExitCode: 0,
				Stdout:   "hello\n",
			},
		},
	}

	// Create a buffer to capture audit logs
	var auditBuf bytes.Buffer
	auditLogger := audit.NewLogger(&auditBuf)

	server := NewServer(lookup, matcher, mockExec, auditLogger)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Args: []string{"echo", "hello"}}
	body, _ := json.Marshal(cmdReq)
	req := httptest.NewRequest(http.MethodPost, "/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, "valid-token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	// Verify audit logs
	auditOutput := auditBuf.String()

	// Should have REQUEST event
	if !strings.Contains(auditOutput, "HOSTEXEC REQUEST") {
		t.Errorf("expected audit log to contain REQUEST event, got: %s", auditOutput)
	}

	// Should have AUTO_APPROVE event
	if !strings.Contains(auditOutput, "HOSTEXEC AUTO_APPROVE") {
		t.Errorf("expected audit log to contain AUTO_APPROVE event, got: %s", auditOutput)
	}

	// Should have COMPLETE event
	if !strings.Contains(auditOutput, "HOSTEXEC COMPLETE") {
		t.Errorf("expected audit log to contain COMPLETE event, got: %s", auditOutput)
	}

	// Verify project and cloister are in the logs
	if !strings.Contains(auditOutput, "project=test-project") {
		t.Errorf("expected audit log to contain project=test-project, got: %s", auditOutput)
	}
	if !strings.Contains(auditOutput, "cloister=test-cloister") {
		t.Errorf("expected audit log to contain cloister=test-cloister, got: %s", auditOutput)
	}
}

func TestServer_HandleRequest_AuditLogging_Deny(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	matcher := &mockPatternMatcher{
		results: map[string]patterns.MatchResult{
			"echo hello": {Action: patterns.AutoApprove, Pattern: "^echo .*$"},
		},
	}

	// Create a buffer to capture audit logs
	var auditBuf bytes.Buffer
	auditLogger := audit.NewLogger(&auditBuf)

	server := NewServer(lookup, matcher, nil, auditLogger)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	// Send a command that doesn't match any pattern
	cmdReq := CommandRequest{Args: []string{"rm", "-rf", "/"}}
	body, _ := json.Marshal(cmdReq)
	req := httptest.NewRequest(http.MethodPost, "/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, "valid-token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	// Verify audit logs
	auditOutput := auditBuf.String()

	// Should have REQUEST event
	if !strings.Contains(auditOutput, "HOSTEXEC REQUEST") {
		t.Errorf("expected audit log to contain REQUEST event, got: %s", auditOutput)
	}

	// Should have DENY event
	if !strings.Contains(auditOutput, "HOSTEXEC DENY") {
		t.Errorf("expected audit log to contain DENY event, got: %s", auditOutput)
	}

	// Should include denial reason
	if !strings.Contains(auditOutput, "does not match") {
		t.Errorf("expected audit log to contain denial reason, got: %s", auditOutput)
	}
}

func TestServer_HandleRequest_AuditLogging_NilMatcher(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	// Create a buffer to capture audit logs
	var auditBuf bytes.Buffer
	auditLogger := audit.NewLogger(&auditBuf)

	// Server with nil pattern matcher - should deny
	server := NewServer(lookup, nil, nil, auditLogger)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Args: []string{"echo", "hello"}}
	body, _ := json.Marshal(cmdReq)
	req := httptest.NewRequest(http.MethodPost, "/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, "valid-token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Verify audit logs
	auditOutput := auditBuf.String()

	// Should have REQUEST event
	if !strings.Contains(auditOutput, "HOSTEXEC REQUEST") {
		t.Errorf("expected audit log to contain REQUEST event, got: %s", auditOutput)
	}

	// Should have DENY event
	if !strings.Contains(auditOutput, "HOSTEXEC DENY") {
		t.Errorf("expected audit log to contain DENY event, got: %s", auditOutput)
	}

	// Should include reason about no patterns configured
	if !strings.Contains(auditOutput, "no approval patterns configured") {
		t.Errorf("expected audit log to contain 'no approval patterns configured', got: %s", auditOutput)
	}
}

func TestServer_HandleRequest_AuditLogging_Timeout(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	matcher := &mockPatternMatcher{
		results: map[string]patterns.MatchResult{
			"docker compose up -d": {Action: patterns.ManualApprove, Pattern: "^docker compose (up|down).*$"},
		},
	}

	// Create a buffer to capture audit logs
	var auditBuf bytes.Buffer
	auditLogger := audit.NewLogger(&auditBuf)

	// Create server with a very short timeout queue
	queue := approval.NewQueueWithTimeout(100 * time.Millisecond)
	server := NewServer(lookup, matcher, nil, auditLogger)
	server.Queue = queue
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Args: []string{"docker", "compose", "up", "-d"}}
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

	// Verify audit logs
	auditOutput := auditBuf.String()

	// Should have REQUEST event
	if !strings.Contains(auditOutput, "HOSTEXEC REQUEST") {
		t.Errorf("expected audit log to contain REQUEST event, got: %s", auditOutput)
	}

	// Should have TIMEOUT event
	if !strings.Contains(auditOutput, "HOSTEXEC TIMEOUT") {
		t.Errorf("expected audit log to contain TIMEOUT event, got: %s", auditOutput)
	}
}

func TestServer_HandleRequest_AuditLogging_NilLogger(t *testing.T) {
	// Verify that nil logger doesn't cause panic
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	matcher := &mockPatternMatcher{
		results: map[string]patterns.MatchResult{
			"echo hello": {Action: patterns.AutoApprove, Pattern: "^echo .*$"},
		},
	}

	mockExec := &mockCommandExecutor{
		responses: map[string]*executor.ExecuteResponse{
			"echo": {
				Status:   executor.StatusCompleted,
				ExitCode: 0,
				Stdout:   "hello\n",
			},
		},
	}

	// Create server with nil audit logger
	server := NewServer(lookup, matcher, mockExec, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Args: []string{"echo", "hello"}}
	body, _ := json.Marshal(cmdReq)
	req := httptest.NewRequest(http.MethodPost, "/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, "valid-token")

	rr := httptest.NewRecorder()
	// Should not panic with nil logger
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
}

// TestServer_HandleRequest_NULByteRejected verifies that arguments containing
// NUL bytes are rejected, as they cannot be safely represented in shell commands.
func TestServer_HandleRequest_NULByteRejected(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	matcher := &mockPatternMatcher{
		results: map[string]patterns.MatchResult{
			"echo hello": {Action: patterns.AutoApprove, Pattern: "^echo .*$"},
		},
	}

	server := NewServer(lookup, matcher, nil, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	// Request with NUL byte embedded in argument
	cmdReq := CommandRequest{Args: []string{"echo", "hello\x00world"}}
	body, _ := json.Marshal(cmdReq)
	req := httptest.NewRequest(http.MethodPost, "/request", bytes.NewReader(body))
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
	if !strings.Contains(resp.Reason, "NUL") {
		t.Errorf("expected reason to mention NUL bytes, got %q", resp.Reason)
	}
}

// TestServer_HandleRequest_CmdFieldIgnored verifies that a malicious cmd field
// is ignored and the canonical command is reconstructed from args only.
// This is the key security fix - prevents validation bypass attacks.
func TestServer_HandleRequest_CmdFieldIgnored(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	// Matcher only allows "docker ps"
	matcher := &mockPatternMatcher{
		results: map[string]patterns.MatchResult{
			"docker ps": {Action: patterns.AutoApprove, Pattern: "^docker ps$"},
		},
	}

	server := NewServer(lookup, matcher, nil, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	// Malicious request: cmd claims "docker ps" but args say "rm -rf /"
	// Before the fix, this would match "docker ps" pattern but execute "rm -rf /"
	body := `{"cmd": "docker ps", "args": ["rm", "-rf", "/"]}`
	req := httptest.NewRequest(http.MethodPost, "/request", strings.NewReader(body))
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

	// Command should be DENIED because canonical cmd from args is "rm -rf /"
	// which doesn't match "docker ps" pattern
	if resp.Status != "denied" {
		t.Errorf("expected status 'denied', got %q (security vulnerability if not denied!)", resp.Status)
	}
	if !strings.Contains(resp.Reason, "does not match") {
		t.Errorf("expected reason to contain 'does not match', got %q", resp.Reason)
	}
}
