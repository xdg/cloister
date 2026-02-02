# Command Reference

Complete reference for all Cloister CLI commands.

## Global Flags

These flags apply to all commands:

| Flag | Description |
|------|-------------|
| `--help`, `-h` | Show help for command |
| `--version` | Show version information |
| `--verbose`, `-v` | Enable verbose output |

## Cloister Commands

### cloister start

Start or enter a cloister for the current project.

```bash
cloister start [name] [flags]
```

**Arguments:**
- `name` — Cloister name (optional, defaults to project name)

**Flags:**
| Flag | Description |
|------|-------------|
| `-d`, `--detach` | Start without entering shell |
| `-b`, `--branch` | Create worktree for branch and start cloister |
| `-p`, `--project` | Specify project explicitly |

**Examples:**
```bash
# Start cloister for current directory
cloister start

# Start detached
cloister start -d

# Enter existing cloister by name
cloister start my-api

# Start cloister for a branch (creates worktree)
cloister start -b feature-auth
```

### cloister stop

Stop and remove a cloister.

```bash
cloister stop [name] [flags]
```

**Arguments:**
- `name` — Cloister name (optional, defaults to current project)

**Flags:**
| Flag | Description |
|------|-------------|
| `--all` | Stop all running cloisters |

**Examples:**
```bash
# Stop cloister for current directory
cloister stop

# Stop specific cloister
cloister stop my-api

# Stop all cloisters
cloister stop --all
```

### cloister list

List running cloisters.

```bash
cloister list [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `-p`, `--project` | Filter by project |

**Output columns:**
- `CLOISTER` — Container name
- `PROJECT` — Project name
- `STATUS` — running/stopped
- `UPTIME` — Time since start

### cloister path

Print the host path for a cloister's worktree.

```bash
cloister path [name]
```

**Examples:**
```bash
# Get path for current project's cloister
cloister path

# Navigate to a cloister's directory
cd $(cloister path my-api-feature)
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

### cloister guardian status

Show guardian status.

```bash
cloister guardian status
```

**Output:**
```
Guardian running (pid 12345, uptime 2h 15m)
Active cloisters: 2
Pending requests: 0
Approval UI: http://localhost:9999/
```

### cloister guardian open

Open the approval UI in default browser.

```bash
cloister guardian open
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

## Worktree Commands

<!-- TODO: Document when implemented -->

### cloister worktree list

List worktrees for a project.

```bash
cloister worktree list [-p project]
```

### cloister worktree remove

Remove a managed worktree.

```bash
cloister worktree remove <branch> [-f]
```

## Configuration Commands

### cloister config show

Display effective configuration.

```bash
cloister config show
```

### cloister config edit

Edit configuration file.

```bash
cloister config edit [project]
```

Without argument, edits global config. With project name, edits project config.

### cloister config set

Set a configuration value.

```bash
cloister config set <key> <value>
```

**Examples:**
```bash
cloister config set default.verbose false
cloister config set default.agent claude
```

## Setup Commands

### cloister setup

Configure an AI agent's credentials.

```bash
cloister setup <agent>
```

**Supported agents:**
- `claude` — Claude Code

**Example:**
```bash
cloister setup claude
```

Runs an interactive wizard for credential configuration.

## Shell Completion

### cloister completion

Generate shell completion script.

```bash
cloister completion <shell>
```

**Supported shells:** `bash`, `zsh`, `fish`, `powershell`

**Setup examples:**
```bash
# Bash (Linux)
cloister completion bash > ~/.local/share/bash-completion/completions/cloister

# Bash (macOS with Homebrew)
cloister completion bash > $(brew --prefix)/etc/bash_completion.d/cloister

# Zsh
echo 'eval "$(cloister completion zsh)"' >> ~/.zshrc

# Fish
cloister completion fish > ~/.config/fish/completions/cloister.fish
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Invalid arguments |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `CLOISTER_CONFIG` | Override config directory (default: `~/.config/cloister`) |
| `EDITOR` | Editor for `config edit` commands |

## Next Steps

- [Configuration](configuration.md) — Config file format
- [Troubleshooting](troubleshooting.md) — Common issues
