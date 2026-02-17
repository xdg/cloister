package guardian

import (
	"errors"
	"testing"
)

func TestAllowlistCache(t *testing.T) {
	t.Run("basic operations", func(t *testing.T) {
		globalAllowlist := NewAllowlist([]string{"global.example.com"})
		cache := NewAllowlistCache(globalAllowlist)

		// GetGlobal returns global allowlist
		if cache.GetGlobal() != globalAllowlist {
			t.Error("GetGlobal should return the global allowlist")
		}

		// GetProject for unknown project returns global
		got, err := cache.GetProject("unknown")
		if err != nil {
			t.Fatalf("GetProject error: %v", err)
		}
		if got != globalAllowlist {
			t.Error("GetProject for unknown should return global")
		}

		// Set project-specific allowlist
		projectAllowlist := NewAllowlist([]string{"project.example.com"})
		cache.SetProject("my-project", projectAllowlist)

		// GetProject returns project allowlist
		got, err = cache.GetProject("my-project")
		if err != nil {
			t.Fatalf("GetProject error: %v", err)
		}
		if got != projectAllowlist {
			t.Error("GetProject should return project allowlist")
		}

		// Other projects still get global
		got, err = cache.GetProject("other-project")
		if err != nil {
			t.Fatalf("GetProject error: %v", err)
		}
		if got != globalAllowlist {
			t.Error("GetProject for other project should return global")
		}
	})

	t.Run("SetGlobal", func(t *testing.T) {
		oldGlobal := NewAllowlist([]string{"old.example.com"})
		cache := NewAllowlistCache(oldGlobal)

		newGlobal := NewAllowlist([]string{"new.example.com"})
		cache.SetGlobal(newGlobal)

		if cache.GetGlobal() != newGlobal {
			t.Error("SetGlobal should update the global allowlist")
		}
	})

	t.Run("Clear", func(t *testing.T) {
		globalAllowlist := NewAllowlist([]string{"global.example.com"})
		cache := NewAllowlistCache(globalAllowlist)

		// Add some project allowlists
		cache.SetProject("project1", NewAllowlist([]string{"p1.example.com"}))
		cache.SetProject("project2", NewAllowlist([]string{"p2.example.com"}))

		// Clear should remove all project allowlists
		cache.Clear()

		// Projects should now return global
		got, err := cache.GetProject("project1")
		if err != nil {
			t.Fatalf("GetProject error: %v", err)
		}
		if got != globalAllowlist {
			t.Error("after Clear, projects should return global")
		}
		got, err = cache.GetProject("project2")
		if err != nil {
			t.Fatalf("GetProject error: %v", err)
		}
		if got != globalAllowlist {
			t.Error("after Clear, projects should return global")
		}
	})

	t.Run("GetProject propagates loader error", func(t *testing.T) {
		cache := NewAllowlistCache(NewAllowlist(nil))
		cache.SetProjectLoader(func(_ string) (*Allowlist, error) {
			return nil, errors.New("bad config")
		})

		got, err := cache.GetProject("broken")
		if err == nil {
			t.Fatal("expected error from GetProject, got nil")
		}
		if got != nil {
			t.Error("expected nil allowlist on error")
		}
	})
}

