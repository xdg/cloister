package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// sampleGlobalConfig is a YAML config matching the schema in config-reference.md.
const sampleGlobalConfig = `
proxy:
  listen: ":3128"
  allow:
    - domain: "golang.org"
    - domain: "api.anthropic.com"
  unlisted_domain_behavior: "request_approval"
  approval_timeout: "60s"
  rate_limit: 120
  max_request_bytes: 10485760

request:
  listen: ":9998"
  timeout: "5m"

approval:
  listen: "127.0.0.1:9999"
  auto_approve:
    - pattern: "^docker compose ps$"
  manual_approve:
    - pattern: "^gh .+$"

devcontainer:
  enabled: true
  features:
    allow:
      - "ghcr.io/devcontainers/features/*"
  blocked_mounts:
    - "~/.ssh"
    - "~/.aws"

agents:
  claude:
    command: "claude"
    env:
      - "ANTHROPIC_*"
      - "CLAUDE_*"

defaults:
  image: "cloister:latest"
  shell: "/bin/bash"
  user: "cloister"
  agent: "claude"

log:
  file: "~/.local/share/cloister/audit.log"
  stdout: true
  level: "info"
  per_cloister: true
  per_cloister_dir: "~/.local/share/cloister/logs/"
`

// sampleProjectConfig is a YAML config for per-project settings.
const sampleProjectConfig = `
remote: "git@github.com:xdg/my-api.git"
root: "~/repos/my-api"
refs:
  - "~/repos/shared-lib"
  - "~/repos/api-docs"

proxy:
  allow:
    - domain: "internal-docs.company.com"

commands:
  auto_approve:
    - pattern: "^make test$"
`

