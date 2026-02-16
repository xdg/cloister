package guardian

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/token"
)

// mockRegistry implements TokenRegistry for testing.
type mockRegistry struct {
	tokens map[string]token.Info
}

func newMockRegistry() *mockRegistry {
	return &mockRegistry{tokens: make(map[string]token.Info)}
}

func (r *mockRegistry) Register(tok, cloisterName string) {
	r.tokens[tok] = token.Info{CloisterName: cloisterName}
}

func (r *mockRegistry) RegisterWithProject(tok, cloisterName, projectName string) {
	r.tokens[tok] = token.Info{CloisterName: cloisterName, ProjectName: projectName}
}

func (r *mockRegistry) RegisterFull(tok, cloisterName, projectName, worktreePath string) {
	r.tokens[tok] = token.Info{
		CloisterName: cloisterName,
		ProjectName:  projectName,
		WorktreePath: worktreePath,
	}
}

func (r *mockRegistry) Validate(tok string) bool {
	_, ok := r.tokens[tok]
	return ok
}

func (r *mockRegistry) Revoke(tok string) bool {
	if _, ok := r.tokens[tok]; ok {
		delete(r.tokens, tok)
		return true
	}
	return false
}

func (r *mockRegistry) List() map[string]token.Info {
	result := make(map[string]token.Info, len(r.tokens))
	for k, v := range r.tokens {
		result[k] = v
	}
	return result
}

func (r *mockRegistry) Count() int {
	return len(r.tokens)
}

func TestAPIServer_StartStop(t *testing.T) {
	registry := newMockRegistry()
	api := NewAPIServer(":0", registry)

	// Start the server
	if err := api.Start(); err != nil {
		t.Fatalf("failed to start API server: %v", err)
	}

	// Verify server is listening
	addr := api.ListenAddr()
	if addr == "" {
		t.Fatal("expected non-empty listen address")
	}

	// Starting again should fail
	if err := api.Start(); err == nil {
		t.Error("expected error when starting already running server")
	}

	// Stop the server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := api.Stop(ctx); err != nil {
		t.Fatalf("failed to stop API server: %v", err)
	}

	// Stopping again should be idempotent
	if err := api.Stop(ctx); err != nil {
		t.Fatalf("expected no error when stopping already stopped server: %v", err)
	}
}

