package guardian

import (
	"testing"

	"github.com/xdg/cloister/internal/guardian/patterns"
)

func TestPatternCache(t *testing.T) {
	t.Run("basic operations", func(t *testing.T) {
		globalMatcher := patterns.NewRegexMatcher([]string{"^echo .*$"}, nil)
		cache := NewPatternCache(globalMatcher)

		// GetGlobal returns global matcher
		if cache.GetGlobal() != globalMatcher {
			t.Error("GetGlobal should return the global matcher")
		}

		// GetProject for unknown project returns global
		if cache.GetProject("unknown") != globalMatcher {
			t.Error("GetProject for unknown should return global")
		}
	})

	t.Run("SetGlobal", func(t *testing.T) {
		oldGlobal := patterns.NewRegexMatcher([]string{"^old$"}, nil)
		cache := NewPatternCache(oldGlobal)

		newGlobal := patterns.NewRegexMatcher([]string{"^new$"}, nil)
		cache.SetGlobal(newGlobal)

		if cache.GetGlobal() != newGlobal {
			t.Error("SetGlobal should update the global matcher")
		}
	})

	t.Run("loader caching", func(t *testing.T) {
		globalMatcher := patterns.NewRegexMatcher([]string{"^global$"}, nil)
		cache := NewPatternCache(globalMatcher)

		loadCount := 0
		projectMatcher := patterns.NewRegexMatcher([]string{"^project$"}, nil)
		cache.SetProjectLoader(func(projectName string) patterns.Matcher {
			loadCount++
			return projectMatcher
		})

		// First call: loader should be invoked
		got := cache.GetProject("my-project")
		if got != projectMatcher {
			t.Error("GetProject should return matcher from loader")
		}
		if loadCount != 1 {
			t.Errorf("loader should have been called once, got %d", loadCount)
		}

		// Second call: should use cache
		got2 := cache.GetProject("my-project")
		if got2 != projectMatcher {
			t.Error("second GetProject should return cached matcher")
		}
		if loadCount != 1 {
			t.Errorf("loader should NOT have been called again, got %d calls", loadCount)
		}
	})

	t.Run("nil loader fallback", func(t *testing.T) {
		globalMatcher := patterns.NewRegexMatcher([]string{"^global$"}, nil)
		cache := NewPatternCache(globalMatcher)
		// No loader set — should return global
		if cache.GetProject("any-project") != globalMatcher {
			t.Error("with nil loader, GetProject should return global")
		}
	})

	t.Run("loader returning nil fallback", func(t *testing.T) {
		globalMatcher := patterns.NewRegexMatcher([]string{"^global$"}, nil)
		cache := NewPatternCache(globalMatcher)

		cache.SetProjectLoader(func(projectName string) patterns.Matcher {
			return nil
		})

		if cache.GetProject("any-project") != globalMatcher {
			t.Error("loader returning nil should fall back to global")
		}
	})

	t.Run("Clear", func(t *testing.T) {
		globalMatcher := patterns.NewRegexMatcher([]string{"^global$"}, nil)
		cache := NewPatternCache(globalMatcher)

		loadCount := 0
		cache.SetProjectLoader(func(projectName string) patterns.Matcher {
			loadCount++
			return patterns.NewRegexMatcher([]string{"^project$"}, nil)
		})

		// Load a project matcher
		cache.GetProject("project1")
		if loadCount != 1 {
			t.Fatalf("expected 1 load, got %d", loadCount)
		}

		// Clear should remove cached matchers
		cache.Clear()

		// Next call should invoke loader again
		cache.GetProject("project1")
		if loadCount != 2 {
			t.Errorf("after Clear, loader should be called again, got %d calls", loadCount)
		}

		// Global should be retained
		if cache.GetGlobal() != globalMatcher {
			t.Error("Clear should not remove global matcher")
		}
	})

	t.Run("different projects get different matchers", func(t *testing.T) {
		globalMatcher := patterns.NewRegexMatcher(nil, nil)
		cache := NewPatternCache(globalMatcher)

		matcherA := patterns.NewRegexMatcher([]string{"^make test$"}, nil)
		matcherB := patterns.NewRegexMatcher([]string{"^npm test$"}, nil)

		cache.SetProjectLoader(func(projectName string) patterns.Matcher {
			switch projectName {
			case "project-a":
				return matcherA
			case "project-b":
				return matcherB
			default:
				return nil
			}
		})

		gotA := cache.GetProject("project-a")
		gotB := cache.GetProject("project-b")

		if gotA != matcherA {
			t.Error("project-a should get matcherA")
		}
		if gotB != matcherB {
			t.Error("project-b should get matcherB")
		}
		if gotA == gotB {
			t.Error("different projects should get different matchers")
		}
	})
}
