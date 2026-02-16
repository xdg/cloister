package config

import (
	"fmt"
	"os"

	"github.com/xdg/cloister/internal/pathutil"
)

// Dir returns the cloister configuration directory path.
// By default, this is ~/.config/cloister/. If the XDG_CONFIG_HOME
// environment variable is set, it uses $XDG_CONFIG_HOME/cloister/ instead.
// The returned path always has a trailing slash.
func Dir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = "~/.config"
	}
	return pathutil.ExpandHome(base) + "/cloister/"
}

// EnsureDir creates the cloister configuration directory if it
// doesn't exist. It uses 0700 permissions for security (user-only access).
// Returns nil if the directory already exists or was successfully created.
func EnsureDir() error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return fmt.Errorf("ensure config dir: %w", err)
	}
	return nil
}

// GlobalConfigPath returns the full path to the global configuration file.
// This is Dir() + "config.yaml".
func GlobalConfigPath() string {
	return Dir() + "config.yaml"
}

// ProjectsDir returns the path to the projects configuration directory.
// This is Dir() + "projects/".
func ProjectsDir() string {
	return Dir() + "projects/"
}

// ProjectConfigPath returns the full path to a project configuration file.
// The returned path is ProjectsDir() + name + ".yaml".
func ProjectConfigPath(name string) string {
	return ProjectsDir() + name + ".yaml"
}
