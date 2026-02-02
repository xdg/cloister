package version

import "testing"

func TestDefaultImage(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected string
	}{
		{
			name:     "dev version returns latest",
			version:  "dev",
			expected: "cloister:latest",
		},
		{
			name:     "version containing dev returns latest",
			version:  "0.1.0-dev",
			expected: "cloister:latest",
		},
		{
			name:     "release version returns versioned image",
			version:  "v1.0.0",
			expected: "cloister:v1.0.0",
		},
		{
			name:     "semver without v prefix",
			version:  "1.2.3",
			expected: "cloister:1.2.3",
		},
		{
			name:     "pre-release version",
			version:  "v1.0.0-rc1",
			expected: "cloister:v1.0.0-rc1",
		},
		{
			name:     "alpha version without dev",
			version:  "v1.0.0-alpha",
			expected: "cloister:v1.0.0-alpha",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original version
			original := Version
			defer func() { Version = original }()

			// Set test version
			Version = tt.version

			got := DefaultImage()
			if got != tt.expected {
				t.Errorf("DefaultImage() = %q, want %q", got, tt.expected)
			}
		})
	}
}
