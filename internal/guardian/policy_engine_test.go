package guardian

import (
	"fmt"
	"os"
	"testing"

	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/token"
)

func TestDecisionString(t *testing.T) {
	tests := []struct {
		d    Decision
		want string
	}{
		{Allow, "Allow"},
		{Deny, "Deny"},
		{AskHuman, "AskHuman"},
		{Decision(99), "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.d.String(); got != tt.want {
			t.Errorf("Decision(%d).String() = %q, want %q", int(tt.d), got, tt.want)
		}
	}
}

func TestProxyPolicyIsAllowed(t *testing.T) {
	allow := NewDomainSet([]string{"example.com", "api.test.io"}, []string{"*.cdn.example.com"})
	policy := &ProxyPolicy{Allow: allow}

	tests := []struct {
		domain string
		want   bool
	}{
		{"example.com", true},
		{"api.test.io", true},
		{"api.test.io:443", true},
		{"img.cdn.example.com", true},
		{"notallowed.com", false},
		{"cdn.example.com", false}, // base domain doesn't match *.cdn.example.com
	}
	for _, tt := range tests {
		if got := policy.IsAllowed(tt.domain); got != tt.want {
			t.Errorf("IsAllowed(%q) = %v, want %v", tt.domain, got, tt.want)
		}
	}
}

func TestProxyPolicyIsDenied(t *testing.T) {
	deny := NewDomainSet([]string{"evil.com"}, []string{"*.malware.net"})
	policy := &ProxyPolicy{Deny: deny}

	tests := []struct {
		domain string
		want   bool
	}{
		{"evil.com", true},
		{"evil.com:8080", true},
		{"sub.malware.net", true},
		{"safe.com", false},
		{"malware.net", false}, // base domain doesn't match *.malware.net
	}
	for _, tt := range tests {
		if got := policy.IsDenied(tt.domain); got != tt.want {
			t.Errorf("IsDenied(%q) = %v, want %v", tt.domain, got, tt.want)
		}
	}
}

func TestProxyPolicyNilSafety(t *testing.T) {
	// Nil receiver
	var nilPolicy *ProxyPolicy
	if nilPolicy.IsAllowed("example.com") {
		t.Error("nil ProxyPolicy.IsAllowed should return false")
	}
	if nilPolicy.IsDenied("example.com") {
		t.Error("nil ProxyPolicy.IsDenied should return false")
	}

	// Nil Allow field
	policyNilAllow := &ProxyPolicy{Allow: nil, Deny: NewDomainSet([]string{"evil.com"}, nil)}
	if policyNilAllow.IsAllowed("example.com") {
		t.Error("ProxyPolicy with nil Allow.IsAllowed should return false")
	}
	if !policyNilAllow.IsDenied("evil.com") {
		t.Error("ProxyPolicy with non-nil Deny.IsDenied should return true for matching domain")
	}

	// Nil Deny field
	policyNilDeny := &ProxyPolicy{Allow: NewDomainSet([]string{"example.com"}, nil), Deny: nil}
	if policyNilDeny.IsDenied("example.com") {
		t.Error("ProxyPolicy with nil Deny.IsDenied should return false")
	}
	if !policyNilDeny.IsAllowed("example.com") {
		t.Error("ProxyPolicy with non-nil Allow.IsAllowed should return true for matching domain")
	}
}

func TestProxyPolicyBothSets(t *testing.T) {
	allow := NewDomainSet([]string{"example.com", "both.com"}, nil)
	deny := NewDomainSet([]string{"evil.com", "both.com"}, nil)
	policy := &ProxyPolicy{Allow: allow, Deny: deny}

	// IsAllowed and IsDenied are independent — both can return true for same domain
	if !policy.IsAllowed("example.com") {
		t.Error("expected IsAllowed true for example.com")
	}
	if policy.IsDenied("example.com") {
		t.Error("expected IsDenied false for example.com")
	}

	if policy.IsAllowed("evil.com") {
		t.Error("expected IsAllowed false for evil.com")
	}
	if !policy.IsDenied("evil.com") {
		t.Error("expected IsDenied true for evil.com")
	}

	// Domain in both sets: both methods return true independently
	if !policy.IsAllowed("both.com") {
		t.Error("expected IsAllowed true for both.com (in both sets)")
	}
	if !policy.IsDenied("both.com") {
		t.Error("expected IsDenied true for both.com (in both sets)")
	}

	// Domain in neither set
	if policy.IsAllowed("unknown.com") {
		t.Error("expected IsAllowed false for unknown.com")
	}
	if policy.IsDenied("unknown.com") {
		t.Error("expected IsDenied false for unknown.com")
	}
}

