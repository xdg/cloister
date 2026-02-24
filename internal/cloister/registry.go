package cloister

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/xdg/cloister/internal/config"
)

// ErrCloisterNotFound indicates that a cloister was not found in the registry.
var ErrCloisterNotFound = errors.New("cloister not found in registry")

// NameCollisionError indicates that a cloister name is already registered
// with a different project.
type NameCollisionError struct {
	CloisterName    string
	ExistingProject string
	NewProject      string
}

// Error implements the error interface.
func (e *NameCollisionError) Error() string {
	return fmt.Sprintf("cloister name %q is already registered with project %q; new project is %q",
		e.CloisterName, e.ExistingProject, e.NewProject)
}

// Clock provides the current time. This interface enables testing with controlled time.
type Clock interface {
	Now() time.Time
}

// RegistryEntry represents a single cloister in the registry.
type RegistryEntry struct {
	CloisterName string    `yaml:"cloister_name"`
	ProjectName  string    `yaml:"project_name"`
	Branch       string    `yaml:"branch,omitempty"` // empty for main checkout
	HostPath     string    `yaml:"host_path"`        // absolute path on host
	IsWorktree   bool      `yaml:"is_worktree"`
	CreatedAt    time.Time `yaml:"created_at"`
}

// Registry contains the list of known cloisters.
type Registry struct {
	Cloisters []RegistryEntry `yaml:"cloisters"`
	clock     Clock           `yaml:"-"`
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

// RegistryPath returns the path to the cloister registry file.
func RegistryPath() string {
	return filepath.Join(config.Dir(), "cloisters.yaml")
}

// LoadRegistry loads the cloister registry from disk.
// If the registry file doesn't exist, an empty Registry is returned (not an error).
func LoadRegistry() (*Registry, error) {
	path := RegistryPath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Registry{}, nil
		}
		return nil, fmt.Errorf("read cloister registry file: %w", err)
	}

	var reg Registry
	if err := yaml.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("unmarshal cloister registry: %w", err)
	}

	return &reg, nil
}

// SaveRegistry saves the cloister registry to disk.
// The config directory is created if it doesn't exist.
// The file is written with 0600 permissions for security.
func SaveRegistry(r *Registry) error {
	if err := config.EnsureDir(); err != nil {
		return err
	}

	data, err := yaml.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshal cloister registry: %w", err)
	}

	if err := os.WriteFile(RegistryPath(), data, 0o600); err != nil {
		return fmt.Errorf("write cloister registry file: %w", err)
	}
	return nil
}

// Register adds or updates a cloister in the registry.
// If a cloister with the same name and same project exists, it updates the entry.
// If a cloister with the same name but a different project exists, it returns a NameCollisionError.
// Otherwise, it appends a new entry.
// This method modifies the registry in-memory; the caller must call SaveRegistry to persist.
func (r *Registry) Register(entry RegistryEntry) error {
	for i, existing := range r.Cloisters {
		if existing.CloisterName == entry.CloisterName {
			if existing.ProjectName != entry.ProjectName {
				return &NameCollisionError{
					CloisterName:    entry.CloisterName,
					ExistingProject: existing.ProjectName,
					NewProject:      entry.ProjectName,
				}
			}
			// Same name and project: upsert, preserving original creation time
			entry.CreatedAt = existing.CreatedAt
			r.Cloisters[i] = entry
			return nil
		}
	}

	// New entry
	entry.CreatedAt = r.now()
	r.Cloisters = append(r.Cloisters, entry)
	return nil
}

// FindByName returns a pointer to the registry entry with the given cloister name,
// or nil if not found.
func (r *Registry) FindByName(name string) *RegistryEntry {
	for i := range r.Cloisters {
		if r.Cloisters[i].CloisterName == name {
			return &r.Cloisters[i]
		}
	}
	return nil
}

// FindByProject returns all cloister entries for the given project name.
// Returns an empty slice (not nil) if no cloisters are found.
func (r *Registry) FindByProject(projectName string) []RegistryEntry {
	var result []RegistryEntry
	for _, entry := range r.Cloisters {
		if entry.ProjectName == projectName {
			result = append(result, entry)
		}
	}
	if result == nil {
		return []RegistryEntry{}
	}
	return result
}

// Remove removes a cloister from the registry by name.
// Returns ErrCloisterNotFound if no cloister with that name exists.
// This method modifies the registry in-memory; the caller must call SaveRegistry to persist.
func (r *Registry) Remove(cloisterName string) error {
	for i := range r.Cloisters {
		if r.Cloisters[i].CloisterName == cloisterName {
			// Remove by replacing with last element and truncating
			r.Cloisters[i] = r.Cloisters[len(r.Cloisters)-1]
			r.Cloisters = r.Cloisters[:len(r.Cloisters)-1]
			return nil
		}
	}
	return ErrCloisterNotFound
}

// List returns a copy of all registry entries.
// Returns an empty slice (not nil) if no cloisters are registered.
func (r *Registry) List() []RegistryEntry {
	if len(r.Cloisters) == 0 {
		return []RegistryEntry{}
	}

	result := make([]RegistryEntry, len(r.Cloisters))
	copy(result, r.Cloisters)
	return result
}
