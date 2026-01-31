package request

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

func TestServer_HandleRequest_ValidRequest_NotImplemented(t *testing.T) {
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

	// Should return 501 Not Implemented for now
	if rr.Code != http.StatusNotImplemented {
		t.Errorf("expected status %d, got %d", http.StatusNotImplemented, rr.Code)
	}

	var resp CommandResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "error" {
		t.Errorf("expected status 'error', got %q", resp.Status)
	}
	if !strings.Contains(resp.Reason, "not yet implemented") {
		t.Errorf("expected reason to contain 'not yet implemented', got %q", resp.Reason)
	}
}

func TestServer_HandleRequest_ViaHTTPServer(t *testing.T) {
	lookup := mockTokenLookup(map[string]TokenInfo{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})

	server := NewServer(lookup, nil, nil)
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

	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("expected status %d, got %d", http.StatusNotImplemented, resp.StatusCode)
	}

	var cmdResp CommandResponse
	if err := json.NewDecoder(resp.Body).Decode(&cmdResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if cmdResp.Status != "error" {
		t.Errorf("expected status 'error', got %q", cmdResp.Status)
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
