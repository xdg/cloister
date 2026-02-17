package guardian

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestProxyPortHandling_StaticAllowlist verifies that domains in the
// PolicyEngine's global allow list match CONNECT requests regardless of the
// port specified. DomainSet.Contains strips ports internally.
func TestProxyPortHandling_StaticAllowlist(t *testing.T) {
	// Create PolicyEngine with one allowed domain (no port).
	pe := newTestProxyPolicyEngine([]string{"api.example.com"}, nil)

	proxy := &ProxyServer{
		PolicyEngine: pe,
	}

	// Test cases for different ports
	tests := []struct {
		name        string
		connectHost string
		shouldMatch bool
	}{
		{
			name:        "standard HTTPS port",
			connectHost: "api.example.com:443",
			shouldMatch: true,
		},
		{
			name:        "non-standard port",
			connectHost: "api.example.com:8443",
			shouldMatch: true,
		},
		{
			name:        "HTTP port",
			connectHost: "api.example.com:80",
			shouldMatch: true,
		},
		{
			name:        "different domain with port",
			connectHost: "other.example.com:443",
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domain := stripPort(tt.connectHost)
			decision := proxy.PolicyEngine.Check("", "", domain)
			matched := decision == Allow
			if matched != tt.shouldMatch {
				t.Errorf("PolicyEngine.Check(%q) = %v, want shouldMatch=%v", tt.connectHost, decision, tt.shouldMatch)
			}
		})
	}
}

// TestProxyPortHandling_ApprovalQueueDomain verifies that domain approval
// requests do NOT include the port in the domain field queued for approval.
// This ensures:
// - Approval UI shows clean domain names without ports
// - Deduplication works correctly (same domain, different ports = same approval)
// - Config persistence matches allowlist behavior (domain-only storage)
func TestProxyPortHandling_ApprovalQueueDomain(t *testing.T) {
	// Capture what domain was requested for approval
	var capturedDomain string
	approver := &mockDomainApprover{
		approveFunc: func(_, _, domain, _ string) (DomainApprovalResult, error) {
			capturedDomain = domain
			return DomainApprovalResult{Approved: false}, nil // Deny to avoid connection attempt
		},
	}

	// Create proxy with empty PolicyEngine (all domains go to AskHuman) and mock approver
	pe := newTestProxyPolicyEngine(nil, nil)
	proxy := &ProxyServer{
		PolicyEngine:   pe,
		DomainApprover: approver,
	}

	// Create CONNECT request for api.example.com:443
	req := httptest.NewRequest(http.MethodConnect, "https://example.com", http.NoBody)
	req.Host = "api.example.com:443"

	// Create mock ResponseWriter
	w := httptest.NewRecorder()

	// Handle the request (will trigger approval which we deny)
	proxy.handleConnect(w, req)

	// Verify response is 403 (denied)
	if w.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden, got %d", w.Code)
	}

	// Verify domain passed to approver has port stripped
	expectedDomain := "api.example.com"
	if capturedDomain != expectedDomain {
		t.Errorf("Domain approver received %q, want %q (without port)",
			capturedDomain, expectedDomain)
	}
}
