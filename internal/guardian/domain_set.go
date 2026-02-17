package guardian

import (
	"net"
	"strings"
	"sync"

	"github.com/xdg/cloister/internal/config"
)

// DomainSet is a thread-safe set of domains with support for exact matches
// and wildcard patterns. It provides set-membership semantics (Contains)
// rather than policy semantics (allow/deny).
type DomainSet struct {
	mu       sync.RWMutex
	domains  map[string]struct{}
	patterns []string // patterns like "*.example.com"
}

// NewDomainSet creates a DomainSet from slices of exact domains and wildcard patterns.
func NewDomainSet(domains, patterns []string) *DomainSet {
	ds := &DomainSet{
		domains:  make(map[string]struct{}, len(domains)),
		patterns: make([]string, 0, len(patterns)),
	}
	for _, d := range domains {
		ds.domains[stripPort(d)] = struct{}{}
	}
	for _, p := range patterns {
		p = strings.ToLower(p)
		if IsValidPattern(p) {
			ds.patterns = append(ds.patterns, p)
		}
	}
	return ds
}

// NewDomainSetFromConfig creates a DomainSet from config AllowEntry slice.
// It handles both exact domains and wildcard patterns.
func NewDomainSetFromConfig(entries []config.AllowEntry) *DomainSet {
	domains := make([]string, 0, len(entries))
	patterns := make([]string, 0)
	for _, e := range entries {
		if e.Pattern != "" {
			patterns = append(patterns, e.Pattern)
		} else if e.Domain != "" {
			domains = append(domains, e.Domain)
		}
	}
	return NewDomainSet(domains, patterns)
}

// Contains checks if the given host is in the domain set.
// The host may include a port (e.g., "api.anthropic.com:443"), which is
// stripped before matching. Checks exact domain matches first, then patterns.
func (ds *DomainSet) Contains(host string) bool {
	hostname := stripPort(host)
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	// Check exact match first
	if _, ok := ds.domains[hostname]; ok {
		return true
	}

	// Check patterns
	for _, pattern := range ds.patterns {
		if matchPattern(pattern, hostname) {
			return true
		}
	}

	return false
}

// Add adds a single exact domain to the set.
func (ds *DomainSet) Add(domain string) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.domains[stripPort(domain)] = struct{}{}
}

// AddPattern adds a single wildcard pattern to the set after validation.
// Invalid patterns are silently ignored.
func (ds *DomainSet) AddPattern(pattern string) {
	pattern = strings.ToLower(pattern)
	if !IsValidPattern(pattern) {
		return
	}
	ds.mu.Lock()
	defer ds.mu.Unlock()
	// Avoid duplicates
	for _, existing := range ds.patterns {
		if existing == pattern {
			return
		}
	}
	ds.patterns = append(ds.patterns, pattern)
}

// stripPort removes the port from a host:port string.
// If no port is present, returns the host unchanged.
func stripPort(host string) string {
	hostname, _, err := net.SplitHostPort(host)
	if err != nil {
		// No port present, or invalid format - return as-is
		return strings.ToLower(host)
	}
	return strings.ToLower(hostname)
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
// Pattern "*.example.com" matches "api.example.com" and "a.b.example.com",
// but NOT "example.com".
func matchPattern(pattern, hostname string) bool {
	pattern = strings.ToLower(pattern)
	hostname = strings.ToLower(hostname)
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

	return true
}

// countDomainComponents counts the number of domain components (labels) in a domain.
// Examples: "api.example.com" -> 3, "example.com" -> 2, "localhost" -> 1
func countDomainComponents(domain string) int {
	if domain == "" {
		return 0
	}
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" {
		return 0
	}
	return strings.Count(domain, ".") + 1
}

// DomainToWildcard converts a domain like "api.example.com" to a wildcard
// pattern like "*.example.com". Returns empty string if the domain doesn't
// have at least three components to prevent overly broad patterns like "*.com".
func DomainToWildcard(domain string) string {
	// Require at least 3 components to prevent overly broad patterns
	if countDomainComponents(domain) < 3 {
		return ""
	}
	// Find the first dot
	idx := strings.Index(domain, ".")
	if idx == -1 || idx == len(domain)-1 {
		return ""
	}
	// Return *.rest_of_domain
	return "*" + domain[idx:]
}
