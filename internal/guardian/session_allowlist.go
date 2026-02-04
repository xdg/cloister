package guardian

import (
	"errors"
	"sync"
)

// MemorySessionAllowlist tracks domains approved with "session" scope.
// These are per-project, ephemeral (lost on guardian restart), and checked
// by the proxy before consulting the persistent allowlist.
// All methods are thread-safe.
//
// Lifecycle contract:
//   - Add() is called when a domain approval request is approved with "session" scope
//   - IsAllowed() is called by the proxy before checking the persistent allowlist
//   - Clear(project) MUST be called when a cloister stops to prevent memory leaks
//   - ClearAll() is called when the guardian restarts to reset all session state
//
// Memory management:
//   - Each project maintains a set of approved domains in memory
//   - Call Size() periodically to monitor memory usage
//   - Clear(project) MUST be called when cloisters stop to prevent unbounded growth
type MemorySessionAllowlist struct {
	mu       sync.RWMutex
	projects map[string]map[string]struct{} // project -> domain set
}

// NewSessionAllowlist creates an empty MemorySessionAllowlist.
func NewSessionAllowlist() *MemorySessionAllowlist {
	return &MemorySessionAllowlist{
		projects: make(map[string]map[string]struct{}),
	}
}

var (
	// ErrEmptyProject is returned when Add() is called with an empty project string.
	ErrEmptyProject = errors.New("project cannot be empty")
	// ErrEmptyDomain is returned when Add() is called with an empty domain string.
	ErrEmptyDomain = errors.New("domain cannot be empty")
)

// Add adds a domain to the project's session set.
// If the project doesn't exist yet, it is created.
// Returns an error if project or domain is empty.
func (s *MemorySessionAllowlist) Add(project, domain string) error {
	if project == "" {
		return ErrEmptyProject
	}
	if domain == "" {
		return ErrEmptyDomain
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.projects[project] == nil {
		s.projects[project] = make(map[string]struct{})
	}
	s.projects[project][domain] = struct{}{}
	return nil
}

// IsAllowed checks if a domain is in the project's session set.
// Returns false if the project doesn't exist or the domain is not in the set.
func (s *MemorySessionAllowlist) IsAllowed(project, domain string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	domainSet, ok := s.projects[project]
	if !ok {
		return false
	}
	_, allowed := domainSet[domain]
	return allowed
}

// Clear removes all session domains for a project.
// This is typically called when a cloister stops.
// If the project doesn't exist, this is a no-op.
func (s *MemorySessionAllowlist) Clear(project string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.projects, project)
}

// ClearAll removes all session domains for all projects.
// This is typically called when the guardian restarts.
func (s *MemorySessionAllowlist) ClearAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects = make(map[string]map[string]struct{})
}

// Size returns the number of tracked projects and total number of domains
// across all projects for memory monitoring.
func (s *MemorySessionAllowlist) Size() (projects int, domains int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	projects = len(s.projects)
	for _, domainSet := range s.projects {
		domains += len(domainSet)
	}
	return projects, domains
}
