package guardian

import (
	"net"
)

// DefaultAllowedDomains contains the initial hardcoded allowlist for Phase 1.
// This will be replaced by configurable allowlists in Phase 2.
var DefaultAllowedDomains = []string{
	"api.anthropic.com",
	"api.openai.com",
	"generativelanguage.googleapis.com",
}

// Allowlist enforces domain-based access control for the proxy.
// It performs exact domain matching (no wildcards).
type Allowlist struct {
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
	_, ok := a.domains[hostname]
	return ok
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
