package guardian

import "testing"

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
