package guardian

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
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
	p := NewProxyServer(":0")
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
		// Allowed domains
		{
			name:           "api.anthropic.com allowed",
			host:           "api.anthropic.com:443",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "api.openai.com allowed",
			host:           "api.openai.com:443",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "generativelanguage.googleapis.com allowed",
			host:           "generativelanguage.googleapis.com:443",
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

			client := &http.Client{
				CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, resp.StatusCode)
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

	client := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
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
