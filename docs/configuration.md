# Configuration

Cloister uses YAML configuration files to control network allowlists, hostexec patterns, and default behaviors.

## Configuration File Locations

| File | Purpose |
|------|---------|
| `~/.config/cloister/config.yaml` | Global defaults |
| `~/.config/cloister/projects/<name>.yaml` | Per-project overrides |

## Global Configuration

The global config applies to all cloisters unless overridden by project config.

```yaml
# ~/.config/cloister/config.yaml

# Default agent to use (if multiple configured)
default_agent: claude

# Verbose output on cloister start
verbose: true

# Network allowlist (applied to all projects)
allowed_domains:
  - api.anthropic.com
  - api.openai.com
  - registry.npmjs.org
  - proxy.golang.org
  # ... more domains

# Hostexec patterns
hostexec:
  # Commands matching these patterns auto-approve
  auto_approve:
    - "^go mod tidy$"
    - "^npm install$"

  # Commands matching these patterns are denied without prompting
  auto_deny:
    - "^rm -rf /$"
```

## Per-Project Configuration

Project configs override or extend global settings:

```yaml
# ~/.config/cloister/projects/my-api.yaml

# Additional allowed domains for this project
allowed_domains:
  - docs.example.com
  - internal-registry.company.com

# Project-specific hostexec patterns
hostexec:
  auto_approve:
    - "^docker compose up"
    - "^docker compose down"
```

## Using the Config Command

<!-- TODO: Document cloister config subcommands -->

```bash
# Show current effective config
cloister config show

# Edit global config in $EDITOR
cloister config edit

# Edit project config
cloister config edit my-api

# Set a single value
cloister config set default.verbose false
```

## Network Allowlist

The allowlist controls which domains containers can reach through the guardian proxy.

### Default Allowlist

Cloister includes sensible defaults for common development:
- AI provider APIs (Anthropic, OpenAI)
- Package registries (npm, PyPI, Go proxy, crates.io)
- Documentation sites
- GitHub/GitLab (for dependency fetching)

### Adding Domains

```yaml
allowed_domains:
  # Exact domain
  - docs.example.com

  # Wildcard subdomain
  - "*.amazonaws.com"
```

### Unlisted Domain Behavior

When a request is made to an unlisted domain:

```yaml
# Options: "reject" or "request_approval"
unlisted_domain_behavior: reject
```

With `request_approval`, the connection is held while a request appears in the approval UI.

## Hostexec Patterns

Hostexec patterns use Go regular expressions to match command strings.

### Pattern Matching

```yaml
hostexec:
  auto_approve:
    # Exact command
    - "^go mod tidy$"

    # Command with any arguments
    - "^npm install"

    # Specific subcommand
    - "^docker compose (up|down|ps)"

  auto_deny:
    # Block dangerous patterns
    - "^rm -rf"
    - "^chmod 777"
```

### Approval Scopes

When manually approving commands in the UI, you can save the pattern:
- **Session only** — Forgotten when cloister stops
- **Project** — Saved to project config
- **Global** — Saved to global config

## Agent Configuration

See [Credentials](credentials.md) for agent-specific authentication setup.

## Next Steps

- [Host Commands](host-commands.md) — How hostexec works
- [Command Reference](command-reference.md) — Full CLI documentation
