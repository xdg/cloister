package request

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestServer_HandleRequest_ManualApprove(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	matcher := &mockPatternMatcher{
		results: map[string]patterns.MatchResult{
			"docker compose up -d": {Action: patterns.ManualApprove, Pattern: "^docker compose (up|down).*$"},
		},
	}

	server := NewServer(lookup, matcher, nil)
	handler := AuthMiddleware(lookup)(http.HandlerFunc(server.handleRequest))

	cmdReq := CommandRequest{Cmd: "docker compose up -d"}
	body, _ := json.Marshal(cmdReq)
	req := httptest.NewRequest(http.MethodPost, "/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, "valid-token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should return 202 Accepted for pending approval
	if rr.Code != http.StatusAccepted {
		t.Errorf("expected status %d, got %d", http.StatusAccepted, rr.Code)
	}

	var resp CommandResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "awaiting_approval" {
		t.Errorf("expected status 'awaiting_approval', got %q", resp.Status)
	}
	if !strings.Contains(resp.Reason, "manual approval") {
		t.Errorf("expected reason to contain 'manual approval', got %q", resp.Reason)
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
