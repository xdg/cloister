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
// It also supports session-scoped domains that are approved per-token
// and cleared when the associated cloister stops.
type AllowlistCache struct {
	mu            sync.RWMutex
	global        *Allowlist
	perProject    map[string]*Allowlist      // project name -> merged allowlist
	sessionScoped map[string]map[string]bool // token -> domain set (session-approved domains)
	projectLoader ProjectAllowlistLoader
}

// NewAllowlistCache creates a new AllowlistCache with the given global allowlist.
func NewAllowlistCache(global *Allowlist) *AllowlistCache {
	return &AllowlistCache{
		global:        global,
		perProject:    make(map[string]*Allowlist),
		sessionScoped: make(map[string]map[string]bool),
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

// Clear removes all project-specific allowlists and session-scoped domains.
func (c *AllowlistCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.perProject = make(map[string]*Allowlist)
	c.sessionScoped = make(map[string]map[string]bool)
}

// AddSessionDomain adds a domain that is approved for this session only.
// The domain is associated with the specific token and will be cleared
// when ClearSession is called (typically when the cloister stops).
func (c *AllowlistCache) AddSessionDomain(token, domain string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sessionScoped[token] == nil {
		c.sessionScoped[token] = make(map[string]bool)
	}
	c.sessionScoped[token][domain] = true
}

// IsSessionAllowed checks if a domain is session-approved for the given token.
// Returns true if the domain was approved for this token's session.
func (c *AllowlistCache) IsSessionAllowed(token, domain string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if domains, ok := c.sessionScoped[token]; ok {
		return domains[domain]
	}
	return false
}

// ClearSession removes all session-scoped domains for the given token.
// This is typically called when a cloister stops or token is revoked.
func (c *AllowlistCache) ClearSession(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.sessionScoped, token)
}

// SessionDomainCount returns the number of session-scoped domains for a token.
// This is useful for testing.
func (c *AllowlistCache) SessionDomainCount(token string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if domains, ok := c.sessionScoped[token]; ok {
		return len(domains)
	}
	return 0
}
