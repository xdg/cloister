package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadGlobalConfig_Missing(t *testing.T) {
	// Use a temp directory with no config file
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig() error = %v", err)
	}

	// Should return default config
	if cfg == nil {
		t.Fatal("LoadGlobalConfig() returned nil config")
	}

	// Verify some default values are present
	if cfg.Proxy.Listen != ":3128" {
		t.Errorf("cfg.Proxy.Listen = %q, want %q", cfg.Proxy.Listen, ":3128")
	}
	if cfg.Defaults.Agent != "claude" {
		t.Errorf("cfg.Defaults.Agent = %q, want %q", cfg.Defaults.Agent, "claude")
	}

	// Verify default config file was created
	configPath := filepath.Join(tmpDir, "cloister", "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("LoadGlobalConfig() should create default config file when missing")
	}
}

func TestLoadGlobalConfig_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create config directory and file
	configDir := filepath.Join(tmpDir, "cloister")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	configContent := `
proxy:
  listen: ":8080"
  rate_limit: 200
defaults:
  agent: codex
  shell: /bin/zsh
log:
  level: debug
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig() error = %v", err)
	}

	if cfg.Proxy.Listen != ":8080" {
		t.Errorf("cfg.Proxy.Listen = %q, want %q", cfg.Proxy.Listen, ":8080")
	}
	if cfg.Proxy.RateLimit != 200 {
		t.Errorf("cfg.Proxy.RateLimit = %d, want %d", cfg.Proxy.RateLimit, 200)
	}
	if cfg.Defaults.Agent != "codex" {
		t.Errorf("cfg.Defaults.Agent = %q, want %q", cfg.Defaults.Agent, "codex")
	}
	if cfg.Defaults.Shell != "/bin/zsh" {
		t.Errorf("cfg.Defaults.Shell = %q, want %q", cfg.Defaults.Shell, "/bin/zsh")
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("cfg.Log.Level = %q, want %q", cfg.Log.Level, "debug")
	}
}

func TestLoadGlobalConfig_Invalid(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	configDir := filepath.Join(tmpDir, "cloister")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	// Config with invalid log level
	configContent := `
log:
  level: invalid_level
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	_, err := LoadGlobalConfig()
	if err == nil {
		t.Fatal("LoadGlobalConfig() expected error for invalid config, got nil")
	}

	if !strings.Contains(err.Error(), "log.level") {
		t.Errorf("error message %q should mention 'log.level'", err.Error())
	}
}

func TestLoadGlobalConfig_Corrupt(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	configDir := filepath.Join(tmpDir, "cloister")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	// Corrupt YAML
	configContent := `
proxy:
  listen: ":8080"
  rate_limit: [this is not valid yaml
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	_, err := LoadGlobalConfig()
	if err == nil {
		t.Fatal("LoadGlobalConfig() expected error for corrupt YAML, got nil")
	}
}

func TestLoadGlobalConfig_ExpandsPaths(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	configDir := filepath.Join(tmpDir, "cloister")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	configContent := `
log:
  file: "~/logs/cloister.log"
  per_cloister_dir: "~/logs/cloisters/"
devcontainer:
  blocked_mounts:
    - "~/.secret-dir"
    - "/absolute/path"
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig() error = %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error = %v", err)
	}

	// Verify log paths are expanded
	wantLogFile := filepath.Join(home, "logs/cloister.log")
	if cfg.Log.File != wantLogFile {
		t.Errorf("cfg.Log.File = %q, want %q", cfg.Log.File, wantLogFile)
	}

	wantPerCloisterDir := filepath.Join(home, "logs/cloisters/")
	if cfg.Log.PerCloisterDir != wantPerCloisterDir {
		t.Errorf("cfg.Log.PerCloisterDir = %q, want %q", cfg.Log.PerCloisterDir, wantPerCloisterDir)
	}

	// Verify blocked mounts are expanded (~ paths only)
	if len(cfg.Devcontainer.BlockedMounts) != 2 {
		t.Fatalf("len(BlockedMounts) = %d, want 2", len(cfg.Devcontainer.BlockedMounts))
	}
	wantBlockedMount := filepath.Join(home, ".secret-dir")
	if cfg.Devcontainer.BlockedMounts[0] != wantBlockedMount {
		t.Errorf("BlockedMounts[0] = %q, want %q", cfg.Devcontainer.BlockedMounts[0], wantBlockedMount)
	}
	// Absolute path should remain unchanged
	if cfg.Devcontainer.BlockedMounts[1] != "/absolute/path" {
		t.Errorf("BlockedMounts[1] = %q, want %q", cfg.Devcontainer.BlockedMounts[1], "/absolute/path")
	}
}

