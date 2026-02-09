// Package config provides configuration types for cloister global and
// per-project settings. These types map to YAML configuration files.
package config

// GlobalConfig represents the top-level global configuration for cloister.
// It is typically stored at ~/.config/cloister/config.yaml.
type GlobalConfig struct {
	Proxy        ProxyConfig            `yaml:"proxy,omitempty"`
	Request      RequestConfig          `yaml:"request,omitempty"`
	Hostexec     HostexecConfig         `yaml:"hostexec,omitempty"`
	Devcontainer DevcontainerConfig     `yaml:"devcontainer,omitempty"`
	Agents       map[string]AgentConfig `yaml:"agents,omitempty"`
	Defaults     DefaultsConfig         `yaml:"defaults,omitempty"`
	Log          LogConfig              `yaml:"log,omitempty"`
}

// ProxyConfig contains HTTP CONNECT proxy settings.
type ProxyConfig struct {
	Listen                 string       `yaml:"listen,omitempty"`
	Allow                  []AllowEntry `yaml:"allow,omitempty"`
	Deny                   []AllowEntry `yaml:"deny,omitempty"`
	UnlistedDomainBehavior string       `yaml:"unlisted_domain_behavior,omitempty"`
	ApprovalTimeout        string       `yaml:"approval_timeout,omitempty"`
	RateLimit              int          `yaml:"rate_limit,omitempty"`
	MaxRequestBytes        int64        `yaml:"max_request_bytes,omitempty"`
}

// AllowEntry represents a single domain or pattern in an allowlist.
// Either Domain or Pattern should be set, not both.
// Pattern supports wildcard matching in the format "*.example.com".
type AllowEntry struct {
	Domain  string `yaml:"domain,omitempty"`
	Pattern string `yaml:"pattern,omitempty"`
}

// RequestConfig contains settings for the request server that handles
// hostexec commands from containers.
type RequestConfig struct {
	Listen  string `yaml:"listen,omitempty"`
	Timeout string `yaml:"timeout,omitempty"`
}

// HostexecConfig contains settings for the hostexec approval server that provides
// the human review interface for hostexec commands.
type HostexecConfig struct {
	Listen        string           `yaml:"listen,omitempty"`
	AutoApprove   []CommandPattern `yaml:"auto_approve,omitempty"`
	ManualApprove []CommandPattern `yaml:"manual_approve,omitempty"`
}

// CommandPattern represents a regex pattern for matching commands.
type CommandPattern struct {
	Pattern string `yaml:"pattern,omitempty"`
}

// DevcontainerConfig contains settings for devcontainer.json integration.
type DevcontainerConfig struct {
	Enabled       bool           `yaml:"enabled,omitempty"`
	Features      FeaturesConfig `yaml:"features,omitempty"`
	BlockedMounts []string       `yaml:"blocked_mounts,omitempty"`
}

// FeaturesConfig specifies which devcontainer features are allowed.
type FeaturesConfig struct {
	Allow []string `yaml:"allow,omitempty"`
}

// AgentConfig contains configuration for a specific AI agent.
type AgentConfig struct {
	Command    string   `yaml:"command,omitempty"`
	Env        []string `yaml:"env,omitempty"`
	AuthMethod string   `yaml:"auth_method,omitempty"`      // "token" or "api_key"
	Token      string   `yaml:"token,omitempty"`            // long-lived OAuth token
	APIKey     string   `yaml:"api_key,omitempty"`          // Anthropic API key
	SkipPerms  *bool    `yaml:"skip_permissions,omitempty"` // default true
}

// DefaultsConfig specifies default settings for new cloisters.
type DefaultsConfig struct {
	Image string `yaml:"image,omitempty"`
	Shell string `yaml:"shell,omitempty"`
	User  string `yaml:"user,omitempty"`
	Agent string `yaml:"agent,omitempty"`
}

// LogConfig contains logging settings.
type LogConfig struct {
	File           string `yaml:"file,omitempty"`
	Stdout         bool   `yaml:"stdout,omitempty"`
	Level          string `yaml:"level,omitempty"`
	PerCloister    bool   `yaml:"per_cloister,omitempty"`
	PerCloisterDir string `yaml:"per_cloister_dir,omitempty"`
}

// ProjectConfig represents per-project configuration.
// It is stored at ~/.config/cloister/projects/<project-name>.yaml.
type ProjectConfig struct {
	Remote   string                `yaml:"remote,omitempty"`
	Root     string                `yaml:"root,omitempty"`
	Refs     []string              `yaml:"refs,omitempty"`
	Proxy    ProjectProxyConfig    `yaml:"proxy,omitempty"`
	Hostexec ProjectHostexecConfig `yaml:"hostexec,omitempty"`
}

// ProjectProxyConfig contains project-specific proxy settings that are
// merged with the global allowlist.
type ProjectProxyConfig struct {
	Allow []AllowEntry `yaml:"allow,omitempty"`
	Deny  []AllowEntry `yaml:"deny,omitempty"`
}

// ProjectHostexecConfig contains project-specific command patterns that are
// merged with global patterns.
type ProjectHostexecConfig struct {
	AutoApprove   []CommandPattern `yaml:"auto_approve,omitempty"`
	ManualApprove []CommandPattern `yaml:"manual_approve,omitempty"`
}
