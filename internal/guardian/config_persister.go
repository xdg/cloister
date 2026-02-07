package guardian

import (
	"fmt"
	"strings"
	"sync"

	"github.com/xdg/cloister/internal/clog"
	"github.com/xdg/cloister/internal/config"
)

// ConfigPersisterImpl implements the ConfigPersister interface for persisting
// approved domains to project and global configuration files.
type ConfigPersisterImpl struct {
	// ReloadNotifier is called after successfully writing a config file to signal
	// that the proxy should reload its allowlist cache. If nil, no notification is sent.
	ReloadNotifier func()

	// projectMu protects concurrent access to project config files
	projectMu sync.Mutex

	// globalMu protects concurrent access to the global config file
	globalMu sync.Mutex
}

// AddDomainToProject adds a domain to a project's approval file if not already present.
// It loads the project approvals, checks for duplicates, appends the domain if needed,
// and writes the updated approvals back to disk.
// The ReloadNotifier callback is invoked after successful write if not nil.
func (p *ConfigPersisterImpl) AddDomainToProject(project, domain string) error {
	// Validate domain before processing
	if err := validateDomain(domain); err != nil {
		return err
	}
	// Strip port if present (CONNECT requests include port, e.g. "example.com:443")
	domain = normalizeDomain(domain)

	// Lock to prevent concurrent modifications
	p.projectMu.Lock()
	defer p.projectMu.Unlock()

	// Load existing project approvals
	approvals, err := config.LoadProjectApprovals(project)
	if err != nil {
		return fmt.Errorf("load project approvals: %w", err)
	}

	// Check if domain already exists in approvals
	for _, d := range approvals.Domains {
		if d == domain {
			// Domain already present, no need to add
			return nil
		}
	}

	// Append new domain
	approvals.Domains = append(approvals.Domains, domain)

	// Write updated approvals
	if err := config.WriteProjectApprovals(project, approvals); err != nil {
		return fmt.Errorf("write project approvals: %w", err)
	}

	// Notify proxy to reload its allowlist cache (panic-safe)
	if p.ReloadNotifier != nil {
		safeNotify(p.ReloadNotifier)
	}

	return nil
}

// AddDomainToGlobal adds a domain to the global approval file if not already present.
// It loads the global approvals, checks for duplicates, appends the domain if needed,
// and writes the updated approvals back to disk.
// The ReloadNotifier callback is invoked after successful write if not nil.
func (p *ConfigPersisterImpl) AddDomainToGlobal(domain string) error {
	// Validate domain before processing
	if err := validateDomain(domain); err != nil {
		return err
	}
	// Strip port if present (CONNECT requests include port, e.g. "example.com:443")
	domain = normalizeDomain(domain)

	// Lock to prevent concurrent modifications
	p.globalMu.Lock()
	defer p.globalMu.Unlock()

	// Load existing global approvals
	approvals, err := config.LoadGlobalApprovals()
	if err != nil {
		return fmt.Errorf("load global approvals: %w", err)
	}

	// Check if domain already exists in approvals
	for _, d := range approvals.Domains {
		if d == domain {
			// Domain already present, no need to add
			return nil
		}
	}

	// Append new domain
	approvals.Domains = append(approvals.Domains, domain)

	// Write updated approvals
	if err := config.WriteGlobalApprovals(approvals); err != nil {
		return fmt.Errorf("write global approvals: %w", err)
	}

	// Notify proxy to reload its allowlist cache (panic-safe)
	if p.ReloadNotifier != nil {
		safeNotify(p.ReloadNotifier)
	}

	return nil
}

// validateDomain checks if a domain string is valid for use in the allowlist.
// It returns an error if the domain is empty or contains invalid characters.
func validateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}
	if strings.ContainsAny(domain, " \t\n\r") {
		return fmt.Errorf("domain cannot contain whitespace: %q", domain)
	}
	return nil
}

// normalizeDomain strips the port from a domain if present.
// CONNECT requests include port (e.g., "example.com:443") but allowlist
// entries should store bare hostnames for consistent matching.
func normalizeDomain(domain string) string {
	return stripPort(domain)
}

