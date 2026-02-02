# Cloister Documentation

Cloister is a sandboxing system for AI coding agents. It runs tools like Claude Code in isolated Docker containers with controlled network access and human-in-the-loop approval for host operations.

## Why Cloister?

AI coding agents are powerful but risky when given unrestricted access:

- **Permission fatigue** — Constant prompts disrupt your flow
- **YOLO mode is dangerous** — Unrestricted access can corrupt your system or leak credentials
- **The middle ground** — Cloister lets agents work autonomously within safe boundaries

## How It Works

Your project runs in an unprivileged Docker container on an isolated network:

- **Filesystem isolation** — Agents see only your project directory
- **Network allowlist** — Only approved domains are reachable
- **Human-in-the-loop** — Host commands require explicit approval

## Quick Start

```bash
# Install
go install github.com/xdg/cloister@latest

# Configure Claude credentials
cloister setup claude

# Start a cloister
cd your-project
cloister start

# You're now in the sandbox
cloister:my-project:/work$ claude
```

See [Getting Started](getting-started.md) for the full walkthrough.

## Documentation

<div class="grid cards" markdown>

- :material-rocket-launch: **[Getting Started](getting-started.md)**

    Install Cloister and run your first sandboxed session

- :material-cog: **[Configuration](configuration.md)**

    Customize allowlists, patterns, and defaults

- :material-layers: **[Working with Cloisters](working-with-cloisters.md)**

    Manage containers, projects, and the guardian

- :material-key: **[Credentials](credentials.md)**

    Set up authentication for AI agents

- :material-console: **[Host Commands](host-commands.md)**

    Use hostexec for git, docker, and more

- :material-book-open-variant: **[Command Reference](command-reference.md)**

    Complete CLI documentation

- :material-help-circle: **[Troubleshooting](troubleshooting.md)**

    Common issues and solutions

</div>

## Key Features

| Feature | Status |
|---------|--------|
| Claude Code support | ✓ Available |
| Per-project allowlists | ✓ Available |
| Automatic credential injection | ✓ Available |
| Approval UI with patterns | ✓ Available |
| Git worktree support | ○ Coming soon |
| Additional agents | ○ Coming soon |

## Getting Help

- [GitHub Issues](https://github.com/xdg/cloister/issues) — Report bugs and request features
- [GitHub Discussions](https://github.com/xdg/cloister/discussions) — Ask questions and share tips
