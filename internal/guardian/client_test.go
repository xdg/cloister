package guardian

import (
	"context"
	"testing"
	"time"
)

func TestClient_RegisterToken(t *testing.T) {
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

	client := NewClient(api.ListenAddr())
	client.HTTPClient = noProxyClient()

	// Test successful registration
	err := client.RegisterToken("test-token-123", "my-cloister", "my-project")
	if err != nil {
		t.Fatalf("failed to register token: %v", err)
	}

	// Verify token was registered
	if info, ok := registry.tokens["test-token-123"]; !ok {
		t.Error("token was not registered")
	} else if info.CloisterName != "my-cloister" {
		t.Errorf("expected cloister name my-cloister, got %s", info.CloisterName)
	}
}

func TestClient_RegisterTokenErrors(t *testing.T) {
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

	client := NewClient(api.ListenAddr())
	client.HTTPClient = noProxyClient()

	// Test empty token
	err := client.RegisterToken("", "my-cloister", "my-project")
	if err == nil {
		t.Error("expected error for empty token")
	}

	// Test empty cloister
	err = client.RegisterToken("test-token", "", "my-project")
	if err == nil {
		t.Error("expected error for empty cloister")
	}
}

func TestClient_RevokeToken(t *testing.T) {
	registry := newMockRegistry()
	registry.tokens["token-to-revoke"] = TokenInfo{CloisterName: "test-cloister"}

	api := NewAPIServer(":0", registry)
	if err := api.Start(); err != nil {
		t.Fatalf("failed to start API server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = api.Stop(ctx)
	}()

	client := NewClient(api.ListenAddr())
	client.HTTPClient = noProxyClient()

	// Test successful revocation
	err := client.RevokeToken("token-to-revoke")
	if err != nil {
		t.Fatalf("failed to revoke token: %v", err)
	}

	// Verify token was revoked
	if _, ok := registry.tokens["token-to-revoke"]; ok {
		t.Error("token was not revoked")
	}

	// Revoking non-existent token should not error (idempotent)
	err = client.RevokeToken("non-existent-token")
	if err != nil {
		t.Errorf("expected no error for non-existent token, got: %v", err)
	}
}

func TestClient_ListTokens(t *testing.T) {
	registry := newMockRegistry()
	registry.tokens["token-a"] = TokenInfo{CloisterName: "cloister-a"}
	registry.tokens["token-b"] = TokenInfo{CloisterName: "cloister-b"}

	api := NewAPIServer(":0", registry)
	if err := api.Start(); err != nil {
		t.Fatalf("failed to start API server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = api.Stop(ctx)
	}()

	client := NewClient(api.ListenAddr())
	client.HTTPClient = noProxyClient()

	tokens, err := client.ListTokens()
	if err != nil {
		t.Fatalf("failed to list tokens: %v", err)
	}

	if len(tokens) != 2 {
		t.Errorf("expected 2 tokens, got %d", len(tokens))
	}

	if tokens["token-a"] != "cloister-a" {
		t.Errorf("expected token-a -> cloister-a, got %s", tokens["token-a"])
	}
	if tokens["token-b"] != "cloister-b" {
		t.Errorf("expected token-b -> cloister-b, got %s", tokens["token-b"])
	}
}

func TestClient_ListTokensEmpty(t *testing.T) {
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

	client := NewClient(api.ListenAddr())
	client.HTTPClient = noProxyClient()

	tokens, err := client.ListTokens()
	if err != nil {
		t.Fatalf("failed to list tokens: %v", err)
	}

	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(tokens))
	}
}

func TestClient_ConnectionError(t *testing.T) {
	// Test with a non-existent server
	client := NewClient("localhost:59999")

	err := client.RegisterToken("test", "test", "test-project")
	if err == nil {
		t.Error("expected connection error")
	}

	err = client.RevokeToken("test")
	if err == nil {
		t.Error("expected connection error")
	}

	_, err = client.ListTokens()
	if err == nil {
		t.Error("expected connection error")
	}
}

func TestNewClient(t *testing.T) {
	client := NewClient("localhost:9997")

	if client.BaseURL != "http://localhost:9997" {
		t.Errorf("expected BaseURL http://localhost:9997, got %s", client.BaseURL)
	}

	if client.HTTPClient == nil {
		t.Error("expected non-nil HTTPClient")
	}
}
