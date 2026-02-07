package guardian

import (
	"net"
	"strings"
	"sync"

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
type Allowlist struct {
	mu       sync.RWMutex
	domains  map[string]struct{}
	patterns []string // patterns like "*.example.com"
}

// NewAllowlist creates an Allowlist from a slice of allowed domains.
func NewAllowlist(domains []string) *Allowlist {
	a := &Allowlist{
		domains:  make(map[string]struct{}, len(domains)),
		patterns: make([]string, 0),
	}
	for _, d := range domains {
		// Strip port if present for consistent matching with IsAllowed
		a.domains[stripPort(d)] = struct{}{}
	}
	return a
}

// NewAllowlistWithPatterns creates an Allowlist from domains and wildcard patterns.
// Patterns should be in the format "*.example.com".
func NewAllowlistWithPatterns(domains, patterns []string) *Allowlist {
	a := &Allowlist{
		domains:  make(map[string]struct{}, len(domains)),
		patterns: make([]string, 0, len(patterns)),
	}
	for _, d := range domains {
		// Strip port if present for consistent matching with IsAllowed
		a.domains[stripPort(d)] = struct{}{}
	}
	for _, p := range patterns {
		if IsValidPattern(p) {
			a.patterns = append(a.patterns, p)
		}
	}
	return a
}

// NewAllowlistFromConfig creates an Allowlist from config AllowEntry slice.
// It handles both exact domains and wildcard patterns.
func NewAllowlistFromConfig(entries []config.AllowEntry) *Allowlist {
	domains := make([]string, 0, len(entries))
	patterns := make([]string, 0)
	for _, e := range entries {
		if e.Pattern != "" {
			patterns = append(patterns, e.Pattern)
		} else if e.Domain != "" {
			domains = append(domains, e.Domain)
		}
	}
	return NewAllowlistWithPatterns(domains, patterns)
}

// NewDefaultAllowlist creates an Allowlist with the default allowed domains.
func NewDefaultAllowlist() *Allowlist {
	return NewAllowlist(DefaultAllowedDomains)
}

// IsAllowed checks if the given host is in the allowlist.
// The host may include a port (e.g., "api.anthropic.com:443"), which is
// stripped before matching. Checks exact domain matches first, then patterns.
func (a *Allowlist) IsAllowed(host string) bool {
	// Strip port if present
	hostname := stripPort(host)
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Check exact match first
	if _, ok := a.domains[hostname]; ok {
		return true
	}

	// Check patterns
	for _, pattern := range a.patterns {
		if matchPattern(pattern, hostname) {
			return true
		}
	}

	return false
}

// Add adds additional domains to the allowlist.
func (a *Allowlist) Add(domains []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, d := range domains {
		a.domains[d] = struct{}{}
	}
}

// AddPatterns adds wildcard patterns to the allowlist.
// Patterns should be in the format "*.example.com".
func (a *Allowlist) AddPatterns(patterns []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, p := range patterns {
		if IsValidPattern(p) {
			// Avoid duplicates
			found := false
			for _, existing := range a.patterns {
				if existing == p {
					found = true
					break
				}
			}
			if !found {
				a.patterns = append(a.patterns, p)
			}
		}
	}
}

// Replace atomically replaces the allowlist domains.
func (a *Allowlist) Replace(domains []string) {
	newDomains := make(map[string]struct{}, len(domains))
	for _, d := range domains {
		newDomains[d] = struct{}{}
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

// stripPort removes the port from a host:port string.
// If no port is present, returns the host unchanged.
func stripPort(host string) string {
	hostname, _, err := net.SplitHostPort(host)
	if err != nil {
		// No port present, or invalid format - return as-is
		return host
	}
	return hostname
}

// IsValidPattern checks if a pattern is a valid wildcard pattern.
// Valid patterns are in the format "*.example.com" (asterisk, dot, then domain).
func IsValidPattern(pattern string) bool {
	if len(pattern) < 3 {
		return false
	}
	// Must start with "*."
	if pattern[0] != '*' || pattern[1] != '.' {
		return false
	}
	// Must have something after "*."
	suffix := pattern[2:]
	if suffix == "" {
		return false
	}
	// Suffix should not start with a dot
	if suffix[0] == '.' {
		return false
	}
	return true
}

// matchPattern checks if a hostname matches a wildcard pattern.
// Pattern "*.example.com" matches "api.example.com" but NOT "example.com"
// and NOT "a.b.example.com" (only single-level subdomain).
func matchPattern(pattern, hostname string) bool {
	if !IsValidPattern(pattern) {
		return false
	}
	// Get the suffix after "*."
	suffix := pattern[1:] // ".example.com"

	// hostname must end with the suffix (e.g., ".example.com")
	if !strings.HasSuffix(hostname, suffix) {
		return false
	}

	// Get the prefix (subdomain part)
	prefix := hostname[:len(hostname)-len(suffix)]

	// prefix must be non-empty (not just the base domain)
	if prefix == "" {
		return false
	}

	// prefix must not contain dots (single-level subdomain only)
	if strings.Contains(prefix, ".") {
		return false
	}

	return true
}

// DomainToWildcard converts a domain like "api.example.com" to a wildcard
// pattern like "*.example.com". Returns empty string if the domain doesn't
// have at least two labels (e.g., "example.com" has no subdomain to wildcard).
func DomainToWildcard(domain string) string {
	// Find the first dot
	idx := strings.Index(domain, ".")
	if idx == -1 || idx == len(domain)-1 {
		return ""
	}
	// Return *.rest_of_domain
	return "*" + domain[idx:]
}
