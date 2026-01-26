package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteDefaultConfig_Creates(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// File should not exist yet
	path := GlobalConfigPath()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("config file should not exist before test: %v", err)
	}

	// Write default config
	if err := WriteDefaultConfig(); err != nil {
		t.Fatalf("WriteDefaultConfig() error = %v", err)
	}

	// Verify file exists
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}

	// Verify permissions are 0600
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("config file permissions = %o, want 0600", perm)
	}

	// Read and verify content has expected structure
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	content := string(data)

	// Verify header comment
	if !strings.HasPrefix(content, "# Cloister global configuration") {
		t.Error("config file should start with header comment")
	}

	// Verify key sections are present
	expectedSections := []string{
		"# Proxy configuration",
		"proxy:",
		"listen: \":3128\"",
		"# Documentation sites",
		"domain: \"golang.org\"",
		"# Package registries",
		"domain: \"registry.npmjs.org\"",
		"# AI provider APIs",
		"domain: \"api.anthropic.com\"",
		"request:",
		"approval:",
		"devcontainer:",
		"agents:",
		"defaults:",
		"log:",
	}
	for _, section := range expectedSections {
		if !strings.Contains(content, section) {
			t.Errorf("config file should contain %q", section)
		}
	}

	// Verify the written config is valid YAML that can be parsed
	cfg, err := ParseGlobalConfig(data)
	if err != nil {
		t.Fatalf("ParseGlobalConfig() on written file error = %v", err)
	}

	// Verify some parsed values match defaults
	if cfg.Proxy.Listen != ":3128" {
		t.Errorf("cfg.Proxy.Listen = %q, want %q", cfg.Proxy.Listen, ":3128")
	}
	if cfg.Defaults.Agent != "claude" {
		t.Errorf("cfg.Defaults.Agent = %q, want %q", cfg.Defaults.Agent, "claude")
	}
}

func TestWriteDefaultConfig_DoesNotOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create config directory and write a custom config
	configDir := filepath.Join(tmpDir, "cloister")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	customContent := "# My custom config\nproxy:\n  listen: \":9999\"\n"
	path := GlobalConfigPath()
	if err := os.WriteFile(path, []byte(customContent), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	// Call WriteDefaultConfig - should not overwrite
	if err := WriteDefaultConfig(); err != nil {
		t.Fatalf("WriteDefaultConfig() error = %v", err)
	}

	// Verify content is unchanged
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(data) != customContent {
		t.Errorf("config file was overwritten, content = %q, want %q", string(data), customContent)
	}
}

func TestWriteDefaultConfig_CreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Verify config directory does not exist
	configDir := ConfigDir()
	if _, err := os.Stat(configDir); !os.IsNotExist(err) {
		t.Fatalf("config dir should not exist before test: %v", err)
	}

	// Write default config
	if err := WriteDefaultConfig(); err != nil {
		t.Fatalf("WriteDefaultConfig() error = %v", err)
	}

	// Verify config directory was created
	info, err := os.Stat(configDir)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Error("config dir should be a directory")
	}

	// Verify directory permissions are 0700
	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("config dir permissions = %o, want 0700", perm)
	}
}

func TestWriteProjectConfig_Creates(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	cfg := &ProjectConfig{
		Remote: "git@github.com:example/repo.git",
		Root:   "~/projects/repo",
		Refs:   []string{"~/docs/api-spec"},
	}

	// Write project config
	if err := WriteProjectConfig("my-project", cfg, false); err != nil {
		t.Fatalf("WriteProjectConfig() error = %v", err)
	}

	// Verify file exists
	path := ProjectsDir() + "my-project.yaml"
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}

	// Verify permissions are 0600
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("project config file permissions = %o, want 0600", perm)
	}

	// Read and verify content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	// Parse the written config
	parsedCfg, err := ParseProjectConfig(data)
	if err != nil {
		t.Fatalf("ParseProjectConfig() error = %v", err)
	}

	// Verify values
	if parsedCfg.Remote != cfg.Remote {
		t.Errorf("parsedCfg.Remote = %q, want %q", parsedCfg.Remote, cfg.Remote)
	}
	if parsedCfg.Root != cfg.Root {
		t.Errorf("parsedCfg.Root = %q, want %q", parsedCfg.Root, cfg.Root)
	}
	if len(parsedCfg.Refs) != 1 || parsedCfg.Refs[0] != cfg.Refs[0] {
		t.Errorf("parsedCfg.Refs = %v, want %v", parsedCfg.Refs, cfg.Refs)
	}
}

