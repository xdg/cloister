package guardian

import (
	"testing"

	"github.com/xdg/cloister/internal/config"
)

func TestIsValidPattern(t *testing.T) {
	tests := []struct {
		pattern string
		valid   bool
	}{
		{"*.example.com", true},
		{"*.api.example.com", true},
		{"*.co.uk", true},
		{"*.com", true},
		{"*example.com", false},   // Missing dot after asterisk
		{"example.com", false},    // No wildcard
		{"*.", false},             // Nothing after *.
		{".", false},              // Too short
		{"", false},               // Empty
		{"**", false},             // Too short
		{"*..", false},            // Double dot after *.
		{".*.example.com", false}, // Doesn't start with *
	}

	for _, tc := range tests {
		t.Run(tc.pattern, func(t *testing.T) {
			result := IsValidPattern(tc.pattern)
			if result != tc.valid {
				t.Errorf("IsValidPattern(%q) = %v, expected %v", tc.pattern, result, tc.valid)
			}
		})
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		hostname string
		expected bool
	}{
		// Basic matching
		{
			name:     "single subdomain matches",
			pattern:  "*.example.com",
			hostname: "api.example.com",
			expected: true,
		},
		{
			name:     "different subdomain matches",
			pattern:  "*.example.com",
			hostname: "www.example.com",
			expected: true,
		},
		{
			name:     "base domain does NOT match",
			pattern:  "*.example.com",
			hostname: "example.com",
			expected: false,
		},
		{
			name:     "multi-level subdomain does NOT match",
			pattern:  "*.example.com",
			hostname: "a.b.example.com",
			expected: false,
		},
		{
			name:     "different domain does not match",
			pattern:  "*.example.com",
			hostname: "api.other.com",
			expected: false,
		},
		{
			name:     "suffix similarity not enough",
			pattern:  "*.example.com",
			hostname: "notexample.com",
			expected: false,
		},

		// Edge cases
		{
			name:     "empty hostname",
			pattern:  "*.example.com",
			hostname: "",
			expected: false,
		},
		{
			name:     "invalid pattern",
			pattern:  "example.com",
			hostname: "api.example.com",
			expected: false,
		},

		// Real-world examples
		{
			name:     "googleapis subdomain",
			pattern:  "*.googleapis.com",
			hostname: "storage.googleapis.com",
			expected: true,
		},
		{
			name:     "googleapis multi-level does not match",
			pattern:  "*.googleapis.com",
			hostname: "www.storage.googleapis.com",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := matchPattern(tc.pattern, tc.hostname)
			if result != tc.expected {
				t.Errorf("matchPattern(%q, %q) = %v, expected %v", tc.pattern, tc.hostname, result, tc.expected)
			}
		})
	}
}

func TestDomainToWildcard(t *testing.T) {
	tests := []struct {
		domain   string
		expected string
	}{
		{"api.example.com", "*.example.com"},
		{"www.google.com", "*.google.com"},
		{"storage.googleapis.com", "*.googleapis.com"},
		{"example.com", "*.com"}, // Just the TLD after the first label
		{"api.sub.example.com", "*.sub.example.com"},
		{"localhost", ""}, // No dot, no wildcard
		{"", ""},          // Empty
		{"example.", ""},  // Trailing dot only
	}

	for _, tc := range tests {
		t.Run(tc.domain, func(t *testing.T) {
			result := DomainToWildcard(tc.domain)
			if result != tc.expected {
				t.Errorf("DomainToWildcard(%q) = %q, expected %q", tc.domain, result, tc.expected)
			}
		})
	}
}

func TestAllowlist_IsAllowed_WithPatterns(t *testing.T) {
	tests := []struct {
		name     string
		domains  []string
		patterns []string
		host     string
		expected bool
	}{
		// Pattern matching
		{
			name:     "pattern matches subdomain",
			patterns: []string{"*.example.com"},
			host:     "api.example.com",
			expected: true,
		},
		{
			name:     "pattern matches subdomain with port",
			patterns: []string{"*.example.com"},
			host:     "api.example.com:443",
			expected: true,
		},
		{
			name:     "pattern does not match base domain",
			patterns: []string{"*.example.com"},
			host:     "example.com",
			expected: false,
		},
		{
			name:     "pattern does not match multi-level subdomain",
			patterns: []string{"*.example.com"},
			host:     "a.b.example.com",
			expected: false,
		},

		// Exact match takes precedence
		{
			name:     "exact match overrides pattern",
			domains:  []string{"specific.example.com"},
			patterns: []string{"*.example.com"},
			host:     "specific.example.com",
			expected: true,
		},

		// Multiple patterns
		{
			name:     "first pattern matches",
			patterns: []string{"*.example.com", "*.other.com"},
			host:     "api.example.com",
			expected: true,
		},
		{
			name:     "second pattern matches",
			patterns: []string{"*.example.com", "*.other.com"},
			host:     "api.other.com",
			expected: true,
		},
		{
			name:     "no pattern matches",
			patterns: []string{"*.example.com", "*.other.com"},
			host:     "api.third.com",
			expected: false,
		},

		// Mixed domains and patterns
		{
			name:     "domain match with patterns present",
			domains:  []string{"exact.domain.com"},
			patterns: []string{"*.example.com"},
			host:     "exact.domain.com",
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			al := NewAllowlistWithPatterns(tc.domains, tc.patterns)
			result := al.IsAllowed(tc.host)
			if result != tc.expected {
				t.Errorf("IsAllowed(%q) = %v, expected %v", tc.host, result, tc.expected)
			}
		})
	}
}

