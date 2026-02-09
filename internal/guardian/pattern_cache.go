package guardian

import (
	"sync"

	"github.com/xdg/cloister/internal/guardian/patterns"
)

// ProjectPatternLoader loads and returns the pattern matcher for a project.
// It should merge the project config with the global config.
type ProjectPatternLoader func(projectName string) patterns.Matcher

// PatternCache provides per-project pattern matcher lookups with caching.
type PatternCache struct {
	mu            sync.RWMutex
	global        patterns.Matcher
	perProject    map[string]patterns.Matcher
	projectLoader ProjectPatternLoader
}

// NewPatternCache creates a new PatternCache with the given global matcher.
func NewPatternCache(global patterns.Matcher) *PatternCache {
	return &PatternCache{
		global:     global,
		perProject: make(map[string]patterns.Matcher),
	}
}

// SetProjectLoader sets the callback for loading project matchers on-demand.
func (c *PatternCache) SetProjectLoader(loader ProjectPatternLoader) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.projectLoader = loader
}

// SetGlobal replaces the global pattern matcher.
func (c *PatternCache) SetGlobal(global patterns.Matcher) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.global = global
}

// GetGlobal returns the global pattern matcher.
func (c *PatternCache) GetGlobal() patterns.Matcher {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.global
}

// GetProject returns the pattern matcher for a specific project.
// If the project is not cached and a projectLoader is set, it loads and caches the matcher.
// If no loader is set or loading returns nil, returns the global matcher.
func (c *PatternCache) GetProject(projectName string) patterns.Matcher {
	// First try with read lock
	c.mu.RLock()
	if matcher, ok := c.perProject[projectName]; ok {
		c.mu.RUnlock()
		return matcher
	}
	loader := c.projectLoader
	c.mu.RUnlock()

	// If no loader, return global
	if loader == nil {
		return c.global
	}

	// Load the project matcher
	matcher := loader(projectName)
	if matcher == nil {
		return c.global
	}

	// Cache and return
	c.mu.Lock()
	c.perProject[projectName] = matcher
	c.mu.Unlock()

	return matcher
}

// Clear removes all project-specific matchers.
// The global matcher is retained.
func (c *PatternCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.perProject = make(map[string]patterns.Matcher)
}
