package claude

import (
	"errors"
	"testing"

	"github.com/xdg/cloister/internal/config"
)

func TestInjector_InjectCredentials_Token(t *testing.T) {
	injector := NewInjector()

	cfg := &config.AgentConfig{
		AuthMethod: "token",
		Token:      "sk-ant-oat01-test-token",
	}

	result, err := injector.InjectCredentials(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have env var set
	if len(result.EnvVars) != 1 {
		t.Errorf("expected 1 env var, got %d", len(result.EnvVars))
	}
	if result.EnvVars[EnvClaudeOAuthToken] != "sk-ant-oat01-test-token" {
		t.Errorf("expected token %q, got %q", "sk-ant-oat01-test-token", result.EnvVars[EnvClaudeOAuthToken])
	}

	// Should have no files
	if len(result.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(result.Files))
	}
}

func TestInjector_InjectCredentials_APIKey(t *testing.T) {
	injector := NewInjector()

	cfg := &config.AgentConfig{
		AuthMethod: "api_key",
		APIKey:     "sk-ant-api01-test-key",
	}

	result, err := injector.InjectCredentials(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have env var set
	if len(result.EnvVars) != 1 {
		t.Errorf("expected 1 env var, got %d", len(result.EnvVars))
	}
	if result.EnvVars[EnvAnthropicAPIKey] != "sk-ant-api01-test-key" {
		t.Errorf("expected API key %q, got %q", "sk-ant-api01-test-key", result.EnvVars[EnvAnthropicAPIKey])
	}

	// Should have no files
	if len(result.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(result.Files))
	}
}

func TestInjector_InjectCredentials_NoAuthMethod(t *testing.T) {
	injector := NewInjector()

	// Test with nil config
	_, err := injector.InjectCredentials(nil)
	if !errors.Is(err, ErrNoAuthMethod) {
		t.Errorf("expected ErrNoAuthMethod for nil config, got %v", err)
	}

	// Test with empty auth_method
	cfg := &config.AgentConfig{}
	_, err = injector.InjectCredentials(cfg)
	if !errors.Is(err, ErrNoAuthMethod) {
		t.Errorf("expected ErrNoAuthMethod for empty auth_method, got %v", err)
	}
}

func TestInjector_InjectCredentials_InvalidAuthMethod(t *testing.T) {
	injector := NewInjector()

	cfg := &config.AgentConfig{
		AuthMethod: "invalid_method",
	}

	_, err := injector.InjectCredentials(cfg)
	if !errors.Is(err, ErrInvalidAuthMethod) {
		t.Errorf("expected ErrInvalidAuthMethod, got %v", err)
	}
}

func TestInjector_InjectCredentials_MissingToken(t *testing.T) {
	injector := NewInjector()

	cfg := &config.AgentConfig{
		AuthMethod: "token",
		Token:      "", // missing
	}

	_, err := injector.InjectCredentials(cfg)
	if !errors.Is(err, ErrMissingToken) {
		t.Errorf("expected ErrMissingToken, got %v", err)
	}
}

func TestInjector_InjectCredentials_MissingAPIKey(t *testing.T) {
	injector := NewInjector()

	cfg := &config.AgentConfig{
		AuthMethod: "api_key",
		APIKey:     "", // missing
	}

	_, err := injector.InjectCredentials(cfg)
	if !errors.Is(err, ErrMissingAPIKey) {
		t.Errorf("expected ErrMissingAPIKey, got %v", err)
	}
}

func TestNewInjector(t *testing.T) {
	injector := NewInjector()

	if injector == nil {
		t.Fatal("NewInjector returned nil")
	}
}

func TestInjectionConfig_Empty(t *testing.T) {
	// Test that an empty InjectionConfig is usable
	cfg := &InjectionConfig{
		EnvVars: make(map[string]string),
		Files:   make(map[string]string),
	}

	if len(cfg.EnvVars) != 0 {
		t.Errorf("expected empty EnvVars, got %d entries", len(cfg.EnvVars))
	}
	if len(cfg.Files) != 0 {
		t.Errorf("expected empty Files, got %d entries", len(cfg.Files))
	}
}

func TestConstants(t *testing.T) {
	// Verify constants have expected values
	if EnvClaudeOAuthToken != "CLAUDE_CODE_OAUTH_TOKEN" {
		t.Errorf("unexpected EnvClaudeOAuthToken: %q", EnvClaudeOAuthToken)
	}
	if EnvAnthropicAPIKey != "ANTHROPIC_API_KEY" {
		t.Errorf("unexpected EnvAnthropicAPIKey: %q", EnvAnthropicAPIKey)
	}
	if config.AuthMethodToken != "token" {
		t.Errorf("unexpected config.AuthMethodToken: %q", config.AuthMethodToken)
	}
	if config.AuthMethodAPIKey != "api_key" {
		t.Errorf("unexpected config.AuthMethodAPIKey: %q", config.AuthMethodAPIKey)
	}
}