// sliceProjectLister is a simple ProjectLister for tests.
type sliceProjectLister struct {
	names []string
}

func (s *sliceProjectLister) List() map[string]token.Info {
	m := make(map[string]token.Info, len(s.names))
	for i, name := range s.names {
		tok := fmt.Sprintf("fake-token-%d", i)
		m[tok] = token.Info{ProjectName: name}
	}
	return m
}

// newTestPolicyEngine builds a PolicyEngine with manually set fields for testing.
// This avoids needing real config loaders.
func newTestPolicyEngine(global ProxyPolicy, projects, tokens map[string]*ProxyPolicy) *PolicyEngine {
	if projects == nil {
		projects = make(map[string]*ProxyPolicy)
	}
	if tokens == nil {
		tokens = make(map[string]*ProxyPolicy)
	}
	return &PolicyEngine{
		global:   global,
		projects: projects,
		tokens:   tokens,
	}
}

func TestPolicyEngine_Check_GlobalAllow(t *testing.T) {
	pe := newTestPolicyEngine(
		ProxyPolicy{Allow: NewDomainSet([]string{"example.com"}, nil)},
		nil, nil,
	)
	if got := pe.Check("tok", "proj", "example.com"); got != Allow {
		t.Errorf("Check = %v, want Allow", got)
	}
}

func TestPolicyEngine_Check_GlobalDeny(t *testing.T) {
	pe := newTestPolicyEngine(
		ProxyPolicy{Deny: NewDomainSet([]string{"evil.com"}, nil)},
		nil, nil,
	)
	if got := pe.Check("tok", "proj", "evil.com"); got != Deny {
		t.Errorf("Check = %v, want Deny", got)
	}
}

func TestPolicyEngine_Check_DenyBeatsAllow(t *testing.T) {
	// Domain in both global allow AND global deny -> Deny (deny pass runs first).
	pe := newTestPolicyEngine(
		ProxyPolicy{
			Allow: NewDomainSet([]string{"both.com"}, nil),
			Deny:  NewDomainSet([]string{"both.com"}, nil),
		},
		nil, nil,
	)
	if got := pe.Check("tok", "proj", "both.com"); got != Deny {
		t.Errorf("Check = %v, want Deny", got)
	}
}

func TestPolicyEngine_Check_GlobalDenyBeatsProjectAllow(t *testing.T) {
	pe := newTestPolicyEngine(
		ProxyPolicy{Deny: NewDomainSet([]string{"blocked.com"}, nil)},
		map[string]*ProxyPolicy{
			"myproj": {Allow: NewDomainSet([]string{"blocked.com"}, nil)},
		},
		nil,
	)
	if got := pe.Check("tok", "myproj", "blocked.com"); got != Deny {
		t.Errorf("Check = %v, want Deny", got)
	}
}

func TestPolicyEngine_Check_TokenDenyBeatsEverything(t *testing.T) {
	pe := newTestPolicyEngine(
		ProxyPolicy{Allow: NewDomainSet([]string{"site.com"}, nil)},
		map[string]*ProxyPolicy{
			"proj": {Allow: NewDomainSet([]string{"site.com"}, nil)},
		},
		map[string]*ProxyPolicy{
			"tok1": {Deny: NewDomainSet([]string{"site.com"}, nil)},
		},
	)
	if got := pe.Check("tok1", "proj", "site.com"); got != Deny {
		t.Errorf("Check = %v, want Deny", got)
	}
}

func TestPolicyEngine_Check_ProjectAllow(t *testing.T) {
	pe := newTestPolicyEngine(
		ProxyPolicy{}, // global has nothing
		map[string]*ProxyPolicy{
			"proj": {Allow: NewDomainSet([]string{"project-only.com"}, nil)},
		},
		nil,
	)
	if got := pe.Check("tok", "proj", "project-only.com"); got != Allow {
		t.Errorf("Check = %v, want Allow", got)
	}
}

