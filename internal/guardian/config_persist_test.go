package guardian

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xdg/cloister/internal/config"
)

func TestFileConfigPersister_AddToProjectAllowlist(t *testing.T) {
	// Create temp directory for config files
	tmpDir := t.TempDir()
	oldConfigDir := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer os.Setenv("XDG_CONFIG_HOME", oldConfigDir)

	// Ensure projects directory exists
	projectsDir := filepath.Join(tmpDir, "cloister", "projects")
	if err := os.MkdirAll(projectsDir, 0700); err != nil {
		t.Fatalf("failed to create projects dir: %v", err)
	}

	persister := NewFileConfigPersister()

	t.Run("adds domain to new project config", func(t *testing.T) {
		projectName := "test-project-1"

		err := persister.AddToProjectAllowlist(projectName, "example.com")
		if err != nil {
			t.Fatalf("AddToProjectAllowlist() error = %v", err)
		}

		// Load and verify
		cfg, err := config.LoadProjectConfig(projectName)
		if err != nil {
			t.Fatalf("failed to load project config: %v", err)
		}

		found := false
		for _, entry := range cfg.Proxy.Allow {
			if entry.Domain == "example.com" {
				found = true
				break
			}
		}
		if !found {
			t.Error("domain not found in project allowlist")
		}
	})

	t.Run("adds domain to existing project config", func(t *testing.T) {
		projectName := "test-project-2"

		// Create initial config with one domain
		initialCfg := &config.ProjectConfig{
			Remote: "git@example.com:user/repo.git",
			Root:   "/path/to/project",
			Proxy: config.ProjectProxyConfig{
				Allow: []config.AllowEntry{
					{Domain: "existing.com"},
				},
			},
		}
		if err := config.WriteProjectConfig(projectName, initialCfg, true); err != nil {
			t.Fatalf("failed to write initial config: %v", err)
		}

		// Add new domain
		err := persister.AddToProjectAllowlist(projectName, "new-domain.com")
		if err != nil {
			t.Fatalf("AddToProjectAllowlist() error = %v", err)
		}

		// Load and verify both domains exist
		cfg, err := config.LoadProjectConfig(projectName)
		if err != nil {
			t.Fatalf("failed to load project config: %v", err)
		}

		if len(cfg.Proxy.Allow) != 2 {
			t.Errorf("expected 2 domains, got %d", len(cfg.Proxy.Allow))
		}

		domains := make(map[string]bool)
		for _, entry := range cfg.Proxy.Allow {
			domains[entry.Domain] = true
		}

		if !domains["existing.com"] {
			t.Error("existing domain should still be present")
		}
		if !domains["new-domain.com"] {
			t.Error("new domain should be present")
		}

		// Verify other config fields preserved
		if cfg.Remote != "git@example.com:user/repo.git" {
			t.Errorf("Remote = %q, want %q", cfg.Remote, "git@example.com:user/repo.git")
		}
	})

	t.Run("adding duplicate domain is idempotent", func(t *testing.T) {
		projectName := "test-project-3"

		// Add same domain twice
		err := persister.AddToProjectAllowlist(projectName, "duplicate.com")
		if err != nil {
			t.Fatalf("first AddToProjectAllowlist() error = %v", err)
		}

		err = persister.AddToProjectAllowlist(projectName, "duplicate.com")
		if err != nil {
			t.Fatalf("second AddToProjectAllowlist() error = %v", err)
		}

		// Load and verify only one entry
		cfg, err := config.LoadProjectConfig(projectName)
		if err != nil {
			t.Fatalf("failed to load project config: %v", err)
		}

		count := 0
		for _, entry := range cfg.Proxy.Allow {
			if entry.Domain == "duplicate.com" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("expected 1 entry for duplicate.com, got %d", count)
		}
	})
}

func TestFileConfigPersister_AddToGlobalAllowlist(t *testing.T) {
	// Create temp directory for config files
	tmpDir := t.TempDir()
	oldConfigDir := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer os.Setenv("XDG_CONFIG_HOME", oldConfigDir)

	// Ensure cloister config directory exists
	configDir := filepath.Join(tmpDir, "cloister")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	persister := NewFileConfigPersister()

	t.Run("adds domain to new global config", func(t *testing.T) {
		err := persister.AddToGlobalAllowlist("global-domain.com")
		if err != nil {
			t.Fatalf("AddToGlobalAllowlist() error = %v", err)
		}

		// Load and verify
		cfg, err := config.LoadGlobalConfig()
		if err != nil {
			t.Fatalf("failed to load global config: %v", err)
		}

		found := false
		for _, entry := range cfg.Proxy.Allow {
			if entry.Domain == "global-domain.com" {
				found = true
				break
			}
		}
		if !found {
			t.Error("domain not found in global allowlist")
		}
	})

	t.Run("adds domain to existing global config", func(t *testing.T) {
		// Ensure config already exists with a domain
		cfg, _ := config.LoadGlobalConfig()
		if cfg == nil {
			cfg = &config.GlobalConfig{}
		}
		cfg.Proxy.Allow = append(cfg.Proxy.Allow, config.AllowEntry{Domain: "existing-global.com"})
		cfg.Proxy.Listen = ":3128"
		if err := config.WriteGlobalConfig(cfg); err != nil {
			t.Fatalf("failed to write initial config: %v", err)
		}

		// Add new domain
		err := persister.AddToGlobalAllowlist("another-global.com")
		if err != nil {
			t.Fatalf("AddToGlobalAllowlist() error = %v", err)
		}

		// Load and verify
		cfg, err = config.LoadGlobalConfig()
		if err != nil {
			t.Fatalf("failed to load global config: %v", err)
		}

		domains := make(map[string]bool)
		for _, entry := range cfg.Proxy.Allow {
			domains[entry.Domain] = true
		}

		if !domains["existing-global.com"] {
			t.Error("existing domain should still be present")
		}
		if !domains["another-global.com"] {
			t.Error("new domain should be present")
		}

		// Verify other config fields preserved
		if cfg.Proxy.Listen != ":3128" {
			t.Errorf("Listen = %q, want %q", cfg.Proxy.Listen, ":3128")
		}
	})

	t.Run("adding duplicate domain is idempotent", func(t *testing.T) {
		initialLen := 0
		cfg, _ := config.LoadGlobalConfig()
		if cfg != nil {
			initialLen = len(cfg.Proxy.Allow)
		}

		// Add same domain twice
		_ = persister.AddToGlobalAllowlist("dup-global.com")
		_ = persister.AddToGlobalAllowlist("dup-global.com")

		// Load and verify only one extra entry
		cfg, err := config.LoadGlobalConfig()
		if err != nil {
			t.Fatalf("failed to load global config: %v", err)
		}

		expectedLen := initialLen + 1
		if len(cfg.Proxy.Allow) != expectedLen {
			t.Errorf("expected %d domains (initial %d + 1), got %d",
				expectedLen, initialLen, len(cfg.Proxy.Allow))
		}
	})
}

func TestFileConfigPersister_Implements_ConfigPersister(t *testing.T) {
	// Compile-time check that FileConfigPersister implements ConfigPersister
	var _ ConfigPersister = (*FileConfigPersister)(nil)
}
