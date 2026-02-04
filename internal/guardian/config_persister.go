package guardian

import (
	"fmt"
	"strings"
	"sync"

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

// AddDomainToProject adds a domain to a project's allowlist if not already present.
// It loads the project config, checks for duplicates, appends the domain if needed,
// and writes the updated config back to disk with autoCreate=true.
// The ReloadNotifier callback is invoked after successful write if not nil.
func (p *ConfigPersisterImpl) AddDomainToProject(project, domain string) error {
	// Validate domain before processing
	if err := validateDomain(domain); err != nil {
		return err
	}

	// Lock to prevent concurrent modifications
	p.projectMu.Lock()
	defer p.projectMu.Unlock()

	// Load existing project config
	cfg, err := config.LoadProjectConfig(project)
	if err != nil {
		return fmt.Errorf("load project config: %w", err)
	}

	// Check if domain already exists in allowlist
	for _, entry := range cfg.Proxy.Allow {
		if entry.Domain == domain {
			// Domain already present, no need to add
			return nil
		}
	}

	// Append new domain
	cfg.Proxy.Allow = append(cfg.Proxy.Allow, config.AllowEntry{Domain: domain})

	// Write updated config with autoCreate=true (overwrite)
	if err := config.WriteProjectConfig(project, cfg, true); err != nil {
		return fmt.Errorf("write project config: %w", err)
	}

	// Notify proxy to reload its allowlist cache (panic-safe)
	if p.ReloadNotifier != nil {
		safeNotify(p.ReloadNotifier)
	}

	return nil
}

// AddDomainToGlobal adds a domain to the global allowlist if not already present.
// It loads the global config, checks for duplicates, appends the domain if needed,
// and writes the updated config back to disk.
// The ReloadNotifier callback is invoked after successful write if not nil.
func (p *ConfigPersisterImpl) AddDomainToGlobal(domain string) error {
	// Validate domain before processing
	if err := validateDomain(domain); err != nil {
		return err
	}

	// Lock to prevent concurrent modifications
	p.globalMu.Lock()
	defer p.globalMu.Unlock()

	// Load existing global config
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		return fmt.Errorf("load global config: %w", err)
	}

	// Check if domain already exists in allowlist
	for _, entry := range cfg.Proxy.Allow {
		if entry.Domain == domain {
			// Domain already present, no need to add
			return nil
		}
	}

	// Append new domain
	cfg.Proxy.Allow = append(cfg.Proxy.Allow, config.AllowEntry{Domain: domain})

	// Write updated config (always overwrites)
	if err := config.WriteGlobalConfig(cfg); err != nil {
		return fmt.Errorf("write global config: %w", err)
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

// safeNotify calls the notifier function in a panic-safe manner.
// If the notifier panics, the panic is caught and discarded.
func safeNotify(notifier func()) {
	defer func() {
		if r := recover(); r != nil {
			// Silently discard panic from notifier callback
			// This prevents a misbehaving callback from crashing the guardian
		}
	}()
	notifier()
}
