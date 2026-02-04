package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xdg/cloister/internal/config"
)

func TestMergeTOMLConfig_EmptyHost(t *testing.T) {
	// Use a temp directory that doesn't have the config file
	tmpDir := t.TempDir()
	originalFunc := UserHomeDirFunc
	UserHomeDirFunc = func() (string, error) { return tmpDir, nil }
	defer func() { UserHomeDirFunc = originalFunc }()

	forcedValues := map[string]any{
		"approval_policy": "full-auto",
	}

	result, err := MergeTOMLConfig(".codex/config.toml", nil, forcedValues)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, `approval_policy = "full-auto"`) {
		t.Errorf("expected approval_policy in result, got: %s", result)
	}
}

func TestMergeTOMLConfig_WithExistingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	originalFunc := UserHomeDirFunc
	UserHomeDirFunc = func() (string, error) { return tmpDir, nil }
	defer func() { UserHomeDirFunc = originalFunc }()

	// Create a config file with existing content
	codexDir := filepath.Join(tmpDir, ".codex")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		t.Fatalf("failed to create .codex dir: %v", err)
	}
	existingConfig := `model = "gpt-5-codex"
approval_policy = "on-request"
`
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(existingConfig), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	forcedValues := map[string]any{
		"approval_policy": "full-auto",
	}

	result, err := MergeTOMLConfig(".codex/config.toml", nil, forcedValues)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should contain original content
	if !strings.Contains(result, `model = "gpt-5-codex"`) {
		t.Errorf("expected original model in result, got: %s", result)
	}

	// Should contain forced value (which overrides the original)
	if !strings.Contains(result, `approval_policy = "full-auto"`) {
		t.Errorf("expected forced approval_policy in result, got: %s", result)
	}

	// Should have cloister marker
	if !strings.Contains(result, "# Cloister forced values") {
		t.Errorf("expected cloister marker in result, got: %s", result)
	}
}

func TestMergeTOMLConfig_NestedValues(t *testing.T) {
	tmpDir := t.TempDir()
	originalFunc := UserHomeDirFunc
	UserHomeDirFunc = func() (string, error) { return tmpDir, nil }
	defer func() { UserHomeDirFunc = originalFunc }()

	forcedValues := map[string]any{
		"sandbox_workspace_write.network_access": true,
	}

	result, err := MergeTOMLConfig(".codex/config.toml", nil, forcedValues)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should contain section header
	if !strings.Contains(result, "[sandbox_workspace_write]") {
		t.Errorf("expected section header in result, got: %s", result)
	}

	// Should contain nested value
	if !strings.Contains(result, "network_access = true") {
		t.Errorf("expected nested value in result, got: %s", result)
	}
}

func TestFormatTOMLValue(t *testing.T) {
	tests := []struct {
		key      string
		value    any
		expected string
	}{
		{"key", "value", `key = "value"` + "\n"},
		{"flag", true, "flag = true\n"},
		{"flag", false, "flag = false\n"},
		{"count", 42, "count = 42\n"},
		{"rate", 3.14, "rate = 3.14\n"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := formatTOMLValue(tt.key, tt.value)
			if result != tt.expected {
				t.Errorf("formatTOMLValue(%q, %v) = %q, want %q", tt.key, tt.value, result, tt.expected)
			}
		})
	}
}

func TestRegistry(t *testing.T) {
	// Test that agents are registered
	names := List()
	if len(names) < 2 {
		t.Errorf("expected at least 2 agents registered, got %d", len(names))
	}

	// Test Get for claude
	claude := Get("claude")
	if claude == nil {
		t.Error("expected claude agent to be registered")
	}
	if claude != nil && claude.Name() != "claude" {
		t.Errorf("expected claude agent name, got %q", claude.Name())
	}

	// Test Get for codex
	codex := Get("codex")
	if codex == nil {
		t.Error("expected codex agent to be registered")
	}
	if codex != nil && codex.Name() != "codex" {
		t.Errorf("expected codex agent name, got %q", codex.Name())
	}

	// Test Get for non-existent agent
	nonexistent := Get("nonexistent")
	if nonexistent != nil {
		t.Error("expected nil for non-existent agent")
	}
}

func TestCodexAgent_Name(t *testing.T) {
	agent := NewCodexAgent()
	if agent.Name() != "codex" {
		t.Errorf("expected name 'codex', got %q", agent.Name())
	}
}

func TestCodexAgent_GetContainerEnvVars_NoConfig(t *testing.T) {
	agent := NewCodexAgent()

	// nil config should return nil
	envVars, err := agent.GetContainerEnvVars(nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if envVars != nil {
		t.Errorf("expected nil env vars for nil config, got %v", envVars)
	}
}

func TestCodexAgent_GetContainerEnvVars_NoAuthMethod(t *testing.T) {
	agent := NewCodexAgent()

	cfg := &config.AgentConfig{}
	envVars, err := agent.GetContainerEnvVars(cfg)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if envVars != nil {
		t.Errorf("expected nil env vars for empty auth method, got %v", envVars)
	}
}

func TestClaudeAgent_Name(t *testing.T) {
	agent := NewClaudeAgent()
	if agent.Name() != "claude" {
		t.Errorf("expected name 'claude', got %q", agent.Name())
	}
}

func TestClaudeAgent_GetContainerEnvVars_NoConfig(t *testing.T) {
	agent := NewClaudeAgent()

	envVars, err := agent.GetContainerEnvVars(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if envVars == nil {
		t.Fatal("expected non-nil env vars even with nil config")
	}
	if envVars["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"] != "1" {
		t.Errorf("expected CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1, got %q",
			envVars["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"])
	}
	// Should only have the one env var (no credentials)
	if len(envVars) != 1 {
		t.Errorf("expected 1 env var, got %d: %v", len(envVars), envVars)
	}
}

func TestClaudeAgent_GetContainerEnvVars_NoAuthMethod(t *testing.T) {
	agent := NewClaudeAgent()

	cfg := &config.AgentConfig{}
	envVars, err := agent.GetContainerEnvVars(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if envVars["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"] != "1" {
		t.Errorf("expected CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1, got %q",
			envVars["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"])
	}
	if len(envVars) != 1 {
		t.Errorf("expected 1 env var, got %d: %v", len(envVars), envVars)
	}
}
