package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "tilde only",
			input:    "~",
			expected: home,
		},
		{
			name:     "tilde with subpath",
			input:    "~/Documents",
			expected: filepath.Join(home, "Documents"),
		},
		{
			name:     "tilde with nested subpath",
			input:    "~/foo/bar/baz",
			expected: filepath.Join(home, "foo", "bar", "baz"),
		},
		{
			name:     "absolute path unchanged",
			input:    "/usr/local/bin",
			expected: "/usr/local/bin",
		},
		{
			name:     "relative path unchanged",
			input:    "relative/path",
			expected: "relative/path",
		},
		{
			name:     "empty string unchanged",
			input:    "",
			expected: "",
		},
		{
			name:     "tilde in middle unchanged",
			input:    "/path/~/test",
			expected: "/path/~/test",
		},
		{
			name:     "tilde without slash unchanged",
			input:    "~user",
			expected: "~user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExpandHome(tt.input)
			if result != tt.expected {
				t.Errorf("ExpandHome(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
