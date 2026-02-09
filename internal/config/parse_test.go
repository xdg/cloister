package config

import (
	"strings"
	"testing"
)

func TestParseGlobalConfig_Valid(t *testing.T) {
	cfg, err := ParseGlobalConfig([]byte(sampleGlobalConfig))
	if err != nil {
		t.Fatalf("ParseGlobalConfig() error = %v", err)
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

	// Verify hostexec settings
	if cfg.Hostexec.Listen != "127.0.0.1:9999" {
		t.Errorf("Hostexec.Listen = %q, want %q", cfg.Hostexec.Listen, "127.0.0.1:9999")
	}
	if len(cfg.Hostexec.AutoApprove) != 1 {
		t.Errorf("len(Hostexec.AutoApprove) = %d, want 1", len(cfg.Hostexec.AutoApprove))
	}

	// Verify agents
	claude, ok := cfg.Agents["claude"]
	if !ok {
		t.Fatal("Agents[\"claude\"] not found")
	}
	if claude.Command != "claude" {
		t.Errorf("Agents[\"claude\"].Command = %q, want %q", claude.Command, "claude")
	}

	// Verify defaults
	if cfg.Defaults.Image != "cloister:latest" {
		t.Errorf("Defaults.Image = %q, want %q", cfg.Defaults.Image, "cloister:latest")
	}
	if cfg.Defaults.Agent != "claude" {
		t.Errorf("Defaults.Agent = %q, want %q", cfg.Defaults.Agent, "claude")
	}

	// Verify log settings
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "info")
	}
	if !cfg.Log.Stdout {
		t.Error("Log.Stdout = false, want true")
	}
}

func TestParseGlobalConfig_Empty(t *testing.T) {
	cfg, err := ParseGlobalConfig([]byte(""))
	if err != nil {
		t.Fatalf("ParseGlobalConfig() error = %v", err)
	}

	// Empty config should have zero values
	if cfg.Proxy.Listen != "" {
		t.Errorf("Proxy.Listen = %q, want empty", cfg.Proxy.Listen)
	}
	if cfg.Proxy.RateLimit != 0 {
		t.Errorf("Proxy.RateLimit = %d, want 0", cfg.Proxy.RateLimit)
	}
	if len(cfg.Proxy.Allow) != 0 {
		t.Errorf("len(Proxy.Allow) = %d, want 0", len(cfg.Proxy.Allow))
	}
	if cfg.Devcontainer.Enabled {
		t.Error("Devcontainer.Enabled = true, want false")
	}
	if cfg.Agents != nil {
		t.Errorf("Agents = %v, want nil", cfg.Agents)
	}
}

func TestParseGlobalConfig_InvalidYAML(t *testing.T) {
	invalidYAML := `
proxy:
  listen: ":3128"
  allow:
    - this is not valid YAML structure
      missing colon here
`
	_, err := ParseGlobalConfig([]byte(invalidYAML))
	if err == nil {
		t.Fatal("ParseGlobalConfig() expected error for invalid YAML")
	}
	if !strings.Contains(err.Error(), "parse global config") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "parse global config")
	}
}

func TestParseGlobalConfig_UnknownField(t *testing.T) {
	yamlWithTypo := `
proxy:
  listenn: ":3128"  # typo: extra 'n'
`
	_, err := ParseGlobalConfig([]byte(yamlWithTypo))
	if err == nil {
		t.Fatal("ParseGlobalConfig() expected error for unknown field 'listenn'")
	}
	if !strings.Contains(err.Error(), "listenn") {
		t.Errorf("error = %q, want to mention unknown field 'listenn'", err.Error())
	}
}

func TestParseGlobalConfig_TypeMismatch(t *testing.T) {
	yamlWithTypeMismatch := `
proxy:
  rate_limit: "not a number"
`
	_, err := ParseGlobalConfig([]byte(yamlWithTypeMismatch))
	if err == nil {
		t.Fatal("ParseGlobalConfig() expected error for type mismatch")
	}
}