func TestAllowlistCacheDenyBasicOperations(t *testing.T) {
	t.Run("SetGlobalDeny/GetGlobalDeny", func(t *testing.T) {
		cache := NewAllowlistCache(NewAllowlist(nil))

		// Initially nil
		if cache.GetGlobalDeny() != nil {
			t.Error("GetGlobalDeny should return nil initially")
		}

		// Set and retrieve
		denylist := NewAllowlist([]string{"evil.com"})
		cache.SetGlobalDeny(denylist)

		if cache.GetGlobalDeny() != denylist {
			t.Error("GetGlobalDeny should return the set denylist")
		}

		// Replace
		newDenylist := NewAllowlist([]string{"worse.com"})
		cache.SetGlobalDeny(newDenylist)

		if cache.GetGlobalDeny() != newDenylist {
			t.Error("SetGlobalDeny should replace the previous denylist")
		}
	})

	t.Run("SetProjectDeny/GetProjectDeny basic", func(t *testing.T) {
		cache := NewAllowlistCache(NewAllowlist(nil))

		projectDeny := NewAllowlist([]string{"bad.example.com"})
		cache.SetProjectDeny("my-project", projectDeny)

		got, err := cache.GetProjectDeny("my-project")
		if err != nil {
			t.Fatalf("GetProjectDeny error: %v", err)
		}
		if got != projectDeny {
			t.Error("GetProjectDeny should return the set project denylist")
		}
	})

	t.Run("GetProjectDeny with no loader returns globalDeny", func(t *testing.T) {
		cache := NewAllowlistCache(NewAllowlist(nil))
		globalDeny := NewAllowlist([]string{"blocked.com"})
		cache.SetGlobalDeny(globalDeny)

		got, err := cache.GetProjectDeny("unknown-project")
		if err != nil {
			t.Fatalf("GetProjectDeny error: %v", err)
		}
		if got != globalDeny {
			t.Error("GetProjectDeny with no loader should fall back to globalDeny")
		}
	})

	t.Run("GetProjectDeny with no loader and nil globalDeny returns nil", func(t *testing.T) {
		cache := NewAllowlistCache(NewAllowlist(nil))

		got, err := cache.GetProjectDeny("unknown-project")
		if err != nil {
			t.Fatalf("GetProjectDeny error: %v", err)
		}
		if got != nil {
			t.Error("GetProjectDeny with no loader and nil globalDeny should return nil")
		}
	})

	t.Run("Clear removes project denylists", func(t *testing.T) {
		cache := NewAllowlistCache(NewAllowlist(nil))
		globalDeny := NewAllowlist([]string{"global-blocked.com"})
		cache.SetGlobalDeny(globalDeny)

		// Add project denylists
		cache.SetProjectDeny("project1", NewAllowlist([]string{"p1-bad.com"}))
		cache.SetProjectDeny("project2", NewAllowlist([]string{"p2-bad.com"}))

		cache.Clear()

		// After clear, project denylists should be gone; with no loader, falls back to globalDeny
		got, err := cache.GetProjectDeny("project1")
		if err != nil {
			t.Fatalf("GetProjectDeny error: %v", err)
		}
		if got != globalDeny {
			t.Error("after Clear, GetProjectDeny should fall back to globalDeny")
		}

		// Global denylist should still be set
		if cache.GetGlobalDeny() != globalDeny {
			t.Error("Clear should not remove the global denylist")
		}
	})

	t.Run("GetProjectDeny propagates loader error", func(t *testing.T) {
		cache := NewAllowlistCache(NewAllowlist(nil))
		cache.SetDenylistLoader(func(_ string) (*Allowlist, error) {
			return nil, errors.New("bad config")
		})

		got, err := cache.GetProjectDeny("broken")
		if err == nil {
			t.Fatal("expected error from GetProjectDeny, got nil")
		}
		if got != nil {
			t.Error("expected nil denylist on error")
		}
	})
}

func TestAllowlistCacheDenylistLoader(t *testing.T) {
	t.Run("cache loads denied_domains from decisions file", func(t *testing.T) {
		cache := NewAllowlistCache(NewAllowlist(nil))

		loadCount := 0
		loader := func(_ string) (*Allowlist, error) {
			loadCount++
			return NewAllowlist([]string{"denied.example.com", "spam.example.com"}), nil
		}
		cache.SetDenylistLoader(loader)

		// First call: loader should be invoked
		denylist, err := cache.GetProjectDeny("my-project")
		if err != nil {
			t.Fatalf("GetProjectDeny error: %v", err)
		}
		if denylist == nil {
			t.Fatal("GetProjectDeny should return a denylist from the loader")
		}
		if loadCount != 1 {
			t.Errorf("loader should have been called once, got %d", loadCount)
		}

		// Verify the denylist contains the denied domain
		if !denylist.IsAllowed("denied.example.com") {
			t.Error("denylist should contain denied.example.com")
		}
		if !denylist.IsAllowed("spam.example.com") {
			t.Error("denylist should contain spam.example.com")
		}

		// Second call: should use cache, not call loader again
		denylist2, err := cache.GetProjectDeny("my-project")
		if err != nil {
			t.Fatalf("GetProjectDeny error: %v", err)
		}
		if loadCount != 1 {
			t.Errorf("loader should NOT have been called again, got %d calls", loadCount)
		}
		if denylist2 != denylist {
			t.Error("second call should return the same cached denylist pointer")
		}
	})

	t.Run("loader returning nil falls back to globalDeny", func(t *testing.T) {
		cache := NewAllowlistCache(NewAllowlist(nil))
		globalDeny := NewAllowlist([]string{"global-denied.com"})
		cache.SetGlobalDeny(globalDeny)

		loader := func(_ string) (*Allowlist, error) {
			return nil, nil
		}
		cache.SetDenylistLoader(loader)

		got, err := cache.GetProjectDeny("my-project")
		if err != nil {
			t.Fatalf("GetProjectDeny error: %v", err)
		}
		if got != globalDeny {
			t.Error("loader returning nil should fall back to globalDeny")
		}
	})
}

