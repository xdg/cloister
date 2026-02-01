# Devcontainer Integration

Cloister can use a project's existing `.devcontainer/devcontainer.json` to build the container image while enforcing security restrictions at runtime.

For the default container image (when no devcontainer is present), see [container-image.md](container-image.md).

---

## Configuration Discovery

The launcher searches for devcontainer configuration in order:

1. Explicit path via `--devcontainer=<path>`
2. `.devcontainer/cloister/devcontainer.json` (cloister-specific config)
3. `.devcontainer/devcontainer.json` (standard location)

---

## Build vs Runtime Security

| Phase | Network Access | Sensitive Mounts | Rationale |
|-------|----------------|------------------|-----------|
| **Image build** | Full | None | Features download packages; no secrets present |
| **Lifecycle hooks** | Via proxy | None | `postCreateCommand` may need package registries |
| **AI runtime** | Via proxy | None | Full security model applies |

---

## Signal Handling and Container Shutdown

User-provided devcontainer images may not include an init process (like tini) to handle signals. Without proper signal handling, `docker stop` sends SIGTERM which is often ignored, causing a 10-second delay before Docker force-kills the container.

Cloister mitigates this by:
1. Using a 1-second stop timeout (`docker stop -t 1`) as a fallback
2. Optionally adding `--init` to container run arguments (uses Docker's built-in tini)

For fastest shutdown, devcontainer images should either:
- Install tini and use it as ENTRYPOINT: `ENTRYPOINT ["/usr/bin/tini", "--"]`
- Or rely on cloister's `--init` flag injection

---

## Security Overrides

Regardless of what devcontainer.json requests, the launcher enforces:

```yaml
# These settings are ALWAYS applied, overriding devcontainer.json

overrides:
  network: "cloister-net"          # Internal network only
  cap_drop: ["ALL"]                # Drop all capabilities

  blocked_mounts:                  # Never mounted, even if requested
    - "~/.ssh"
    - "~/.aws"
    - "~/.config/gcloud"
    - "~/.gnupg"
    - "~/.config/gh"
    - "/var/run/docker.sock"

  ignored_fields:                  # devcontainer.json fields ignored
    - "forwardPorts"               # Not relevant for AI agent
    - "portsAttributes"
    - "appPort"
```

---

## Feature Trust Model

Devcontainer features are OCI artifacts with install scripts. Trust levels:

| Source | Trust | Action |
|--------|-------|--------|
| `ghcr.io/devcontainers/features/*` | High | Allow by default |
| Configured allowlist | Explicit | Allow |
| Unknown | Low | Warn and require confirmation |

---

## Dual-Container Workflow

For projects using devcontainers, a recommended pattern:

![Dual-Container Workflow](diagrams/dual-container-workflow.svg) ([diagram source](diagrams/dual-container-workflow.d2))

Both containers see identical project state:
- Human edits a file → AI sees it immediately
- AI writes code → human's editor picks it up

---

## Example Configurations

### Cloister-Specific Devcontainer

```json
// .devcontainer/cloister/devcontainer.json
{
  "name": "my-project-cloister",
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "features": {
    "ghcr.io/devcontainers/features/go:1": {},
    "ghcr.io/devcontainers/features/node:1": { "version": "20" }
  },
  "postCreateCommand": "npm install && go mod download",
  "remoteUser": "vscode"
  // Note: No sensitive mounts - they'd be blocked anyway
}
```

### Shared Config with Human Devcontainer

```json
// .devcontainer/devcontainer.json (used by both human and cloister)
{
  "name": "my-project",
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "features": {
    "ghcr.io/devcontainers/features/go:1": {},
    "ghcr.io/devcontainers/features/node:1": { "version": "20" }
  },
  "mounts": [
    // These work for human devcontainer, blocked for cloister
    "source=${localEnv:HOME}/.ssh,target=/home/vscode/.ssh,type=bind,readonly",
    "source=${localEnv:HOME}/.gitconfig,target=/home/vscode/.gitconfig,type=bind,readonly"
  ],
  "forwardPorts": [3000, 8080],
  "postCreateCommand": "npm install && go mod download"
}
```

When cloister uses this config:
- Features installed normally
- `~/.ssh` and `~/.gitconfig` mounts blocked (logged as warning)
- `forwardPorts` ignored
- `postCreateCommand` runs via proxy
