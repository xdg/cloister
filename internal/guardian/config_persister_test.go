package guardian

import (
	"os"
	"testing"

	"github.com/xdg/cloister/internal/config"
)

func TestAddDomainToProject_WritesAndReloads(t *testing.T) {
	// Setup isolated config environment
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create a test project config
	projectName := "test-project"
	initialCfg := &config.ProjectConfig{
		Remote: "git@github.com:example/repo.git",
		Root:   "/test/path",
		Proxy: config.ProjectProxyConfig{
			Allow: []config.AllowEntry{
				{Domain: "example.com"},
			},
		},
	}
	if err := config.WriteProjectConfig(projectName, initialCfg, false); err != nil {
		t.Fatalf("WriteProjectConfig() error = %v", err)
	}

	// Track reload calls
	reloadCalled := false
	persister := &ConfigPersisterImpl{
		ReloadNotifier: func() {
			reloadCalled = true
		},
	}

	// Add new domain
	newDomain := "docs.example.com"
	if err := persister.AddDomainToProject(projectName, newDomain); err != nil {
		t.Fatalf("AddDomainToProject() error = %v", err)
	}

	// Verify reload notifier was called
	if !reloadCalled {
		t.Error("ReloadNotifier should have been called")
	}

	// Verify domain was added by reloading config
	cfg, err := config.LoadProjectConfig(projectName)
	if err != nil {
		t.Fatalf("LoadProjectConfig() error = %v", err)
	}

	// Check that both original and new domain are present
	found := map[string]bool{
		"example.com":      false,
		"docs.example.com": false,
	}
	for _, entry := range cfg.Proxy.Allow {
		if _, ok := found[entry.Domain]; ok {
			found[entry.Domain] = true
		}
	}

	for domain, present := range found {
		if !present {
			t.Errorf("domain %q should be present in allowlist", domain)
		}
	}

	// Verify total count
	if len(cfg.Proxy.Allow) != 2 {
		t.Errorf("cfg.Proxy.Allow length = %d, want 2", len(cfg.Proxy.Allow))
	}
}

func TestAddDomainToProject_NoDuplicate(t *testing.T) {
	// Setup isolated config environment
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create a test project config with a domain
	projectName := "test-project"
	existingDomain := "example.com"
	initialCfg := &config.ProjectConfig{
		Remote: "git@github.com:example/repo.git",
		Root:   "/test/path",
		Proxy: config.ProjectProxyConfig{
			Allow: []config.AllowEntry{
				{Domain: existingDomain},
			},
		},
	}
	if err := config.WriteProjectConfig(projectName, initialCfg, false); err != nil {
		t.Fatalf("WriteProjectConfig() error = %v", err)
	}

	// Track reload calls
	reloadCalled := false
	persister := &ConfigPersisterImpl{
		ReloadNotifier: func() {
			reloadCalled = true
		},
	}

	// Try to add the same domain again
	if err := persister.AddDomainToProject(projectName, existingDomain); err != nil {
		t.Fatalf("AddDomainToProject() error = %v", err)
	}

	// Reload notifier should NOT be called since no write occurred
	if reloadCalled {
		t.Error("ReloadNotifier should NOT have been called when domain already exists")
	}

	// Verify only one entry exists
	cfg, err := config.LoadProjectConfig(projectName)
	if err != nil {
		t.Fatalf("LoadProjectConfig() error = %v", err)
	}

	if len(cfg.Proxy.Allow) != 1 {
		t.Errorf("cfg.Proxy.Allow length = %d, want 1 (no duplicate)", len(cfg.Proxy.Allow))
	}

	if cfg.Proxy.Allow[0].Domain != existingDomain {
		t.Errorf("cfg.Proxy.Allow[0].Domain = %q, want %q", cfg.Proxy.Allow[0].Domain, existingDomain)
	}
}

