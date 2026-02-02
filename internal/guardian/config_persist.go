package guardian

import (
	"sync"

	"github.com/xdg/cloister/internal/config"
)

// FileConfigPersister implements ConfigPersister using the config package.
// It provides thread-safe methods for persisting domain approvals to config files.
type FileConfigPersister struct {
	mu sync.Mutex // Serialize file writes to avoid race conditions
}

// NewFileConfigPersister creates a new FileConfigPersister.
func NewFileConfigPersister() *FileConfigPersister {
	return &FileConfigPersister{}
}

// AddToProjectAllowlist adds a domain to a project's allowlist config.
// The domain is added only if it doesn't already exist in the allowlist.
// The config file is created if it doesn't exist.
func (p *FileConfigPersister) AddToProjectAllowlist(projectName, domain string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Load existing project config, or create empty one
	cfg, err := config.LoadProjectConfig(projectName)
	if err != nil {
		// If file doesn't exist, create a new config
		cfg = &config.ProjectConfig{}
	}

	// Check if domain already exists
	for _, entry := range cfg.Proxy.Allow {
		if entry.Domain == domain {
			// Already exists, nothing to do
			return nil
		}
	}

	// Add the domain
	cfg.Proxy.Allow = append(cfg.Proxy.Allow, config.AllowEntry{Domain: domain})

	// Write the updated config
	return config.WriteProjectConfig(projectName, cfg, true)
}

// AddToGlobalAllowlist adds a domain to the global allowlist config.
// The domain is added only if it doesn't already exist in the allowlist.
func (p *FileConfigPersister) AddToGlobalAllowlist(domain string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Load existing global config
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		// If file doesn't exist or has errors, create with defaults and add domain
		cfg = &config.GlobalConfig{}
	}

	// Check if domain already exists
	for _, entry := range cfg.Proxy.Allow {
		if entry.Domain == domain {
			// Already exists, nothing to do
			return nil
		}
	}

	// Add the domain
	cfg.Proxy.Allow = append(cfg.Proxy.Allow, config.AllowEntry{Domain: domain})

	// Write the updated config
	return config.WriteGlobalConfig(cfg)
}
