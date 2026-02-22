package guardian

import (
	"fmt"
	"strings"
	"sync"

	"github.com/xdg/cloister/internal/config"
	tokenpkg "github.com/xdg/cloister/internal/token"
)

// ProjectLister provides the list of active projects for cache reloading.
// Satisfied by TokenRegistry and by test mocks.
type ProjectLister interface {
	List() map[string]tokenpkg.Info
}

// normalizeDomain strips the port from a domain if present and lowercases it.
// CONNECT requests include port (e.g., "example.com:443") but allowlist
// entries should store bare hostnames for consistent matching.
func normalizeDomain(domain string) string {
	return strings.ToLower(stripPort(domain))
}

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

// Scope represents the persistence scope for a domain access decision.
type Scope string

// Scope constants for domain access decisions.
const (
	ScopeOnce    Scope = "once"
	ScopeSession Scope = "session"
	ScopeProject Scope = "project"
	ScopeGlobal  Scope = "global"
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

// PolicyChecker evaluates domain access control decisions. ProxyServer
// depends on this interface rather than *PolicyEngine directly, enabling
// lightweight mocks in proxy tests.
type PolicyChecker interface {
	Check(token, project, domain string) Decision
}

// TokenRevoker clears session-level policy state for a revoked token.
type TokenRevoker interface {
	RevokeToken(token string)
}

// DecisionRecorder persists a domain access decision at the appropriate scope.
// *PolicyEngine implements this interface via its RecordDecision method.
type DecisionRecorder interface {
	RecordDecision(RecordDecisionParams) error
}

// PolicyEngine owns all domain access policy state across three tiers:
// global, per-project, and per-token (session). It evaluates domain access
// using a deny-first, then allow, then fallback-to-AskHuman strategy.
type PolicyEngine struct {
	mu       sync.RWMutex
	global   ProxyPolicy
	projects map[string]*ProxyPolicy
	tokens   map[string]*ProxyPolicy

	// Mutexes for file persistence (prevent concurrent writes).
	projectMu sync.Mutex
	globalMu  sync.Mutex

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

// ReloadGlobal re-reads the global config and decisions from disk and rebuilds
// the global ProxyPolicy. Loading happens outside the write lock; only the
// swap is protected.
func (pe *PolicyEngine) ReloadGlobal() error {
	cfg, err := pe.configLoader()
	if err != nil {
		return fmt.Errorf("reload global config: %w", err)
	}
	dec, err := pe.decisionLoader()
	if err != nil {
		return fmt.Errorf("reload global decisions: %w", err)
	}

	global := buildGlobalPolicy(cfg, dec)

	pe.mu.Lock()
	pe.global = global
	pe.mu.Unlock()

	return nil
}

// ReloadProject re-reads a project's config and decisions from disk and
// rebuilds its ProxyPolicy. Loading happens outside the write lock; only the
// swap is protected.
func (pe *PolicyEngine) ReloadProject(name string) error {
	policy, err := loadProjectPolicy(name, pe.projectConfigLoader, pe.projectDecisionLoader)
	if err != nil {
		return fmt.Errorf("reload project %q: %w", name, err)
	}

	pe.mu.Lock()
	pe.projects[name] = policy
	pe.mu.Unlock()

	return nil
}

// ReloadAll re-reads the global config/decisions and all project policies from
// disk and replaces all in-memory state. This is intended for SIGHUP handling.
// All loading happens outside the write lock to avoid holding it during I/O.
func (pe *PolicyEngine) ReloadAll() error {
	cfg, err := pe.configLoader()
	if err != nil {
		return fmt.Errorf("reload all global config: %w", err)
	}
	dec, err := pe.decisionLoader()
	if err != nil {
		return fmt.Errorf("reload all global decisions: %w", err)
	}

	global := buildGlobalPolicy(cfg, dec)

	projects := make(map[string]*ProxyPolicy)
	if pe.projectLister != nil {
		seen := make(map[string]struct{})
		for _, info := range pe.projectLister.List() {
			name := info.ProjectName
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			p, err := loadProjectPolicy(name, pe.projectConfigLoader, pe.projectDecisionLoader)
			if err != nil {
				return fmt.Errorf("reload all project %q: %w", name, err)
			}
			projects[name] = p
		}
	}

	pe.mu.Lock()
	pe.global = global
	pe.projects = projects
	pe.mu.Unlock()

	return nil
}

// RecordDecisionParams holds the parameters for RecordDecision.
type RecordDecisionParams struct {
	Token     string
	Project   string
	Domain    string
	Scope     Scope
	Allowed   bool
	IsPattern bool
}

// RecordDecision records a domain access decision at the given scope.
//   - ScopeOnce: no-op (caller already has the decision).
//   - ScopeSession: mutates the token's in-memory ProxyPolicy.
//   - ScopeProject: persists to the project decisions file, then reloads.
//   - ScopeGlobal: persists to the global decisions file, then reloads.
func (pe *PolicyEngine) RecordDecision(p RecordDecisionParams) error {
	switch p.Scope {
	case ScopeOnce:
		return nil
	case ScopeSession:
		return pe.recordSessionDecision(p.Token, p.Domain, p.Allowed, p.IsPattern)
	case ScopeProject:
		if err := pe.persistDecision(p.Project, p.Domain, p.Scope, p.Allowed, p.IsPattern); err != nil {
			return err
		}
		return pe.ReloadProject(p.Project)
	case ScopeGlobal:
		if err := pe.persistDecision(p.Project, p.Domain, p.Scope, p.Allowed, p.IsPattern); err != nil {
			return err
		}
		return pe.ReloadGlobal()
	default:
		return fmt.Errorf("unknown scope: %q", p.Scope)
	}
}

// recordSessionDecision adds a domain to the token's in-memory ProxyPolicy.
// The entire mutation is held under pe.mu.Lock to prevent races with Check.
func (pe *PolicyEngine) recordSessionDecision(token, domain string, allowed, isPattern bool) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	tp := pe.tokens[token]
	if tp == nil {
		tp = &ProxyPolicy{}
		pe.tokens[token] = tp
	}

	if allowed {
		if tp.Allow == nil {
			tp.Allow = NewDomainSet(nil, nil)
		}
		addToDomainSet(tp.Allow, domain, isPattern)
	} else {
		if tp.Deny == nil {
			tp.Deny = NewDomainSet(nil, nil)
		}
		addToDomainSet(tp.Deny, domain, isPattern)
	}
	return nil
}

// addToDomainSet adds a domain or pattern to a DomainSet.
func addToDomainSet(ds *DomainSet, value string, isPattern bool) {
	if isPattern {
		ds.AddPattern(value)
	} else {
		ds.Add(value)
	}
}

// persistDecision writes a domain or pattern to the appropriate decisions file.
// It uses the load-check-dedup-append-write pattern from ConfigPersisterImpl.
func (pe *PolicyEngine) persistDecision(project, domain string, scope Scope, allowed, isPattern bool) error {
	entry := buildEntry(domain, isPattern)

	switch scope {
	case ScopeProject:
		return pe.persistProjectDecision(project, entry, allowed)
	case ScopeGlobal:
		return pe.persistGlobalDecision(entry, allowed)
	default:
		return fmt.Errorf("persistDecision called with non-persistent scope: %q", scope)
	}
}

// buildEntry creates an AllowEntry, normalizing the domain if it is not a pattern.
func buildEntry(domain string, isPattern bool) config.AllowEntry {
	if isPattern {
		return config.AllowEntry{Pattern: domain}
	}
	return config.AllowEntry{Domain: normalizeDomain(domain)}
}

// appendEntry adds an entry to the allow or deny list if not already present.
func appendEntry(proxy *config.DecisionsProxy, entry config.AllowEntry, allowed bool) {
	if allowed {
		if !containsEntry(proxy.Allow, entry) {
			proxy.Allow = append(proxy.Allow, entry)
		}
	} else {
		if !containsEntry(proxy.Deny, entry) {
			proxy.Deny = append(proxy.Deny, entry)
		}
	}
}

func (pe *PolicyEngine) persistProjectDecision(project string, entry config.AllowEntry, allowed bool) error {
	pe.projectMu.Lock()
	defer pe.projectMu.Unlock()

	decisions, err := pe.projectDecisionLoader(project)
	if err != nil {
		return fmt.Errorf("load project decisions: %w", err)
	}

	appendEntry(&decisions.Proxy, entry, allowed)

	if err := config.WriteProjectDecisions(project, decisions); err != nil {
		return fmt.Errorf("write project decisions: %w", err)
	}
	return nil
}

func (pe *PolicyEngine) persistGlobalDecision(entry config.AllowEntry, allowed bool) error {
	pe.globalMu.Lock()
	defer pe.globalMu.Unlock()

	decisions, err := pe.decisionLoader()
	if err != nil {
		return fmt.Errorf("load global decisions: %w", err)
	}

	appendEntry(&decisions.Proxy, entry, allowed)

	if err := config.WriteGlobalDecisions(decisions); err != nil {
		return fmt.Errorf("write global decisions: %w", err)
	}
	return nil
}

// containsEntry checks if an AllowEntry already exists in the slice by
// comparing both Domain and Pattern fields.
func containsEntry(entries []config.AllowEntry, entry config.AllowEntry) bool {
	for _, e := range entries {
		if e.Domain == entry.Domain && e.Pattern == entry.Pattern {
			return true
		}
	}
	return false
}

// EnsureProject loads a project's policy if it hasn't been loaded yet.
// This is called when a new token is registered after the PolicyEngine was
// constructed, so that project-scoped allowlist entries are available
// immediately without waiting for a domain approval or SIGHUP.
func (pe *PolicyEngine) EnsureProject(name string) error {
	if name == "" {
		return nil
	}
	pe.mu.RLock()
	_, exists := pe.projects[name]
	pe.mu.RUnlock()
	if exists {
		return nil
	}
	return pe.ReloadProject(name)
}

// RevokeToken removes all session-scoped decisions for the given token.
func (pe *PolicyEngine) RevokeToken(token string) {
	pe.mu.Lock()
	delete(pe.tokens, token)
	pe.mu.Unlock()
}
