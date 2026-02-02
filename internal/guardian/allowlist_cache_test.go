package guardian

import (
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
		if cache.GetProject("unknown") != globalAllowlist {
			t.Error("GetProject for unknown should return global")
		}

		// Set project-specific allowlist
		projectAllowlist := NewAllowlist([]string{"project.example.com"})
		cache.SetProject("my-project", projectAllowlist)

		// GetProject returns project allowlist
		if cache.GetProject("my-project") != projectAllowlist {
			t.Error("GetProject should return project allowlist")
		}

		// Other projects still get global
		if cache.GetProject("other-project") != globalAllowlist {
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
		if cache.GetProject("project1") != globalAllowlist {
			t.Error("after Clear, projects should return global")
		}
		if cache.GetProject("project2") != globalAllowlist {
			t.Error("after Clear, projects should return global")
		}
	})
}

func TestAllowlistCache_SessionScoped(t *testing.T) {
	t.Run("AddSessionDomain and IsSessionAllowed", func(t *testing.T) {
		cache := NewAllowlistCache(NewAllowlist(nil))

		// Initially, no session domains
		if cache.IsSessionAllowed("token-1", "example.com") {
			t.Error("expected no session domain before adding")
		}

		// Add session domain
		cache.AddSessionDomain("token-1", "example.com")

		// Should be allowed now
		if !cache.IsSessionAllowed("token-1", "example.com") {
			t.Error("expected session domain to be allowed after adding")
		}

		// Different domain should not be allowed
		if cache.IsSessionAllowed("token-1", "other.com") {
			t.Error("expected different domain to not be allowed")
		}
	})

	t.Run("session isolation between tokens", func(t *testing.T) {
		cache := NewAllowlistCache(NewAllowlist(nil))

		// Add domain for token-1
		cache.AddSessionDomain("token-1", "example.com")

		// token-1 should have access
		if !cache.IsSessionAllowed("token-1", "example.com") {
			t.Error("token-1 should have access to example.com")
		}

		// token-2 should NOT have access (session isolation)
		if cache.IsSessionAllowed("token-2", "example.com") {
			t.Error("token-2 should NOT have access to token-1's session domain")
		}
	})

	t.Run("ClearSession", func(t *testing.T) {
		cache := NewAllowlistCache(NewAllowlist(nil))

		// Add domains for two tokens
		cache.AddSessionDomain("token-1", "example.com")
		cache.AddSessionDomain("token-1", "test.com")
		cache.AddSessionDomain("token-2", "other.com")

		// Verify both have domains
		if cache.SessionDomainCount("token-1") != 2 {
			t.Errorf("token-1 should have 2 domains, got %d", cache.SessionDomainCount("token-1"))
		}
		if cache.SessionDomainCount("token-2") != 1 {
			t.Errorf("token-2 should have 1 domain, got %d", cache.SessionDomainCount("token-2"))
		}

		// Clear token-1's session
		cache.ClearSession("token-1")

		// token-1 should have no domains
		if cache.SessionDomainCount("token-1") != 0 {
			t.Errorf("token-1 should have 0 domains after clear, got %d", cache.SessionDomainCount("token-1"))
		}
		if cache.IsSessionAllowed("token-1", "example.com") {
			t.Error("token-1 should not have access after session cleared")
		}

		// token-2 should still have its domain
		if cache.SessionDomainCount("token-2") != 1 {
			t.Errorf("token-2 should still have 1 domain, got %d", cache.SessionDomainCount("token-2"))
		}
		if !cache.IsSessionAllowed("token-2", "other.com") {
			t.Error("token-2 should still have access to its domain")
		}
	})

	t.Run("Clear also clears session domains", func(t *testing.T) {
		cache := NewAllowlistCache(NewAllowlist(nil))

		cache.AddSessionDomain("token-1", "example.com")
		cache.SetProject("project", NewAllowlist([]string{"project.com"}))

		cache.Clear()

		// Both project and session should be cleared
		if cache.IsSessionAllowed("token-1", "example.com") {
			t.Error("session domain should be cleared after Clear()")
		}
	})

	t.Run("ClearSession nonexistent token is no-op", func(t *testing.T) {
		cache := NewAllowlistCache(NewAllowlist(nil))

		// Should not panic
		cache.ClearSession("nonexistent-token")

		// Should still work
		cache.AddSessionDomain("token-1", "example.com")
		if !cache.IsSessionAllowed("token-1", "example.com") {
			t.Error("cache should still work after clearing nonexistent token")
		}
	})

	t.Run("multiple domains per token", func(t *testing.T) {
		cache := NewAllowlistCache(NewAllowlist(nil))

		domains := []string{"a.com", "b.com", "c.com", "d.com", "e.com"}
		for _, d := range domains {
			cache.AddSessionDomain("token-1", d)
		}

		if cache.SessionDomainCount("token-1") != len(domains) {
			t.Errorf("expected %d domains, got %d", len(domains), cache.SessionDomainCount("token-1"))
		}

		for _, d := range domains {
			if !cache.IsSessionAllowed("token-1", d) {
				t.Errorf("expected %s to be allowed", d)
			}
		}
	})

	t.Run("adding same domain twice is idempotent", func(t *testing.T) {
		cache := NewAllowlistCache(NewAllowlist(nil))

		cache.AddSessionDomain("token-1", "example.com")
		cache.AddSessionDomain("token-1", "example.com")

		if cache.SessionDomainCount("token-1") != 1 {
			t.Errorf("expected 1 domain after duplicate add, got %d", cache.SessionDomainCount("token-1"))
		}
	})
}

