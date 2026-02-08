// Package guardian implements the HTTP CONNECT proxy and related services
// that mediate network access for cloister containers.
package guardian

import (
	"sync"
)

// TokenLookupResult holds the result of a token lookup.
type TokenLookupResult struct {
	ProjectName  string
	CloisterName string
}

// TokenLookupFunc looks up token info and returns project and cloister names.
// It returns the result and true if the token is valid, zero value and false otherwise.
type TokenLookupFunc func(token string) (TokenLookupResult, bool)

// ProjectAllowlistLoader loads and returns the allowlist for a project.
// It should merge the project config with the global config.
type ProjectAllowlistLoader func(projectName string) *Allowlist

// ProjectDenylistLoader loads and returns the denylist for a project.
// It should merge the project config with the global config.
type ProjectDenylistLoader func(projectName string) *Allowlist

// AllowlistCache provides per-project allowlist lookups with caching.
type AllowlistCache struct {
	mu              sync.RWMutex
	global          *Allowlist
	perProject      map[string]*Allowlist // project name -> merged allowlist
	projectLoader   ProjectAllowlistLoader
	globalDeny      *Allowlist
	perProjectDeny  map[string]*Allowlist // project name -> merged denylist
	denylistLoader  ProjectDenylistLoader
}

// NewAllowlistCache creates a new AllowlistCache with the given global allowlist.
func NewAllowlistCache(global *Allowlist) *AllowlistCache {
	return &AllowlistCache{
		global:         global,
		perProject:     make(map[string]*Allowlist),
		perProjectDeny: make(map[string]*Allowlist),
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

// SetDenylistLoader sets the callback for loading project denylists on-demand.
func (c *AllowlistCache) SetDenylistLoader(loader ProjectDenylistLoader) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.denylistLoader = loader
}

// SetGlobalDeny replaces the global denylist.
func (c *AllowlistCache) SetGlobalDeny(denylist *Allowlist) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.globalDeny = denylist
}

// GetGlobalDeny returns the global denylist (may be nil).
func (c *AllowlistCache) GetGlobalDeny() *Allowlist {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.globalDeny
}

// SetProjectDeny sets or replaces the denylist for a specific project.
func (c *AllowlistCache) SetProjectDeny(projectName string, denylist *Allowlist) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.perProjectDeny[projectName] = denylist
}

// GetProjectDeny returns the denylist for a specific project.
// If the project is not cached and a denylistLoader is set, it loads and caches the denylist.
// If no loader is set or loading returns nil, falls back to the global denylist.
// Returns nil if globalDeny is also nil.
func (c *AllowlistCache) GetProjectDeny(projectName string) *Allowlist {
	// First try with read lock
	c.mu.RLock()
	if denylist, ok := c.perProjectDeny[projectName]; ok {
		c.mu.RUnlock()
		return denylist
	}
	loader := c.denylistLoader
	globalDeny := c.globalDeny
	c.mu.RUnlock()

	// If no loader, return globalDeny (may be nil)
	if loader == nil {
		return globalDeny
	}

	// Load the project denylist
	denylist := loader(projectName)
	if denylist == nil {
		return globalDeny
	}

	// Cache and return
	c.mu.Lock()
	c.perProjectDeny[projectName] = denylist
	c.mu.Unlock()

	return denylist
}

// IsBlocked checks if a domain is blocked by the global or project denylist.
// Returns true if either the global denylist or the project denylist matches the domain.
//
// Note: Denylists reuse the Allowlist type as a set-membership structure.
// Calling IsAllowed on a denylist checks whether the domain is in the deny set
// (i.e., IsAllowed returning true means the domain IS denied/blocked).
//
// The globalDeny snapshot and GetProjectDeny call are not atomic with respect to
// concurrent SetGlobalDeny calls, but SetGlobalDeny is only called at startup
// and on SIGHUP reload, not per-request, so this is safe in practice.
func (c *AllowlistCache) IsBlocked(projectName, domain string) bool {
	c.mu.RLock()
	globalDeny := c.globalDeny
	c.mu.RUnlock()

	// Check global denylist first (IsAllowed = "is this domain in the deny set?")
	if globalDeny != nil && globalDeny.IsAllowed(domain) {
		return true
	}

	// Check project denylist (uses GetProjectDeny which handles loader + fallback).
	// Skip if projectDeny is the same pointer as globalDeny to avoid double-checking.
	projectDeny := c.GetProjectDeny(projectName)
	if projectDeny != nil && projectDeny != globalDeny && projectDeny.IsAllowed(domain) {
		return true
	}

	return false
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

// Clear removes all project-specific allowlists and denylists.
// Global allowlist and denylist are retained.
func (c *AllowlistCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.perProject = make(map[string]*Allowlist)
	c.perProjectDeny = make(map[string]*Allowlist)
}
