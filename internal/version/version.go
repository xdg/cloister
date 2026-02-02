// Package version provides version information for cloister.
// The Version variable is set at build time via ldflags.
package version

import (
	"os"
	"strings"
)

// Version is the current version of cloister.
// Set at build time via: -ldflags "-X github.com/xdg/cloister/internal/version.Version=v1.0.0"
// Defaults to "dev" for development builds.
var Version = "dev"

// DefaultRegistry is the container registry for production images.
const DefaultRegistry = "ghcr.io/xdg/cloister"

// ImageEnvVar is the environment variable that overrides the default image.
// This is useful for local development (CLOISTER_IMAGE=cloister:latest) or
// CI/CD pipelines that need to pin a specific image.
const ImageEnvVar = "CLOISTER_IMAGE"

// DefaultImage returns the Docker image to use for cloister containers.
//
// Precedence (highest to lowest):
//  1. CLOISTER_IMAGE environment variable (for local dev / CI)
//  2. Version-tagged GHCR image (ghcr.io/xdg/cloister:vX.Y.Z)
//  3. Latest GHCR image (ghcr.io/xdg/cloister:latest) for dev builds
func DefaultImage() string {
	// Highest priority: explicit env var override
	if img := os.Getenv(ImageEnvVar); img != "" {
		return img
	}

	// Production: GHCR with version tag
	if Version == "dev" || strings.Contains(Version, "dev") {
		return DefaultRegistry + ":latest"
	}
	return DefaultRegistry + ":" + Version
}
