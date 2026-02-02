# Getting Started

This guide walks you through installing Cloister and running your first sandboxed AI agent session.

## Prerequisites

- **Docker** (or OrbStack on macOS) — Cloister runs containers on an isolated network
- **Go 1.22+** — Required for `go install`
- **A git repository** — Cloister is designed for project-based workflows

## Installation

### Using Go Install

```bash
go install github.com/xdg/cloister@latest
```

### Building from Source

```bash
git clone https://github.com/xdg/cloister.git
cd cloister
make build
# Binary is at ./cloister
```

## First-Time Setup

Before using Cloister, configure your AI agent credentials. For Claude Code:

```bash
# Run Claude's OAuth flow (opens browser)
claude setup-token
# Copy the displayed token

# Store the token in Cloister config
cloister setup claude
# Paste token when prompted
```

See [Credentials](credentials.md) for other agents and authentication methods.

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
Starting guardian: use http://localhost:9999/ to monitor activity
Detected project: my-app
Creating cloister: my-app

Entering cloister my-app. Type 'exit' to leave.
cloister:my-app:/work$
```

## Running Claude Inside the Cloister

Inside the cloister, Claude Code runs with `--dangerously-skip-permissions` by default — the sandbox provides the safety net:

```bash
cloister:my-app:/work$ claude
```

Claude can now:
- Read and write files in `/work` (your project)
- Access allowlisted domains (AI APIs, package registries, docs)
- Request host commands via the approval UI

## Monitoring Activity

Open http://localhost:9999 in your browser to:
- See active cloisters
- Review pending command requests
- Approve or deny hostexec requests

## Exiting and Stopping

```bash
# Exit the shell (container keeps running)
cloister:my-app:/work$ exit

# Re-enter the running cloister
cloister start

# Stop and remove the cloister
cloister stop
```

## Next Steps

- [Configuration](configuration.md) — Customize allowlists and settings
- [Working with Cloisters](working-with-cloisters.md) — Multiple cloisters, detached mode
- [Host Commands](host-commands.md) — Using hostexec for git push, docker, etc.