func TestAPIServer_RegisterToken(t *testing.T) {
	registry := newMockRegistry()
	api := NewAPIServer(":0", registry)
	if err := api.Start(); err != nil {
		t.Fatalf("failed to start API server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = api.Stop(ctx)
	}()

	baseURL := "http://" + api.ListenAddr()

	tests := []struct {
		name           string
		body           string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "valid registration",
			body:           `{"token":"abc123","cloister":"my-project-main"}`,
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "missing token",
			body:           `{"cloister":"my-project-main"}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "token is required",
		},
		{
			name:           "empty token",
			body:           `{"token":"","cloister":"my-project-main"}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "token is required",
		},
		{
			name:           "missing cloister",
			body:           `{"token":"abc123"}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "cloister is required",
		},
		{
			name:           "empty cloister",
			body:           `{"token":"abc123","cloister":""}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "cloister is required",
		},
		{
			name:           "invalid JSON",
			body:           `{invalid}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid JSON body",
		},
	}

	client := noProxyClient()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/tokens", bytes.NewBufferString(tc.body))
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, resp.StatusCode)
			}

			if tc.expectedError != "" {
				var errResp errorResponse
				if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
					t.Fatalf("failed to decode error response: %v", err)
				}
				if errResp.Error != tc.expectedError {
					t.Errorf("expected error %q, got %q", tc.expectedError, errResp.Error)
				}
			}
		})
	}

	// Verify token was actually registered
	if info, ok := registry.tokens["abc123"]; !ok {
		t.Error("token abc123 was not registered")
	} else if info.CloisterName != "my-project-main" {
		t.Errorf("expected cloister name my-project-main, got %s", info.CloisterName)
	}
}

func TestAPIServer_RevokeToken(t *testing.T) {
	registry := newMockRegistry()
	registry.tokens["existing-token"] = token.Info{CloisterName: "test-cloister"}

	api := NewAPIServer(":0", registry)
	if err := api.Start(); err != nil {
		t.Fatalf("failed to start API server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = api.Stop(ctx)
	}()

	baseURL := "http://" + api.ListenAddr()

	tests := []struct {
		name           string
		token          string
		expectedStatus int
	}{
		{
			name:           "revoke existing token",
			token:          "existing-token",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "revoke non-existent token",
			token:          "non-existent-token",
			expectedStatus: http.StatusNotFound,
		},
	}

	client := noProxyClient()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, baseURL+"/tokens/"+tc.token, http.NoBody)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, resp.StatusCode)
			}
		})
	}

	// Verify token was actually revoked
	if _, ok := registry.tokens["existing-token"]; ok {
		t.Error("token existing-token was not revoked")
	}
}

func TestAPIServer_ListTokens(t *testing.T) {
	registry := newMockRegistry()
	registry.tokens["token1"] = token.Info{CloisterName: "cloister1"}
	registry.tokens["token2"] = token.Info{CloisterName: "cloister2"}

	api := NewAPIServer(":0", registry)
	if err := api.Start(); err != nil {
		t.Fatalf("failed to start API server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = api.Stop(ctx)
	}()

	baseURL := "http://" + api.ListenAddr()

	client := noProxyClient()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+"/tokens", http.NoBody)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var listResp listTokensResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(listResp.Tokens) != 2 {
		t.Errorf("expected 2 tokens, got %d", len(listResp.Tokens))
	}

	// Build map for easy checking
	tokenMap := make(map[string]string)
	for _, t := range listResp.Tokens {
		tokenMap[t.Token] = t.Cloister
	}

	if tokenMap["token1"] != "cloister1" {
		t.Errorf("expected token1 -> cloister1, got %s", tokenMap["token1"])
	}
	if tokenMap["token2"] != "cloister2" {
		t.Errorf("expected token2 -> cloister2, got %s", tokenMap["token2"])
	}
}

func TestAPIServer_ListTokensEmpty(t *testing.T) {
	registry := newMockRegistry()

	api := NewAPIServer(":0", registry)
	if err := api.Start(); err != nil {
		t.Fatalf("failed to start API server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = api.Stop(ctx)
	}()

	baseURL := "http://" + api.ListenAddr()

	client := noProxyClient()
	req2, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+"/tokens", http.NoBody)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp, err := client.Do(req2)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var listResp listTokensResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(listResp.Tokens) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(listResp.Tokens))
	}
}

func TestAPIServer_ContentType(t *testing.T) {
	registry := newMockRegistry()

	api := NewAPIServer(":0", registry)
	if err := api.Start(); err != nil {
		t.Fatalf("failed to start API server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = api.Stop(ctx)
	}()

	baseURL := "http://" + api.ListenAddr()

	// Check that responses have JSON content type
	client := noProxyClient()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+"/tokens", http.NoBody)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}
}

// mockSessionAllowlist implements SessionAllowlist for testing.
type mockSessionAllowlist struct {
	clearedTokens []string
}

func (m *mockSessionAllowlist) IsAllowed(_, _ string) bool {
	return false
}

func (m *mockSessionAllowlist) Add(_, _ string) error {
	return nil
}

func (m *mockSessionAllowlist) Clear(tok string) {
	m.clearedTokens = append(m.clearedTokens, tok)
}

func TestAPIServer_RevokeTokenClearsSessionAllowlist(t *testing.T) {
	registry := newMockRegistry()
	registry.tokens["test-token"] = token.Info{CloisterName: "test-cloister"}

	sessionAllowlist := &mockSessionAllowlist{}

	api := NewAPIServer(":0", registry)
	api.SessionAllowlist = sessionAllowlist

	if err := api.Start(); err != nil {
		t.Fatalf("failed to start API server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = api.Stop(ctx)
	}()

	baseURL := "http://" + api.ListenAddr()
	client := noProxyClient()

	// Revoke the token
	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, baseURL+"/tokens/test-token", http.NoBody)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Verify token was revoked from registry
	if _, ok := registry.tokens["test-token"]; ok {
		t.Error("token was not revoked from registry")
	}

	// Verify session allowlist was cleared for this token
	if len(sessionAllowlist.clearedTokens) != 1 {
		t.Errorf("expected 1 cleared token, got %d", len(sessionAllowlist.clearedTokens))
	}
	if len(sessionAllowlist.clearedTokens) > 0 && sessionAllowlist.clearedTokens[0] != "test-token" {
		t.Errorf("expected cleared token 'test-token', got %q", sessionAllowlist.clearedTokens[0])
	}
}

func TestAPIServer_RevokeTokenWithNilSessionAllowlist(t *testing.T) {
	registry := newMockRegistry()
	registry.tokens["test-token"] = token.Info{CloisterName: "test-cloister"}

	// API server without session allowlist (nil)
	api := NewAPIServer(":0", registry)
	// api.SessionAllowlist is nil by default

	if err := api.Start(); err != nil {
		t.Fatalf("failed to start API server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = api.Stop(ctx)
	}()

	baseURL := "http://" + api.ListenAddr()
	client := noProxyClient()

	// Revoke the token - should not panic with nil SessionAllowlist
	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, baseURL+"/tokens/test-token", http.NoBody)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Verify token was revoked from registry
	if _, ok := registry.tokens["test-token"]; ok {
		t.Error("token was not revoked from registry")
	}
}
