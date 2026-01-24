# Cloister

**Secure sandboxing for AI coding agents**

Cloister isolates CLI-based AI coding tools (Claude Code, Codex, Gemini CLI, Aider, etc.) in Docker containers with strict security controls. It breaks the "lethal trifecta" that makes AI agents dangerous: private data access, untrusted content exposure, and unrestricted external communication.

## The Problem

AI coding agents are powerful but risky. They can:
- Access your entire filesystem, including SSH keys, AWS credentials, and sensitive configs
- Fetch arbitrary content from the internet, exposing them to prompt injection
- Exfiltrate data to any external endpoint

Most users choose between two bad options: run agents with full access and hope for the best, or constantly interrupt work with permission prompts.

## The Solution

Cloister provides a third option: **secure-by-default sandboxing that doesn't kill your productivity**.

| Capability | Uncontrolled | With Cloister |
|------------|--------------|---------------|
| **File access** | Full filesystem | Project directory only |
| **Network access** | Anywhere | Allowlisted domains only |
| **Sensitive paths** | Exposed | Blocked (~/.ssh, ~/.aws, etc.) |
| **Host commands** | Direct execution | Human approval required |

## Features

- **Agent agnostic** — Works with any CLI-based AI coding tool
- **Allowlist proxy** — Network traffic restricted to approved domains (AI APIs, package registries, documentation)
- **Devcontainer compatible** — Leverages existing devcontainer.json while enforcing security
- **Human-in-the-loop** — Host command execution requires explicit approval via web UI
- **Zero-trust containers** — Unprivileged, capability-dropped Docker containers on internal networks

## Quick Start

```bash
# Install cloister
go install github.com/anthropics/cloister/cmd/cloister@latest

# Start a sandboxed session for your project
cloister ./my-project

# Inside the container, your AI agent runs with restricted access
claude  # or codex, gemini, aider, etc.
```

## How It Works

```
┌─────────────────────────────────────────────────────────────┐
│  Host                                                       │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  cloister-guardian                                    │  │
│  │  ├─ Allowlist HTTP Proxy (:3128)                      │  │
│  │  └─ Command Approval Server (:9999)                   │  │
│  └───────────────────────────────────────────────────────┘  │
│                            │                                │
│         ┌──────────────────┼──────────────────┐             │
│         │    Internal Docker Network          │             │
│         │                  │                  │             │
│  ┌──────▼──────┐    ┌──────▼──────┐          │             │
│  │  Cloister   │    │  Cloister   │          │             │
│  │  Container  │    │  Container  │   ...    │             │
│  │  (project1) │    │  (project2) │          │             │
│  └─────────────┘    └─────────────┘          │             │
│         └──────────────────┴──────────────────┘             │
└─────────────────────────────────────────────────────────────┘
```

1. **Isolated containers** run on an internal Docker network with no direct internet access
2. **All HTTP(S) traffic** routes through cloister-guardian's allowlist proxy
3. **Host commands** (git push, docker build, etc.) require approval via web UI
4. **Project files** are bind-mounted; everything else is inaccessible

## Configuration

Create a `.cloister.yaml` in your project or use the global config at `~/.config/cloister/config.yaml`:

```yaml
# Domains allowed through the proxy
allowlist:
  - api.anthropic.com
  - api.openai.com
  - pypi.org
  - registry.npmjs.org
  - docs.python.org
  - pkg.go.dev

# Additional read-only paths to mount
references:
  - ~/.gitconfig:ro
  - ~/shared-libs:ro

# Commands that auto-approve (careful with these)
auto_approve:
  - git status
  - git diff
```

## Requirements

- Docker (with Docker Compose v2)
- Go 1.22+ (for building from source)
- Linux or macOS

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

Before submitting a PR:
1. Run tests: `go test ./...`
2. Run linter: `golangci-lint run`
3. Ensure your changes don't weaken security controls

## Security

If you discover a security vulnerability, please report it via [GitHub Security Advisories](https://github.com/anthropics/cloister/security/advisories/new) rather than opening a public issue.

## License

Apache License 2.0 — See [LICENSE](LICENSE) for details.

## Acknowledgments

- Simon Willison for articulating the [lethal trifecta](https://simonwillison.net/2024/Mar/5/prompt-injection-jailbreaking/) threat model
- The devcontainer community for container-based development patterns