func TestGlobalConfigUnmarshal(t *testing.T) {
	var cfg GlobalConfig
	if err := yaml.Unmarshal([]byte(sampleGlobalConfig), &cfg); err != nil {
		t.Fatalf("failed to unmarshal global config: %v", err)
	}

	// Verify proxy settings
	if cfg.Proxy.Listen != ":3128" {
		t.Errorf("Proxy.Listen = %q, want %q", cfg.Proxy.Listen, ":3128")
	}
	if len(cfg.Proxy.Allow) != 2 {
		t.Errorf("len(Proxy.Allow) = %d, want 2", len(cfg.Proxy.Allow))
	}
	if cfg.Proxy.Allow[0].Domain != "golang.org" {
		t.Errorf("Proxy.Allow[0].Domain = %q, want %q", cfg.Proxy.Allow[0].Domain, "golang.org")
	}
	if cfg.Proxy.UnlistedDomainBehavior != "request_approval" {
		t.Errorf("Proxy.UnlistedDomainBehavior = %q, want %q", cfg.Proxy.UnlistedDomainBehavior, "request_approval")
	}
	if cfg.Proxy.ApprovalTimeout != "60s" {
		t.Errorf("Proxy.ApprovalTimeout = %q, want %q", cfg.Proxy.ApprovalTimeout, "60s")
	}
	if cfg.Proxy.RateLimit != 120 {
		t.Errorf("Proxy.RateLimit = %d, want 120", cfg.Proxy.RateLimit)
	}
	if cfg.Proxy.MaxRequestBytes != 10485760 {
		t.Errorf("Proxy.MaxRequestBytes = %d, want 10485760", cfg.Proxy.MaxRequestBytes)
	}

	// Verify request settings
	if cfg.Request.Listen != ":9998" {
		t.Errorf("Request.Listen = %q, want %q", cfg.Request.Listen, ":9998")
	}
	if cfg.Request.Timeout != "5m" {
		t.Errorf("Request.Timeout = %q, want %q", cfg.Request.Timeout, "5m")
	}

	// Verify approval settings
	if cfg.Approval.Listen != "127.0.0.1:9999" {
		t.Errorf("Approval.Listen = %q, want %q", cfg.Approval.Listen, "127.0.0.1:9999")
	}
	if len(cfg.Approval.AutoApprove) != 1 {
		t.Errorf("len(Approval.AutoApprove) = %d, want 1", len(cfg.Approval.AutoApprove))
	}
	if cfg.Approval.AutoApprove[0].Pattern != "^docker compose ps$" {
		t.Errorf("Approval.AutoApprove[0].Pattern = %q, want %q", cfg.Approval.AutoApprove[0].Pattern, "^docker compose ps$")
	}
	if len(cfg.Approval.ManualApprove) != 1 {
		t.Errorf("len(Approval.ManualApprove) = %d, want 1", len(cfg.Approval.ManualApprove))
	}

	// Verify devcontainer settings
	if !cfg.Devcontainer.Enabled {
		t.Error("Devcontainer.Enabled = false, want true")
	}
	if len(cfg.Devcontainer.Features.Allow) != 1 {
		t.Errorf("len(Devcontainer.Features.Allow) = %d, want 1", len(cfg.Devcontainer.Features.Allow))
	}
	if len(cfg.Devcontainer.BlockedMounts) != 2 {
		t.Errorf("len(Devcontainer.BlockedMounts) = %d, want 2", len(cfg.Devcontainer.BlockedMounts))
	}

	// Verify agents
	claude, ok := cfg.Agents["claude"]
	if !ok {
		t.Fatal("Agents[\"claude\"] not found")
	}
	if claude.Command != "claude" {
		t.Errorf("Agents[\"claude\"].Command = %q, want %q", claude.Command, "claude")
	}
	if len(claude.Env) != 2 {
		t.Errorf("len(Agents[\"claude\"].Env) = %d, want 2", len(claude.Env))
	}

	// Verify defaults
	if cfg.Defaults.Image != "cloister:latest" {
		t.Errorf("Defaults.Image = %q, want %q", cfg.Defaults.Image, "cloister:latest")
	}
	if cfg.Defaults.Shell != "/bin/bash" {
		t.Errorf("Defaults.Shell = %q, want %q", cfg.Defaults.Shell, "/bin/bash")
	}
	if cfg.Defaults.User != "cloister" {
		t.Errorf("Defaults.User = %q, want %q", cfg.Defaults.User, "cloister")
	}
	if cfg.Defaults.Agent != "claude" {
		t.Errorf("Defaults.Agent = %q, want %q", cfg.Defaults.Agent, "claude")
	}

	// Verify log settings
	if cfg.Log.File != "~/.local/share/cloister/audit.log" {
		t.Errorf("Log.File = %q, want %q", cfg.Log.File, "~/.local/share/cloister/audit.log")
	}
	if !cfg.Log.Stdout {
		t.Error("Log.Stdout = false, want true")
	}
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "info")
	}
	if !cfg.Log.PerCloister {
		t.Error("Log.PerCloister = false, want true")
	}
	if cfg.Log.PerCloisterDir != "~/.local/share/cloister/logs/" {
		t.Errorf("Log.PerCloisterDir = %q, want %q", cfg.Log.PerCloisterDir, "~/.local/share/cloister/logs/")
	}
}

