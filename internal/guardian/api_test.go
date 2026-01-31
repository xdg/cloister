package guardian

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// mockRegistry implements TokenRegistry for testing.
type mockRegistry struct {
	tokens map[string]TokenInfo
}

func newMockRegistry() *mockRegistry {
	return &mockRegistry{tokens: make(map[string]TokenInfo)}
}

func (r *mockRegistry) Register(token, cloisterName string) {
	r.tokens[token] = TokenInfo{CloisterName: cloisterName}
}

func (r *mockRegistry) RegisterWithProject(token, cloisterName, projectName string) {
	r.tokens[token] = TokenInfo{CloisterName: cloisterName, ProjectName: projectName}
}

func (r *mockRegistry) RegisterFull(token, cloisterName, projectName, worktreePath string) {
	r.tokens[token] = TokenInfo{
		CloisterName: cloisterName,
		ProjectName:  projectName,
		WorktreePath: worktreePath,
	}
}

func (r *mockRegistry) Validate(token string) bool {
	_, ok := r.tokens[token]
	return ok
}

func (r *mockRegistry) Revoke(token string) bool {
	if _, ok := r.tokens[token]; ok {
		delete(r.tokens, token)
		return true
	}
	return false
}

func (r *mockRegistry) List() map[string]TokenInfo {
	result := make(map[string]TokenInfo, len(r.tokens))
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(baseURL+"/tokens", "application/json", bytes.NewBufferString(tc.body))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

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
	registry.tokens["existing-token"] = TokenInfo{CloisterName: "test-cloister"}

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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodDelete, baseURL+"/tokens/"+tc.token, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

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
	registry.tokens["token1"] = TokenInfo{CloisterName: "cloister1"}
	registry.tokens["token2"] = TokenInfo{CloisterName: "cloister2"}

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

	resp, err := http.Get(baseURL + "/tokens")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

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

	resp, err := http.Get(baseURL + "/tokens")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

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
	resp, err := http.Get(baseURL + "/tokens")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}
}
