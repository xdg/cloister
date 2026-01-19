# Cloister

Cloister is an AI agent sandboxing system that isolates CLI-based AI coding tools (Claude Code, Codex, Gemini CLI, etc.) in Docker containers with strict security controls.

## Threat Model: The Lethal Trifecta

Based on Simon Willison's framework, AI agents become dangerous when they have all three:

| Capability | Uncontrolled | Cloister's Control |
|------------|--------------|-------------------|
| **Private data access** | Full filesystem | Project dir only + explicit read-only refs |
| **Untrusted content exposure** | Arbitrary web | Allowlisted documentation + package registries |
| **External communication** | Any endpoint | Blocked except allowlist; human-approved commands |

Cloister breaks this trifecta by controlling all three vectors simultaneously.

## Project Goals

1. **Prevent unintentional destruction** — AI cannot modify files outside the project or corrupt system config
2. **Block data exfiltration** — Network traffic is allowlist-only via HTTP proxy; no direct external access
3. **Preserve development velocity** — Long-running sessions without constant permission prompts
4. **Maintain flexibility** — Read-only access to reference materials; controlled escape hatch via hostexec
5. **Agent agnostic** — Works with any CLI-based AI coding tool
6. **Devcontainer compatible** — Leverages existing devcontainer.json configs while enforcing security
7. **Worktree native** — First-class support for git worktrees with uniform project permissions

## Key Concepts

- **Project**: A git repository as a logical entity (identified by remote URL), owns permission config
- **Worktree**: A directory containing a working copy; main checkout or git worktree
- **Cloister**: A container session with a worktree mounted at `/work`, named `<project>-<branch>`

## Architecture

- **cloister binary**: Single Go binary with two modes:
  - CLI mode (default): Container lifecycle, project/worktree management
  - Guardian mode (`cloister guardian`): Runs as `cloister-guardian` container on `cloister-net`
- **Guardian services**:
  - Proxy Server (:3128): HTTP CONNECT proxy with domain allowlist
  - Request Server (:9998): Command execution requests from hostexec
  - Approval Server (:9999, localhost only): Web UI for human review
- **Cloister containers**: Docker containers on internal network (`--internal`) with no direct egress
- **hostexec**: In-container wrapper that requests host command execution through the request server

Key security properties:
- All network traffic routes through the allowlist proxy (AI APIs, package registries, documentation sites)
- Per-cloister tokens authenticate proxy and hostexec requests
- Sensitive paths blocked: `~/.ssh`, `~/.aws`, `~/.gnupg`, Docker socket
- Containers run unprivileged with `--cap-drop=ALL --security-opt=no-new-privileges`
- Host commands require human approval via web UI

## Implementation

This is a greenfield Go project. Primary components:

1. `cloister` binary — CLI + guardian mode (HTTP CONNECT proxy + approval web UI)
2. Default container image — Ubuntu 24.04 with Go/Node/Python/AI CLIs

**Tech choices:**
- Approval web UI uses [htmx](https://htmx.org/) (~14kb) with SSE for real-time updates
- All assets (HTML templates, htmx, CSS) embedded via `go:embed` for single-binary distribution

## Documentation

| Document | Contents |
|----------|----------|
| [cloister-spec.md](docs/cloister-spec.md) | Main specification: threat model, architecture, security analysis |
| [cli-workflows.md](docs/cli-workflows.md) | CLI commands and workflow examples |
| [guardian-api.md](docs/guardian-api.md) | Guardian endpoint reference |
| [container-image.md](docs/container-image.md) | Default Dockerfile and hostexec |
| [devcontainer-integration.md](docs/devcontainer-integration.md) | Devcontainer security and configuration |
| [config-reference.md](docs/config-reference.md) | Full configuration schema |
| [implementation-phases.md](docs/implementation-phases.md) | Development roadmap |
| [leash-cloister-comparison.md](docs/leash-cloister-comparison.md) | Comparison with similar tools |
