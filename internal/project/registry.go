// Package project provides functions for detecting and extracting project
// information from git repositories.
package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/pathutil"
)

// ErrProjectNotFound indicates that a project was not found in the registry.
var ErrProjectNotFound = errors.New("project not found in registry")

// NameCollisionError indicates that a project name is already registered
// with a different remote URL.
type NameCollisionError struct {
	Name           string
	ExistingRemote string
	NewRemote      string
}

// Error implements the error interface.
func (e *NameCollisionError) Error() string {
	return fmt.Sprintf("project name %q is already registered with remote %q; new remote is %q (consider renaming)",
		e.Name, e.ExistingRemote, e.NewRemote)
}

// RegistryEntry represents a single project in the registry.
type RegistryEntry struct {
	Name     string    `yaml:"name"`
	Root     string    `yaml:"root"`      // Absolute path to git root
	Remote   string    `yaml:"remote"`    // Remote URL (origin)
	LastUsed time.Time `yaml:"last_used"` // Last time a cloister was started
}

// Clock provides the current time. This interface enables testing with controlled time.
type Clock interface {
	Now() time.Time
}

// Registry contains the list of known projects.
type Registry struct {
	Projects []RegistryEntry `yaml:"projects"`
	clock    Clock           `yaml:"-"` // not serialized
}

// SetClock sets a custom clock for testing. If not called, uses real time.
func (r *Registry) SetClock(c Clock) {
	r.clock = c
}

// now returns the current time using the registry's clock.
// If no clock is set, it falls back to time.Now().
func (r *Registry) now() time.Time {
	if r.clock == nil {
		return time.Now()
	}
	return r.clock.Now()
}

// RegistryPath returns the path to the project registry file.
// This is ConfigDir() + "projects.yaml".
func RegistryPath() string {
	return config.ConfigDir() + "projects.yaml"
}

// LoadRegistry loads the project registry from disk.
// If the registry file doesn't exist, an empty Registry is returned (not an error).
// The ~ prefix in Root paths is expanded to the user's home directory.
func LoadRegistry() (*Registry, error) {
	path := RegistryPath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Registry{}, nil
		}
		return nil, err
	}

	var reg Registry
	if err := yaml.Unmarshal(data, &reg); err != nil {
		return nil, err
	}

	// Expand ~ in Root paths
	for i := range reg.Projects {
		reg.Projects[i].Root = pathutil.ExpandHome(reg.Projects[i].Root)
	}

	return &reg, nil
}

// SaveRegistry saves the project registry to disk.
// The config directory is created if it doesn't exist.
// The file is written with 0600 permissions for security.
func SaveRegistry(r *Registry) error {
	if err := config.EnsureConfigDir(); err != nil {
		return err
	}

	data, err := yaml.Marshal(r)
	if err != nil {
		return err
	}

	return os.WriteFile(RegistryPath(), data, 0600)
}

// Register adds or updates a project in the registry based on ProjectInfo.
// If a project with the same name and remote exists, it updates Root and LastUsed.
// If a project with the same name but different remote exists, it returns a NameCollisionError.
// Otherwise, it appends a new entry.
// This method modifies the registry in-memory; the caller must call SaveRegistry to persist.
func (r *Registry) Register(info *ProjectInfo) error {
	now := r.now()

	// Check for existing project with the same name
	for i, entry := range r.Projects {
		if entry.Name == info.Name {
			// Same name - check if same remote
			if entry.Remote == info.Remote {
				// Same project, update Root and LastUsed
				r.Projects[i].Root = info.Root
				r.Projects[i].LastUsed = now
				return nil
			}
			// Different remote - name collision
			return &NameCollisionError{
				Name:           info.Name,
				ExistingRemote: entry.Remote,
				NewRemote:      info.Remote,
			}
		}
	}

	// No collision, append new entry
	r.Projects = append(r.Projects, RegistryEntry{
		Name:     info.Name,
		Root:     info.Root,
		Remote:   info.Remote,
		LastUsed: now,
	})
	return nil
}

// FindByName returns a pointer to the registry entry with the given name,
// or nil if not found.
func (r *Registry) FindByName(name string) *RegistryEntry {
	for i := range r.Projects {
		if r.Projects[i].Name == name {
			return &r.Projects[i]
		}
	}
	return nil
}

// FindByRemote returns a pointer to the registry entry with the given remote URL,
// or nil if not found.
func (r *Registry) FindByRemote(remote string) *RegistryEntry {
	for i := range r.Projects {
		if r.Projects[i].Remote == remote {
			return &r.Projects[i]
		}
	}
	return nil
}

// UpdateLastUsed updates the LastUsed timestamp for the project with the given name.
// Returns ErrProjectNotFound if no project with that name exists.
func (r *Registry) UpdateLastUsed(name string) error {
	for i := range r.Projects {
		if r.Projects[i].Name == name {
			r.Projects[i].LastUsed = r.now()
			return nil
		}
	}
	return ErrProjectNotFound
}

// FindByPath returns a pointer to the registry entry where the given path
// is within the project's Root directory. The path can be the Root itself
// or any subdirectory. Both absolute paths and paths with unexpanded ~ are
// handled. Returns nil if no match is found.
func (r *Registry) FindByPath(path string) *RegistryEntry {
	// Expand ~ in the input path
	path = pathutil.ExpandHome(path)

	// Clean the path to normalize it
	path = filepath.Clean(path)

	for i := range r.Projects {
		root := filepath.Clean(r.Projects[i].Root)

		// Check if path equals root or is a subdirectory of root
		if path == root {
			return &r.Projects[i]
		}

		// Check if path starts with root + separator
		if strings.HasPrefix(path, root+string(filepath.Separator)) {
			return &r.Projects[i]
		}
	}
	return nil
}

// List returns a copy of all registry entries.
// Returns an empty slice (not nil) if no projects are registered.
func (r *Registry) List() []RegistryEntry {
	if len(r.Projects) == 0 {
		return []RegistryEntry{}
	}

	// Return a copy to prevent modification of the original
	result := make([]RegistryEntry, len(r.Projects))
	copy(result, r.Projects)
	return result
}

// Remove removes a project from the registry by name.
// Returns ErrProjectNotFound if no project with that name exists.
// This method modifies the registry in-memory; the caller must call SaveRegistry to persist.
func (r *Registry) Remove(name string) error {
	for i := range r.Projects {
		if r.Projects[i].Name == name {
			// Remove by replacing with last element and truncating
			r.Projects[i] = r.Projects[len(r.Projects)-1]
			r.Projects = r.Projects[:len(r.Projects)-1]
			return nil
		}
	}
	return ErrProjectNotFound
}

// Lookup loads the registry from disk and finds a project by name.
// Returns ErrProjectNotFound if the project does not exist.
func Lookup(name string) (*RegistryEntry, error) {
	reg, err := LoadRegistry()
	if err != nil {
		return nil, err
	}

	entry := reg.FindByName(name)
	if entry == nil {
		return nil, ErrProjectNotFound
	}
	return entry, nil
}

// LookupByPath loads the registry from disk and finds a project containing
// the given path. The path can be a project root or any subdirectory.
// Returns ErrProjectNotFound if no project contains the path.
func LookupByPath(path string) (*RegistryEntry, error) {
	reg, err := LoadRegistry()
	if err != nil {
		return nil, err
	}

	entry := reg.FindByPath(path)
	if entry == nil {
		return nil, ErrProjectNotFound
	}
	return entry, nil
}
