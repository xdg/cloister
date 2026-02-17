package guardian

import (
	"github.com/xdg/cloister/internal/config"
)

// DefaultAllowedDomains contains the initial hardcoded allowlist for Phase 1.
// This is used as a fallback when no configuration is provided.
var DefaultAllowedDomains = []string{
	// AI provider APIs
	"api.anthropic.com",
	"api.openai.com",
	"generativelanguage.googleapis.com",

	// Go module proxy and toolchain downloads
	"proxy.golang.org",
	"sum.golang.org",
	"storage.googleapis.com",

	// Ubuntu package repositories
	"archive.ubuntu.com",
	"ports.ubuntu.com",
	"security.ubuntu.com",
	"deb.nodesource.com",
}

// Allowlist enforces domain-based access control for the proxy.
// It supports both exact domain matching and wildcard patterns (*.example.com).
// Wildcard patterns match any subdomain but not the base domain itself.
// All methods are thread-safe.
//
// Allowlist is a thin wrapper around DomainSet, delegating all logic to it.
type Allowlist struct {
	*DomainSet
}

// NewAllowlist creates an Allowlist from a slice of allowed domains.
func NewAllowlist(domains []string) *Allowlist {
	return &Allowlist{DomainSet: NewDomainSet(domains, nil)}
}

// NewAllowlistWithPatterns creates an Allowlist from domains and wildcard patterns.
// Patterns should be in the format "*.example.com".
func NewAllowlistWithPatterns(domains, patterns []string) *Allowlist {
	return &Allowlist{DomainSet: NewDomainSet(domains, patterns)}
}

// NewAllowlistFromConfig creates an Allowlist from config AllowEntry slice.
// It handles both exact domains and wildcard patterns.
func NewAllowlistFromConfig(entries []config.AllowEntry) *Allowlist {
	return &Allowlist{DomainSet: NewDomainSetFromConfig(entries)}
}

// NewDefaultAllowlist creates an Allowlist with the default allowed domains.
func NewDefaultAllowlist() *Allowlist {
	return NewAllowlist(DefaultAllowedDomains)
}

// IsAllowed checks if the given host is in the allowlist.
// The host may include a port (e.g., "api.anthropic.com:443"), which is
// stripped before matching. Checks exact domain matches first, then patterns.
func (a *Allowlist) IsAllowed(host string) bool {
	return a.Contains(host)
}

// Add adds additional domains to the allowlist.
func (a *Allowlist) Add(domains []string) {
	for _, d := range domains {
		a.DomainSet.Add(d)
	}
}

// AddPatterns adds wildcard patterns to the allowlist.
// Patterns should be in the format "*.example.com".
func (a *Allowlist) AddPatterns(patterns []string) {
	for _, p := range patterns {
		a.AddPattern(p)
	}
}

// Replace atomically replaces the allowlist domains.
func (a *Allowlist) Replace(domains []string) {
	newDomains := make(map[string]struct{}, len(domains))
	for _, d := range domains {
		newDomains[stripPort(d)] = struct{}{}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.domains = newDomains
}

// ReplacePatterns atomically replaces the allowlist patterns.
func (a *Allowlist) ReplacePatterns(patterns []string) {
	newPatterns := make([]string, 0, len(patterns))
	for _, p := range patterns {
		if IsValidPattern(p) {
			newPatterns = append(newPatterns, p)
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.patterns = newPatterns
}

// Domains returns a copy of the current allowed domains.
func (a *Allowlist) Domains() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make([]string, 0, len(a.domains))
	for d := range a.domains {
		result = append(result, d)
	}
	return result
}

// Patterns returns a copy of the current allowed patterns.
func (a *Allowlist) Patterns() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make([]string, len(a.patterns))
	copy(result, a.patterns)
	return result
}