func TestPolicyEngine_Check_TokenAllow(t *testing.T) {
	pe := newTestPolicyEngine(
		ProxyPolicy{}, // global has nothing
		nil,
		map[string]*ProxyPolicy{
			"tok1": {Allow: NewDomainSet([]string{"session-only.com"}, nil)},
		},
	)
	if got := pe.Check("tok1", "proj", "session-only.com"); got != Allow {
		t.Errorf("Check = %v, want Allow", got)
	}
}

func TestPolicyEngine_Check_AskHuman(t *testing.T) {
	pe := newTestPolicyEngine(ProxyPolicy{}, nil, nil)
	if got := pe.Check("tok", "proj", "unknown.com"); got != AskHuman {
		t.Errorf("Check = %v, want AskHuman", got)
	}
}

func TestPolicyEngine_Check_MissingProject(t *testing.T) {
	// Unknown project name should be skipped gracefully.
	pe := newTestPolicyEngine(ProxyPolicy{}, nil, nil)
	if got := pe.Check("tok", "no-such-project", "example.com"); got != AskHuman {
		t.Errorf("Check = %v, want AskHuman", got)
	}
}

func TestPolicyEngine_Check_MissingToken(t *testing.T) {
	// Unknown token should be skipped gracefully.
	pe := newTestPolicyEngine(ProxyPolicy{}, nil, nil)
	if got := pe.Check("no-such-token", "proj", "example.com"); got != AskHuman {
		t.Errorf("Check = %v, want AskHuman", got)
	}
}

func TestNewPolicyEngine(t *testing.T) {
	cfg := &config.GlobalConfig{
		Proxy: config.ProxyConfig{
			Allow: []config.AllowEntry{{Domain: "cfg-allowed.com"}},
			Deny:  []config.AllowEntry{{Domain: "cfg-denied.com"}},
		},
	}
	decisions := &config.Decisions{
		Proxy: config.DecisionsProxy{
			Allow: []config.AllowEntry{{Domain: "dec-allowed.com"}},
			Deny:  []config.AllowEntry{{Domain: "dec-denied.com"}},
		},
	}

	projectCfgLoader := func(name string) (*config.ProjectConfig, error) {
		if name == "testproj" {
			return &config.ProjectConfig{
				Proxy: config.ProjectProxyConfig{
					Allow: []config.AllowEntry{{Domain: "proj-allowed.com"}},
				},
			}, nil
		}
		return &config.ProjectConfig{}, nil
	}
	projectDecLoader := func(_ string) (*config.Decisions, error) {
		return &config.Decisions{}, nil
	}

	lister := &sliceProjectLister{names: []string{"testproj"}}

	pe, err := NewPolicyEngine(cfg, decisions, lister,
		WithProjectConfigLoader(projectCfgLoader),
		WithProjectDecisionLoader(projectDecLoader),
	)
	if err != nil {
		t.Fatalf("NewPolicyEngine error: %v", err)
	}

	// Global allow from config.
	if got := pe.Check("tok", "other", "cfg-allowed.com"); got != Allow {
		t.Errorf("cfg-allowed.com: got %v, want Allow", got)
	}
	// Global allow from decisions.
	if got := pe.Check("tok", "other", "dec-allowed.com"); got != Allow {
		t.Errorf("dec-allowed.com: got %v, want Allow", got)
	}
	// Global deny from config.
	if got := pe.Check("tok", "other", "cfg-denied.com"); got != Deny {
		t.Errorf("cfg-denied.com: got %v, want Deny", got)
	}
	// Global deny from decisions.
	if got := pe.Check("tok", "other", "dec-denied.com"); got != Deny {
		t.Errorf("dec-denied.com: got %v, want Deny", got)
	}
	// Default allowed domain.
	if got := pe.Check("tok", "other", "api.anthropic.com"); got != Allow {
		t.Errorf("api.anthropic.com (default): got %v, want Allow", got)
	}
	// Project allow.
	if got := pe.Check("tok", "testproj", "proj-allowed.com"); got != Allow {
		t.Errorf("proj-allowed.com: got %v, want Allow", got)
	}
	// Unknown domain -> AskHuman.
	if got := pe.Check("tok", "testproj", "random.com"); got != AskHuman {
		t.Errorf("random.com: got %v, want AskHuman", got)
	}
}

