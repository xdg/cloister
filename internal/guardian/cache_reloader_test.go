package guardian

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/token"
)

// mockProjectLister is a test double for ProjectLister.
type mockProjectLister struct {
	projects map[string]token.Info
}

func (m *mockProjectLister) List() map[string]token.Info {
	return m.projects
}

func TestNewCacheReloader_StoresAndReturnsGlobalDecisions(t *testing.T) {
	cache := NewAllowlistCache(NewAllowlist(nil))
	lister := &mockProjectLister{projects: map[string]token.Info{}}

	decisions := &config.Decisions{
		Proxy: config.DecisionsProxy{
			Allow: []config.AllowEntry{{Domain: "example.com"}},
			Deny:  []config.AllowEntry{{Domain: "evil.com"}},
		},
	}

	staticAllow := []config.AllowEntry{{Domain: "static-allow.com"}}
	staticDeny := []config.AllowEntry{{Domain: "static-deny.com"}}

	reloader := NewCacheReloader(cache, lister, staticAllow, staticDeny, decisions)

	got := reloader.GlobalDecisions()
	if got != decisions {
		t.Fatalf("GlobalDecisions() returned different pointer: got %p, want %p", got, decisions)
	}
	if len(got.Proxy.Allow) != 1 || got.Proxy.Allow[0].Domain != "example.com" {
		t.Errorf("GlobalDecisions().Proxy.Allow = %v, want [{Domain:example.com}]", got.Proxy.Allow)
	}
	if len(got.Proxy.Deny) != 1 || got.Proxy.Deny[0].Domain != "evil.com" {
		t.Errorf("GlobalDecisions().Proxy.Deny = %v, want [{Domain:evil.com}]", got.Proxy.Deny)
	}
}

func TestNewCacheReloader_NilDecisionsDefaultsToEmpty(t *testing.T) {
	cache := NewAllowlistCache(NewAllowlist(nil))
	lister := &mockProjectLister{projects: map[string]token.Info{}}

	reloader := NewCacheReloader(cache, lister, nil, nil, nil)

	got := reloader.GlobalDecisions()
	if got == nil {
		t.Fatal("GlobalDecisions() returned nil, want empty Decisions")
	}
	if len(got.Proxy.Allow) != 0 {
		t.Errorf("GlobalDecisions().Proxy.Allow = %v, want empty", got.Proxy.Allow)
	}
	if len(got.Proxy.Deny) != 0 {
		t.Errorf("GlobalDecisions().Proxy.Deny = %v, want empty", got.Proxy.Deny)
	}
}

func TestCacheReloader_SetStaticConfig(t *testing.T) {
	cache := NewAllowlistCache(NewAllowlist(nil))
	lister := &mockProjectLister{projects: map[string]token.Info{}}

	reloader := NewCacheReloader(cache, lister, nil, nil, &config.Decisions{})

	newAllow := []config.AllowEntry{{Domain: "new-allow.com"}}
	newDeny := []config.AllowEntry{{Domain: "new-deny.com"}}
	reloader.SetStaticConfig(newAllow, newDeny)

	// Verify via internal state (read lock path exercises the lock)
	reloader.mu.RLock()
	defer reloader.mu.RUnlock()
	if len(reloader.staticAllow) != 1 || reloader.staticAllow[0].Domain != "new-allow.com" {
		t.Errorf("staticAllow = %v, want [{Domain:new-allow.com}]", reloader.staticAllow)
	}
	if len(reloader.staticDeny) != 1 || reloader.staticDeny[0].Domain != "new-deny.com" {
		t.Errorf("staticDeny = %v, want [{Domain:new-deny.com}]", reloader.staticDeny)
	}
}

