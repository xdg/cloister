package guardian

import (
	"net"
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
}

// Allowlist enforces domain-based access control for the proxy.
// It performs exact domain matching (no wildcards).
// All methods are thread-safe.
type Allowlist struct {
	mu      sync.RWMutex
	domains map[string]struct{}
}

// NewAllowlist creates an Allowlist from a slice of allowed domains.
func NewAllowlist(domains []string) *Allowlist {
	a := &Allowlist{
		domains: make(map[string]struct{}, len(domains)),
	}
	for _, d := range domains {
		a.domains[d] = struct{}{}
	}
	return a
}

// NewAllowlistFromConfig creates an Allowlist from config AllowEntry slice.
func NewAllowlistFromConfig(entries []config.AllowEntry) *Allowlist {
	domains := make([]string, len(entries))
	for i, e := range entries {
		domains[i] = e.Domain
	}
	return NewAllowlist(domains)
}

// NewDefaultAllowlist creates an Allowlist with the default allowed domains.
func NewDefaultAllowlist() *Allowlist {
	return NewAllowlist(DefaultAllowedDomains)
}

// IsAllowed checks if the given host is in the allowlist.
// The host may include a port (e.g., "api.anthropic.com:443"), which is
// stripped before matching.
func (a *Allowlist) IsAllowed(host string) bool {
	// Strip port if present
	hostname := stripPort(host)
	a.mu.RLock()
	defer a.mu.RUnlock()
	_, ok := a.domains[hostname]
	return ok
}

// Add adds additional domains to the allowlist.
func (a *Allowlist) Add(domains []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, d := range domains {
		a.domains[d] = struct{}{}
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