func TestAllowlistCacheIsBlocked(t *testing.T) {
	tests := []struct {
		name        string
		globalDeny  *Allowlist
		projectDeny *Allowlist // set via SetProjectDeny (not loader)
		denyLoader  ProjectDenylistLoader
		project     string
		domain      string
		wantBlocked bool
	}{
		{
			name:        "global denylist blocks a domain",
			globalDeny:  NewAllowlist([]string{"evil.com"}),
			project:     "my-project",
			domain:      "evil.com",
			wantBlocked: true,
		},
		{
			name:        "project denylist blocks a domain not in global",
			globalDeny:  NewAllowlist([]string{"global-bad.com"}),
			projectDeny: NewAllowlist([]string{"project-bad.com"}),
			project:     "my-project",
			domain:      "project-bad.com",
			wantBlocked: true,
		},
		{
			name:        "domain not in any denylist",
			globalDeny:  NewAllowlist([]string{"evil.com"}),
			projectDeny: NewAllowlist([]string{"bad.com"}),
			project:     "my-project",
			domain:      "safe.example.com",
			wantBlocked: false,
		},
		{
			name:        "no denylists set (nil)",
			globalDeny:  nil,
			projectDeny: nil,
			project:     "my-project",
			domain:      "anything.com",
			wantBlocked: false,
		},
		{
			name:        "both global and project match",
			globalDeny:  NewAllowlist([]string{"evil.com"}),
			projectDeny: NewAllowlist([]string{"evil.com", "also-bad.com"}),
			project:     "my-project",
			domain:      "evil.com",
			wantBlocked: true,
		},
		{
			name:        "global denylist with pattern blocks subdomain",
			globalDeny:  NewAllowlistWithPatterns(nil, []string{"*.evil.com"}),
			project:     "my-project",
			domain:      "sub.evil.com",
			wantBlocked: true,
		},
		{
			name:        "global denylist pattern does not block base domain",
			globalDeny:  NewAllowlistWithPatterns(nil, []string{"*.evil.com"}),
			project:     "my-project",
			domain:      "evil.com",
			wantBlocked: false,
		},
		{
			name:        "project denylist via loader blocks domain",
			globalDeny:  nil,
			projectDeny: nil,
			denyLoader: func(_ string) (*Allowlist, error) {
				return NewAllowlist([]string{"loaded-bad.com"}), nil
			},
			project:     "my-project",
			domain:      "loaded-bad.com",
			wantBlocked: true,
		},
		{
			name:        "domain not blocked by project denylist via loader",
			globalDeny:  nil,
			projectDeny: nil,
			denyLoader: func(_ string) (*Allowlist, error) {
				return NewAllowlist([]string{"loaded-bad.com"}), nil
			},
			project:     "my-project",
			domain:      "safe.com",
			wantBlocked: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache := NewAllowlistCache(NewAllowlist(nil))

			if tc.globalDeny != nil {
				cache.SetGlobalDeny(tc.globalDeny)
			}
			if tc.projectDeny != nil {
				cache.SetProjectDeny(tc.project, tc.projectDeny)
			}
			if tc.denyLoader != nil {
				cache.SetDenylistLoader(tc.denyLoader)
			}

			got, err := cache.IsBlocked(tc.project, tc.domain)
			if err != nil {
				t.Fatalf("IsBlocked error: %v", err)
			}
			if got != tc.wantBlocked {
				t.Errorf("IsBlocked(%q, %q) = %v, want %v",
					tc.project, tc.domain, got, tc.wantBlocked)
			}
		})
	}
}

func TestAllowlistCacheIsBlocked_PropagatesLoaderError(t *testing.T) {
	cache := NewAllowlistCache(NewAllowlist(nil))
	cache.SetDenylistLoader(func(_ string) (*Allowlist, error) {
		return nil, errors.New("corrupt decisions file")
	})

	_, err := cache.IsBlocked("broken-project", "example.com")
	if err == nil {
		t.Fatal("expected error from IsBlocked, got nil")
	}
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Errorf("expected ConfigError, got %T: %v", err, err)
	}
}
