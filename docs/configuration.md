# Configuration

Cloister uses YAML configuration files to control network allowlists, hostexec patterns, and default behaviors.

## Configuration File Locations

| File | Purpose |
|------|---------|
| `~/.config/cloister/config.yaml` | Global defaults |
| `~/.config/cloister/projects/<name>.yaml` | Per-project overrides |
| `~/.config/cloister/decisions/global.yaml` | Globally approved/denied domains (from web UI) |
| `~/.config/cloister/decisions/projects/<name>.yaml` | Per-project approved/denied domains (from web UI) |

## Global Configuration

The global config applies to all cloisters unless overridden by project config.

```yaml
# ~/.config/cloister/config.yaml

# Default agent (claude or codex)
defaults:
  agent: claude  # or "codex"

# Network allowlist (applied to all projects)
proxy:
  allow:
    - domain: api.anthropic.com
    - domain: api.openai.com
    - domain: registry.npmjs.org
    - domain: proxy.golang.org
  # Options: "reject" or "request_approval"
  unlisted_domain_behavior: request_approval

# Hostexec patterns
hostexec:
  # Commands matching these patterns auto-approve
  auto_approve:
    - pattern: "^go mod tidy$"
    - pattern: "^npm install$"
  # Commands matching these patterns require manual approval
  manual_approve:
    - pattern: "^docker compose"
```

## Per-Project Configuration

Project configs override or extend global settings:

```yaml
# ~/.config/cloister/projects/my-api.yaml

# Additional allowed domains for this project
proxy:
  allow:
    - domain: docs.example.com
    - domain: internal-registry.company.com

# Project-specific command patterns
hostexec:
  auto_approve:
    - pattern: "^docker compose up"
    - pattern: "^docker compose down"
```

## Using the Config Command

```bash
# Show current effective config
cloister config show

# Edit global config in $EDITOR
cloister config edit

# Edit project config
cloister project edit my-api
```

## Network Allowlist

The allowlist controls which domains containers can reach through the guardian proxy.

### Default Allowlist

Cloister includes sensible defaults for common development:
- AI provider APIs (Anthropic, OpenAI)
- Package registries (npm, PyPI, Go proxy, crates.io)
- Documentation sites (Go docs, etc.)

### Adding Domains

```yaml
proxy:
  allow:
    # Exact domain
    - domain: docs.example.com
    # Each domain needs its own entry
    - domain: api.example.com
```

### Approved Domains

Domains approved or denied via the web UI are stored separately from static config files:

```
~/.config/cloister/decisions/
├── global.yaml                # Globally approved/denied domains
└── projects/
    └── my-api.yaml            # Per-project approved/denied domains
```

This separation ensures the guardian container only has write access to decision data, not your static configuration. At load time, static config and decision files are merged automatically.

**Decision file format:**
```yaml
# Example: ~/.config/cloister/decisions/projects/my-api.yaml
proxy:
  allow:
    - domain: docs.example.com
    - domain: internal-api.company.com
    - pattern: "*.cdn.example.com"
  deny:
    - domain: known-bad-site.example.com
```

To consolidate decisions into static config, move entries from a decision file into the corresponding config file (e.g., from `decisions/global.yaml` into `config.yaml`), then delete the decision file.

### Unlisted Domain Behavior

When a request is made to an unlisted domain:

```yaml
proxy:
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
    # Exact command only
    - pattern: "^go mod tidy$"

    # Command with any arguments
    - pattern: "^npm install"

    # Specific subcommands
    - pattern: "^docker compose (up|down|ps)"

  manual_approve:
    # Commands that need human review
    - pattern: "^git push"
    - pattern: "^rm -rf"
```

Commands not matching any pattern are denied by default.

## Agent Configuration

See [Credentials](credentials.md) for agent-specific authentication setup.

## Next Steps

- [Host Commands](host-commands.md) — How hostexec works
- [Command Reference](command-reference.md) — Full CLI documentation
