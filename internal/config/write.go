package config

import (
	"errors"
	"os"
)

// WriteDefaultConfig creates the default global configuration file with helpful comments.
// If the config file already exists, it returns nil without overwriting.
// The config directory is created if it doesn't exist.
// The file is written with 0600 permissions (user read/write only).
func WriteDefaultConfig() error {
	path := GlobalConfigPath()

	// Check if file already exists
	_, err := os.Stat(path)
	if err == nil {
		// File exists, don't overwrite
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		// Some other error occurred
		return err
	}

	// Ensure config directory exists
	if err := EnsureConfigDir(); err != nil {
		return err
	}

	// Write the config file with user-only permissions
	return os.WriteFile(path, []byte(defaultConfigTemplate), 0600)
}

// EnsureProjectsDir creates the projects configuration directory if it
// doesn't exist. It uses 0700 permissions for security (user-only access).
// Returns nil if the directory already exists or was successfully created.
func EnsureProjectsDir() error {
	return os.MkdirAll(ProjectsDir(), 0700)
}

// InitProjectConfig creates a minimal project configuration file if it doesn't exist.
// The config file is written to ProjectConfigPath(name).
// If the config file already exists, it returns nil without overwriting.
// The projects directory is created if it doesn't exist.
// The file is written with 0600 permissions (user read/write only).
func InitProjectConfig(name string, remote string, root string) error {
	path := ProjectConfigPath(name)

	// Check if file already exists
	_, err := os.Stat(path)
	if err == nil {
		// File exists, don't overwrite
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		// Some other error occurred
		return err
	}

	// Create a minimal config with just remote and root
	cfg := &ProjectConfig{
		Remote: remote,
		Root:   root,
	}

	// Ensure projects directory exists
	if err := EnsureProjectsDir(); err != nil {
		return err
	}

	// Marshal the config to YAML
	data, err := MarshalProjectConfig(cfg)
	if err != nil {
		return err
	}

	// Write the config file with user-only permissions
	return os.WriteFile(path, data, 0600)
}

// WriteProjectConfig writes a project configuration to the projects directory.
// The config file is written to ProjectConfigPath(name).
// If the config file already exists and overwrite is false, it returns nil.
// The projects directory is created if it doesn't exist.
// The file is written with 0600 permissions (user read/write only).
func WriteProjectConfig(name string, cfg *ProjectConfig, overwrite bool) error {
	path := ProjectConfigPath(name)

	// Check if file already exists
	_, err := os.Stat(path)
	if err == nil && !overwrite {
		// File exists and overwrite is false, don't overwrite
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		// Some other error occurred
		return err
	}

	// Ensure projects directory exists
	if err := EnsureProjectsDir(); err != nil {
		return err
	}

	// Marshal the config to YAML
	data, err := MarshalProjectConfig(cfg)
	if err != nil {
		return err
	}

	// Write the config file with user-only permissions
	return os.WriteFile(path, data, 0600)
}

// WriteGlobalConfig writes a global configuration to the config directory.
// The config file is written to GlobalConfigPath().
// The config directory is created if it doesn't exist.
// The file is written with 0600 permissions (user read/write only).
// This will overwrite any existing config file.
func WriteGlobalConfig(cfg *GlobalConfig) error {
	path := GlobalConfigPath()

	// Ensure config directory exists
	if err := EnsureConfigDir(); err != nil {
		return err
	}

	// Marshal the config to YAML
	data, err := MarshalGlobalConfig(cfg)
	if err != nil {
		return err
	}

	// Write the config file with user-only permissions
	return os.WriteFile(path, data, 0600)
}
