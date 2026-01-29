// Package guardian implements the HTTP CONNECT proxy and related services
// that mediate network access for cloister containers.
package guardian

import (
	"sync"
)

// TokenLookupFunc looks up token info and returns the project name.
// It returns the project name and true if the token is valid, empty string and false otherwise.
type TokenLookupFunc func(token string) (projectName string, valid bool)

// ProjectAllowlistLoader loads and returns the allowlist for a project.
// It should merge the project config with the global config.
type ProjectAllowlistLoader func(projectName string) *Allowlist

// AllowlistCache provides per-project allowlist lookups with caching.
type AllowlistCache struct {
	mu            sync.RWMutex
	global        *Allowlist
	perProject    map[string]*Allowlist // project name -> merged allowlist
	projectLoader ProjectAllowlistLoader
}

// NewAllowlistCache creates a new AllowlistCache with the given global allowlist.
func NewAllowlistCache(global *Allowlist) *AllowlistCache {
	return &AllowlistCache{
		global:     global,
		perProject: make(map[string]*Allowlist),
	}
}

// SetProjectLoader sets the callback for loading project allowlists on-demand.
func (c *AllowlistCache) SetProjectLoader(loader ProjectAllowlistLoader) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.projectLoader = loader
}

// SetGlobal replaces the global allowlist.
func (c *AllowlistCache) SetGlobal(global *Allowlist) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.global = global
}

// GetGlobal returns the global allowlist.
func (c *AllowlistCache) GetGlobal() *Allowlist {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.global
}

// SetProject sets or replaces the allowlist for a specific project.
func (c *AllowlistCache) SetProject(projectName string, allowlist *Allowlist) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.perProject[projectName] = allowlist
}

// GetProject returns the allowlist for a specific project.
// If the project is not cached and a projectLoader is set, it loads and caches the allowlist.
// If no loader is set or loading fails, returns the global allowlist.
func (c *AllowlistCache) GetProject(projectName string) *Allowlist {
	// First try with read lock
	c.mu.RLock()
	if allowlist, ok := c.perProject[projectName]; ok {
		c.mu.RUnlock()
		return allowlist
	}
	loader := c.projectLoader
	c.mu.RUnlock()

	// If no loader, return global
	if loader == nil {
		return c.global
	}

	// Load the project allowlist
	allowlist := loader(projectName)
	if allowlist == nil {
		return c.global
	}

	// Cache and return
	c.mu.Lock()
	c.perProject[projectName] = allowlist
	c.mu.Unlock()

	return allowlist
}

// Clear removes all project-specific allowlists.
func (c *AllowlistCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.perProject = make(map[string]*Allowlist)
}