func TestAllowlistCache_ProjectIsolation(t *testing.T) {
	t.Run("project allowlists are isolated", func(t *testing.T) {
		globalAllowlist := NewAllowlist([]string{"global.com"})
		cache := NewAllowlistCache(globalAllowlist)

		// Set project-specific allowlists
		projectAAllowlist := NewAllowlist([]string{"global.com", "project-a.com"})
		projectBAllowlist := NewAllowlist([]string{"global.com", "project-b.com"})
		cache.SetProject("ProjectA", projectAAllowlist)
		cache.SetProject("ProjectB", projectBAllowlist)

		// ProjectA should access global.com and project-a.com
		aList := cache.GetProject("ProjectA")
		if !aList.IsAllowed("global.com") {
			t.Error("ProjectA should access global.com")
		}
		if !aList.IsAllowed("project-a.com") {
			t.Error("ProjectA should access project-a.com")
		}
		if aList.IsAllowed("project-b.com") {
			t.Error("ProjectA should NOT access project-b.com")
		}

		// ProjectB should access global.com and project-b.com
		bList := cache.GetProject("ProjectB")
		if !bList.IsAllowed("global.com") {
			t.Error("ProjectB should access global.com")
		}
		if !bList.IsAllowed("project-b.com") {
			t.Error("ProjectB should access project-b.com")
		}
		if bList.IsAllowed("project-a.com") {
			t.Error("ProjectB should NOT access project-a.com")
		}
	})
}

func TestAllowlistCache_ScopeIsolationMatrix(t *testing.T) {
	// This test verifies the isolation matrix:
	// | Scope   | Same Token | Different Token, Same Project | Different Project |
	// |---------|------------|------------------------------|-------------------|
	// | Session | allowed    | denied                       | denied            |
	// | Project | allowed    | allowed                      | denied            |
	// | Global  | allowed    | allowed                      | allowed           |

	// Setup: Create cache with project allowlists
	globalAllowlist := NewAllowlist([]string{"global-domain.com"})
	cache := NewAllowlistCache(globalAllowlist)

	// ProjectA allowlist includes global + project-a-domain.com
	projectAAllowlist := NewAllowlist([]string{"global-domain.com", "project-a-domain.com"})
	// ProjectB allowlist includes global + project-b-domain.com
	projectBAllowlist := NewAllowlist([]string{"global-domain.com", "project-b-domain.com"})
	cache.SetProject("ProjectA", projectAAllowlist)
	cache.SetProject("ProjectB", projectBAllowlist)

	// Tokens: tokenA1 and tokenA2 are for ProjectA, tokenB is for ProjectB
	// Add session domain for tokenA1
	cache.AddSessionDomain("tokenA1", "session-domain.com")

	tests := []struct {
		name          string
		checkToken    string
		checkProject  string
		domain        string
		expectSession bool // from session scope
		expectProject bool // from project scope
		expectGlobal  bool // from global scope
	}{
		// Session scope tests - only tokenA1 should have access to session-domain.com
		{"session/same-token", "tokenA1", "ProjectA", "session-domain.com", true, false, false},
		{"session/diff-token-same-project", "tokenA2", "ProjectA", "session-domain.com", false, false, false},
		{"session/diff-project", "tokenB", "ProjectB", "session-domain.com", false, false, false},

		// Project scope tests - project-a-domain.com should be accessible by ProjectA tokens only
		{"project/same-token", "tokenA1", "ProjectA", "project-a-domain.com", false, true, false},
		{"project/diff-token-same-project", "tokenA2", "ProjectA", "project-a-domain.com", false, true, false},
		{"project/diff-project", "tokenB", "ProjectB", "project-a-domain.com", false, false, false},

		// Global scope tests - global-domain.com should be accessible by all
		{"global/same-token", "tokenA1", "ProjectA", "global-domain.com", false, true, true},
		{"global/diff-token-same-project", "tokenA2", "ProjectA", "global-domain.com", false, true, true},
		{"global/diff-project", "tokenB", "ProjectB", "global-domain.com", false, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Check session scope
			sessionAllowed := cache.IsSessionAllowed(tt.checkToken, tt.domain)
			if sessionAllowed != tt.expectSession {
				t.Errorf("IsSessionAllowed(%s, %s) = %v, want %v",
					tt.checkToken, tt.domain, sessionAllowed, tt.expectSession)
			}

			// Check project scope
			projectList := cache.GetProject(tt.checkProject)
			projectAllowed := projectList.IsAllowed(tt.domain)
			if projectAllowed != tt.expectProject {
				t.Errorf("GetProject(%s).IsAllowed(%s) = %v, want %v",
					tt.checkProject, tt.domain, projectAllowed, tt.expectProject)
			}

			// Check global scope
			globalAllowed := cache.GetGlobal().IsAllowed(tt.domain)
			if globalAllowed != tt.expectGlobal {
				t.Errorf("GetGlobal().IsAllowed(%s) = %v, want %v",
					tt.domain, globalAllowed, tt.expectGlobal)
			}
		})
	}
}