func TestCacheReloader_LoadProjectAllowlist_AllFourSources(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	projectName := "test-project"

	// Write project decisions file with one allow entry
	err := config.WriteProjectDecisions(projectName, &config.Decisions{
		Proxy: config.DecisionsProxy{
			Allow: []config.AllowEntry{{Domain: "project-decision.com"}},
		},
	})
	if err != nil {
		t.Fatalf("WriteProjectDecisions: %v", err)
	}

	// Write project config with one allow entry
	err = config.WriteProjectConfig(projectName, &config.ProjectConfig{
		Proxy: config.ProjectProxyConfig{
			Allow: []config.AllowEntry{{Domain: "project-config.com"}},
		},
	}, true)
	if err != nil {
		t.Fatalf("WriteProjectConfig: %v", err)
	}

	staticAllow := []config.AllowEntry{{Domain: "static-global.com"}}
	globalDecisions := &config.Decisions{
		Proxy: config.DecisionsProxy{
			Allow: []config.AllowEntry{{Domain: "global-decision.com"}},
		},
	}

	cache := NewAllowlistCache(NewAllowlist(nil))
	lister := &mockProjectLister{projects: map[string]token.Info{}}
	reloader := NewCacheReloader(cache, lister, staticAllow, nil, globalDecisions)

	allowlist, err := reloader.LoadProjectAllowlist(projectName)
	if err != nil {
		t.Fatalf("LoadProjectAllowlist error: %v", err)
	}
	if allowlist == nil {
		t.Fatal("LoadProjectAllowlist returned nil, expected merged allowlist")
	}

	// Verify all four sources are present
	for _, domain := range []string{"static-global.com", "project-config.com", "global-decision.com", "project-decision.com"} {
		if !allowlist.IsAllowed(domain) {
			t.Errorf("domain %q not allowed, expected it in merged allowlist", domain)
		}
	}
}

func TestCacheReloader_LoadProjectAllowlist_NilWhenNoProjectEntries(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	staticAllow := []config.AllowEntry{{Domain: "static-global.com"}}
	cache := NewAllowlistCache(NewAllowlist(nil))
	lister := &mockProjectLister{projects: map[string]token.Info{}}
	reloader := NewCacheReloader(cache, lister, staticAllow, nil, &config.Decisions{})

	// No project config or decisions files exist → nil
	allowlist, err := reloader.LoadProjectAllowlist("nonexistent-project")
	if err != nil {
		t.Fatalf("LoadProjectAllowlist error: %v", err)
	}
	if allowlist != nil {
		t.Errorf("LoadProjectAllowlist returned non-nil for project with no entries: %v", allowlist.Domains())
	}
}

func TestCacheReloader_LoadProjectAllowlist_IncludesGlobalDecisions(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	projectName := "test-project"

	// Write project decisions so the method doesn't return nil
	err := config.WriteProjectDecisions(projectName, &config.Decisions{
		Proxy: config.DecisionsProxy{
			Allow: []config.AllowEntry{{Domain: "project.com"}},
		},
	})
	if err != nil {
		t.Fatalf("WriteProjectDecisions: %v", err)
	}

	globalDecisions := &config.Decisions{
		Proxy: config.DecisionsProxy{
			Allow: []config.AllowEntry{
				{Domain: "global-d1.com"},
				{Domain: "global-d2.com"},
			},
		},
	}

	cache := NewAllowlistCache(NewAllowlist(nil))
	lister := &mockProjectLister{projects: map[string]token.Info{}}
	reloader := NewCacheReloader(cache, lister, nil, nil, globalDecisions)

	allowlist, err := reloader.LoadProjectAllowlist(projectName)
	if err != nil {
		t.Fatalf("LoadProjectAllowlist error: %v", err)
	}
	if allowlist == nil {
		t.Fatal("LoadProjectAllowlist returned nil")
	}

	for _, domain := range []string{"global-d1.com", "global-d2.com", "project.com"} {
		if !allowlist.IsAllowed(domain) {
			t.Errorf("domain %q not allowed, expected it in merged allowlist", domain)
		}
	}
}

func TestCacheReloader_LoadProjectDenylist_MergesConfigAndDecisions(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	projectName := "test-project"

	// Write project decisions with a deny entry
	err := config.WriteProjectDecisions(projectName, &config.Decisions{
		Proxy: config.DecisionsProxy{
			Deny: []config.AllowEntry{{Domain: "decision-deny.com"}},
		},
	})
	if err != nil {
		t.Fatalf("WriteProjectDecisions: %v", err)
	}

	// Write project config with a deny entry
	err = config.WriteProjectConfig(projectName, &config.ProjectConfig{
		Proxy: config.ProjectProxyConfig{
			Deny: []config.AllowEntry{{Domain: "config-deny.com"}},
		},
	}, true)
	if err != nil {
		t.Fatalf("WriteProjectConfig: %v", err)
	}

	cache := NewAllowlistCache(NewAllowlist(nil))
	lister := &mockProjectLister{projects: map[string]token.Info{}}
	reloader := NewCacheReloader(cache, lister, nil, nil, &config.Decisions{})

	denylist, err := reloader.LoadProjectDenylist(projectName)
	if err != nil {
		t.Fatalf("LoadProjectDenylist error: %v", err)
	}
	if denylist == nil {
		t.Fatal("LoadProjectDenylist returned nil, expected merged denylist")
	}

	for _, domain := range []string{"config-deny.com", "decision-deny.com"} {
		if !denylist.IsAllowed(domain) {
			t.Errorf("domain %q not in denylist, expected it in merged denylist", domain)
		}
	}
}

