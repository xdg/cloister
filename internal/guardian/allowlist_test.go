package guardian

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/config"
)

func TestAllowlist_IsAllowed(t *testing.T) {
	tests := []struct {
		name     string
		domains  []string
		host     string
		expected bool
	}{
		// Exact matches
		{
			name:     "exact match allowed",
			domains:  []string{"api.anthropic.com"},
			host:     "api.anthropic.com",
			expected: true,
		},
		{
			name:     "exact match with port allowed",
			domains:  []string{"api.anthropic.com"},
			host:     "api.anthropic.com:443",
			expected: true,
		},
		{
			name:     "exact match with custom port allowed",
			domains:  []string{"api.anthropic.com"},
			host:     "api.anthropic.com:8443",
			expected: true,
		},

		// Not in allowlist
		{
			name:     "domain not in allowlist",
			domains:  []string{"api.anthropic.com"},
			host:     "github.com",
			expected: false,
		},
		{
			name:     "domain not in allowlist with port",
			domains:  []string{"api.anthropic.com"},
			host:     "github.com:443",
			expected: false,
		},

		// Subdomain handling (exact match, no wildcards)
		{
			name:     "subdomain not matched by parent",
			domains:  []string{"anthropic.com"},
			host:     "api.anthropic.com",
			expected: false,
		},
		{
			name:     "parent domain not matched by subdomain",
			domains:  []string{"api.anthropic.com"},
			host:     "anthropic.com",
			expected: false,
		},

		// Multiple domains
		{
			name:     "first of multiple domains",
			domains:  []string{"api.anthropic.com", "api.openai.com"},
			host:     "api.anthropic.com",
			expected: true,
		},
		{
			name:     "second of multiple domains",
			domains:  []string{"api.anthropic.com", "api.openai.com"},
			host:     "api.openai.com",
			expected: true,
		},
		{
			name:     "neither of multiple domains",
			domains:  []string{"api.anthropic.com", "api.openai.com"},
			host:     "github.com",
			expected: false,
		},

		// Empty allowlist
		{
			name:     "empty allowlist denies all",
			domains:  []string{},
			host:     "api.anthropic.com",
			expected: false,
		},

		// Edge cases
		{
			name:     "empty host denied",
			domains:  []string{"api.anthropic.com"},
			host:     "",
			expected: false,
		},
		{
			name:     "host with only port",
			domains:  []string{"api.anthropic.com"},
			host:     ":443",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			al := NewAllowlist(tc.domains)
			result := al.IsAllowed(tc.host)
			if result != tc.expected {
				t.Errorf("IsAllowed(%q) = %v, expected %v", tc.host, result, tc.expected)
			}
		})
	}
}

func TestNewDefaultAllowlist(t *testing.T) {
	al := NewDefaultAllowlist()

	// Verify default domains are present
	defaultDomains := []string{
		"api.anthropic.com",
		"api.openai.com",
		"generativelanguage.googleapis.com",
		"proxy.golang.org",
		"sum.golang.org",
		"storage.googleapis.com",
	}

	for _, domain := range defaultDomains {
		if !al.IsAllowed(domain) {
			t.Errorf("default allowlist should allow %s", domain)
		}
	}

	// Verify non-default domains are blocked
	blockedDomains := []string{
		"github.com",
		"google.com",
		"evil.com",
	}

	for _, domain := range blockedDomains {
		if al.IsAllowed(domain) {
			t.Errorf("default allowlist should block %s", domain)
		}
	}
}

func TestStripPort(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"api.anthropic.com:443", "api.anthropic.com"},
		{"api.anthropic.com:8080", "api.anthropic.com"},
		{"api.anthropic.com", "api.anthropic.com"},
		{"localhost:3000", "localhost"},
		{"localhost", "localhost"},
		{":443", ""},
		{"", ""},
		// IPv6 with port
		{"[::1]:443", "::1"},
		// IPv6 without port
		{"::1", "::1"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := stripPort(tc.input)
			if result != tc.expected {
				t.Errorf("stripPort(%q) = %q, expected %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestProxyServer_AllowlistEnforcement(t *testing.T) {
	// Start a mock upstream server
	echoHandler := func(conn net.Conn) {
		buf := make([]byte, 1024)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			_, _ = conn.Write(buf[:n])
		}
	}
	upstreamAddr, cleanupUpstream := startMockUpstream(t, echoHandler)
	defer cleanupUpstream()

	// Extract host from upstream address
	upstreamHost, _, _ := net.SplitHostPort(upstreamAddr)

	// Create proxy with allowlist that includes the mock upstream
	// We'll add standard domains to the allowlist for testing
	p := NewProxyServer(":0")
	p.Allowlist = NewAllowlist([]string{
		upstreamHost, // Mock upstream for successful connection tests
		"api.anthropic.com",
		"api.openai.com",
		"generativelanguage.googleapis.com",
	})

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	addr := p.ListenAddr()

	tests := []struct {
		name           string
		host           string
		expectedStatus int
	}{
		// Allowed domains - use mock upstream for actual connection
		{
			name:           "api.anthropic.com allowed",
			host:           upstreamAddr, // Use mock upstream
			expectedStatus: http.StatusOK,
		},
		{
			name:           "api.openai.com allowed",
			host:           upstreamAddr, // Use mock upstream
			expectedStatus: http.StatusOK,
		},
		{
			name:           "generativelanguage.googleapis.com allowed",
			host:           upstreamAddr, // Use mock upstream
			expectedStatus: http.StatusOK,
		},

		// Blocked domains
		{
			name:           "github.com blocked",
			host:           "github.com:443",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "google.com blocked",
			host:           "google.com:443",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "evil.com blocked",
			host:           "evil.com:443",
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodConnect, fmt.Sprintf("http://%s", addr), nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}
			req.Host = tc.host

			client := noProxyClient()
			client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			}

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.expectedStatus {
				body := make([]byte, 1024)
				n, _ := resp.Body.Read(body)
				t.Errorf("expected status %d, got %d (body: %s)", tc.expectedStatus, resp.StatusCode, string(body[:n]))
			}
		})
	}
}