func TestProjectConfigUnmarshal(t *testing.T) {
	var cfg ProjectConfig
	if err := yaml.Unmarshal([]byte(sampleProjectConfig), &cfg); err != nil {
		t.Fatalf("failed to unmarshal project config: %v", err)
	}

	if cfg.Remote != "git@github.com:xdg/my-api.git" {
		t.Errorf("Remote = %q, want %q", cfg.Remote, "git@github.com:xdg/my-api.git")
	}
	if cfg.Root != "~/repos/my-api" {
		t.Errorf("Root = %q, want %q", cfg.Root, "~/repos/my-api")
	}
	if len(cfg.Refs) != 2 {
		t.Errorf("len(Refs) = %d, want 2", len(cfg.Refs))
	}
	if cfg.Refs[0] != "~/repos/shared-lib" {
		t.Errorf("Refs[0] = %q, want %q", cfg.Refs[0], "~/repos/shared-lib")
	}
	if len(cfg.Proxy.Allow) != 1 {
		t.Errorf("len(Proxy.Allow) = %d, want 1", len(cfg.Proxy.Allow))
	}
	if cfg.Proxy.Allow[0].Domain != "internal-docs.company.com" {
		t.Errorf("Proxy.Allow[0].Domain = %q, want %q", cfg.Proxy.Allow[0].Domain, "internal-docs.company.com")
	}
	if len(cfg.Commands.AutoApprove) != 1 {
		t.Errorf("len(Commands.AutoApprove) = %d, want 1", len(cfg.Commands.AutoApprove))
	}
	if cfg.Commands.AutoApprove[0].Pattern != "^make test$" {
		t.Errorf("Commands.AutoApprove[0].Pattern = %q, want %q", cfg.Commands.AutoApprove[0].Pattern, "^make test$")
	}
}