func TestLoadGlobalConfig_ExpandsDefaultPaths(t *testing.T) {
	// Test that default config paths are also expanded
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// No config file, so defaults are used
	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig() error = %v", err)
	}

	// Default log file should be expanded
	if strings.HasPrefix(cfg.Log.File, "~") {
		t.Errorf("cfg.Log.File = %q should not start with ~", cfg.Log.File)
	}
}

func TestLoadProjectConfig_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	cfg, err := LoadProjectConfig("nonexistent-project")
	if err != nil {
		t.Fatalf("LoadProjectConfig() error = %v", err)
	}

	// Should return default (empty) project config
	if cfg == nil {
		t.Fatal("LoadProjectConfig() returned nil config")
	}

	// Default project config should be mostly empty
	if cfg.Remote != "" {
		t.Errorf("cfg.Remote = %q, want empty string", cfg.Remote)
	}
}

func TestLoadProjectConfig_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create projects directory and file
	projectsDir := filepath.Join(tmpDir, "cloister", "projects")
	if err := os.MkdirAll(projectsDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	configContent := `
remote: "git@github.com:example/repo.git"
root: "~/projects/repo"
refs:
  - "~/docs/api-spec"
  - "/shared/common-libs"
proxy:
  allow:
    - domain: "custom.example.com"
commands:
  auto_approve:
    - pattern: "^make test$"
`
	configPath := filepath.Join(projectsDir, "my-project.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	cfg, err := LoadProjectConfig("my-project")
	if err != nil {
		t.Fatalf("LoadProjectConfig() error = %v", err)
	}

	if cfg.Remote != "git@github.com:example/repo.git" {
		t.Errorf("cfg.Remote = %q, want %q", cfg.Remote, "git@github.com:example/repo.git")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error = %v", err)
	}

	// Verify root path is expanded
	wantRoot := filepath.Join(home, "projects/repo")
	if cfg.Root != wantRoot {
		t.Errorf("cfg.Root = %q, want %q", cfg.Root, wantRoot)
	}

	// Verify refs are expanded (~ paths only)
	if len(cfg.Refs) != 2 {
		t.Fatalf("len(cfg.Refs) = %d, want 2", len(cfg.Refs))
	}
	wantRef := filepath.Join(home, "docs/api-spec")
	if cfg.Refs[0] != wantRef {
		t.Errorf("cfg.Refs[0] = %q, want %q", cfg.Refs[0], wantRef)
	}
	// Absolute path should remain unchanged
	if cfg.Refs[1] != "/shared/common-libs" {
		t.Errorf("cfg.Refs[1] = %q, want %q", cfg.Refs[1], "/shared/common-libs")
	}

	// Verify proxy allow list
	if len(cfg.Proxy.Allow) != 1 {
		t.Fatalf("len(cfg.Proxy.Allow) = %d, want 1", len(cfg.Proxy.Allow))
	}
	if cfg.Proxy.Allow[0].Domain != "custom.example.com" {
		t.Errorf("cfg.Proxy.Allow[0].Domain = %q, want %q", cfg.Proxy.Allow[0].Domain, "custom.example.com")
	}

	// Verify command patterns
	if len(cfg.Commands.AutoApprove) != 1 {
		t.Fatalf("len(cfg.Commands.AutoApprove) = %d, want 1", len(cfg.Commands.AutoApprove))
	}
	if cfg.Commands.AutoApprove[0].Pattern != "^make test$" {
		t.Errorf("cfg.Commands.AutoApprove[0].Pattern = %q, want %q", cfg.Commands.AutoApprove[0].Pattern, "^make test$")
	}
}

func TestLoadProjectConfig_InvalidRegex(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	projectsDir := filepath.Join(tmpDir, "cloister", "projects")
	if err := os.MkdirAll(projectsDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	// Config with invalid regex pattern
	configContent := `
commands:
  auto_approve:
    - pattern: "[invalid(regex"
`
	configPath := filepath.Join(projectsDir, "bad-project.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	_, err := LoadProjectConfig("bad-project")
	if err == nil {
		t.Fatal("LoadProjectConfig() expected error for invalid regex, got nil")
	}

	if !strings.Contains(err.Error(), "pattern") {
		t.Errorf("error message %q should mention 'pattern'", err.Error())
	}
}

func TestLoadProjectConfig_UnknownField(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	projectsDir := filepath.Join(tmpDir, "cloister", "projects")
	if err := os.MkdirAll(projectsDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	// Config with unknown field (strict parsing should reject this)
	configContent := `
remote: "git@github.com:example/repo.git"
unknown_field: "this should cause an error"
`
	configPath := filepath.Join(projectsDir, "unknown-field.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	_, err := LoadProjectConfig("unknown-field")
	if err == nil {
		t.Fatal("LoadProjectConfig() expected error for unknown field, got nil")
	}
}