func TestParseGlobalConfig_NestedUnknownField(t *testing.T) {
	yamlWithNestedTypo := `
hostexec:
  listen: "127.0.0.1:9999"
  auto_aprove:  # typo: missing 'p'
    - pattern: "^test$"
`
	_, err := ParseGlobalConfig([]byte(yamlWithNestedTypo))
	if err == nil {
		t.Fatal("ParseGlobalConfig() expected error for unknown field 'auto_aprove'")
	}
	if !strings.Contains(err.Error(), "auto_aprove") {
		t.Errorf("error = %q, want to mention unknown field 'auto_aprove'", err.Error())
	}
}

func TestParseProjectConfig_Valid(t *testing.T) {
	cfg, err := ParseProjectConfig([]byte(sampleProjectConfig))
	if err != nil {
		t.Fatalf("ParseProjectConfig() error = %v", err)
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
	if len(cfg.Hostexec.AutoApprove) != 1 {
		t.Errorf("len(Hostexec.AutoApprove) = %d, want 1", len(cfg.Hostexec.AutoApprove))
	}
	if cfg.Hostexec.AutoApprove[0].Pattern != "^make test$" {
		t.Errorf("Hostexec.AutoApprove[0].Pattern = %q, want %q", cfg.Hostexec.AutoApprove[0].Pattern, "^make test$")
	}
	if len(cfg.Hostexec.ManualApprove) != 1 {
		t.Errorf("len(Hostexec.ManualApprove) = %d, want 1", len(cfg.Hostexec.ManualApprove))
	}
	if cfg.Hostexec.ManualApprove[0].Pattern != "^./deploy\\.sh.*$" {
		t.Errorf("Hostexec.ManualApprove[0].Pattern = %q, want %q", cfg.Hostexec.ManualApprove[0].Pattern, "^./deploy\\.sh.*$")
	}
}

func TestParseProjectConfig_Empty(t *testing.T) {
	cfg, err := ParseProjectConfig([]byte(""))
	if err != nil {
		t.Fatalf("ParseProjectConfig() error = %v", err)
	}

	// Empty config should have zero values
	if cfg.Remote != "" {
		t.Errorf("Remote = %q, want empty", cfg.Remote)
	}
	if cfg.Root != "" {
		t.Errorf("Root = %q, want empty", cfg.Root)
	}
	if len(cfg.Refs) != 0 {
		t.Errorf("len(Refs) = %d, want 0", len(cfg.Refs))
	}
	if len(cfg.Proxy.Allow) != 0 {
		t.Errorf("len(Proxy.Allow) = %d, want 0", len(cfg.Proxy.Allow))
	}
	if len(cfg.Hostexec.AutoApprove) != 0 {
		t.Errorf("len(Hostexec.AutoApprove) = %d, want 0", len(cfg.Hostexec.AutoApprove))
	}
	if len(cfg.Hostexec.ManualApprove) != 0 {
		t.Errorf("len(Hostexec.ManualApprove) = %d, want 0", len(cfg.Hostexec.ManualApprove))
	}
}

func TestParseProjectConfig_InvalidYAML(t *testing.T) {
	invalidYAML := `
remote: "git@github.com:test/repo.git"
refs:
  - one
  two  # missing dash
`
	_, err := ParseProjectConfig([]byte(invalidYAML))
	if err == nil {
		t.Fatal("ParseProjectConfig() expected error for invalid YAML")
	}
	if !strings.Contains(err.Error(), "parse project config") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "parse project config")
	}
}

func TestParseProjectConfig_UnknownField(t *testing.T) {
	yamlWithUnknownField := `
remote: "git@github.com:test/repo.git"
unknownfield: "value"
`
	_, err := ParseProjectConfig([]byte(yamlWithUnknownField))
	if err == nil {
		t.Fatal("ParseProjectConfig() expected error for unknown field")
	}
	if !strings.Contains(err.Error(), "unknownfield") {
		t.Errorf("error = %q, want to mention unknown field 'unknownfield'", err.Error())
	}
}

func TestParseProjectConfig_NestedUnknownField(t *testing.T) {
	yamlWithNestedTypo := `
proxy:
  alow:  # typo: missing 'l'
    - domain: "example.com"
`
	_, err := ParseProjectConfig([]byte(yamlWithNestedTypo))
	if err == nil {
		t.Fatal("ParseProjectConfig() expected error for unknown field 'alow'")
	}
	if !strings.Contains(err.Error(), "alow") {
		t.Errorf("error = %q, want to mention unknown field 'alow'", err.Error())
	}
}
