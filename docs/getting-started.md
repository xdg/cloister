# Getting Started

This guide walks you through installing Cloister and running your first sandboxed AI agent session.

## Prerequisites

- **Docker** (or OrbStack on macOS) — Cloister runs containers on an isolated network
- **A git repository** — Cloister is designed for project-based workflows

## Installation

### Recommended: Install Script

```bash
curl -fsSL https://raw.githubusercontent.com/xdg/cloister/main/install.sh | sh
```

This downloads the latest release and installs it to `~/.local/bin`. If this directory isn't already in your PATH, the script will offer to add it to your shell configuration (bash, zsh, or fish).

To install a specific version:

```bash
VERSION=v1.0.0 curl -fsSL https://raw.githubusercontent.com/xdg/cloister/main/install.sh | sh
```

### Alternative: Build from Source

Requires Go 1.22+.

```bash
go install github.com/xdg/cloister/cmd/cloister@latest
```

Or clone and build manually:

```bash
git clone https://github.com/xdg/cloister.git
cd cloister
make build
# Binary is at ./cloister
```

## First-Time Setup

Before using Cloister, configure your AI agent credentials. For Claude Code:

```bash
cloister setup claude
```

The setup wizard will prompt you to choose an authentication method:
1. **Use existing Claude login** — If you're already logged in on the host
2. **Long-lived OAuth token** — From running `claude setup-token`
3. **API key** — From console.anthropic.com

See [Credentials](credentials.md) for details on each method.

## Your First Cloister

Navigate to any git repository and start a cloister:

```bash
cd ~/projects/my-app
cloister start
```

**What happens:**

1. The guardian proxy starts automatically (if not running)
2. A container is created on an isolated Docker network
3. Your project directory is mounted at `/work`
4. You're dropped into a shell inside the container

```
Started cloister: my-app
Project: my-app (branch: main)
Token: cloister_abc123...

Attaching interactive shell...

cloister@container:/work$
```

## Running Claude Inside the Cloister

Inside the cloister, Claude Code runs with `--dangerously-skip-permissions` by default — the sandbox provides the safety net:

```bash
cloister@container:/work$ claude
```

Claude can now:
- Read and write files in `/work` (your project)
- Access allowlisted domains (AI APIs, package registries, docs)
- Request host commands via the approval UI

## Monitoring Activity

Open http://localhost:9999 in your browser to:
- See pending hostexec requests
- Approve or deny commands

## Exiting and Stopping

```bash
# Exit the shell (container keeps running)
cloister@container:/work$ exit

# Re-enter the running cloister
cloister start

# Stop and remove the cloister
cloister stop
```

When you exit the shell:
```
Shell exited with code 0. Cloister still running.
Use 'cloister stop my-app' to terminate.
```

## Next Steps

- [Configuration](configuration.md) — Customize allowlists and settings
- [Working with Cloisters](working-with-cloisters.md) — Managing cloister lifecycle
- [Host Commands](host-commands.md) — Using hostexec for git push, docker, etc.