func TestPolicyEngine_RecordDecision_Session(t *testing.T) {
	pe := newTestPolicyEngine(ProxyPolicy{}, nil, nil)

	// Before recording, domain is unknown.
	if got := pe.Check("tok1", "proj", "new-domain.com"); got != AskHuman {
		t.Fatalf("before record: got %v, want AskHuman", got)
	}

	// Record an allow at session scope.
	if err := pe.RecordDecision(RecordDecisionParams{Token: "tok1", Project: "proj", Domain: "new-domain.com", Scope: ScopeSession, Allowed: true}); err != nil {
		t.Fatalf("RecordDecision: %v", err)
	}

	// Now Check should return Allow for that token.
	if got := pe.Check("tok1", "proj", "new-domain.com"); got != Allow {
		t.Errorf("after record: got %v, want Allow", got)
	}

	// Different token should still be AskHuman.
	if got := pe.Check("tok2", "proj", "new-domain.com"); got != AskHuman {
		t.Errorf("other token: got %v, want AskHuman", got)
	}
}

func TestPolicyEngine_RecordDecision_Session_Deny(t *testing.T) {
	// Start with global allow so we can verify session deny overrides it.
	pe := newTestPolicyEngine(
		ProxyPolicy{Allow: NewDomainSet([]string{"allowed.com"}, nil)},
		nil, nil,
	)

	if got := pe.Check("tok1", "proj", "allowed.com"); got != Allow {
		t.Fatalf("before deny: got %v, want Allow", got)
	}

	// Record a deny at session scope.
	if err := pe.RecordDecision(RecordDecisionParams{Token: "tok1", Project: "proj", Domain: "allowed.com", Scope: ScopeSession}); err != nil {
		t.Fatalf("RecordDecision: %v", err)
	}

	// Token's deny should override the global allow.
	if got := pe.Check("tok1", "proj", "allowed.com"); got != Deny {
		t.Errorf("after deny: got %v, want Deny", got)
	}

	// Different token still gets the global allow.
	if got := pe.Check("tok2", "proj", "allowed.com"); got != Allow {
		t.Errorf("other token: got %v, want Allow", got)
	}
}

func TestPolicyEngine_RecordDecision_Once(t *testing.T) {
	pe := newTestPolicyEngine(ProxyPolicy{}, nil, nil)

	// ScopeOnce should be a no-op.
	if err := pe.RecordDecision(RecordDecisionParams{Token: "tok1", Project: "proj", Domain: "once-domain.com", Scope: ScopeOnce, Allowed: true}); err != nil {
		t.Fatalf("RecordDecision: %v", err)
	}

	// Check should still return AskHuman since nothing was persisted.
	if got := pe.Check("tok1", "proj", "once-domain.com"); got != AskHuman {
		t.Errorf("after once: got %v, want AskHuman", got)
	}
}

// setupXDGTempDir creates a temp directory and sets XDG_CONFIG_HOME to redirect
// config paths during tests. Returns a cleanup function.
func setupXDGTempDir(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func TestPolicyEngine_RecordDecision_Project(t *testing.T) {
	setupXDGTempDir(t)

	projectCfgLoader := func(_ string) (*config.ProjectConfig, error) {
		return &config.ProjectConfig{}, nil
	}
	projectDecLoader := config.LoadProjectDecisions

	pe, err := NewPolicyEngine(
		&config.GlobalConfig{}, &config.Decisions{}, nil,
		WithProjectConfigLoader(projectCfgLoader),
		WithProjectDecisionLoader(projectDecLoader),
	)
	if err != nil {
		t.Fatalf("NewPolicyEngine: %v", err)
	}

	// Before recording, domain is unknown.
	if got := pe.Check("tok1", "testproj", "persisted.com"); got != AskHuman {
		t.Fatalf("before record: got %v, want AskHuman", got)
	}

	// Record an allow at project scope.
	if err := pe.RecordDecision(RecordDecisionParams{Token: "tok1", Project: "testproj", Domain: "persisted.com", Scope: ScopeProject, Allowed: true}); err != nil {
		t.Fatalf("RecordDecision: %v", err)
	}

	// Check should now return Allow (ReloadProject was called internally).
	if got := pe.Check("tok1", "testproj", "persisted.com"); got != Allow {
		t.Errorf("after record: got %v, want Allow", got)
	}

	// Verify the file was actually written.
	path := config.ProjectDecisionPath("testproj")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read decisions file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("decisions file is empty")
	}

	// Record a deny at project scope.
	if err := pe.RecordDecision(RecordDecisionParams{Token: "tok1", Project: "testproj", Domain: "denied.com", Scope: ScopeProject}); err != nil {
		t.Fatalf("RecordDecision deny: %v", err)
	}

	if got := pe.Check("tok1", "testproj", "denied.com"); got != Deny {
		t.Errorf("after deny record: got %v, want Deny", got)
	}

	// Verify dedup: recording same domain again should not error.
	if err := pe.RecordDecision(RecordDecisionParams{Token: "tok1", Project: "testproj", Domain: "persisted.com", Scope: ScopeProject, Allowed: true}); err != nil {
		t.Fatalf("RecordDecision dedup: %v", err)
	}
}