func TestWriteProjectConfig_DoesNotOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create projects directory and write a custom config
	projectsDir := ProjectsDir()
	if err := os.MkdirAll(projectsDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	customContent := "remote: \"original-remote\"\n"
	path := projectsDir + "test-project.yaml"
	if err := os.WriteFile(path, []byte(customContent), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	// Call WriteProjectConfig with overwrite=false
	newCfg := &ProjectConfig{Remote: "new-remote"}
	if err := WriteProjectConfig("test-project", newCfg, false); err != nil {
		t.Fatalf("WriteProjectConfig() error = %v", err)
	}

	// Verify content is unchanged
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(data) != customContent {
		t.Errorf("config file was overwritten, content = %q, want %q", string(data), customContent)
	}
}

func TestWriteProjectConfig_Overwrite(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create projects directory and write a custom config
	projectsDir := ProjectsDir()
	if err := os.MkdirAll(projectsDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	originalCfg := &ProjectConfig{Remote: "original-remote"}
	if err := WriteProjectConfig("test-project", originalCfg, false); err != nil {
		t.Fatalf("WriteProjectConfig() initial error = %v", err)
	}

	// Call WriteProjectConfig with overwrite=true
	newCfg := &ProjectConfig{Remote: "new-remote"}
	if err := WriteProjectConfig("test-project", newCfg, true); err != nil {
		t.Fatalf("WriteProjectConfig() overwrite error = %v", err)
	}

	// Verify content was updated
	path := projectsDir + "test-project.yaml"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	parsedCfg, err := ParseProjectConfig(data)
	if err != nil {
		t.Fatalf("ParseProjectConfig() error = %v", err)
	}

	if parsedCfg.Remote != "new-remote" {
		t.Errorf("parsedCfg.Remote = %q, want %q", parsedCfg.Remote, "new-remote")
	}
}

func TestWriteProjectConfig_CreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Verify projects directory does not exist
	projectsDir := ProjectsDir()
	if _, err := os.Stat(projectsDir); !os.IsNotExist(err) {
		t.Fatalf("projects dir should not exist before test: %v", err)
	}

	cfg := &ProjectConfig{Remote: "test-remote"}
	if err := WriteProjectConfig("test-project", cfg, false); err != nil {
		t.Fatalf("WriteProjectConfig() error = %v", err)
	}

	// Verify projects directory was created
	info, err := os.Stat(projectsDir)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Error("projects dir should be a directory")
	}

	// Verify directory permissions are 0700
	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("projects dir permissions = %o, want 0700", perm)
	}
}

func TestEnsureProjectsDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	projectsDir := ProjectsDir()

	// Directory should not exist yet
	if _, err := os.Stat(projectsDir); !os.IsNotExist(err) {
		t.Fatalf("projects dir should not exist before test: %v", err)
	}

	// Create it
	if err := EnsureProjectsDir(); err != nil {
		t.Fatalf("EnsureProjectsDir() error = %v", err)
	}

	// Verify it exists
	info, err := os.Stat(projectsDir)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Error("projects dir should be a directory")
	}

	// Verify permissions are 0700
	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("projects dir permissions = %o, want 0700", perm)
	}

	// Calling again should succeed (idempotent)
	if err := EnsureProjectsDir(); err != nil {
		t.Errorf("EnsureProjectsDir() second call error = %v", err)
	}
}

func TestInitProjectConfig_Creates(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// File should not exist yet
	path := ProjectConfigPath("test-project")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("config file should not exist before test: %v", err)
	}

	// Init project config
	if err := InitProjectConfig("test-project", "git@github.com:example/repo.git", "/home/user/projects/repo"); err != nil {
		t.Fatalf("InitProjectConfig() error = %v", err)
	}

	// Verify file exists
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}

	// Verify permissions are 0600
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("config file permissions = %o, want 0600", perm)
	}

	// Read and verify content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	// Parse the written config
	parsedCfg, err := ParseProjectConfig(data)
	if err != nil {
		t.Fatalf("ParseProjectConfig() error = %v", err)
	}

	// Verify values
	if parsedCfg.Remote != "git@github.com:example/repo.git" {
		t.Errorf("parsedCfg.Remote = %q, want %q", parsedCfg.Remote, "git@github.com:example/repo.git")
	}
	if parsedCfg.Root != "/home/user/projects/repo" {
		t.Errorf("parsedCfg.Root = %q, want %q", parsedCfg.Root, "/home/user/projects/repo")
	}
}

func TestInitProjectConfig_DoesNotOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create projects directory and write a custom config
	projectsDir := ProjectsDir()
	if err := os.MkdirAll(projectsDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	customContent := "remote: \"original-remote\"\nroot: \"/original/path\"\n"
	path := ProjectConfigPath("test-project")
	if err := os.WriteFile(path, []byte(customContent), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	// Call InitProjectConfig - should not overwrite
	if err := InitProjectConfig("test-project", "new-remote", "/new/path"); err != nil {
		t.Fatalf("InitProjectConfig() error = %v", err)
	}

	// Verify content is unchanged
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(data) != customContent {
		t.Errorf("config file was overwritten, content = %q, want %q", string(data), customContent)
	}
}

func TestInitProjectConfig_CreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Verify projects directory does not exist
	projectsDir := ProjectsDir()
	if _, err := os.Stat(projectsDir); !os.IsNotExist(err) {
		t.Fatalf("projects dir should not exist before test: %v", err)
	}

	// Init project config
	if err := InitProjectConfig("test-project", "test-remote", "/test/path"); err != nil {
		t.Fatalf("InitProjectConfig() error = %v", err)
	}

	// Verify projects directory was created
	info, err := os.Stat(projectsDir)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Error("projects dir should be a directory")
	}

	// Verify directory permissions are 0700
	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("projects dir permissions = %o, want 0700", perm)
	}
}
