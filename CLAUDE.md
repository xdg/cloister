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
- Host commands require human approval via web UI

## Implementation

This is a greenfield Go project. Primary components:

1. `cloister` binary — CLI + guardian mode (HTTP CONNECT proxy + approval web UI)
2. Default container image — Ubuntu 24.04 with Go/Node/Python/AI CLIs

**Tech choices:**
- Approval web UI uses vanilla JavaScript with SSE for real-time updates
- All assets (HTML templates, CSS) embedded via `go:embed` for single-binary distribution

## Testing

Tests are split into three tiers based on what they require:

| Tier | Command | Docker | Guardian | What It Tests |
|------|---------|--------|----------|---------------|
| Unit | `make test` | Mocked/self-skip | In-process/mocked | Logic, handlers, protocol |
| Integration | `make test-integration` | Real | Self-managed | Lifecycle, container ops |
| E2E | `make test-e2e` | Real | TestMain-managed | Workflows assuming stable guardian |

**For coding agents:** Use `make test` for fast iteration — it covers ~90% of the test suite and runs without Docker. Use `make test-integration` or `make test-e2e` only when changing Docker/container code or before final commit, because they require Docker and may need human approval to run outside a sandbox.

**Build tags:**
- Tests in `*_integration_test.go` files use `//go:build integration` — lifecycle tests that manage their own guardian
- Tests in `test/e2e/` use `//go:build e2e` — workflow tests that share a guardian via TestMain
- All other tests are sandbox-safe (use httptest, t.TempDir(), mock interfaces)

**Shared test helpers:** `internal/testutil/` provides `RequireDocker`, `RequireGuardian`, `CleanupContainer`, and unique name generators.

## Makefile Targets

| Target | Description |
|--------|-------------|
| `make build` | Build the `cloister` binary |
| `make docker` | Build the `cloister:latest` Docker image |
| `make install` | Install binary via `go install` |
| `make test` | Run unit tests (sandbox-safe, no Docker required) |
| `make test-race` | Run unit tests with race detector |
| `make test-integration` | Run integration tests (lifecycle tests, require Docker) |
| `make test-e2e` | Run E2E tests (workflow tests with shared guardian) |
| `make test-all` | Run both integration and E2E tests |
| `make fmt` | Format code with `goimports` (run before commits) |
| `make lint` | Run `golangci-lint` |
| `make clean` | Remove built binary |
| `make diagrams` | Generate SVG diagrams from D2 sources |
| `make clean-diagrams` | Remove generated diagram SVGs |

**Tip:** Use `COUNT=1` to bypass test cache: `make test COUNT=1`

## Internal Packages

| Package | Purpose |
|---------|---------|
| `internal/agent` | Agent interface and utilities for AI agent setup in containers. Defines the `Agent` interface, provides helper functions (`CopyDirToContainer`, `WriteFileToContainer`, `MergeJSONConfig`), and includes the `ClaudeAgent` implementation. |
| `internal/cloister` | High-level orchestration for starting/stopping cloister containers with guardian integration. Coordinates token registration, container creation, and agent setup. |
| `internal/cmd` | CLI command implementations using cobra. Handles `start`, `stop`, `list`, `config`, `project`, and `guardian` subcommands. |
| `internal/config` | Configuration types, YAML parsing, validation, and merging. Manages global config (`~/.config/cloister/config.yaml`), per-project configs, and decision file I/O (`~/.config/cloister/decisions/`). |
| `internal/container` | Docker container lifecycle management. Creates containers with security constraints, manages start/stop/attach operations. |
| `internal/docker` | Low-level Docker CLI wrapper. Provides `Run`, `RunJSON`, `RunJSONLines` helpers and network management for `cloister-net`. |
| `internal/guardian` | HTTP CONNECT proxy server with domain allowlist, token validation API, and per-project allowlist caching with decision file merging. Runs inside the guardian container. |
| `internal/project` | Git repository detection and project registry. Tracks known projects in `~/.config/cloister/projects.yaml` with remote URLs and paths. |
| `internal/testutil` | Shared test helpers for Docker/guardian tests. Provides `RequireDocker`, `RequireGuardian`, `CleanupContainer`, and unique name generators. |
| `internal/token` | Cryptographic token generation, in-memory registry, and disk persistence. Also provides proxy environment variable configuration for containers. |

## Documentation

| Document | Contents |
|----------|----------|
| [cloister-spec.md](specs/cloister-spec.md) | Main specification: threat model, architecture, security analysis |
| [cli-workflows.md](specs/cli-workflows.md) | CLI commands and workflow examples |
| [agent-configuration.md](specs/agent-configuration.md) | AI agent auth setup (Claude, Codex, etc.) |
| [guardian-api.md](specs/guardian-api.md) | Guardian endpoint reference |
| [container-image.md](specs/container-image.md) | Default Dockerfile and hostexec |
| [devcontainer-integration.md](specs/devcontainer-integration.md) | Devcontainer security and configuration |
| [config-reference.md](specs/config-reference.md) | Full configuration schema |
| [implementation-phases.md](specs/implementation-phases.md) | Development roadmap |
| [comparison-leash.md](specs/comparison-leash.md) | Comparison with similar tools |
