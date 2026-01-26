package config

// EffectiveConfig represents the merged configuration for a cloister session.
// It combines global settings with project-specific overrides.
type EffectiveConfig struct {
	// Proxy settings (from global, port/behavior settings)
	ProxyListen            string
	UnlistedDomainBehavior string
	ApprovalTimeout        string
	RateLimit              int
	MaxRequestBytes        int64

	// Merged allowlist (global + project)
	Allow []AllowEntry

	// Request server settings
	RequestListen  string
	RequestTimeout string

	// Approval server settings
	ApprovalListen string
	AutoApprove    []CommandPattern // Merged
	ManualApprove  []CommandPattern // Merged

	// Container defaults
	Image string
	Shell string
	User  string
	Agent string

	// Project-specific (if applicable)
	ProjectName   string
	ProjectRoot   string
	ProjectRemote string
	ProjectRefs   []string
}

// MergeAllowlists combines global and project allowlist entries.
// Project entries ADD to global (don't replace).
// Entries are deduplicated by domain.
func MergeAllowlists(global, project []AllowEntry) []AllowEntry {
	if len(global) == 0 && len(project) == 0 {
		return nil
	}

	// Use a map to track seen domains for deduplication
	seen := make(map[string]bool, len(global)+len(project))
	result := make([]AllowEntry, 0, len(global)+len(project))

	// Add global entries first
	for _, entry := range global {
		if !seen[entry.Domain] {
			seen[entry.Domain] = true
			result = append(result, entry)
		}
	}

	// Add project entries, skipping duplicates
	for _, entry := range project {
		if !seen[entry.Domain] {
			seen[entry.Domain] = true
			result = append(result, entry)
		}
	}

	return result
}

// MergeCommandPatterns combines global and project command patterns.
// Project patterns ADD to global (don't replace).
// Patterns are deduplicated by pattern string.
func MergeCommandPatterns(global, project []CommandPattern) []CommandPattern {
	if len(global) == 0 && len(project) == 0 {
		return nil
	}

	// Use a map to track seen patterns for deduplication
	seen := make(map[string]bool, len(global)+len(project))
	result := make([]CommandPattern, 0, len(global)+len(project))

	// Add global patterns first
	for _, pattern := range global {
		if !seen[pattern.Pattern] {
			seen[pattern.Pattern] = true
			result = append(result, pattern)
		}
	}

	// Add project patterns, skipping duplicates
	for _, pattern := range project {
		if !seen[pattern.Pattern] {
			seen[pattern.Pattern] = true
			result = append(result, pattern)
		}
	}

	return result
}

// ResolveConfig loads and merges global and project configurations into an
// EffectiveConfig. If projectName is empty, only global config is used.
// If projectName is provided but the project config doesn't exist, the
// default (empty) project config is used.
func ResolveConfig(projectName string) (*EffectiveConfig, error) {
	// Load global config
	global, err := LoadGlobalConfig()
	if err != nil {
		return nil, err
	}

	// Start with global values
	effective := &EffectiveConfig{
		// Proxy settings
		ProxyListen:            global.Proxy.Listen,
		UnlistedDomainBehavior: global.Proxy.UnlistedDomainBehavior,
		ApprovalTimeout:        global.Proxy.ApprovalTimeout,
		RateLimit:              global.Proxy.RateLimit,
		MaxRequestBytes:        global.Proxy.MaxRequestBytes,

		// Start with global allowlist
		Allow: global.Proxy.Allow,

		// Request server settings
		RequestListen:  global.Request.Listen,
		RequestTimeout: global.Request.Timeout,

		// Approval server settings
		ApprovalListen: global.Approval.Listen,
		AutoApprove:    global.Approval.AutoApprove,
		ManualApprove:  global.Approval.ManualApprove,

		// Container defaults
		Image: global.Defaults.Image,
		Shell: global.Defaults.Shell,
		User:  global.Defaults.User,
		Agent: global.Defaults.Agent,
	}

	// If no project specified, return global-only config
	if projectName == "" {
		return effective, nil
	}

	// Load project config
	project, err := LoadProjectConfig(projectName)
	if err != nil {
		return nil, err
	}

	// Set project-specific fields
	effective.ProjectName = projectName
	effective.ProjectRoot = project.Root
	effective.ProjectRemote = project.Remote
	effective.ProjectRefs = project.Refs

	// Merge allowlists (global + project)
	effective.Allow = MergeAllowlists(global.Proxy.Allow, project.Proxy.Allow)

	// Merge command patterns (global + project)
	effective.AutoApprove = MergeCommandPatterns(global.Approval.AutoApprove, project.Commands.AutoApprove)

	return effective, nil
}
