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
4. **Agent agnostic** — Works with any CLI-based AI coding tool
5. **Devcontainer compatible** — Leverages existing devcontainer.json configs while enforcing security

## Architecture

- **cloister**: Single Go binary with two modes:
  - CLI mode (default): Container lifecycle, project/worktree management
  - Guardian mode (`cloister guardian`): Allowlist HTTP proxy (:3128) and command approval server (:9999)
- **Cloister containers**: Docker containers on an internal network (`--internal`) with no direct egress
- **hostexec**: In-container wrapper that requests host command execution through the approval server

Key security properties:
- All network traffic routes through the allowlist proxy (AI APIs, package registries, documentation sites)
- Sensitive paths blocked: `~/.ssh`, `~/.aws`, `~/.gnupg`, Docker socket
- Containers run unprivileged with `--cap-drop=ALL`
- Host commands require human approval via web UI

## Implementation

This is a greenfield Go project. Primary components:

1. `cloister` binary — CLI + guardian mode (HTTP CONNECT proxy + approval web UI)
2. Default container image — Ubuntu 24.04 with Go/Node/Python/AI CLIs

## Further Reading

- [Full Specification](docs/cloister-spec.md) — Detailed architecture, configuration schema, API endpoints, security analysis, and implementation phases
