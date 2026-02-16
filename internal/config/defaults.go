package config

// defaultConfigTemplate is the commented YAML template for the default global config.
// Using a template string provides control over comments and formatting.
//
// IMPORTANT: This template must stay in sync with DefaultGlobalConfig() in defaults.go.
// The TestDefaultConfigTemplateMatchesDefaults test enforces this.
const defaultConfigTemplate = `# Cloister global configuration
# See https://github.com/xdg/cloister/specs/config-reference.md for full documentation

# Proxy configuration
proxy:
  listen: ":3128"

  # Allowlisted destinations
  allow:
    # Documentation sites
    - domain: "golang.org"
    - domain: "pkg.go.dev"
    - domain: "go.dev"
    - domain: "docs.rs"
    - domain: "doc.rust-lang.org"
    - domain: "docs.python.org"
    - domain: "developer.mozilla.org"
    - domain: "devdocs.io"
    - domain: "stackoverflow.com"
    - domain: "man7.org"
    - domain: "linux.die.net"
    - domain: "cppreference.com"
    - domain: "en.cppreference.com"
    - domain: "typescriptlang.org"
    - domain: "nodejs.org"
    - domain: "docs.docker.com"
    - domain: "kubernetes.io"
    - domain: "ruby-doc.org"
    - domain: "docs.npmjs.com"

    # Package registries (for in-container package installs)
    - domain: "registry.npmjs.org"
    - domain: "proxy.golang.org"
    - domain: "sum.golang.org"
    - domain: "pypi.org"
    - domain: "files.pythonhosted.org"
    - domain: "crates.io"
    - domain: "static.crates.io"
    - domain: "rubygems.org"
    - domain: "index.rubygems.org"
    - domain: "yarnpkg.com"

    # AI provider APIs (required for agents to function)
    - domain: "api.anthropic.com"
    - domain: "api.openai.com"
    - domain: "generativelanguage.googleapis.com"

  # Behavior when request hits an unlisted domain
  # "request_approval" - hold connection, create approval request, wait for human
  # "reject" - immediately return 403
  unlisted_domain_behavior: "request_approval"

  # Timeout for domain approval requests (reject if not approved in time)
  approval_timeout: "60s"

  # Rate limiting (requests per minute per cloister)
  rate_limit: 120

  # Maximum request body size (bytes)
  max_request_bytes: 10485760  # 10MB (for API calls)

# Request server configuration (container-facing)
request:
  listen: ":9998"  # Exposed on cloister-net

  # Default timeout waiting for approval
  timeout: "5m"

# Hostexec server configuration (host-facing)
hostexec:
  listen: "127.0.0.1:9999"  # Localhost only

  # Allowed command patterns (regex)
  # These bypass the approval UI and execute immediately
  # NOTE: Package installs (npm, pip, cargo, go) run inside the container
  # via proxy, not via hostexec. hostexec is for host-specific operations.
  auto_approve:
    # Read-only container inspection (safe, no side effects)
    - pattern: "^docker ps.*$"

  # Patterns that require manual approval. All other requests are logged
  # and denied.
  #
  # Security philosophy: only read-only operations by default.
  # Broad patterns like "aws .+" or "curl .+" are intentionally excluded
  # because they could be used for data exfiltration or credential abuse.
  # Network access should go through the proxy allowlist, not hostexec.
  # Users can add broader patterns to their project config if needed.
  manual_approve:
    # GitHub CLI - read-only operations only
    - pattern: "^gh pr (view|list|status|checks|diff)( .+)?$"
    - pattern: "^gh issue (view|list)( .+)?$"
    - pattern: "^gh repo view( .+)?$"
    - pattern: "^gh run (list|view|watch)( .+)?$"

# AI agent configurations
agents:
  claude:
    command: "claude"
    env:
      - "ANTHROPIC_*"
      - "CLAUDE_*"
    skip_permissions: true

  codex:
    command: "codex"
    env:
      - "OPENAI_API_KEY"

  gemini:
    command: "gemini"
    env:
      - "GOOGLE_API_KEY"
      - "GEMINI_*"

# Default settings for new cloisters
defaults:
  # image intentionally omitted — signals use of version-matched image
  shell: "/bin/bash"
  user: "cloister"
  agent: "claude"  # Default agent if not specified

# Logging
log:
  file: "~/.local/share/cloister/audit.log"
  stdout: true
  level: "info"  # debug, info, warn, error

  # Per-cloister log files (in addition to main log)
  per_cloister: true
  per_cloister_dir: "~/.local/share/cloister/logs/"
`

// boolPtr returns a pointer to a bool value.
func boolPtr(b bool) *bool {
	return &b
}

