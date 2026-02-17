package guardian

import (
	"fmt"
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