func TestCacheReloader_LoadProjectDenylist_NilWhenNoDenyEntries(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	cache := NewAllowlistCache(NewAllowlist(nil))
	lister := &mockProjectLister{projects: map[string]token.Info{}}
	reloader := NewCacheReloader(cache, lister, nil, nil, &config.Decisions{})

	denylist, err := reloader.LoadProjectDenylist("nonexistent-project")
	if err != nil {
		t.Fatalf("LoadProjectDenylist error: %v", err)
	}
	if denylist != nil {
		t.Errorf("LoadProjectDenylist returned non-nil for project with no deny entries")
	}
}

func TestCacheReloader_Reload_GlobalAllowlist(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Write global decisions with an allow entry
	err := config.WriteGlobalDecisions(&config.Decisions{
		Proxy: config.DecisionsProxy{
			Allow: []config.AllowEntry{{Domain: "global-decision.com"}},
		},
	})
	if err != nil {
		t.Fatalf("WriteGlobalDecisions: %v", err)
	}

	staticAllow := []config.AllowEntry{{Domain: "static.com"}}
	cache := NewAllowlistCache(NewAllowlist(nil))
	lister := &mockProjectLister{projects: map[string]token.Info{}}
	reloader := NewCacheReloader(cache, lister, staticAllow, nil, &config.Decisions{})

	reloader.Reload()

	// Verify GlobalDecisions() is updated
	gd := reloader.GlobalDecisions()
	if len(gd.Proxy.Allow) != 1 || gd.Proxy.Allow[0].Domain != "global-decision.com" {
		t.Errorf("GlobalDecisions() after Reload = %v, want [{Domain:global-decision.com}]", gd.Proxy.Allow)
	}

	global := cache.GetGlobal()
	if !global.IsAllowed("static.com") {
		t.Error("GetGlobal() does not allow static.com after Reload")
	}
	if !global.IsAllowed("global-decision.com") {
		t.Error("GetGlobal() does not allow global-decision.com after Reload")
	}
}

func TestCacheReloader_Reload_GlobalDenylist(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Write global decisions with a deny entry
	err := config.WriteGlobalDecisions(&config.Decisions{
		Proxy: config.DecisionsProxy{
			Deny: []config.AllowEntry{{Domain: "denied.com"}},
		},
	})
	if err != nil {
		t.Fatalf("WriteGlobalDecisions: %v", err)
	}

	staticDeny := []config.AllowEntry{{Domain: "static-deny.com"}}
	cache := NewAllowlistCache(NewAllowlist(nil))
	lister := &mockProjectLister{projects: map[string]token.Info{}}
	reloader := NewCacheReloader(cache, lister, nil, staticDeny, &config.Decisions{})

	reloader.Reload()

	globalDeny := cache.GetGlobalDeny()
	if globalDeny == nil {
		t.Fatal("GetGlobalDeny() returned nil after Reload")
	}
	if !globalDeny.IsAllowed("denied.com") {
		t.Error("global denylist does not contain denied.com after Reload")
	}
	if !globalDeny.IsAllowed("static-deny.com") {
		t.Error("global denylist does not contain static-deny.com after Reload")
	}
}

func TestCacheReloader_Reload_ProjectAllowlist(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	projectName := "test-proj"
	tok := "tok1"

	// Write project decisions
	err := config.WriteProjectDecisions(projectName, &config.Decisions{
		Proxy: config.DecisionsProxy{
			Allow: []config.AllowEntry{{Domain: "project.com"}},
		},
	})
	if err != nil {
		t.Fatalf("WriteProjectDecisions: %v", err)
	}

	cache := NewAllowlistCache(NewAllowlist(nil))
	lister := &mockProjectLister{projects: map[string]token.Info{
		tok: {ProjectName: projectName, CloisterName: "test-cloister"},
	}}
	reloader := NewCacheReloader(cache, lister, nil, nil, &config.Decisions{})
	cache.SetProjectLoader(reloader.LoadProjectAllowlist)

	reloader.Reload()

	// GetProject should return the merged allowlist directly from cache
	// (not via loader, since Reload eagerly populates it)
	proj, err := cache.GetProject(projectName)
	if err != nil {
		t.Fatalf("GetProject error: %v", err)
	}
	if proj == nil {
		t.Fatal("GetProject returned nil after Reload")
	}
	if !proj.IsAllowed("project.com") {
		t.Error("project allowlist does not contain project.com after Reload")
	}
}

