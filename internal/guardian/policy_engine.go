package guardian

import (
	"sync"

	"github.com/xdg/cloister/internal/config"
)

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

// PolicyEngine owns all domain access policy state across three tiers:
// global, per-project, and per-token (session). It evaluates domain access
// using a deny-first, then allow, then fallback-to-AskHuman strategy.
type PolicyEngine struct {
	mu       sync.RWMutex
	global   ProxyPolicy
	projects map[string]*ProxyPolicy
	tokens   map[string]*ProxyPolicy

	// Dependencies for reload (set via options or defaults).
	configLoader          func() (*config.GlobalConfig, error)
	decisionLoader        func() (*config.Decisions, error)
	projectLister         ProjectLister
	projectConfigLoader   func(name string) (*config.ProjectConfig, error)
	projectDecisionLoader func(name string) (*config.Decisions, error)
}

// PolicyEngineOption configures a PolicyEngine during construction.
type PolicyEngineOption func(*PolicyEngine)

// WithConfigLoader sets the function used to reload the global config.
func WithConfigLoader(f func() (*config.GlobalConfig, error)) PolicyEngineOption {
	return func(pe *PolicyEngine) {
		pe.configLoader = f
	}
}

// WithDecisionLoader sets the function used to reload global decisions.
func WithDecisionLoader(f func() (*config.Decisions, error)) PolicyEngineOption {
	return func(pe *PolicyEngine) {
		pe.decisionLoader = f
	}
}

// WithProjectConfigLoader sets the function used to load a project's config.
func WithProjectConfigLoader(f func(string) (*config.ProjectConfig, error)) PolicyEngineOption {
	return func(pe *PolicyEngine) {
		pe.projectConfigLoader = f
	}
}

// WithProjectDecisionLoader sets the function used to load a project's decisions.
func WithProjectDecisionLoader(f func(string) (*config.Decisions, error)) PolicyEngineOption {
	return func(pe *PolicyEngine) {
		pe.projectDecisionLoader = f
	}
}

// NewPolicyEngine creates a PolicyEngine from the given global config, global
// decisions, and project lister. It eagerly loads policies for all known
// projects. Options can override the default loader functions used for reload.
func NewPolicyEngine(
	cfg *config.GlobalConfig,
	globalDecisions *config.Decisions,
	projectLister ProjectLister,
	opts ...PolicyEngineOption,
) (*PolicyEngine, error) {
	pe := &PolicyEngine{
		projects:              make(map[string]*ProxyPolicy),
		tokens:                make(map[string]*ProxyPolicy),
		configLoader:          config.LoadGlobalConfig,
		decisionLoader:        config.LoadGlobalDecisions,
		projectLister:         projectLister,
		projectConfigLoader:   config.LoadProjectConfig,
		projectDecisionLoader: config.LoadProjectDecisions,
	}

	for _, opt := range opts {
		opt(pe)
	}

	pe.global = buildGlobalPolicy(cfg, globalDecisions)

	// Eagerly load all project policies.
	if projectLister != nil {
		seen := make(map[string]struct{})
		for _, info := range projectLister.List() {
			name := info.ProjectName
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			policy, err := loadProjectPolicy(name, pe.projectConfigLoader, pe.projectDecisionLoader)
			if err != nil {
				return nil, err
			}
			pe.projects[name] = policy
		}
	}

	return pe, nil
}

// Check evaluates domain access across all policy tiers.
// Evaluation order: deny pass (global -> project -> token), then allow pass
// (global -> project -> token), then fallback to AskHuman.
func (pe *PolicyEngine) Check(token, project, domain string) Decision {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	// Deny pass: if ANY tier denies, return Deny.
	if pe.global.IsDenied(domain) {
		return Deny
	}
	if p, ok := pe.projects[project]; ok && p.IsDenied(domain) {
		return Deny
	}
	if t, ok := pe.tokens[token]; ok && t.IsDenied(domain) {
		return Deny
	}

	// Allow pass: if ANY tier allows, return Allow.
	if pe.global.IsAllowed(domain) {
		return Allow
	}
	if p, ok := pe.projects[project]; ok && p.IsAllowed(domain) {
		return Allow
	}
	if t, ok := pe.tokens[token]; ok && t.IsAllowed(domain) {
		return Allow
	}

	return AskHuman
}

// splitEntries separates a slice of AllowEntry into domain and pattern lists.
func splitEntries(entries []config.AllowEntry) (domains, patterns []string) {
	for _, e := range entries {
		if e.Pattern != "" {
			patterns = append(patterns, e.Pattern)
		} else if e.Domain != "" {
			domains = append(domains, e.Domain)
		}
	}
	return domains, patterns
}

// buildGlobalPolicy constructs a ProxyPolicy from global config and decisions,
// including the DefaultAllowedDomains in the allow set.
func buildGlobalPolicy(cfg *config.GlobalConfig, decisions *config.Decisions) ProxyPolicy {
	allowDomains := make([]string, 0, len(DefaultAllowedDomains))
	allowDomains = append(allowDomains, DefaultAllowedDomains...)
	var allowPatterns, denyDomains, denyPatterns []string

	if cfg != nil {
		d, p := splitEntries(cfg.Proxy.Allow)
		allowDomains = append(allowDomains, d...)
		allowPatterns = append(allowPatterns, p...)
		d, p = splitEntries(cfg.Proxy.Deny)
		denyDomains = append(denyDomains, d...)
		denyPatterns = append(denyPatterns, p...)
	}

	if decisions != nil {
		d, p := splitEntries(decisions.Proxy.Allow)
		allowDomains = append(allowDomains, d...)
		allowPatterns = append(allowPatterns, p...)
		d, p = splitEntries(decisions.Proxy.Deny)
		denyDomains = append(denyDomains, d...)
		denyPatterns = append(denyPatterns, p...)
	}

	return ProxyPolicy{
		Allow: NewDomainSet(allowDomains, allowPatterns),
		Deny:  NewDomainSet(denyDomains, denyPatterns),
	}
}

// loadProjectPolicy loads a project's config and decisions and merges them
// into a ProxyPolicy.
func loadProjectPolicy(
	name string,
	cfgLoader func(string) (*config.ProjectConfig, error),
	decLoader func(string) (*config.Decisions, error),
) (*ProxyPolicy, error) {
	var allowDomains, allowPatterns, denyDomains, denyPatterns []string

	if cfgLoader != nil {
		cfg, err := cfgLoader(name)
		if err != nil {
			return nil, err
		}
		if cfg != nil {
			d, p := splitEntries(cfg.Proxy.Allow)
			allowDomains = append(allowDomains, d...)
			allowPatterns = append(allowPatterns, p...)
			d, p = splitEntries(cfg.Proxy.Deny)
			denyDomains = append(denyDomains, d...)
			denyPatterns = append(denyPatterns, p...)
		}
	}

	if decLoader != nil {
		dec, err := decLoader(name)
		if err != nil {
			return nil, err
		}
		if dec != nil {
			d, p := splitEntries(dec.Proxy.Allow)
			allowDomains = append(allowDomains, d...)
			allowPatterns = append(allowPatterns, p...)
			d, p = splitEntries(dec.Proxy.Deny)
			denyDomains = append(denyDomains, d...)
			denyPatterns = append(denyPatterns, p...)
		}
	}

	return &ProxyPolicy{
		Allow: NewDomainSet(allowDomains, allowPatterns),
		Deny:  NewDomainSet(denyDomains, denyPatterns),
	}, nil
}
