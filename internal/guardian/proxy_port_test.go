package guardian

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestProxyPortHandling_StaticAllowlist verifies that domains in the static
// allowlist match CONNECT requests regardless of the port specified.
// This tests the current behavior where IsAllowed strips ports internally.
func TestProxyPortHandling_StaticAllowlist(t *testing.T) {
	// Create allowlist with domain (no port)
	allowlist := NewAllowlist([]string{"api.example.com"})

	proxy := &ProxyServer{
		Allowlist: allowlist,
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
			// Create a CONNECT request with the host:port
			req := httptest.NewRequest(http.MethodConnect, "https://example.com", nil)
			req.Host = tt.connectHost

			// Check if allowlist matches (simulating proxy.handleConnect logic)
			_, _, _, _ = proxy.resolveRequest(req)
			matched := proxy.Allowlist.IsAllowed(tt.connectHost)

			if matched != tt.shouldMatch {
				t.Errorf("IsAllowed(%q) = %v, want %v", tt.connectHost, matched, tt.shouldMatch)
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
		approveFunc: func(project, cloister, domain, token string) (DomainApprovalResult, error) {
			capturedDomain = domain
			return DomainApprovalResult{Approved: false}, nil // Deny to avoid connection attempt
		},
	}

	// Create proxy with empty allowlist and mock approver
	proxy := &ProxyServer{
		Allowlist:      NewAllowlist([]string{}),
		DomainApprover: approver,
	}

	// Create CONNECT request for api.example.com:443
	req := httptest.NewRequest(http.MethodConnect, "https://example.com", nil)
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
