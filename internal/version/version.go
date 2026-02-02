// Package version provides version information for cloister.
// The Version variable is set at build time via ldflags.
package version

import "strings"

// Version is the current version of cloister.
// Set at build time via: -ldflags "-X github.com/xdg/cloister/internal/version.Version=v1.0.0"
// Defaults to "dev" for development builds.
var Version = "dev"

// DefaultImage returns the Docker image name with the appropriate tag.
// If Version is set to a release version (not containing "dev"), it returns
// "cloister:<version>". Otherwise, it returns "cloister:latest".
func DefaultImage() string {
	if Version == "dev" || strings.Contains(Version, "dev") {
		return "cloister:latest"
	}
	return "cloister:" + Version
}
