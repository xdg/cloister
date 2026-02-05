package config

import (
	"errors"
	"os"
)

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

// WriteDefaultConfig creates the default global configuration file with helpful comments.
// If the config file already exists, it returns nil without overwriting.
// The config directory is created if it doesn't exist.
// The file is written with 0600 permissions (user read/write only).
func WriteDefaultConfig() error {
	path := GlobalConfigPath()

	// Check if file already exists
	_, err := os.Stat(path)
	if err == nil {
		// File exists, don't overwrite
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		// Some other error occurred
		return err
	}

	// Ensure config directory exists
	if err := EnsureConfigDir(); err != nil {
		return err
	}

	// Write the config file with user-only permissions
	return os.WriteFile(path, []byte(defaultConfigTemplate), 0600)
}

// EnsureProjectsDir creates the projects configuration directory if it
// doesn't exist. It uses 0700 permissions for security (user-only access).
// Returns nil if the directory already exists or was successfully created.
func EnsureProjectsDir() error {
	return os.MkdirAll(ProjectsDir(), 0700)
}

// InitProjectConfig creates a minimal project configuration file if it doesn't exist.
// The config file is written to ProjectConfigPath(name).
// If the config file already exists, it returns nil without overwriting.
// The projects directory is created if it doesn't exist.
// The file is written with 0600 permissions (user read/write only).
func InitProjectConfig(name string, remote string, root string) error {
	path := ProjectConfigPath(name)

	// Check if file already exists
	_, err := os.Stat(path)
	if err == nil {
		// File exists, don't overwrite
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		// Some other error occurred
		return err
	}

	// Create a minimal config with just remote and root
	cfg := &ProjectConfig{
		Remote: remote,
		Root:   root,
	}

	// Ensure projects directory exists
	if err := EnsureProjectsDir(); err != nil {
		return err
	}

	// Marshal the config to YAML
	data, err := MarshalProjectConfig(cfg)
	if err != nil {
		return err
	}

	// Write the config file with user-only permissions
	return os.WriteFile(path, data, 0600)
}

// WriteProjectConfig writes a project configuration to the projects directory.
// The config file is written to ProjectConfigPath(name).
// If the config file already exists and overwrite is false, it returns nil.
// The projects directory is created if it doesn't exist.
// The file is written with 0600 permissions (user read/write only).
func WriteProjectConfig(name string, cfg *ProjectConfig, overwrite bool) error {
	path := ProjectConfigPath(name)

	// Check if file already exists
	_, err := os.Stat(path)
	if err == nil && !overwrite {
		// File exists and overwrite is false, don't overwrite
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		// Some other error occurred
		return err
	}

	// Ensure projects directory exists
	if err := EnsureProjectsDir(); err != nil {
		return err
	}

	// Marshal the config to YAML
	data, err := MarshalProjectConfig(cfg)
	if err != nil {
		return err
	}

	// Write the config file with user-only permissions
	return os.WriteFile(path, data, 0600)
}

// WriteGlobalConfig writes a global configuration to the config directory.
// The config file is written to GlobalConfigPath().
// The config directory is created if it doesn't exist.
// The file is written with 0600 permissions (user read/write only).
// This will overwrite any existing config file.
func WriteGlobalConfig(cfg *GlobalConfig) error {
	path := GlobalConfigPath()

	// Ensure config directory exists
	if err := EnsureConfigDir(); err != nil {
		return err
	}

	// Marshal the config to YAML
	data, err := MarshalGlobalConfig(cfg)
	if err != nil {
		return err
	}

	// Write the config file with user-only permissions
	return os.WriteFile(path, data, 0600)
}
