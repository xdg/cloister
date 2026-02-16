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
	t.Setenv("XDG_STATE_HOME", t.TempDir())

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
	if perm != 0o600 {
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
		"hostexec:",
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
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create config directory and write a custom config
	configDir := filepath.Join(tmpDir, "cloister")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	customContent := "# My custom config\nproxy:\n  listen: \":9999\"\n"
	path := GlobalConfigPath()
	if err := os.WriteFile(path, []byte(customContent), 0o600); err != nil {
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
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Verify config directory does not exist
	configDir := Dir()
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
	if perm != 0o700 {
		t.Errorf("config dir permissions = %o, want 0700", perm)
	}
}

func TestWriteProjectConfig_Creates(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

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
	if perm != 0o600 {
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
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create projects directory and write a custom config
	projectsDir := ProjectsDir()
	if err := os.MkdirAll(projectsDir, 0o700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	customContent := "remote: \"original-remote\"\n"
	path := projectsDir + "test-project.yaml"
	if err := os.WriteFile(path, []byte(customContent), 0o600); err != nil {
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
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create projects directory and write a custom config
	projectsDir := ProjectsDir()
	if err := os.MkdirAll(projectsDir, 0o700); err != nil {
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
	t.Setenv("XDG_STATE_HOME", t.TempDir())

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
	if perm != 0o700 {
		t.Errorf("projects dir permissions = %o, want 0700", perm)
	}
}

func TestEnsureProjectsDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

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
	if perm != 0o700 {
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
	t.Setenv("XDG_STATE_HOME", t.TempDir())

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
	if perm != 0o600 {
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
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create projects directory and write a custom config
	projectsDir := ProjectsDir()
	if err := os.MkdirAll(projectsDir, 0o700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	customContent := "remote: \"original-remote\"\nroot: \"/original/path\"\n"
	path := ProjectConfigPath("test-project")
	if err := os.WriteFile(path, []byte(customContent), 0o600); err != nil {
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
	t.Setenv("XDG_STATE_HOME", t.TempDir())

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
	if perm != 0o700 {
		t.Errorf("projects dir permissions = %o, want 0700", perm)
	}
}

func TestWriteGlobalConfig_Creates(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	skipPerms := true
	cfg := &GlobalConfig{
		Proxy: ProxyConfig{
			Listen: ":9999",
		},
		Agents: map[string]AgentConfig{
			"claude": {
				AuthMethod: "token",
				Token:      "test-token-value",
				SkipPerms:  &skipPerms,
			},
		},
	}

	// Write global config
	if err := WriteGlobalConfig(cfg); err != nil {
		t.Fatalf("WriteGlobalConfig() error = %v", err)
	}

	// Verify file exists
	path := GlobalConfigPath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}

	// Verify permissions are 0600
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("config file permissions = %o, want 0600", perm)
	}

	// Read and verify content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	// Parse the written config
	parsedCfg, err := ParseGlobalConfig(data)
	if err != nil {
		t.Fatalf("ParseGlobalConfig() error = %v", err)
	}

	// Verify values
	if parsedCfg.Proxy.Listen != ":9999" {
		t.Errorf("parsedCfg.Proxy.Listen = %q, want %q", parsedCfg.Proxy.Listen, ":9999")
	}
	claudeCfg, ok := parsedCfg.Agents["claude"]
	if !ok {
		t.Fatal("parsedCfg.Agents should have claude entry")
	}
	if claudeCfg.AuthMethod != "token" {
		t.Errorf("claudeCfg.AuthMethod = %q, want %q", claudeCfg.AuthMethod, "token")
	}
	if claudeCfg.Token != "test-token-value" {
		t.Errorf("claudeCfg.Token = %q, want %q", claudeCfg.Token, "test-token-value")
	}
	if claudeCfg.SkipPerms == nil || !*claudeCfg.SkipPerms {
		t.Error("claudeCfg.SkipPerms should be true")
	}
}

func TestWriteGlobalConfig_Overwrites(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create config directory and write initial config
	configDir := filepath.Join(tmpDir, "cloister")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	initialContent := "proxy:\n  listen: \":1111\"\n"
	path := GlobalConfigPath()
	if err := os.WriteFile(path, []byte(initialContent), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	// Write new config
	cfg := &GlobalConfig{
		Proxy: ProxyConfig{
			Listen: ":2222",
		},
	}
	if err := WriteGlobalConfig(cfg); err != nil {
		t.Fatalf("WriteGlobalConfig() error = %v", err)
	}

	// Verify content was updated
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	parsedCfg, err := ParseGlobalConfig(data)
	if err != nil {
		t.Fatalf("ParseGlobalConfig() error = %v", err)
	}

	if parsedCfg.Proxy.Listen != ":2222" {
		t.Errorf("parsedCfg.Proxy.Listen = %q, want %q", parsedCfg.Proxy.Listen, ":2222")
	}
}

// TestDefaultConfigTemplateMatchesDefaults ensures the hard-coded YAML template
// in defaultConfigTemplate stays in sync with DefaultGlobalConfig(). If this
// test fails, one of them was updated without the other.
func TestDefaultConfigTemplateMatchesDefaults(t *testing.T) {
	parsed, err := ParseGlobalConfig([]byte(defaultConfigTemplate))
	if err != nil {
		t.Fatalf("ParseGlobalConfig(defaultConfigTemplate) error = %v", err)
	}
	expected := DefaultGlobalConfig()

	// --- Proxy ---
	if parsed.Proxy.Listen != expected.Proxy.Listen {
		t.Errorf("Proxy.Listen: template=%q, defaults=%q", parsed.Proxy.Listen, expected.Proxy.Listen)
	}
	if parsed.Proxy.UnlistedDomainBehavior != expected.Proxy.UnlistedDomainBehavior {
		t.Errorf("Proxy.UnlistedDomainBehavior: template=%q, defaults=%q",
			parsed.Proxy.UnlistedDomainBehavior, expected.Proxy.UnlistedDomainBehavior)
	}
	if parsed.Proxy.ApprovalTimeout != expected.Proxy.ApprovalTimeout {
		t.Errorf("Proxy.ApprovalTimeout: template=%q, defaults=%q",
			parsed.Proxy.ApprovalTimeout, expected.Proxy.ApprovalTimeout)
	}
	if parsed.Proxy.RateLimit != expected.Proxy.RateLimit {
		t.Errorf("Proxy.RateLimit: template=%d, defaults=%d", parsed.Proxy.RateLimit, expected.Proxy.RateLimit)
	}
	if parsed.Proxy.MaxRequestBytes != expected.Proxy.MaxRequestBytes {
		t.Errorf("Proxy.MaxRequestBytes: template=%d, defaults=%d",
			parsed.Proxy.MaxRequestBytes, expected.Proxy.MaxRequestBytes)
	}

	// Compare allow domains as sets
	templateDomains := make(map[string]bool)
	for _, e := range parsed.Proxy.Allow {
		templateDomains[e.Domain] = true
	}
	defaultDomains := make(map[string]bool)
	for _, e := range expected.Proxy.Allow {
		defaultDomains[e.Domain] = true
	}
	for d := range defaultDomains {
		if !templateDomains[d] {
			t.Errorf("domain %q in DefaultGlobalConfig but missing from template", d)
		}
	}
	for d := range templateDomains {
		if !defaultDomains[d] {
			t.Errorf("domain %q in template but missing from DefaultGlobalConfig", d)
		}
	}

	// --- Request ---
	if parsed.Request.Listen != expected.Request.Listen {
		t.Errorf("Request.Listen: template=%q, defaults=%q", parsed.Request.Listen, expected.Request.Listen)
	}
	if parsed.Request.Timeout != expected.Request.Timeout {
		t.Errorf("Request.Timeout: template=%q, defaults=%q", parsed.Request.Timeout, expected.Request.Timeout)
	}

	// --- Hostexec ---
	if parsed.Hostexec.Listen != expected.Hostexec.Listen {
		t.Errorf("Hostexec.Listen: template=%q, defaults=%q", parsed.Hostexec.Listen, expected.Hostexec.Listen)
	}

	templatePatterns := make(map[string]bool)
	for _, p := range parsed.Hostexec.AutoApprove {
		templatePatterns[p.Pattern] = true
	}
	defaultPatterns := make(map[string]bool)
	for _, p := range expected.Hostexec.AutoApprove {
		defaultPatterns[p.Pattern] = true
	}
	for p := range defaultPatterns {
		if !templatePatterns[p] {
			t.Errorf("auto_approve pattern %q in DefaultGlobalConfig but missing from template", p)
		}
	}
	for p := range templatePatterns {
		if !defaultPatterns[p] {
			t.Errorf("auto_approve pattern %q in template but missing from DefaultGlobalConfig", p)
		}
	}

	templatePatterns = make(map[string]bool)
	for _, p := range parsed.Hostexec.ManualApprove {
		templatePatterns[p.Pattern] = true
	}
	defaultPatterns = make(map[string]bool)
	for _, p := range expected.Hostexec.ManualApprove {
		defaultPatterns[p.Pattern] = true
	}
	for p := range defaultPatterns {
		if !templatePatterns[p] {
			t.Errorf("manual_approve pattern %q in DefaultGlobalConfig but missing from template", p)
		}
	}
	for p := range templatePatterns {
		if !defaultPatterns[p] {
			t.Errorf("manual_approve pattern %q in template but missing from DefaultGlobalConfig", p)
		}
	}

	// --- Agents ---
	for name, expectedAgent := range expected.Agents {
		parsedAgent, ok := parsed.Agents[name]
		if !ok {
			t.Errorf("agent %q in DefaultGlobalConfig but missing from template", name)
			continue
		}
		if parsedAgent.Command != expectedAgent.Command {
			t.Errorf("agent %q Command: template=%q, defaults=%q", name, parsedAgent.Command, expectedAgent.Command)
		}
		if len(parsedAgent.Env) != len(expectedAgent.Env) {
			t.Errorf("agent %q Env count: template=%d, defaults=%d", name, len(parsedAgent.Env), len(expectedAgent.Env))
		} else {
			for i, env := range expectedAgent.Env {
				if parsedAgent.Env[i] != env {
					t.Errorf("agent %q Env[%d]: template=%q, defaults=%q", name, i, parsedAgent.Env[i], env)
				}
			}
		}
		// SkipPerms
		switch {
		case expectedAgent.SkipPerms == nil && parsedAgent.SkipPerms == nil:
			// ok
		case expectedAgent.SkipPerms == nil || parsedAgent.SkipPerms == nil:
			t.Errorf("agent %q SkipPerms: template=%v, defaults=%v", name, parsedAgent.SkipPerms, expectedAgent.SkipPerms)
		case *expectedAgent.SkipPerms != *parsedAgent.SkipPerms:
			t.Errorf("agent %q SkipPerms: template=%v, defaults=%v", name, *parsedAgent.SkipPerms, *expectedAgent.SkipPerms)
		}
	}
	for name := range parsed.Agents {
		if _, ok := expected.Agents[name]; !ok {
			t.Errorf("agent %q in template but missing from DefaultGlobalConfig", name)
		}
	}

	// --- Defaults ---
	if parsed.Defaults.Image != expected.Defaults.Image {
		t.Errorf("Defaults.Image: template=%q, defaults=%q", parsed.Defaults.Image, expected.Defaults.Image)
	}
	if parsed.Defaults.Shell != expected.Defaults.Shell {
		t.Errorf("Defaults.Shell: template=%q, defaults=%q", parsed.Defaults.Shell, expected.Defaults.Shell)
	}
	if parsed.Defaults.User != expected.Defaults.User {
		t.Errorf("Defaults.User: template=%q, defaults=%q", parsed.Defaults.User, expected.Defaults.User)
	}
	if parsed.Defaults.Agent != expected.Defaults.Agent {
		t.Errorf("Defaults.Agent: template=%q, defaults=%q", parsed.Defaults.Agent, expected.Defaults.Agent)
	}

	// --- Log ---
	if parsed.Log.File != expected.Log.File {
		t.Errorf("Log.File: template=%q, defaults=%q", parsed.Log.File, expected.Log.File)
	}
	if parsed.Log.Stdout != expected.Log.Stdout {
		t.Errorf("Log.Stdout: template=%v, defaults=%v", parsed.Log.Stdout, expected.Log.Stdout)
	}
	if parsed.Log.Level != expected.Log.Level {
		t.Errorf("Log.Level: template=%q, defaults=%q", parsed.Log.Level, expected.Log.Level)
	}
	if parsed.Log.PerCloister != expected.Log.PerCloister {
		t.Errorf("Log.PerCloister: template=%v, defaults=%v", parsed.Log.PerCloister, expected.Log.PerCloister)
	}
	if parsed.Log.PerCloisterDir != expected.Log.PerCloisterDir {
		t.Errorf("Log.PerCloisterDir: template=%q, defaults=%q",
			parsed.Log.PerCloisterDir, expected.Log.PerCloisterDir)
	}

	// --- Devcontainer (should both be zero value) ---
	if parsed.Devcontainer.Enabled != expected.Devcontainer.Enabled {
		t.Errorf("Devcontainer.Enabled: template=%v, defaults=%v",
			parsed.Devcontainer.Enabled, expected.Devcontainer.Enabled)
	}
	if len(parsed.Devcontainer.Features.Allow) != len(expected.Devcontainer.Features.Allow) {
		t.Errorf("Devcontainer.Features.Allow: template has %d, defaults has %d",
			len(parsed.Devcontainer.Features.Allow), len(expected.Devcontainer.Features.Allow))
	}
	if len(parsed.Devcontainer.BlockedMounts) != len(expected.Devcontainer.BlockedMounts) {
		t.Errorf("Devcontainer.BlockedMounts: template has %d, defaults has %d",
			len(parsed.Devcontainer.BlockedMounts), len(expected.Devcontainer.BlockedMounts))
	}
}

func TestWriteGlobalConfig_CreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Verify config directory does not exist
	configDir := Dir()
	if _, err := os.Stat(configDir); !os.IsNotExist(err) {
		t.Fatalf("config dir should not exist before test: %v", err)
	}

	cfg := &GlobalConfig{
		Proxy: ProxyConfig{
			Listen: ":3333",
		},
	}
	if err := WriteGlobalConfig(cfg); err != nil {
		t.Fatalf("WriteGlobalConfig() error = %v", err)
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
	if perm != 0o700 {
		t.Errorf("config dir permissions = %o, want 0700", perm)
	}
}