func TestGlobalConfigRoundTrip(t *testing.T) {
	// Test that marshal -> unmarshal preserves all fields
	original := GlobalConfig{
		Proxy: ProxyConfig{
			Listen: ":3128",
			Allow: []AllowEntry{
				{Domain: "example.com"},
			},
			UnlistedDomainBehavior: "reject",
			ApprovalTimeout:        "30s",
			RateLimit:              60,
			MaxRequestBytes:        1024,
		},
		Request: RequestConfig{
			Listen:  ":9998",
			Timeout: "2m",
		},
		Approval: ApprovalConfig{
			Listen: "127.0.0.1:9999",
			AutoApprove: []CommandPattern{
				{Pattern: "^test$"},
			},
			ManualApprove: []CommandPattern{
				{Pattern: "^deploy$"},
			},
		},
		Devcontainer: DevcontainerConfig{
			Enabled: true,
			Features: FeaturesConfig{
				Allow: []string{"feature1", "feature2"},
			},
			BlockedMounts: []string{"/secret"},
		},
		Agents: map[string]AgentConfig{
			"test-agent": {
				Command: "test-cmd",
				Env:     []string{"VAR1", "VAR2"},
			},
		},
		Defaults: DefaultsConfig{
			Image: "test:latest",
			Shell: "/bin/zsh",
			User:  "testuser",
			Agent: "test-agent",
		},
		Log: LogConfig{
			File:           "/var/log/test.log",
			Stdout:         true,
			Level:          "debug",
			PerCloister:    true,
			PerCloisterDir: "/var/log/cloisters/",
		},
	}

	data, err := yaml.Marshal(&original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var roundTripped GlobalConfig
	if err := yaml.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Verify key fields survived the round trip
	if roundTripped.Proxy.Listen != original.Proxy.Listen {
		t.Errorf("Proxy.Listen = %q, want %q", roundTripped.Proxy.Listen, original.Proxy.Listen)
	}
	if roundTripped.Proxy.RateLimit != original.Proxy.RateLimit {
		t.Errorf("Proxy.RateLimit = %d, want %d", roundTripped.Proxy.RateLimit, original.Proxy.RateLimit)
	}
	if roundTripped.Proxy.MaxRequestBytes != original.Proxy.MaxRequestBytes {
		t.Errorf("Proxy.MaxRequestBytes = %d, want %d", roundTripped.Proxy.MaxRequestBytes, original.Proxy.MaxRequestBytes)
	}
	if len(roundTripped.Agents) != len(original.Agents) {
		t.Errorf("len(Agents) = %d, want %d", len(roundTripped.Agents), len(original.Agents))
	}
	agent, ok := roundTripped.Agents["test-agent"]
	if !ok {
		t.Fatal("Agents[\"test-agent\"] not found")
	}
	if agent.Command != "test-cmd" {
		t.Errorf("Agents[\"test-agent\"].Command = %q, want %q", agent.Command, "test-cmd")
	}
}

func TestAgentConfigCredentialFieldsRoundTrip(t *testing.T) {
	// Test YAML round-trip for AgentConfig credential fields (Phase 3.1.1)
	skipPerms := true

	tests := []struct {
		name   string
		agent  AgentConfig
		verify func(t *testing.T, got AgentConfig)
	}{
		{
			name: "all_credential_fields",
			agent: AgentConfig{
				Command:    "claude",
				Env:        []string{"ANTHROPIC_*"},
				AuthMethod: "token",
				Token:      "oauth-token-value",
				APIKey:     "sk-ant-api-key",
				SkipPerms:  &skipPerms,
			},
			verify: func(t *testing.T, got AgentConfig) {
				if got.AuthMethod != "token" {
					t.Errorf("AuthMethod = %q, want %q", got.AuthMethod, "token")
				}
				if got.Token != "oauth-token-value" {
					t.Errorf("Token = %q, want %q", got.Token, "oauth-token-value")
				}
				if got.APIKey != "sk-ant-api-key" {
					t.Errorf("APIKey = %q, want %q", got.APIKey, "sk-ant-api-key")
				}
				if got.SkipPerms == nil || *got.SkipPerms != true {
					t.Errorf("SkipPerms = %v, want ptr to true", got.SkipPerms)
				}
			},
		},
		{
			name: "auth_method_existing",
			agent: AgentConfig{
				AuthMethod: "existing",
			},
			verify: func(t *testing.T, got AgentConfig) {
				if got.AuthMethod != "existing" {
					t.Errorf("AuthMethod = %q, want %q", got.AuthMethod, "existing")
				}
			},
		},
		{
			name: "auth_method_api_key",
			agent: AgentConfig{
				AuthMethod: "api_key",
				APIKey:     "sk-ant-test-key",
			},
			verify: func(t *testing.T, got AgentConfig) {
				if got.AuthMethod != "api_key" {
					t.Errorf("AuthMethod = %q, want %q", got.AuthMethod, "api_key")
				}
				if got.APIKey != "sk-ant-test-key" {
					t.Errorf("APIKey = %q, want %q", got.APIKey, "sk-ant-test-key")
				}
			},
		},
		{
			name: "skip_perms_false",
			agent: AgentConfig{
				SkipPerms: func() *bool { b := false; return &b }(),
			},
			verify: func(t *testing.T, got AgentConfig) {
				if got.SkipPerms == nil || *got.SkipPerms != false {
					t.Errorf("SkipPerms = %v, want ptr to false", got.SkipPerms)
				}
			},
		},
		{
			name: "skip_perms_nil",
			agent: AgentConfig{
				AuthMethod: "token",
				SkipPerms:  nil,
			},
			verify: func(t *testing.T, got AgentConfig) {
				if got.SkipPerms != nil {
					t.Errorf("SkipPerms = %v, want nil", got.SkipPerms)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := yaml.Marshal(&tc.agent)
			if err != nil {
				t.Fatalf("failed to marshal: %v", err)
			}

			var roundTripped AgentConfig
			if err := yaml.Unmarshal(data, &roundTripped); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			tc.verify(t, roundTripped)
		})
	}
}

func TestAgentConfigYAMLFieldNames(t *testing.T) {
	// Verify yaml tags produce correct snake_case field names
	skipPerms := true
	agent := AgentConfig{
		AuthMethod: "token",
		Token:      "test-token",
		APIKey:     "test-key",
		SkipPerms:  &skipPerms,
	}

	data, err := yaml.Marshal(&agent)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	yamlStr := string(data)

	// Verify snake_case field names in YAML output
	expectedFields := []string{
		"auth_method:",
		"token:",
		"api_key:",
		"skip_permissions:",
	}

	for _, field := range expectedFields {
		if !strings.Contains(yamlStr, field) {
			t.Errorf("YAML output missing field %q\nGot:\n%s", field, yamlStr)
		}
	}

	// Verify camelCase is NOT used
	unexpectedFields := []string{
		"authMethod:",
		"apiKey:",
		"skipPerms:",
		"skipPermissions:", // should be skip_permissions
	}

	for _, field := range unexpectedFields {
		if strings.Contains(yamlStr, field) {
			t.Errorf("YAML output should not contain %q\nGot:\n%s", field, yamlStr)
		}
	}
}

func TestAgentConfigInGlobalConfigRoundTrip(t *testing.T) {
	// Test credential fields work correctly when nested in GlobalConfig
	skipPerms := true
	original := GlobalConfig{
		Agents: map[string]AgentConfig{
			"claude": {
				Command:    "claude",
				AuthMethod: "token",
				Token:      "my-oauth-token",
				SkipPerms:  &skipPerms,
			},
			"codex": {
				Command:    "codex",
				AuthMethod: "api_key",
				APIKey:     "sk-openai-key",
			},
		},
	}

	data, err := yaml.Marshal(&original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var roundTripped GlobalConfig
	if err := yaml.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Verify claude agent
	claude, ok := roundTripped.Agents["claude"]
	if !ok {
		t.Fatal("Agents[\"claude\"] not found")
	}
	if claude.AuthMethod != "token" {
		t.Errorf("claude.AuthMethod = %q, want %q", claude.AuthMethod, "token")
	}
	if claude.Token != "my-oauth-token" {
		t.Errorf("claude.Token = %q, want %q", claude.Token, "my-oauth-token")
	}
	if claude.SkipPerms == nil || *claude.SkipPerms != true {
		t.Errorf("claude.SkipPerms = %v, want ptr to true", claude.SkipPerms)
	}

	// Verify codex agent
	codex, ok := roundTripped.Agents["codex"]
	if !ok {
		t.Fatal("Agents[\"codex\"] not found")
	}
	if codex.AuthMethod != "api_key" {
		t.Errorf("codex.AuthMethod = %q, want %q", codex.AuthMethod, "api_key")
	}
	if codex.APIKey != "sk-openai-key" {
		t.Errorf("codex.APIKey = %q, want %q", codex.APIKey, "sk-openai-key")
	}
}

func TestProjectConfigRoundTrip(t *testing.T) {
	original := ProjectConfig{
		Remote: "git@github.com:test/repo.git",
		Root:   "/path/to/repo",
		Refs:   []string{"/ref1", "/ref2"},
		Proxy: ProjectProxyConfig{
			Allow: []AllowEntry{
				{Domain: "internal.example.com"},
			},
		},
		Commands: ProjectCommandsConfig{
			AutoApprove: []CommandPattern{
				{Pattern: "^make build$"},
			},
		},
	}

	data, err := yaml.Marshal(&original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var roundTripped ProjectConfig
	if err := yaml.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if roundTripped.Remote != original.Remote {
		t.Errorf("Remote = %q, want %q", roundTripped.Remote, original.Remote)
	}
	if roundTripped.Root != original.Root {
		t.Errorf("Root = %q, want %q", roundTripped.Root, original.Root)
	}
	if len(roundTripped.Refs) != len(original.Refs) {
		t.Errorf("len(Refs) = %d, want %d", len(roundTripped.Refs), len(original.Refs))
	}
	if len(roundTripped.Proxy.Allow) != len(original.Proxy.Allow) {
		t.Errorf("len(Proxy.Allow) = %d, want %d", len(roundTripped.Proxy.Allow), len(original.Proxy.Allow))
	}
	if len(roundTripped.Commands.AutoApprove) != len(original.Commands.AutoApprove) {
		t.Errorf("len(Commands.AutoApprove) = %d, want %d", len(roundTripped.Commands.AutoApprove), len(original.Commands.AutoApprove))
	}
}