func TestProxyServer_NilAllowlist(t *testing.T) {
	p := NewProxyServer(":0")
	p.Allowlist = nil // Explicitly set to nil

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	addr := p.ListenAddr()

	// With nil allowlist, all domains should be blocked
	req, err := http.NewRequest(http.MethodConnect, fmt.Sprintf("http://%s", addr), nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Host = "api.anthropic.com:443"

	client := noProxyClient()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected status 403 with nil allowlist, got %d", resp.StatusCode)
	}
}

func TestNewAllowlistFromConfig(t *testing.T) {
	entries := []config.AllowEntry{
		{Domain: "api.anthropic.com"},
		{Domain: "api.openai.com"},
		{Domain: "example.com"},
	}

	al := NewAllowlistFromConfig(entries)

	// All domains from config should be allowed
	for _, entry := range entries {
		if !al.IsAllowed(entry.Domain) {
			t.Errorf("domain %q should be allowed", entry.Domain)
		}
	}

	// Domains not in config should be blocked
	if al.IsAllowed("github.com") {
		t.Error("github.com should not be allowed")
	}
}

func TestNewAllowlistFromConfig_Empty(t *testing.T) {
	al := NewAllowlistFromConfig(nil)

	if al.IsAllowed("api.anthropic.com") {
		t.Error("empty config should not allow any domains")
	}
}

func TestAllowlist_Add(t *testing.T) {
	al := NewAllowlist([]string{"example.com"})

	// Verify initial state
	if !al.IsAllowed("example.com") {
		t.Error("example.com should be allowed initially")
	}
	if al.IsAllowed("api.anthropic.com") {
		t.Error("api.anthropic.com should not be allowed initially")
	}

	// Add new domains
	al.Add([]string{"api.anthropic.com", "api.openai.com"})

	// Verify all domains are now allowed
	if !al.IsAllowed("example.com") {
		t.Error("example.com should still be allowed")
	}
	if !al.IsAllowed("api.anthropic.com") {
		t.Error("api.anthropic.com should now be allowed")
	}
	if !al.IsAllowed("api.openai.com") {
		t.Error("api.openai.com should now be allowed")
	}
}

func TestAllowlist_Add_Empty(t *testing.T) {
	al := NewAllowlist([]string{"example.com"})
	al.Add(nil)

	// Should still work after adding empty slice
	if !al.IsAllowed("example.com") {
		t.Error("example.com should still be allowed")
	}
}

func TestAllowlist_Replace(t *testing.T) {
	al := NewAllowlist([]string{"example.com", "old.com"})

	// Verify initial state
	if !al.IsAllowed("example.com") {
		t.Error("example.com should be allowed initially")
	}
	if !al.IsAllowed("old.com") {
		t.Error("old.com should be allowed initially")
	}

	// Replace with new domains
	al.Replace([]string{"new.com", "api.anthropic.com"})

	// Old domains should no longer be allowed
	if al.IsAllowed("example.com") {
		t.Error("example.com should no longer be allowed after replace")
	}
	if al.IsAllowed("old.com") {
		t.Error("old.com should no longer be allowed after replace")
	}

	// New domains should be allowed
	if !al.IsAllowed("new.com") {
		t.Error("new.com should be allowed after replace")
	}
	if !al.IsAllowed("api.anthropic.com") {
		t.Error("api.anthropic.com should be allowed after replace")
	}
}

func TestAllowlist_Replace_Empty(t *testing.T) {
	al := NewAllowlist([]string{"example.com"})
	al.Replace(nil)

	// All domains should be blocked after replacing with empty
	if al.IsAllowed("example.com") {
		t.Error("example.com should not be allowed after replace with empty")
	}
}

func TestAllowlist_Domains(t *testing.T) {
	original := []string{"example.com", "api.anthropic.com", "api.openai.com"}
	al := NewAllowlist(original)

	domains := al.Domains()

	// Should have same number of domains
	if len(domains) != len(original) {
		t.Errorf("expected %d domains, got %d", len(original), len(domains))
	}

	// Sort both for comparison
	sort.Strings(original)
	sort.Strings(domains)

	for i, d := range original {
		if domains[i] != d {
			t.Errorf("domain mismatch at %d: expected %q, got %q", i, d, domains[i])
		}
	}
}