func TestAddDomainToProject_NoReloadNotifier(t *testing.T) {
	// Setup isolated config environment
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create a test project config
	projectName := "test-project"
	initialCfg := &config.ProjectConfig{
		Remote: "git@github.com:example/repo.git",
		Root:   "/test/path",
	}
	if err := config.WriteProjectConfig(projectName, initialCfg, false); err != nil {
		t.Fatalf("WriteProjectConfig() error = %v", err)
	}

	// Persister with nil ReloadNotifier
	persister := &ConfigPersisterImpl{
		ReloadNotifier: nil,
	}

	// Should not panic
	if err := persister.AddDomainToProject(projectName, "example.com"); err != nil {
		t.Fatalf("AddDomainToProject() error = %v", err)
	}

	// Verify domain was added
	cfg, err := config.LoadProjectConfig(projectName)
	if err != nil {
		t.Fatalf("LoadProjectConfig() error = %v", err)
	}

	if len(cfg.Proxy.Allow) != 1 {
		t.Errorf("cfg.Proxy.Allow length = %d, want 1", len(cfg.Proxy.Allow))
	}
}

func TestAddDomainToProject_CreatesConfigIfMissing(t *testing.T) {
	// Setup isolated config environment
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Don't create initial config - LoadProjectConfig will return defaults
	projectName := "nonexistent-project"

	persister := &ConfigPersisterImpl{}

	// Add domain should succeed and create config
	newDomain := "example.com"
	if err := persister.AddDomainToProject(projectName, newDomain); err != nil {
		t.Fatalf("AddDomainToProject() error = %v", err)
	}

	// Verify config file was created
	configPath := config.ProjectConfigPath(projectName)
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config file should have been created: %v", err)
	}

	// Verify domain was added
	cfg, err := config.LoadProjectConfig(projectName)
	if err != nil {
		t.Fatalf("LoadProjectConfig() error = %v", err)
	}

	if len(cfg.Proxy.Allow) != 1 {
		t.Errorf("cfg.Proxy.Allow length = %d, want 1", len(cfg.Proxy.Allow))
	}

	if cfg.Proxy.Allow[0].Domain != newDomain {
		t.Errorf("cfg.Proxy.Allow[0].Domain = %q, want %q", cfg.Proxy.Allow[0].Domain, newDomain)
	}
}

func TestAddDomainToGlobal_WritesAndReloads(t *testing.T) {
	// Setup isolated config environment
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create a test global config
	initialCfg := &config.GlobalConfig{
		Proxy: config.ProxyConfig{
			Listen: ":3128",
			Allow: []config.AllowEntry{
				{Domain: "golang.org"},
			},
		},
	}
	if err := config.WriteGlobalConfig(initialCfg); err != nil {
		t.Fatalf("WriteGlobalConfig() error = %v", err)
	}

	// Track reload calls
	reloadCalled := false
	persister := &ConfigPersisterImpl{
		ReloadNotifier: func() {
			reloadCalled = true
		},
	}

	// Add new domain
	newDomain := "docs.golang.org"
	if err := persister.AddDomainToGlobal(newDomain); err != nil {
		t.Fatalf("AddDomainToGlobal() error = %v", err)
	}

	// Verify reload notifier was called
	if !reloadCalled {
		t.Error("ReloadNotifier should have been called")
	}

	// Verify domain was added by reloading config
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig() error = %v", err)
	}

	// Check that both original and new domain are present
	found := map[string]bool{
		"golang.org":      false,
		"docs.golang.org": false,
	}
	for _, entry := range cfg.Proxy.Allow {
		if _, ok := found[entry.Domain]; ok {
			found[entry.Domain] = true
		}
	}

	for domain, present := range found {
		if !present {
			t.Errorf("domain %q should be present in allowlist", domain)
		}
	}

	// Verify total count (at least 2, may include defaults from LoadGlobalConfig)
	if len(cfg.Proxy.Allow) < 2 {
		t.Errorf("cfg.Proxy.Allow length = %d, want at least 2", len(cfg.Proxy.Allow))
	}
}