// DefaultGlobalConfig returns a GlobalConfig with all defaults populated.
// These defaults provide a secure baseline configuration for cloister.
//
// Security philosophy for hostexec patterns:
//   - AutoApprove: Only read-only commands with no side effects
//   - ManualApprove: Specific patterns for common operations; broad patterns
//     like "aws .+" or "curl .+" are intentionally excluded because they
//     could be used for data exfiltration or credential abuse
//   - Network access should go through the proxy allowlist, not hostexec
func DefaultGlobalConfig() *GlobalConfig {
	return &GlobalConfig{
		Proxy:    defaultProxyConfig(),
		Request:  RequestConfig{Listen: ":9998", Timeout: "5m"},
		Hostexec: defaultHostexecConfig(),
		Agents:   defaultAgentConfigs(),
		Defaults: DefaultsConfig{Shell: "/bin/bash", User: "cloister", Agent: "claude"},
		Log: LogConfig{
			File: "~/.local/share/cloister/audit.log", Stdout: true, Level: "info",
			PerCloister: true, PerCloisterDir: "~/.local/share/cloister/logs/",
		},
	}
}

// defaultProxyConfig returns the default proxy configuration with allowlisted domains.
func defaultProxyConfig() ProxyConfig {
	return ProxyConfig{
		Listen:                 ":3128",
		Allow:                  defaultAllowEntries(),
		UnlistedDomainBehavior: "request_approval",
		ApprovalTimeout:        "60s",
		RateLimit:              120,
		MaxRequestBytes:        10485760, // 10MB
	}
}

// defaultAllowEntries returns the default allowlist entries: doc sites, package registries, AI APIs.
func defaultAllowEntries() []AllowEntry {
	return []AllowEntry{
		// Documentation sites
		{Domain: "golang.org"}, {Domain: "pkg.go.dev"}, {Domain: "go.dev"},
		{Domain: "docs.rs"}, {Domain: "doc.rust-lang.org"}, {Domain: "docs.python.org"},
		{Domain: "developer.mozilla.org"}, {Domain: "devdocs.io"}, {Domain: "stackoverflow.com"},
		{Domain: "man7.org"}, {Domain: "linux.die.net"},
		{Domain: "cppreference.com"}, {Domain: "en.cppreference.com"},
		{Domain: "typescriptlang.org"}, {Domain: "nodejs.org"},
		{Domain: "docs.docker.com"}, {Domain: "kubernetes.io"},
		{Domain: "ruby-doc.org"}, {Domain: "docs.npmjs.com"},
		// Package registries
		{Domain: "registry.npmjs.org"}, {Domain: "proxy.golang.org"}, {Domain: "sum.golang.org"},
		{Domain: "pypi.org"}, {Domain: "files.pythonhosted.org"},
		{Domain: "crates.io"}, {Domain: "static.crates.io"},
		{Domain: "rubygems.org"}, {Domain: "index.rubygems.org"}, {Domain: "yarnpkg.com"},
		// AI provider APIs
		{Domain: "api.anthropic.com"}, {Domain: "api.openai.com"},
		{Domain: "generativelanguage.googleapis.com"},
	}
}

// defaultHostexecConfig returns the default hostexec configuration with command patterns.
func defaultHostexecConfig() HostexecConfig {
	return HostexecConfig{
		Listen: "127.0.0.1:9999",
		AutoApprove: []CommandPattern{
			{Pattern: "^docker ps.*$"},
		},
		ManualApprove: []CommandPattern{
			{Pattern: `^gh pr (view|list|status|checks|diff)( .+)?$`},
			{Pattern: `^gh issue (view|list)( .+)?$`},
			{Pattern: `^gh repo view( .+)?$`},
			{Pattern: `^gh run (list|view|watch)( .+)?$`},
		},
	}
}

// defaultAgentConfigs returns the default agent configurations.
func defaultAgentConfigs() map[string]AgentConfig {
	return map[string]AgentConfig{
		"claude": {
			Command:   "claude",
			Env:       []string{"ANTHROPIC_*", "CLAUDE_*"},
			SkipPerms: boolPtr(true),
		},
		"codex": {
			Command: "codex",
			Env:     []string{"OPENAI_API_KEY"},
		},
		"gemini": {
			Command: "gemini",
			Env:     []string{"GOOGLE_API_KEY", "GEMINI_*"},
		},
	}
}

// DefaultProjectConfig returns an empty ProjectConfig. Projects start with
// minimal configuration and are populated when a project is detected or
// explicitly configured.
func DefaultProjectConfig() *ProjectConfig {
	return &ProjectConfig{}
}
