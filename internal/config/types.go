// Package config provides configuration types for cloister global and
// per-project settings. These types map to YAML configuration files.
package config

// GlobalConfig represents the top-level global configuration for cloister.
// It is typically stored at ~/.config/cloister/config.yaml.
type GlobalConfig struct {
	Proxy        ProxyConfig            `yaml:"proxy,omitempty"`
	Request      RequestConfig          `yaml:"request,omitempty"`
	Approval     ApprovalConfig         `yaml:"approval,omitempty"`
	Devcontainer DevcontainerConfig     `yaml:"devcontainer,omitempty"`
	Agents       map[string]AgentConfig `yaml:"agents,omitempty"`
	Defaults     DefaultsConfig         `yaml:"defaults,omitempty"`
	Log          LogConfig              `yaml:"log,omitempty"`
}

// ProxyConfig contains HTTP CONNECT proxy settings.
type ProxyConfig struct {
	Listen                 string       `yaml:"listen,omitempty"`
	Allow                  []AllowEntry `yaml:"allow,omitempty"`
	UnlistedDomainBehavior string       `yaml:"unlisted_domain_behavior,omitempty"`
	ApprovalTimeout        string       `yaml:"approval_timeout,omitempty"`
	RateLimit              int          `yaml:"rate_limit,omitempty"`
	MaxRequestBytes        int64        `yaml:"max_request_bytes,omitempty"`
}

// AllowEntry represents a single domain in an allowlist.
type AllowEntry struct {
	Domain string `yaml:"domain,omitempty"`
}

// RequestConfig contains settings for the request server that handles
// hostexec commands from containers.
type RequestConfig struct {
	Listen  string `yaml:"listen,omitempty"`
	Timeout string `yaml:"timeout,omitempty"`
}

// ApprovalConfig contains settings for the approval server that provides
// the human review interface for hostexec commands.
type ApprovalConfig struct {
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
	Command     string   `yaml:"command,omitempty"`
	ConfigMount string   `yaml:"config_mount,omitempty"`
	Env         []string `yaml:"env,omitempty"`
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
	Commands ProjectCommandsConfig `yaml:"commands,omitempty"`
}

// ProjectProxyConfig contains project-specific proxy settings that are
// merged with the global allowlist.
type ProjectProxyConfig struct {
	Allow []AllowEntry `yaml:"allow,omitempty"`
}

// ProjectCommandsConfig contains project-specific command patterns that are
// merged with global patterns.
type ProjectCommandsConfig struct {
	AutoApprove []CommandPattern `yaml:"auto_approve,omitempty"`
}
