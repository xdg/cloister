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
