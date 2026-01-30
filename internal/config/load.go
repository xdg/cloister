package config

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/xdg/cloister/internal/pathutil"
)

// LoadGlobalConfig loads the global configuration from the default config path.
// If the config file doesn't exist, it returns DefaultGlobalConfig().
// If the file exists but cannot be read or parsed, it returns an error.
// All paths containing ~ are expanded to the actual home directory.
func LoadGlobalConfig() (*GlobalConfig, error) {
	path := GlobalConfigPath()
	log.Printf("config: loading global config from %s", path)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Printf("config: file not found, creating defaults")
			if writeErr := WriteDefaultConfig(); writeErr != nil {
				log.Printf("config: warning: failed to create default config: %v", writeErr)
			}
			cfg := DefaultGlobalConfig()
			expandGlobalPaths(cfg)
			return cfg, nil
		}
		return nil, fmt.Errorf("read global config: %w", err)
	}

	cfg, err := ParseGlobalConfig(data)
	if err != nil {
		return nil, fmt.Errorf("load global config: %w", err)
	}

	if err := ValidateGlobalConfig(cfg); err != nil {
		return nil, fmt.Errorf("load global config: %w", err)
	}

	expandGlobalPaths(cfg)
	return cfg, nil
}

// LoadProjectConfig loads a project configuration by name.
// The config file is expected at ProjectConfigPath(name).
// If the config file doesn't exist, it returns DefaultProjectConfig().
// If the file exists but cannot be read or parsed, it returns an error.
// All paths containing ~ are expanded to the actual home directory.
//
// Note: Callers should validate that the loaded config's Remote field matches
// the expected remote URL from the project registry to detect configuration drift.
func LoadProjectConfig(name string) (*ProjectConfig, error) {
	path := ProjectConfigPath(name)
	log.Printf("config: loading project config from %s", path)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Printf("config: file not found, using defaults")
			cfg := DefaultProjectConfig()
			expandProjectPaths(cfg)
			return cfg, nil
		}
		return nil, fmt.Errorf("read project config %q: %w", name, err)
	}

	cfg, err := ParseProjectConfig(data)
	if err != nil {
		return nil, fmt.Errorf("load project config %q: %w", name, err)
	}

	if err := ValidateProjectConfig(cfg); err != nil {
		return nil, fmt.Errorf("load project config %q: %w", name, err)
	}

	expandProjectPaths(cfg)
	return cfg, nil
}

// expandGlobalPaths expands ~ to the home directory in all path fields
// of the global configuration.
func expandGlobalPaths(cfg *GlobalConfig) {
	// Expand log paths
	cfg.Log.File = pathutil.ExpandHome(cfg.Log.File)
	cfg.Log.PerCloisterDir = pathutil.ExpandHome(cfg.Log.PerCloisterDir)

	// Expand blocked mount paths
	for i, mount := range cfg.Devcontainer.BlockedMounts {
		cfg.Devcontainer.BlockedMounts[i] = pathutil.ExpandHome(mount)
	}
}

// expandProjectPaths expands ~ to the home directory in all path fields
// of the project configuration.
func expandProjectPaths(cfg *ProjectConfig) {
	// Expand root path
	cfg.Root = pathutil.ExpandHome(cfg.Root)

	// Expand refs paths
	for i, ref := range cfg.Refs {
		cfg.Refs[i] = pathutil.ExpandHome(ref)
	}
}
