package guardian

// Decision represents a proxy access control decision.
type Decision int

// Decision constants for proxy access control.
const (
	Allow Decision = iota
	Deny
	AskHuman
)

// String returns a human-readable representation of a Decision.
func (d Decision) String() string {
	switch d {
	case Allow:
		return "Allow"
	case Deny:
		return "Deny"
	case AskHuman:
		return "AskHuman"
	default:
		return "Unknown"
	}
}

// ProxyPolicy holds allow and deny domain sets for a single policy tier.
// Nil Allow or Deny means empty (no matches). Deny takes precedence over Allow.
type ProxyPolicy struct {
	Allow *DomainSet
	Deny  *DomainSet
}

// IsAllowed returns true if the domain is in the allow set.
// Returns false if the receiver or Allow field is nil.
func (p *ProxyPolicy) IsAllowed(domain string) bool {
	if p == nil || p.Allow == nil {
		return false
	}
	return p.Allow.Contains(domain)
}

// IsDenied returns true if the domain is in the deny set.
// Returns false if the receiver or Deny field is nil.
func (p *ProxyPolicy) IsDenied(domain string) bool {
	if p == nil || p.Deny == nil {
		return false
	}
	return p.Deny.Contains(domain)
}