func TestAddDomainToGlobal_NoDuplicate(t *testing.T) {
	// Setup isolated config environment
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create a test global config with a domain
	existingDomain := "golang.org"
	initialCfg := &config.GlobalConfig{
		Proxy: config.ProxyConfig{
			Listen: ":3128",
			Allow: []config.AllowEntry{
				{Domain: existingDomain},
			},
		},
	}
	if err := config.WriteGlobalConfig(initialCfg); err != nil {
		t.Fatalf("WriteGlobalConfig() error = %v", err)
	}

	// Track reload calls
	reloadCalled := false
	persister := &ConfigPersisterImpl{
		ReloadNotifier: func() {
			reloadCalled = true
		},
	}

	// Try to add the same domain again
	if err := persister.AddDomainToGlobal(existingDomain); err != nil {
		t.Fatalf("AddDomainToGlobal() error = %v", err)
	}

	// Reload notifier should NOT be called since no write occurred
	if reloadCalled {
		t.Error("ReloadNotifier should NOT have been called when domain already exists")
	}

	// Verify only one entry exists
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig() error = %v", err)
	}

	if len(cfg.Proxy.Allow) != 1 {
		t.Errorf("cfg.Proxy.Allow length = %d, want 1 (no duplicate)", len(cfg.Proxy.Allow))
	}

	if cfg.Proxy.Allow[0].Domain != existingDomain {
		t.Errorf("cfg.Proxy.Allow[0].Domain = %q, want %q", cfg.Proxy.Allow[0].Domain, existingDomain)
	}
}

func TestAddDomainToGlobal_NoReloadNotifier(t *testing.T) {
	// Setup isolated config environment
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create a test global config
	initialCfg := &config.GlobalConfig{
		Proxy: config.ProxyConfig{
			Listen: ":3128",
		},
	}
	if err := config.WriteGlobalConfig(initialCfg); err != nil {
		t.Fatalf("WriteGlobalConfig() error = %v", err)
	}

	// Persister with nil ReloadNotifier
	persister := &ConfigPersisterImpl{
		ReloadNotifier: nil,
	}

	// Should not panic
	if err := persister.AddDomainToGlobal("example.com"); err != nil {
		t.Fatalf("AddDomainToGlobal() error = %v", err)
	}

	// Verify domain was added
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig() error = %v", err)
	}

	// Should have at least 1 entry
	if len(cfg.Proxy.Allow) < 1 {
		t.Errorf("cfg.Proxy.Allow length = %d, want at least 1", len(cfg.Proxy.Allow))
	}

	// Verify new domain is present
	found := false
	for _, entry := range cfg.Proxy.Allow {
		if entry.Domain == "example.com" {
			found = true
			break
		}
	}
	if !found {
		t.Error("example.com should be present in allowlist")
	}
}

func TestAddDomainToGlobal_CreatesConfigIfMissing(t *testing.T) {
	// Setup isolated config environment
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Don't create initial config - LoadGlobalConfig will create defaults
	persister := &ConfigPersisterImpl{}

	// Add domain should succeed
	newDomain := "example.com"
	if err := persister.AddDomainToGlobal(newDomain); err != nil {
		t.Fatalf("AddDomainToGlobal() error = %v", err)
	}

	// Verify config file was created
	configPath := config.GlobalConfigPath()
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config file should have been created: %v", err)
	}

	// Verify domain was added
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig() error = %v", err)
	}

	// Should have at least 1 entry
	if len(cfg.Proxy.Allow) < 1 {
		t.Errorf("cfg.Proxy.Allow length = %d, want at least 1", len(cfg.Proxy.Allow))
	}

	// Verify new domain is present
	found := false
	for _, entry := range cfg.Proxy.Allow {
		if entry.Domain == newDomain {
			found = true
			break
		}
	}
	if !found {
		t.Error("example.com should be present in allowlist")
	}
}

