// Package version provides version information for cloister.
// The Version variable is set at build time via ldflags.
package version

// Version is the current version of cloister.
// Set at build time via: -ldflags "-X github.com/xdg/cloister/internal/version.Version=v1.0.0"
// Defaults to "dev" for development builds.
var Version = "dev"
