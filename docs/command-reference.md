# Command Reference

Complete reference for all Cloister CLI commands.

## Global Flags

These flags apply to all commands:

| Flag | Description |
|------|-------------|
| `--help`, `-h` | Show help for command |
| `--version` | Show version information |

## Cloister Commands

### cloister start

Start or enter a cloister for the current project.

```bash
cloister start [--agent <name>]
```

Must be run from within a git repository. Detects the project automatically and starts a sandboxed container with the project mounted at `/work`.

**Flags:**

| Flag | Description |
|------|-------------|
| `--agent` | Override the default agent (e.g., `claude`, `codex`) |

**Behavior:**
- If no cloister exists, creates one and attaches an interactive shell
- If a cloister already exists, attaches to it
- The guardian proxy auto-starts if not running
- Agent selection: CLI flag > config default > "claude"

**Examples:**
```bash
# Start cloister for current directory (uses default agent)
cd ~/projects/my-app
cloister start

# Start with a specific agent
cloister start --agent codex
```

### cloister stop

Stop and remove a cloister.

```bash
cloister stop [name]
```

**Arguments:**
- `name` — Cloister name (optional, defaults to current project)

**Examples:**
```bash
# Stop cloister for current directory
cloister stop

# Stop specific cloister
cloister stop my-app
```

### cloister list

List running cloisters.

```bash
cloister list
```

**Aliases:** `ls`

**Output columns:**
- `NAME` — Cloister name
- `PROJECT` — Project name
- `BRANCH` — Git branch
- `UPTIME` — Time since start
- `STATUS` — running/stopped

**Example output:**
```
NAME      PROJECT    BRANCH    UPTIME    STATUS
my-app    my-app     main      2h 15m    running
```

## Guardian Commands

### cloister guardian start

Start the guardian daemon.

```bash
cloister guardian start
```

Usually not needed — the guardian auto-starts on first `cloister start`.

### cloister guardian stop

Stop the guardian and all cloisters.

```bash
cloister guardian stop
```

Warns if there are running cloister containers that will lose network access.

### cloister guardian status

Show guardian status.

```bash
cloister guardian status
```

**Output:**
```
Status: running
Uptime: 2 hours, 15 minutes
Active tokens: 2
Executor: running (PID 12345)
```

## Project Commands

### cloister project list

List registered projects.

```bash
cloister project list
```

### cloister project show

Show project details.

```bash
cloister project show <name>
```

### cloister project edit

Open project config in `$EDITOR`.

```bash
cloister project edit <name>
```

### cloister project remove

Remove project registration.

```bash
cloister project remove <name>
```

Does not delete project files, only the Cloister registration.

## Configuration Commands

### cloister config show

Display effective configuration.

```bash
cloister config show
```

### cloister config edit

Edit global configuration file in `$EDITOR`.

```bash
cloister config edit
```

### cloister config path

Show the path to the global configuration file.

```bash
cloister config path
```

### cloister config init

Create a default configuration file if one doesn't exist.

```bash
cloister config init
```

## Setup Commands

### cloister setup claude

Configure Claude Code credentials for use in cloisters.

```bash
cloister setup claude
```

Runs an interactive wizard that offers two authentication methods:
1. Long-lived OAuth token (from `claude setup-token`)
2. API key (from console.anthropic.com)

### cloister setup codex

Configure Codex CLI credentials for use in cloisters.

```bash
cloister setup codex
```

Runs an interactive wizard that prompts for:
1. OpenAI API key (from platform.openai.com/api-keys)
2. Full-auto mode preference (default: enabled)

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Invalid arguments |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `EDITOR` | Editor for `config edit` and `project edit` commands |

## Next Steps

- [Configuration](configuration.md) — Config file format
- [Troubleshooting](troubleshooting.md) — Common issues
