# Configuration Reference

This document contains the full configuration schemas for cloister.

Configuration files live in `~/.config/cloister/`. See [cloister-spec.md](cloister-spec.md#file-structure) for the complete file layout.

---

## Global Config Schema

```yaml
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

# Hostexec server configuration (host-facing)
hostexec:
  listen: "127.0.0.1:9999"  # Localhost only

  # Allowed command patterns (regex)
  # These bypass the approval UI and execute immediately
  # NOTE: Package installs (npm, pip, cargo, go) run inside the container
  # via proxy, not via hostexec. hostexec is for host-specific operations.
  #
  # PATTERN MATCHING
  # ----------------
  # Patterns match against the canonical command string reconstructed from
  # the args array using shell quoting rules:
  #
  #   - Simple args (alphanumeric, -_./:@+= chars): used as-is
  #   - Args with spaces or shell metacharacters: wrapped in single quotes
  #   - Embedded single quotes: escaped using POSIX '\'' idiom
  #
  # The '\'' idiom works by: ending quote (') + escaped quote (\') + new quote (')
  # The shell concatenates these adjacent strings.
  #
  # Examples of canonical strings:
  #   args: ["docker", "ps"]           → "docker ps"
  #   args: ["echo", "hello world"]    → "echo 'hello world'"
  #   args: ["echo", "it's fine"]      → "echo 'it'\''s fine'"
  #   args: ["git", "commit", "-m", "fix: bug"] → "git commit -m 'fix: bug'"
  #
  # When writing patterns for commands that may have quoted arguments:
  #   - Use .* to match any quoted content: ^echo '.*'$
  #   - Match specific quoted strings: ^echo 'hello world'$
  #   - The single quotes ARE part of the canonical string
  #
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
    env:
      - "ANTHROPIC_*"
      - "CLAUDE_*"
      - "<other env vars from claude code docs>"

  codex:
    command: "codex"
    env:
      - "OPENAI_API_KEY"

  gemini:
    command: "gemini"
    env:
      - "GOOGLE_API_KEY"
      - "GEMINI_*"
      - "<other env vars from gemini docs>"

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
```

---

## Per-Project Configuration

Project-specific configuration files are auto-created in `~/.config/cloister/projects/` on first use. These files extend or override global settings for all cloisters working on that project.

```yaml
# ~/.config/cloister/projects/my-api.yaml

# Remote URL (set automatically during project detection)
remote: "git@github.com:xdg/my-api.git"

# Canonical checkout location (for worktree creation)
root: "~/repos/my-api"

# Read-only reference mounts for this project
refs:
  - "~/repos/shared-lib"
  - "~/repos/api-docs"

# Project-specific proxy additions (merged with global allowlist)
proxy:
  allow:
    - domain: "internal-docs.company.com"
    - domain: "private-registry.company.com"

# Project-specific command patterns (merged with global patterns)
commands:
  auto_approve:
    - pattern: "^make test$"
    - pattern: "^./scripts/lint\\.sh$"
```

---

## Approval File Schema

Domains approved via the web UI are stored in a separate `approvals/` directory, not in the static config files above. This ensures the guardian container has write access only to approval data.

```yaml
# ~/.config/cloister/approvals/global.yaml
# or
# ~/.config/cloister/approvals/projects/<name>.yaml

# Exact domains approved via the web UI
domains:
  - docs.example.com
  - internal-api.company.com

# Wildcard patterns approved via the web UI
patterns:
  - "*.cdn.example.com"
```

At load time, approval files are merged with static config: global config + project config + global approvals + project approvals. Deduplication is handled automatically.

To consolidate accumulated approvals into static config, move entries from an approval file into the corresponding config file (e.g., from `approvals/global.yaml` into `config.yaml`), then delete the approval file. This is optional — both sources are merged at load time.

---

## Audit Log Format

Unified log for proxy and approval events, tagged by project, branch, and cloister.

```
# Proxy events (include project and branch context)
2024-01-15T14:32:01Z PROXY ALLOW pkg.go.dev/fmt project=my-api branch=main cloister=my-api
2024-01-15T14:32:02Z PROXY ALLOW api.anthropic.com/v1/messages project=my-api branch=feature-auth cloister=my-api-feature-auth
2024-01-15T14:32:03Z PROXY DENY github.com/api/repos project=my-api branch=main cloister=my-api reason="domain not in allowlist"

# Domain approval events
2024-01-15T14:33:00Z PROXY REQUEST project=my-api branch=main cloister=my-api domain="docs.example.com"
2024-01-15T14:33:15Z PROXY APPROVE project=my-api branch=main cloister=my-api domain="docs.example.com" scope=project user="david"
2024-01-15T14:34:00Z PROXY TIMEOUT project=my-api branch=main cloister=my-api domain="sketchy.io"

# Hostexec approval events
2024-01-15T14:32:05Z HOSTEXEC REQUEST project=my-api branch=main cloister=my-api cmd="docker compose up -d"
2024-01-15T14:32:06Z HOSTEXEC AUTO_APPROVE project=my-api branch=feature-auth cloister=my-api-feature-auth cmd="docker compose ps" pattern="^docker compose ps$"
2024-01-15T14:32:12Z HOSTEXEC APPROVE project=my-api branch=main cloister=my-api cmd="docker compose up -d" user="david"
2024-01-15T14:32:15Z HOSTEXEC COMPLETE project=my-api branch=main cloister=my-api cmd="docker compose up -d" exit=0 duration=2.3s
2024-01-15T14:35:00Z HOSTEXEC DENY project=my-api branch=main cloister=my-api cmd="docker run --privileged alpine" reason="pattern denied"

# Lifecycle events
2024-01-15T14:30:00Z CLOISTER START project=my-api branch=main cloister=my-api agent=claude devcontainer=true
2024-01-15T14:30:05Z CLOISTER LIFECYCLE project=my-api branch=main cloister=my-api hook=postCreateCommand cmd="npm install"
2024-01-15T18:45:00Z CLOISTER STOP project=my-api branch=main cloister=my-api duration=4h15m

# Non-git mode (branch marked as "-", git=false flag)
2024-01-15T14:32:01Z PROXY ALLOW pkg.go.dev project=scratch-dir branch=- cloister=scratch-dir git=false
```