func TestPolicyEngine_RecordDecision_Global(t *testing.T) {
	setupXDGTempDir(t)

	cfgLoader := func() (*config.GlobalConfig, error) {
		return &config.GlobalConfig{}, nil
	}
	decLoader := config.LoadGlobalDecisions

	pe, err := NewPolicyEngine(
		&config.GlobalConfig{}, &config.Decisions{}, nil,
		WithConfigLoader(cfgLoader),
		WithDecisionLoader(decLoader),
	)
	if err != nil {
		t.Fatalf("NewPolicyEngine: %v", err)
	}

	// Before recording, domain is unknown (not in defaults either).
	if got := pe.Check("tok1", "proj", "global-new.com"); got != AskHuman {
		t.Fatalf("before record: got %v, want AskHuman", got)
	}

	// Record an allow at global scope.
	if err := pe.RecordDecision(RecordDecisionParams{Token: "tok1", Project: "proj", Domain: "global-new.com", Scope: ScopeGlobal, Allowed: true}); err != nil {
		t.Fatalf("RecordDecision: %v", err)
	}

	// Check should now return Allow (ReloadGlobal was called internally).
	if got := pe.Check("tok1", "proj", "global-new.com"); got != Allow {
		t.Errorf("after record: got %v, want Allow", got)
	}

	// Verify the file was actually written.
	path := config.GlobalDecisionPath()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read decisions file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("decisions file is empty")
	}

	// Record a deny at global scope.
	if err := pe.RecordDecision(RecordDecisionParams{Token: "tok1", Project: "proj", Domain: "global-denied.com", Scope: ScopeGlobal}); err != nil {
		t.Fatalf("RecordDecision deny: %v", err)
	}

	if got := pe.Check("tok1", "proj", "global-denied.com"); got != Deny {
		t.Errorf("after deny record: got %v, want Deny", got)
	}
}

func TestPolicyEngine_RevokeToken(t *testing.T) {
	pe := newTestPolicyEngine(ProxyPolicy{}, nil, nil)

	// Add a session decision.
	if err := pe.RecordDecision(RecordDecisionParams{Token: "tok1", Project: "proj", Domain: "session-domain.com", Scope: ScopeSession, Allowed: true}); err != nil {
		t.Fatalf("RecordDecision: %v", err)
	}

	// Verify it's visible.
	if got := pe.Check("tok1", "proj", "session-domain.com"); got != Allow {
		t.Fatalf("before revoke: got %v, want Allow", got)
	}

	// Revoke the token.
	pe.RevokeToken("tok1")

	// Check should no longer see the session decision.
	if got := pe.Check("tok1", "proj", "session-domain.com"); got != AskHuman {
		t.Errorf("after revoke: got %v, want AskHuman", got)
	}
}

