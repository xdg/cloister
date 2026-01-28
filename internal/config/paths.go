package config

import (
	"os"

	"github.com/xdg/cloister/internal/pathutil"
)

// ConfigDir returns the cloister configuration directory path.
// By default, this is ~/.config/cloister/. If the XDG_CONFIG_HOME
// environment variable is set, it uses $XDG_CONFIG_HOME/cloister/ instead.
// The returned path always has a trailing slash.
func ConfigDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = "~/.config"
	}
	return pathutil.ExpandHome(base) + "/cloister/"
}

// EnsureConfigDir creates the cloister configuration directory if it
// doesn't exist. It uses 0700 permissions for security (user-only access).
// Returns nil if the directory already exists or was successfully created.
func EnsureConfigDir() error {
	return os.MkdirAll(ConfigDir(), 0700)
}

// GlobalConfigPath returns the full path to the global configuration file.
// This is ConfigDir() + "config.yaml".
func GlobalConfigPath() string {
	return ConfigDir() + "config.yaml"
}

// ProjectsDir returns the path to the projects configuration directory.
// This is ConfigDir() + "projects/".
func ProjectsDir() string {
	return ConfigDir() + "projects/"
}

// ProjectConfigPath returns the full path to a project configuration file.
// The returned path is ProjectsDir() + name + ".yaml".
func ProjectConfigPath(name string) string {
	return ProjectsDir() + name + ".yaml"
}
