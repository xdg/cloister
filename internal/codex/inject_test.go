package codex

import (
	"errors"
	"testing"

	"github.com/xdg/cloister/internal/config"
)

func TestInjector_InjectCredentials_APIKey(t *testing.T) {
	injector := NewInjector()

	cfg := &config.AgentConfig{
		AuthMethod: "api_key",
		APIKey:     "sk-test-openai-key",
	}

	result, err := injector.InjectCredentials(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have env var set
	if len(result.EnvVars) != 1 {
		t.Errorf("expected 1 env var, got %d", len(result.EnvVars))
	}
	if result.EnvVars[EnvOpenAIAPIKey] != "sk-test-openai-key" {
		t.Errorf("expected API key %q, got %q", "sk-test-openai-key", result.EnvVars[EnvOpenAIAPIKey])
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
	if EnvOpenAIAPIKey != "OPENAI_API_KEY" {
		t.Errorf("unexpected EnvOpenAIAPIKey: %q", EnvOpenAIAPIKey)
	}
	if AuthMethodAPIKey != "api_key" {
		t.Errorf("unexpected AuthMethodAPIKey: %q", AuthMethodAPIKey)
	}
}
