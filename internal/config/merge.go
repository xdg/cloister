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

	// Merged denylist (global + project)
	Deny []AllowEntry

	// Request server settings
	RequestListen  string
	RequestTimeout string

	// Hostexec server settings
	HostexecListen string
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

// mergeSlices combines two slices, deduplicating by a key function.
// Items from the first slice take precedence. The key function extracts
// the string used for deduplication.
func mergeSlices[T any](global, project []T, key func(T) string) []T {
	if len(global) == 0 && len(project) == 0 {
		return nil
	}

	seen := make(map[string]bool, len(global)+len(project))
	result := make([]T, 0, len(global)+len(project))

	for _, item := range global {
		k := key(item)
		if !seen[k] {
			seen[k] = true
			result = append(result, item)
		}
	}

	for _, item := range project {
		k := key(item)
		if !seen[k] {
			seen[k] = true
			result = append(result, item)
		}
	}

	return result
}

// MergeAllowlists combines global and project allowlist entries.
// Project entries ADD to global (don't replace).
// Entries are deduplicated by domain.
func MergeAllowlists(global, project []AllowEntry) []AllowEntry {
	return mergeSlices(global, project, func(e AllowEntry) string { return e.Domain })
}

// MergeDenylists combines global and project denylist entries.
// Project entries ADD to global (don't replace).
// Entries are deduplicated by domain.
func MergeDenylists(global, project []AllowEntry) []AllowEntry {
	return mergeSlices(global, project, func(e AllowEntry) string { return e.Domain })
}

// MergeCommandPatterns combines global and project command patterns.
// Project patterns ADD to global (don't replace).
// Patterns are deduplicated by pattern string.
func MergeCommandPatterns(global, project []CommandPattern) []CommandPattern {
	return mergeSlices(global, project, func(p CommandPattern) string { return p.Pattern })
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

		// Start with global denylist
		Deny: global.Proxy.Deny,

		// Request server settings
		RequestListen:  global.Request.Listen,
		RequestTimeout: global.Request.Timeout,

		// Hostexec server settings
		HostexecListen: global.Hostexec.Listen,
		AutoApprove:    global.Hostexec.AutoApprove,
		ManualApprove:  global.Hostexec.ManualApprove,

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

	// Merge denylists (global + project)
	effective.Deny = MergeDenylists(global.Proxy.Deny, project.Proxy.Deny)

	// Merge command patterns (global + project)
	effective.AutoApprove = MergeCommandPatterns(global.Hostexec.AutoApprove, project.Hostexec.AutoApprove)
	effective.ManualApprove = MergeCommandPatterns(global.Hostexec.ManualApprove, project.Hostexec.ManualApprove)

	return effective, nil
}
