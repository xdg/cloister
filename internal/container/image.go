package container

import (
	"os"
	"strings"

	"github.com/xdg/cloister/internal/version"
)

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
	if version.Version == "dev" || strings.Contains(version.Version, "dev") {
		return DefaultRegistry + ":latest"
	}
	return DefaultRegistry + ":" + version.Version
}
