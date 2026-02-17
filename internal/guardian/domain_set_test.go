package guardian

import (
	"sync"
	"testing"

	"github.com/xdg/cloister/internal/config"
)

func TestDomainSet_Contains_ExactMatch(t *testing.T) {
	tests := []struct {
		name     string
		domains  []string
		host     string
		expected bool
	}{
		{
			name:     "exact match found",
			domains:  []string{"api.anthropic.com"},
			host:     "api.anthropic.com",
			expected: true,
		},
		{
			name:     "exact match not found",
			domains:  []string{"api.anthropic.com"},
			host:     "github.com",
			expected: false,
		},
		{
			name:     "multiple domains first matches",
			domains:  []string{"api.anthropic.com", "api.openai.com"},
			host:     "api.anthropic.com",
			expected: true,
		},
		{
			name:     "multiple domains second matches",
			domains:  []string{"api.anthropic.com", "api.openai.com"},
			host:     "api.openai.com",
			expected: true,
		},
		{
			name:     "case insensitive match",
			domains:  []string{"API.Anthropic.COM"},
			host:     "api.anthropic.com",
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ds := NewDomainSet(tc.domains, nil)
			result := ds.Contains(tc.host)
			if result != tc.expected {
				t.Errorf("Contains(%q) = %v, expected %v", tc.host, result, tc.expected)
			}
		})
	}
}

func TestDomainSet_Contains_PatternMatch(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		host     string
		expected bool
	}{
		{
			name:     "pattern matches subdomain",
			patterns: []string{"*.example.com"},
			host:     "api.example.com",
			expected: true,
		},
		{
			name:     "pattern matches deep subdomain",
			patterns: []string{"*.example.com"},
			host:     "a.b.example.com",
			expected: true,
		},
		{
			name:     "pattern does not match base domain",
			patterns: []string{"*.example.com"},
			host:     "example.com",
			expected: false,
		},
		{
			name:     "pattern does not match different domain",
			patterns: []string{"*.example.com"},
			host:     "api.other.com",
			expected: false,
		},
		{
			name:     "second pattern matches",
			patterns: []string{"*.example.com", "*.other.com"},
			host:     "api.other.com",
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ds := NewDomainSet(nil, tc.patterns)
			result := ds.Contains(tc.host)
			if result != tc.expected {
				t.Errorf("Contains(%q) = %v, expected %v", tc.host, result, tc.expected)
			}
		})
	}
}

func TestDomainSet_Contains_PortStripping(t *testing.T) {
	tests := []struct {
		name     string
		domains  []string
		patterns []string
		host     string
		expected bool
	}{
		{
			name:     "exact match with standard port",
			domains:  []string{"api.anthropic.com"},
			host:     "api.anthropic.com:443",
			expected: true,
		},
		{
			name:     "exact match with custom port",
			domains:  []string{"api.anthropic.com"},
			host:     "api.anthropic.com:8443",
			expected: true,
		},
		{
			name:     "pattern match with port",
			patterns: []string{"*.example.com"},
			host:     "api.example.com:443",
			expected: true,
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
			ds := NewDomainSet(tc.domains, tc.patterns)
			result := ds.Contains(tc.host)
			if result != tc.expected {
				t.Errorf("Contains(%q) = %v, expected %v", tc.host, result, tc.expected)
			}
		})
	}
}

func TestDomainSet_Add(t *testing.T) {
	ds := NewDomainSet([]string{"example.com"}, nil)

	// Verify initial state
	if !ds.Contains("example.com") {
		t.Error("example.com should be in the set initially")
	}
	if ds.Contains("api.anthropic.com") {
		t.Error("api.anthropic.com should not be in the set initially")
	}

	// Add single domain
	ds.Add("api.anthropic.com")

	if !ds.Contains("api.anthropic.com") {
		t.Error("api.anthropic.com should be in the set after Add")
	}
	if !ds.Contains("example.com") {
		t.Error("example.com should still be in the set")
	}
}

func TestDomainSet_Add_WithPort(t *testing.T) {
	ds := NewDomainSet(nil, nil)
	ds.Add("api.anthropic.com:443")

	if !ds.Contains("api.anthropic.com") {
		t.Error("api.anthropic.com should be in the set after Add with port")
	}
}

func TestDomainSet_AddPattern(t *testing.T) {
	ds := NewDomainSet(nil, nil)

	// Initially no patterns
	if ds.Contains("api.example.com") {
		t.Error("api.example.com should not be in the set initially")
	}

	// Add pattern
	ds.AddPattern("*.example.com")

	if !ds.Contains("api.example.com") {
		t.Error("api.example.com should match after AddPattern")
	}
	if ds.Contains("example.com") {
		t.Error("example.com should not match wildcard pattern")
	}
}