func TestPolicyEngine_ReloadAll(t *testing.T) {
	setupXDGTempDir(t)

	// Start with a config that allows "initial.com".
	globalCfg := &config.GlobalConfig{
		Proxy: config.ProxyConfig{
			Allow: []config.AllowEntry{{Domain: "initial.com"}},
		},
	}

	projectCfgLoader := func(name string) (*config.ProjectConfig, error) {
		if name == "myproj" {
			return &config.ProjectConfig{
				Proxy: config.ProjectProxyConfig{
					Allow: []config.AllowEntry{{Domain: "proj-initial.com"}},
				},
			}, nil
		}
		return &config.ProjectConfig{}, nil
	}
	projectDecLoader := func(_ string) (*config.Decisions, error) {
		return &config.Decisions{}, nil
	}

	lister := &sliceProjectLister{names: []string{"myproj"}}

	// Track which config is returned (mutable for reload testing).
	currentCfg := globalCfg
	cfgLoader := func() (*config.GlobalConfig, error) {
		return currentCfg, nil
	}
	decLoader := func() (*config.Decisions, error) {
		return &config.Decisions{}, nil
	}

	pe, err := NewPolicyEngine(
		globalCfg, &config.Decisions{}, lister,
		WithConfigLoader(cfgLoader),
		WithDecisionLoader(decLoader),
		WithProjectConfigLoader(projectCfgLoader),
		WithProjectDecisionLoader(projectDecLoader),
	)
	if err != nil {
		t.Fatalf("NewPolicyEngine: %v", err)
	}

	// Verify initial state.
	if got := pe.Check("tok", "myproj", "initial.com"); got != Allow {
		t.Errorf("initial.com before reload: got %v, want Allow", got)
	}
	if got := pe.Check("tok", "myproj", "proj-initial.com"); got != Allow {
		t.Errorf("proj-initial.com before reload: got %v, want Allow", got)
	}
	if got := pe.Check("tok", "myproj", "added-later.com"); got != AskHuman {
		t.Errorf("added-later.com before reload: got %v, want AskHuman", got)
	}

	// Change the config to include a new domain.
	currentCfg = &config.GlobalConfig{
		Proxy: config.ProxyConfig{
			Allow: []config.AllowEntry{
				{Domain: "initial.com"},
				{Domain: "added-later.com"},
			},
		},
	}

	// ReloadAll should pick up the change.
	if err := pe.ReloadAll(); err != nil {
		t.Fatalf("ReloadAll: %v", err)
	}

	if got := pe.Check("tok", "myproj", "added-later.com"); got != Allow {
		t.Errorf("added-later.com after reload: got %v, want Allow", got)
	}
	// Project policy should still work.
	if got := pe.Check("tok", "myproj", "proj-initial.com"); got != Allow {
		t.Errorf("proj-initial.com after reload: got %v, want Allow", got)
	}
}

func TestPolicyEngine_EnsureProject(t *testing.T) {
	projectCfgLoader := func(name string) (*config.ProjectConfig, error) {
		if name == "late-project" {
			return &config.ProjectConfig{
				Proxy: config.ProjectProxyConfig{
					Allow: []config.AllowEntry{{Domain: "late-domain.com"}},
				},
			}, nil
		}
		return &config.ProjectConfig{}, nil
	}
	projectDecLoader := func(_ string) (*config.Decisions, error) {
		return &config.Decisions{}, nil
	}

	pe, err := NewPolicyEngine(
		&config.GlobalConfig{}, &config.Decisions{}, nil,
		WithProjectConfigLoader(projectCfgLoader),
		WithProjectDecisionLoader(projectDecLoader),
	)
	if err != nil {
		t.Fatalf("NewPolicyEngine: %v", err)
	}

	// Before EnsureProject, project domain is not loaded.
	if got := pe.Check("tok", "late-project", "late-domain.com"); got != AskHuman {
		t.Fatalf("before EnsureProject: got %v, want AskHuman", got)
	}

	// EnsureProject loads the project policy.
	if err := pe.EnsureProject("late-project"); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	if got := pe.Check("tok", "late-project", "late-domain.com"); got != Allow {
		t.Errorf("after EnsureProject: got %v, want Allow", got)
	}

	// Calling again is a no-op (idempotent).
	if err := pe.EnsureProject("late-project"); err != nil {
		t.Fatalf("EnsureProject second call: %v", err)
	}

	// Empty name is a no-op.
	if err := pe.EnsureProject(""); err != nil {
		t.Fatalf("EnsureProject empty: %v", err)
	}
}