func TestCacheReloader_LoadProjectAllowlist_InvalidProjectConfigReturnsError(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	projectName := "bad-config-proj"

	// Write a project config file with an unknown field (triggers strictUnmarshal error)
	projectsDir := filepath.Join(configDir, "cloister", "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	badConfig := []byte("unknown_field: true\n")
	if err := os.WriteFile(filepath.Join(projectsDir, projectName+".yaml"), badConfig, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Write valid project decisions
	err := config.WriteProjectDecisions(projectName, &config.Decisions{
		Proxy: config.DecisionsProxy{
			Allow: []config.AllowEntry{{Domain: "decision-domain.com"}},
		},
	})
	if err != nil {
		t.Fatalf("WriteProjectDecisions: %v", err)
	}

	cache := NewAllowlistCache(NewAllowlist(nil))
	lister := &mockProjectLister{projects: map[string]token.Info{}}
	reloader := NewCacheReloader(cache, lister, nil, nil, &config.Decisions{})

	_, loadErr := reloader.LoadProjectAllowlist(projectName)
	if loadErr == nil {
		t.Fatal("LoadProjectAllowlist should return error for invalid project config")
	}
}

func TestCacheReloader_LoadProjectDenylist_InvalidProjectConfigReturnsError(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	projectName := "bad-config-proj"

	// Write a project config file with an unknown field (triggers strictUnmarshal error)
	projectsDir := filepath.Join(configDir, "cloister", "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	badConfig := []byte("unknown_field: true\n")
	if err := os.WriteFile(filepath.Join(projectsDir, projectName+".yaml"), badConfig, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Write valid project decisions with a deny entry
	err := config.WriteProjectDecisions(projectName, &config.Decisions{
		Proxy: config.DecisionsProxy{
			Deny: []config.AllowEntry{{Domain: "denied-domain.com"}},
		},
	})
	if err != nil {
		t.Fatalf("WriteProjectDecisions: %v", err)
	}

	cache := NewAllowlistCache(NewAllowlist(nil))
	lister := &mockProjectLister{projects: map[string]token.Info{}}
	reloader := NewCacheReloader(cache, lister, nil, nil, &config.Decisions{})

	_, loadErr := reloader.LoadProjectDenylist(projectName)
	if loadErr == nil {
		t.Fatal("LoadProjectDenylist should return error for invalid project config")
	}
}

func TestCacheReloader_Reload_StaleCacheCleared(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	projectName := "test-proj"
	tok := "tok1"

	// Write project decisions
	err := config.WriteProjectDecisions(projectName, &config.Decisions{
		Proxy: config.DecisionsProxy{
			Allow: []config.AllowEntry{{Domain: "first.com"}},
		},
	})
	if err != nil {
		t.Fatalf("WriteProjectDecisions: %v", err)
	}

	cache := NewAllowlistCache(NewAllowlist(nil))
	lister := &mockProjectLister{projects: map[string]token.Info{
		tok: {ProjectName: projectName, CloisterName: "test-cloister"},
	}}
	reloader := NewCacheReloader(cache, lister, nil, nil, &config.Decisions{})
	cache.SetProjectLoader(reloader.LoadProjectAllowlist)
	cache.SetDenylistLoader(reloader.LoadProjectDenylist)

	// First reload — should populate with first.com
	reloader.Reload()
	proj, err := cache.GetProject(projectName)
	if err != nil {
		t.Fatalf("GetProject error: %v", err)
	}
	if !proj.IsAllowed("first.com") {
		t.Error("first.com not in project allowlist after first Reload")
	}

	// Now update the decisions file and reload
	err = config.WriteProjectDecisions(projectName, &config.Decisions{
		Proxy: config.DecisionsProxy{
			Allow: []config.AllowEntry{{Domain: "second.com"}},
		},
	})
	if err != nil {
		t.Fatalf("WriteProjectDecisions: %v", err)
	}

	reloader.Reload()
	proj, err = cache.GetProject(projectName)
	if err != nil {
		t.Fatalf("GetProject error: %v", err)
	}
	if !proj.IsAllowed("second.com") {
		t.Error("second.com not in project allowlist after second Reload")
	}
	// first.com should no longer be in the project-specific allowlist
	// (stale cache was cleared and repopulated)
	if proj.IsAllowed("first.com") {
		t.Error("first.com still in project allowlist after second Reload — stale cache not cleared")
	}
}