func TestDomainSet_AddPattern_Invalid(t *testing.T) {
	ds := NewDomainSet(nil, nil)

	// Add invalid pattern - should be silently ignored
	ds.AddPattern("example.com")

	if ds.Contains("api.example.com") {
		t.Error("invalid pattern should not have been added")
	}
}

func TestDomainSet_AddPattern_NoDuplicates(t *testing.T) {
	ds := NewDomainSet(nil, []string{"*.example.com"})

	// Add same pattern again
	ds.AddPattern("*.example.com")

	ds.mu.RLock()
	count := len(ds.patterns)
	ds.mu.RUnlock()

	if count != 1 {
		t.Errorf("expected 1 pattern, got %d", count)
	}
}

func TestDomainSet_ConcurrentAccess(t *testing.T) {
	ds := NewDomainSet([]string{"initial.com"}, []string{"*.initial.com"})

	var wg sync.WaitGroup
	const goroutines = 50

	// Concurrent Contains calls
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = ds.Contains("initial.com")
			_ = ds.Contains("api.initial.com")
			_ = ds.Contains("notfound.com")
		}()
	}

	// Concurrent Add calls
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ds.Add("added.com")
			ds.AddPattern("*.added.com")
		}()
	}

	// Mixed concurrent reads and writes
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ds.Add("mixed.com")
			_ = ds.Contains("mixed.com")
			ds.AddPattern("*.mixed.com")
			_ = ds.Contains("api.mixed.com")
		}()
	}

	wg.Wait()

	// Verify final state is consistent
	if !ds.Contains("initial.com") {
		t.Error("initial.com should still be in the set")
	}
	if !ds.Contains("added.com") {
		t.Error("added.com should be in the set after concurrent adds")
	}
}

func TestNewDomainSetFromConfig(t *testing.T) {
	entries := []config.AllowEntry{
		{Domain: "api.anthropic.com"},
		{Domain: "api.openai.com"},
		{Pattern: "*.googleapis.com"},
		{Pattern: "*.example.com"},
	}

	ds := NewDomainSetFromConfig(entries)

	// Test exact domains
	if !ds.Contains("api.anthropic.com") {
		t.Error("api.anthropic.com should be in the set")
	}
	if !ds.Contains("api.openai.com") {
		t.Error("api.openai.com should be in the set")
	}

	// Test pattern matches
	if !ds.Contains("storage.googleapis.com") {
		t.Error("storage.googleapis.com should match *.googleapis.com")
	}
	if !ds.Contains("sub.example.com") {
		t.Error("sub.example.com should match *.example.com")
	}

	// Test non-matches
	if ds.Contains("googleapis.com") {
		t.Error("googleapis.com should not match *.googleapis.com")
	}
	if ds.Contains("unrelated.com") {
		t.Error("unrelated.com should not be in the set")
	}
}

func TestNewDomainSetFromConfig_Empty(t *testing.T) {
	ds := NewDomainSetFromConfig(nil)

	if ds.Contains("anything.com") {
		t.Error("empty config should not contain any domains")
	}
}

func TestNewDomainSetFromConfig_SkipsEmptyEntries(t *testing.T) {
	entries := []config.AllowEntry{
		{Domain: "api.anthropic.com"},
		{Domain: "", Pattern: ""}, // Should be skipped
		{Pattern: "*.example.com"},
	}

	ds := NewDomainSetFromConfig(entries)

	if !ds.Contains("api.anthropic.com") {
		t.Error("api.anthropic.com should be in the set")
	}
	if !ds.Contains("sub.example.com") {
		t.Error("sub.example.com should match pattern")
	}
}

func TestDomainSet_EmptySet(t *testing.T) {
	ds := NewDomainSet(nil, nil)

	if ds.Contains("anything.com") {
		t.Error("empty set should not contain anything")
	}
	if ds.Contains("") {
		t.Error("empty set should not contain empty string")
	}
}

func TestDomainSet_EmptySlices(t *testing.T) {
	ds := NewDomainSet([]string{}, []string{})

	if ds.Contains("anything.com") {
		t.Error("empty set should not contain anything")
	}
}

func TestDomainSet_InvalidPatternsFiltered(t *testing.T) {
	ds := NewDomainSet(nil, []string{"*.valid.com", "invalid", "*.also-valid.com"})

	if !ds.Contains("api.valid.com") {
		t.Error("api.valid.com should match valid pattern")
	}
	if !ds.Contains("api.also-valid.com") {
		t.Error("api.also-valid.com should match valid pattern")
	}

	ds.mu.RLock()
	count := len(ds.patterns)
	ds.mu.RUnlock()

	if count != 2 {
		t.Errorf("expected 2 valid patterns, got %d", count)
	}
}