func TestAddDomainToProject_MultipleAdditions(t *testing.T) {
	// Setup isolated config environment
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create a test project config
	projectName := "test-project"
	initialCfg := &config.ProjectConfig{
		Remote: "git@github.com:example/repo.git",
		Root:   "/test/path",
	}
	if err := config.WriteProjectConfig(projectName, initialCfg, false); err != nil {
		t.Fatalf("WriteProjectConfig() error = %v", err)
	}

	persister := &ConfigPersisterImpl{}

	// Add multiple domains
	domains := []string{"api.example.com", "docs.example.com", "cdn.example.com"}
	for _, domain := range domains {
		if err := persister.AddDomainToProject(projectName, domain); err != nil {
			t.Fatalf("AddDomainToProject(%q) error = %v", domain, err)
		}
	}

	// Verify all domains were added
	cfg, err := config.LoadProjectConfig(projectName)
	if err != nil {
		t.Fatalf("LoadProjectConfig() error = %v", err)
	}

	if len(cfg.Proxy.Allow) != len(domains) {
		t.Errorf("cfg.Proxy.Allow length = %d, want %d", len(cfg.Proxy.Allow), len(domains))
	}

	// Verify each domain is present
	foundDomains := make(map[string]bool)
	for _, entry := range cfg.Proxy.Allow {
		foundDomains[entry.Domain] = true
	}

	for _, domain := range domains {
		if !foundDomains[domain] {
			t.Errorf("domain %q should be present in allowlist", domain)
		}
	}
}

// Tests for pattern persistence

func TestAddPatternToProject_WritesAndReloads(t *testing.T) {
	// Setup isolated config environment
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create a test project config
	projectName := "test-project"
	initialCfg := &config.ProjectConfig{
		Remote: "git@github.com:example/repo.git",
		Root:   "/test/path",
		Proxy: config.ProjectProxyConfig{
			Allow: []config.AllowEntry{
				{Domain: "example.com"},
			},
		},
	}
	if err := config.WriteProjectConfig(projectName, initialCfg, false); err != nil {
		t.Fatalf("WriteProjectConfig() error = %v", err)
	}

	// Track reload calls
	reloadCalled := false
	persister := &ConfigPersisterImpl{
		ReloadNotifier: func() {
			reloadCalled = true
		},
	}

	// Add new pattern
	newPattern := "*.example.com"
	if err := persister.AddPatternToProject(projectName, newPattern); err != nil {
		t.Fatalf("AddPatternToProject() error = %v", err)
	}

	// Verify reload notifier was called
	if !reloadCalled {
		t.Error("ReloadNotifier should have been called")
	}

	// Verify pattern was added by reloading config
	cfg, err := config.LoadProjectConfig(projectName)
	if err != nil {
		t.Fatalf("LoadProjectConfig() error = %v", err)
	}

	// Check that domain and pattern are both present
	foundDomain := false
	foundPattern := false
	for _, entry := range cfg.Proxy.Allow {
		if entry.Domain == "example.com" {
			foundDomain = true
		}
		if entry.Pattern == "*.example.com" {
			foundPattern = true
		}
	}

	if !foundDomain {
		t.Error("domain 'example.com' should be present in allowlist")
	}
	if !foundPattern {
		t.Error("pattern '*.example.com' should be present in allowlist")
	}

	// Verify total count
	if len(cfg.Proxy.Allow) != 2 {
		t.Errorf("cfg.Proxy.Allow length = %d, want 2", len(cfg.Proxy.Allow))
	}
}

func TestAddPatternToProject_NoDuplicate(t *testing.T) {
	// Setup isolated config environment
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create a test project config with a pattern
	projectName := "test-project"
	existingPattern := "*.example.com"
	initialCfg := &config.ProjectConfig{
		Remote: "git@github.com:example/repo.git",
		Root:   "/test/path",
		Proxy: config.ProjectProxyConfig{
			Allow: []config.AllowEntry{
				{Pattern: existingPattern},
			},
		},
	}
	if err := config.WriteProjectConfig(projectName, initialCfg, false); err != nil {
		t.Fatalf("WriteProjectConfig() error = %v", err)
	}

	// Track reload calls
	reloadCalled := false
	persister := &ConfigPersisterImpl{
		ReloadNotifier: func() {
			reloadCalled = true
		},
	}

	// Try to add the same pattern again
	if err := persister.AddPatternToProject(projectName, existingPattern); err != nil {
		t.Fatalf("AddPatternToProject() error = %v", err)
	}

	// Reload notifier should NOT be called since no write occurred
	if reloadCalled {
		t.Error("ReloadNotifier should NOT have been called when pattern already exists")
	}

	// Verify only one entry exists
	cfg, err := config.LoadProjectConfig(projectName)
	if err != nil {
		t.Fatalf("LoadProjectConfig() error = %v", err)
	}

	if len(cfg.Proxy.Allow) != 1 {
		t.Errorf("cfg.Proxy.Allow length = %d, want 1 (no duplicate)", len(cfg.Proxy.Allow))
	}

	if cfg.Proxy.Allow[0].Pattern != existingPattern {
		t.Errorf("cfg.Proxy.Allow[0].Pattern = %q, want %q", cfg.Proxy.Allow[0].Pattern, existingPattern)
	}
}