// validatePattern checks if a pattern string is valid for use in the allowlist.
// Valid patterns are in the format "*.example.com".
func validatePattern(pattern string) error {
	if pattern == "" {
		return fmt.Errorf("pattern cannot be empty")
	}
	if strings.ContainsAny(pattern, " \t\n\r") {
		return fmt.Errorf("pattern cannot contain whitespace: %q", pattern)
	}
	if !strings.HasPrefix(pattern, "*.") {
		return fmt.Errorf("pattern must start with '*.': %q", pattern)
	}
	if len(pattern) < 4 {
		return fmt.Errorf("pattern too short: %q", pattern)
	}
	// Validate suffix has at least 2 components for safety
	suffix := pattern[2:] // Skip "*."
	suffixComponents := strings.Count(suffix, ".") + 1
	if suffixComponents < 2 {
		return fmt.Errorf("pattern suffix must have at least 2 components to prevent overly broad patterns: %q", pattern)
	}
	return nil
}

// AddPatternToProject adds a wildcard pattern to a project's approval file if not already present.
// It loads the project approvals, checks for duplicates, appends the pattern if needed,
// and writes the updated approvals back to disk.
// The ReloadNotifier callback is invoked after successful write if not nil.
func (p *ConfigPersisterImpl) AddPatternToProject(project, pattern string) error {
	// Validate pattern before processing
	if err := validatePattern(pattern); err != nil {
		return err
	}

	// Lock to prevent concurrent modifications
	p.projectMu.Lock()
	defer p.projectMu.Unlock()

	// Load existing project approvals
	approvals, err := config.LoadProjectApprovals(project)
	if err != nil {
		return fmt.Errorf("load project approvals: %w", err)
	}

	// Check if pattern already exists in approvals
	for _, existing := range approvals.Patterns {
		if existing == pattern {
			// Pattern already present, no need to add
			return nil
		}
	}

	// Append new pattern
	approvals.Patterns = append(approvals.Patterns, pattern)

	// Write updated approvals
	if err := config.WriteProjectApprovals(project, approvals); err != nil {
		return fmt.Errorf("write project approvals: %w", err)
	}

	// Notify proxy to reload its allowlist cache (panic-safe)
	if p.ReloadNotifier != nil {
		safeNotify(p.ReloadNotifier)
	}

	return nil
}

// AddPatternToGlobal adds a wildcard pattern to the global approval file if not already present.
// It loads the global approvals, checks for duplicates, appends the pattern if needed,
// and writes the updated approvals back to disk.
// The ReloadNotifier callback is invoked after successful write if not nil.
func (p *ConfigPersisterImpl) AddPatternToGlobal(pattern string) error {
	// Validate pattern before processing
	if err := validatePattern(pattern); err != nil {
		return err
	}

	// Lock to prevent concurrent modifications
	p.globalMu.Lock()
	defer p.globalMu.Unlock()

	// Load existing global approvals
	approvals, err := config.LoadGlobalApprovals()
	if err != nil {
		return fmt.Errorf("load global approvals: %w", err)
	}

	// Check if pattern already exists in approvals
	for _, existing := range approvals.Patterns {
		if existing == pattern {
			// Pattern already present, no need to add
			return nil
		}
	}

	// Append new pattern
	approvals.Patterns = append(approvals.Patterns, pattern)

	// Write updated approvals
	if err := config.WriteGlobalApprovals(approvals); err != nil {
		return fmt.Errorf("write global approvals: %w", err)
	}

	// Notify proxy to reload its allowlist cache (panic-safe)
	if p.ReloadNotifier != nil {
		safeNotify(p.ReloadNotifier)
	}

	return nil
}

// safeNotify calls the notifier function in a panic-safe manner.
// If the notifier panics, the panic is caught and discarded.
func safeNotify(notifier func()) {
	defer func() {
		if r := recover(); r != nil {
			clog.Warn("ReloadNotifier panicked: %v", r)
		}
	}()
	notifier()
}
