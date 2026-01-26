package config

import (
	"errors"
	"os"
)

// defaultConfigTemplate is the commented YAML template for the default global config.
// Using a template string provides control over comments and formatting.
const defaultConfigTemplate = `# Cloister global configuration
# See https://github.com/xdg/cloister/docs/config-reference.md for full documentation

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

    # Package registries (for in-container package installs)
    - domain: "registry.npmjs.org"
    - domain: "proxy.golang.org"
    - domain: "sum.golang.org"
    - domain: "pypi.org"
    - domain: "files.pythonhosted.org"
    - domain: "crates.io"
    - domain: "static.crates.io"

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

# Approval server configuration (host-facing)
approval:
  listen: "127.0.0.1:9999"  # Localhost only

  # Allowed command patterns (regex)
  # These bypass the approval UI and execute immediately
  # NOTE: Package installs (npm, pip, cargo, go) run inside the container
  # via proxy, not via hostexec. hostexec is for host-specific operations.
  auto_approve:
    - pattern: "^docker compose ps$"
    - pattern: "^docker compose logs.*$"

  # Patterns that require manual approval. All other requests are logged
  # and denied.
  manual_approve:
    # Dev environment lifecycle
    - pattern: "^docker compose (up|down|restart|build).*$"

    # External tools requiring credentials (human can inspect args)
    - pattern: "^gh .+$"
    - pattern: "^jira .+$"
    - pattern: "^aws .+$"
    - pattern: "^gcloud .+$"

    # Network access with full path visibility (proxy can't inspect paths)
    - pattern: "^curl .+$"
    - pattern: "^wget .+$"

# Devcontainer integration
devcontainer:
  enabled: true

  # Feature allowlist
  features:
    allow:
      - "ghcr.io/devcontainers/features/*"
      - "ghcr.io/devcontainers-contrib/features/*"

  # Always block these mounts regardless of devcontainer.json
  blocked_mounts:
    - "~/.ssh"
    - "~/.aws"
    - "~/.config/gcloud"
    - "~/.gnupg"
    - "~/.config/gh"
    - "/var/run/docker.sock"

# AI agent configurations
agents:
  claude:
    command: "claude"
    config_mount: "~/.claude"
    env:
      - "ANTHROPIC_*"
      - "CLAUDE_*"

  codex:
    command: "codex"
    config_mount: "~/.codex"
    env:
      - "OPENAI_API_KEY"

  gemini:
    command: "gemini"
    config_mount: "~/.config/gemini"
    env:
      - "GOOGLE_API_KEY"
      - "GEMINI_*"

# Default settings for new cloisters
defaults:
  image: "cloister:latest"
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