func TestAddPatternToProject_InvalidPattern(t *testing.T) {
	// Setup isolated config environment
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	persister := &ConfigPersisterImpl{}

	tests := []struct {
		name    string
		pattern string
	}{
		{"empty pattern", ""},
		{"missing asterisk prefix", "example.com"},
		{"whitespace", "*.example .com"},
		{"too short", "*."},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := persister.AddPatternToProject("test-project", tc.pattern)
			if err == nil {
				t.Errorf("AddPatternToProject(%q) should return error", tc.pattern)
			}
		})
	}
}

func TestAddPatternToGlobal_WritesAndReloads(t *testing.T) {
	// Setup isolated config environment
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create a test global config
	initialCfg := &config.GlobalConfig{
		Proxy: config.ProxyConfig{
			Listen: ":3128",
			Allow: []config.AllowEntry{
				{Domain: "golang.org"},
			},
		},
	}
	if err := config.WriteGlobalConfig(initialCfg); err != nil {
		t.Fatalf("WriteGlobalConfig() error = %v", err)
	}

	// Track reload calls
	reloadCalled := false
	persister := &ConfigPersisterImpl{
		ReloadNotifier: func() {
			reloadCalled = true
		},
	}

	// Add new pattern
	newPattern := "*.googleapis.com"
	if err := persister.AddPatternToGlobal(newPattern); err != nil {
		t.Fatalf("AddPatternToGlobal() error = %v", err)
	}

	// Verify reload notifier was called
	if !reloadCalled {
		t.Error("ReloadNotifier should have been called")
	}

	// Verify pattern was added by reloading config
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig() error = %v", err)
	}

	// Check that domain and pattern are both present
	foundDomain := false
	foundPattern := false
	for _, entry := range cfg.Proxy.Allow {
		if entry.Domain == "golang.org" {
			foundDomain = true
		}
		if entry.Pattern == "*.googleapis.com" {
			foundPattern = true
		}
	}

	if !foundDomain {
		t.Error("domain 'golang.org' should be present in allowlist")
	}
	if !foundPattern {
		t.Error("pattern '*.googleapis.com' should be present in allowlist")
	}
}

func TestAddPatternToGlobal_NoDuplicate(t *testing.T) {
	// Setup isolated config environment
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create a test global config with a pattern
	existingPattern := "*.googleapis.com"
	initialCfg := &config.GlobalConfig{
		Proxy: config.ProxyConfig{
			Listen: ":3128",
			Allow: []config.AllowEntry{
				{Pattern: existingPattern},
			},
		},
	}
	if err := config.WriteGlobalConfig(initialCfg); err != nil {
		t.Fatalf("WriteGlobalConfig() error = %v", err)
	}

	// Track reload calls
	reloadCalled := false
	persister := &ConfigPersisterImpl{
		ReloadNotifier: func() {
			reloadCalled = true
		},
	}

	// Try to add the same pattern again
	if err := persister.AddPatternToGlobal(existingPattern); err != nil {
		t.Fatalf("AddPatternToGlobal() error = %v", err)
	}

	// Reload notifier should NOT be called since no write occurred
	if reloadCalled {
		t.Error("ReloadNotifier should NOT have been called when pattern already exists")
	}

	// Verify only one entry exists
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig() error = %v", err)
	}

	if len(cfg.Proxy.Allow) != 1 {
		t.Errorf("cfg.Proxy.Allow length = %d, want 1 (no duplicate)", len(cfg.Proxy.Allow))
	}

	if cfg.Proxy.Allow[0].Pattern != existingPattern {
		t.Errorf("cfg.Proxy.Allow[0].Pattern = %q, want %q", cfg.Proxy.Allow[0].Pattern, existingPattern)
	}
}
