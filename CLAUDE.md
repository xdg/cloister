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

## Testing

Tests are split into two tiers based on what they require:

| Command | What it runs | Requirements |
|---------|--------------|--------------|
| `make test` | Unit tests + sandbox-safe integration tests | None (runs in any sandbox) |
| `make test-integration` | All tests including Docker | Docker daemon running |

**For coding agents:** Use `make test` for fast iteration — it covers ~90% of the test suite and runs without Docker. Use `make test-integration` only when changing Docker/container code or before final commit, because it will require either human approval or for a human to run it outside a sandbox.

**Build tags:**
- Tests in `*_integration_test.go` files use `//go:build integration`
- These tests create real Docker containers/networks and require the daemon
- All other tests are sandbox-safe (use httptest, t.TempDir(), mock interfaces)

## Internal Packages

| Package | Purpose |
|---------|---------|
| `internal/cloister` | High-level orchestration for starting/stopping cloister containers with guardian integration. Coordinates token registration, container creation, and user settings injection. |
| `internal/cmd` | CLI command implementations using cobra. Handles `start`, `stop`, `list`, `config`, `project`, and `guardian` subcommands. |
| `internal/config` | Configuration types, YAML parsing, validation, and merging. Manages global config (`~/.config/cloister/config.yaml`) and per-project configs. |
| `internal/container` | Docker container lifecycle management. Creates containers with security constraints, manages start/stop/attach operations. |
| `internal/docker` | Low-level Docker CLI wrapper. Provides `Run`, `RunJSON`, `RunJSONLines` helpers and network management for `cloister-net`. |
| `internal/guardian` | HTTP CONNECT proxy server with domain allowlist, token validation API, and per-project allowlist caching. Runs inside the guardian container. |
| `internal/project` | Git repository detection and project registry. Tracks known projects in `~/.config/cloister/projects.yaml` with remote URLs and paths. |
| `internal/token` | Cryptographic token generation, in-memory registry, and disk persistence. Also provides proxy environment variable configuration for containers. |

## Documentation

| Document | Contents |
|----------|----------|
| [cloister-spec.md](docs/cloister-spec.md) | Main specification: threat model, architecture, security analysis |
| [cli-workflows.md](docs/cli-workflows.md) | CLI commands and workflow examples |
| [agent-configuration.md](docs/agent-configuration.md) | AI agent auth setup (Claude, Codex, etc.) |
| [guardian-api.md](docs/guardian-api.md) | Guardian endpoint reference |
| [container-image.md](docs/container-image.md) | Default Dockerfile and hostexec |
| [devcontainer-integration.md](docs/devcontainer-integration.md) | Devcontainer security and configuration |
| [config-reference.md](docs/config-reference.md) | Full configuration schema |
| [implementation-phases.md](docs/implementation-phases.md) | Development roadmap |
| [leash-cloister-comparison.md](docs/leash-cloister-comparison.md) | Comparison with similar tools |
