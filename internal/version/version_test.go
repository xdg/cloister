package version

import (
	"os"
	"testing"
)

func TestDefaultImage_VersionBased(t *testing.T) {
	// Ensure env var doesn't interfere
	_ = os.Unsetenv(ImageEnvVar)

	tests := []struct {
		name     string
		version  string
		expected string
	}{
		{
			name:     "dev version returns GHCR latest",
			version:  "dev",
			expected: "ghcr.io/xdg/cloister:latest",
		},
		{
			name:     "version containing dev returns GHCR latest",
			version:  "0.1.0-dev",
			expected: "ghcr.io/xdg/cloister:latest",
		},
		{
			name:     "release version returns GHCR versioned",
			version:  "v1.0.0",
			expected: "ghcr.io/xdg/cloister:v1.0.0",
		},
		{
			name:     "semver without v prefix",
			version:  "1.2.3",
			expected: "ghcr.io/xdg/cloister:1.2.3",
		},
		{
			name:     "pre-release version",
			version:  "v1.0.0-rc1",
			expected: "ghcr.io/xdg/cloister:v1.0.0-rc1",
		},
		{
			name:     "alpha version without dev",
			version:  "v1.0.0-alpha",
			expected: "ghcr.io/xdg/cloister:v1.0.0-alpha",
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

func TestDefaultImage_EnvVarOverride(t *testing.T) {
	// Save original state
	originalVersion := Version
	originalEnv := os.Getenv(ImageEnvVar)
	defer func() {
		Version = originalVersion
		if originalEnv == "" {
			_ = os.Unsetenv(ImageEnvVar)
		} else {
			_ = os.Setenv(ImageEnvVar, originalEnv)
		}
	}()

	tests := []struct {
		name     string
		version  string
		envValue string
		expected string
	}{
		{
			name:     "env var overrides dev version",
			version:  "dev",
			envValue: "cloister:latest",
			expected: "cloister:latest",
		},
		{
			name:     "env var overrides release version",
			version:  "v1.0.0",
			envValue: "my-registry/cloister:custom",
			expected: "my-registry/cloister:custom",
		},
		{
			name:     "empty env var uses default",
			version:  "v1.0.0",
			envValue: "",
			expected: "ghcr.io/xdg/cloister:v1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Version = tt.version
			if tt.envValue == "" {
				_ = os.Unsetenv(ImageEnvVar)
			} else {
				_ = os.Setenv(ImageEnvVar, tt.envValue)
			}

			got := DefaultImage()
			if got != tt.expected {
				t.Errorf("DefaultImage() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestConstants(t *testing.T) {
	if DefaultRegistry != "ghcr.io/xdg/cloister" {
		t.Errorf("DefaultRegistry = %q, want %q", DefaultRegistry, "ghcr.io/xdg/cloister")
	}
	if ImageEnvVar != "CLOISTER_IMAGE" {
		t.Errorf("ImageEnvVar = %q, want %q", ImageEnvVar, "CLOISTER_IMAGE")
	}
}