func TestNewAllowlistFromConfig_WithPatterns(t *testing.T) {
	entries := []config.AllowEntry{
		{Domain: "exact.example.com"},
		{Pattern: "*.googleapis.com"},
		{Domain: "api.anthropic.com"},
		{Pattern: "*.openai.com"},
	}

	al := NewAllowlistFromConfig(entries)

	// Test exact domains
	if !al.IsAllowed("exact.example.com") {
		t.Error("exact.example.com should be allowed")
	}
	if !al.IsAllowed("api.anthropic.com") {
		t.Error("api.anthropic.com should be allowed")
	}

	// Test pattern matches
	if !al.IsAllowed("storage.googleapis.com") {
		t.Error("storage.googleapis.com should be allowed by pattern")
	}
	if !al.IsAllowed("api.openai.com") {
		t.Error("api.openai.com should be allowed by pattern")
	}

	// Test non-matches
	if al.IsAllowed("googleapis.com") {
		t.Error("googleapis.com should not match *.googleapis.com")
	}
	if al.IsAllowed("unrelated.com") {
		t.Error("unrelated.com should not be allowed")
	}
}

func TestAllowlist_AddPatterns(t *testing.T) {
	al := NewAllowlist([]string{"exact.com"})

	// Initially pattern not allowed
	if al.IsAllowed("api.example.com") {
		t.Error("api.example.com should not be allowed initially")
	}

	// Add pattern
	al.AddPatterns([]string{"*.example.com"})

	// Now pattern should match
	if !al.IsAllowed("api.example.com") {
		t.Error("api.example.com should be allowed after AddPatterns")
	}

	// Exact domain still allowed
	if !al.IsAllowed("exact.com") {
		t.Error("exact.com should still be allowed")
	}
}

func TestAllowlist_AddPatterns_NoDuplicates(t *testing.T) {
	al := NewAllowlistWithPatterns(nil, []string{"*.example.com"})

	// Add same pattern again
	al.AddPatterns([]string{"*.example.com"})

	patterns := al.Patterns()
	if len(patterns) != 1 {
		t.Errorf("expected 1 pattern, got %d", len(patterns))
	}
}

func TestAllowlist_AddPatterns_InvalidIgnored(t *testing.T) {
	al := NewAllowlist(nil)

	// Add mix of valid and invalid patterns
	al.AddPatterns([]string{"*.valid.com", "invalid", "*.also-valid.com"})

	patterns := al.Patterns()
	if len(patterns) != 2 {
		t.Errorf("expected 2 valid patterns, got %d", len(patterns))
	}
}

func TestAllowlist_ReplacePatterns(t *testing.T) {
	al := NewAllowlistWithPatterns(nil, []string{"*.old.com"})

	// Verify old pattern works
	if !al.IsAllowed("api.old.com") {
		t.Error("api.old.com should be allowed before replace")
	}

	// Replace with new patterns
	al.ReplacePatterns([]string{"*.new.com"})

	// Old pattern should no longer work
	if al.IsAllowed("api.old.com") {
		t.Error("api.old.com should not be allowed after replace")
	}

	// New pattern should work
	if !al.IsAllowed("api.new.com") {
		t.Error("api.new.com should be allowed after replace")
	}
}

func TestAllowlist_Patterns(t *testing.T) {
	patterns := []string{"*.example.com", "*.other.com"}
	al := NewAllowlistWithPatterns(nil, patterns)

	result := al.Patterns()

	if len(result) != len(patterns) {
		t.Errorf("expected %d patterns, got %d", len(patterns), len(result))
	}

	// Verify patterns are in the result (order may vary)
	patternSet := make(map[string]bool)
	for _, p := range result {
		patternSet[p] = true
	}

	for _, p := range patterns {
		if !patternSet[p] {
			t.Errorf("expected pattern %q in result", p)
		}
	}
}
