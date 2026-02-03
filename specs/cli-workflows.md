# Cloister CLI Workflows

This document defines cloister CLI commands through concrete scenarios.

---

## Prerequisites

- Docker or OrbStack installed and running
- `cloister` binary downloaded and in `$PATH`

---

## One-Time Setup

Before using cloister, configure your AI agent credentials.

### Claude Code

```bash
# Run Claude's OAuth flow (opens browser)
$ claude setup-token
# Displays a token valid for 1 year — copy it

# Store the token in cloister config
$ cloister setup claude
# Prompts for token (hidden input)
```

By default, this aliases `claude` to run with `--dangerously-skip-permissions` within the cloister container.

### Codex CLI

```bash
# Store your OpenAI API key in cloister config
$ cloister setup codex
# Prompts for API key (from platform.openai.com/api-keys)
```

By default, this aliases `codex` to run with `--approval-mode full-auto` within the cloister container.

### Agent Selection

The default agent is `claude`. To change it:

```bash
# Edit config to change default
$ cloister config edit
# Set defaults.agent: codex

# Or override per-session
$ cloister start --agent codex
```

See [agent-configuration.md](agent-configuration.md) for detailed setup and configuration.

---

## Scenario: Quick Start

**Goal:** See what cloister does with minimal effort.

**Starting point:** A git repository you want to work in.

```bash
$ cd ~/repos/my-project
$ cloister start
```

**What happens:**

1. Guardian starts (if not already running)
2. Project auto-detected from directory, registered as `my-project`
3. Container created on internal network
4. User dropped into shell at `/work` (bind-mounted project directory)
5. Proxy environment variables set so network goes through guardian
6. `claude` aliased with `--dangerously-skip-permissions` if so configured

```
Starting guardian: use http://localhost:9999/ to monitor activity
Detected project: my-project (from ~/repos/my-project)
Creating cloister: my-project

Entering cloister my-project. Type 'exit' to leave.
cloister:my-project:/work$
```

Open `http://localhost:9999` to monitor agent requests for exceptional domains
or to run commands on the host.

**Inside the cloister:**

```bash
# If using Claude Code (default):
cloister:my-project:/work$ claude

# If using Codex CLI:
cloister:my-project:/work$ codex

# Agent can edit files in /work, network proxied through guardian
# When done:
cloister:my-project:/work$ exit
```

After exiting, the cloister container is still running. You can get a new shell inside the cloister or you can stop and clean it up.

**Start a new shell in the container:**

```
$ cloister start
Entering cloister my-project. Type 'exit' to leave.
cloister:my-project:/work$
```

**Stop and remove the cloister:**

```
$ cloister stop
Cloister my-project stopped.
```

**Start detached (to enter from another terminal):**

```bash
# Terminal 1: start without entering
$ cloister start -d
Starting guardian: use http://localhost:9999/ to monitor activity
Cloister my-project running (detached).
Run 'cloister start' to open a shell.

# Terminal 2: enter the running cloister
$ cloister start
Entering cloister my-project.
cloister:my-project:/work$
```

**Reducing output verbosity:**

To suppress the startup messages on subsequent runs:

```bash
$ cloister config default.verbose false
```

---

## Scenario: Managing the Guardian

The guardian is a background service that handles proxy requests and the approval UI. It auto-starts on first `cloister start`, but you can manage it explicitly.

```bash
# Start guardian as background daemon
$ cloister guardian start
Guardian started (pid 12345).
Approval UI: http://localhost:9999/

# Check status
$ cloister guardian status
Guardian running (pid 12345, uptime 2h 15m).
Active cloisters: 2
Pending requests: 0

# Open approval UI in browser
$ cloister guardian open

# Stop guardian (also stops all cloisters)
$ cloister guardian stop
Stopping 2 cloisters...
Guardian stopped.
```

The guardian runs as a Docker container (`cloister-guardian`) on the `cloister-net` network.

---

## Scenario: Managing Projects

Projects are auto-registered on first use. You can view and edit their configuration.

```bash
# List registered projects
$ cloister project list
PROJECT        PATH                    CLOISTERS
my-api         ~/repos/my-api          1 running
frontend       ~/repos/frontend        0
shared-lib     ~/repos/shared-lib      0

# Show project details
$ cloister project show my-api
Project: my-api
Path: ~/repos/my-api
Remote: git@github.com:user/my-api.git
Config: ~/.config/cloister/projects/my-api.yaml

Worktrees:
  (main)       ~/repos/my-api
  feature-auth ~/.local/share/cloister/worktrees/my-api/feature-auth

Running cloisters:
  my-api

# Edit project config (opens in $EDITOR)
$ cloister project edit my-api

# Remove project registration (keeps files, stops any running cloisters)
$ cloister project remove my-api
Stop 1 running cloister? [y/N] y
Cloister my-api stopped.
Project my-api removed.
```

---

## Scenario: Managing Cloisters

Top-level commands operate on cloisters. When run from a project directory, they target the cloister for that project.

```bash
# List all running cloisters
$ cloister list
CLOISTER              PROJECT      STATUS    UPTIME
my-api                my-api       running   2h 15m
my-api-feature-auth   my-api       running   30m
frontend              frontend     running   45m

# Start/enter cloister (from repo directory)
$ cd ~/repos/my-api
$ cloister start

# Start detached
$ cloister start -d

# Stop cloister for current directory
$ cloister stop

# Stop specific cloister by name (from anywhere)
$ cloister stop my-api

# Stop all cloisters
$ cloister stop --all

# Navigate to a cloister's directory
$ cd $(cloister path my-api)
```

Use `-p <project>` to specify a project explicitly:

```
# List cloisters for a specific project
$ cloister list -p another-proj
```

---

## Scenario: Working on Multiple Branches (Worktrees)

**Goal:** Work on a feature branch in isolation while keeping the main checkout undisturbed.

**Starting point:** Project `my-api` from `~/repos/my-api` already exists.

### Create worktree and start cloister

```bash
$ cd ~/repos/my-api
$ cloister start -b feature-auth
Creating worktree: ~/.local/share/cloister/worktrees/my-api/feature-auth
Starting cloister: my-api-feature-auth

Entering cloister my-api-feature-auth. Type 'exit' to leave.
cloister:my-api-feature-auth:/work$
```

**What happens:**

1. Branch `feature-auth` created if it doesn't exist (from HEAD or tracking remote)
2. Git worktree created at `~/.local/share/cloister/worktrees/my-api/feature-auth`
3. Cloister `my-api-feature-auth` started with worktree mounted at `/work`
4. Project config (allowlists, refs) inherited from `my-api`

### List worktrees

Worktree commands operate on the project detected from the current directory. (Use `-p <project>` to specify a project explicitly.)

```bash
$ cloister worktree list
WORKTREE       PATH                                                      CLOISTER
(main)         ~/repos/my-api                                            my-api (running)
feature-auth   ~/.local/share/cloister/worktrees/my-api/feature-auth     my-api-feature-auth (running)
```

### Work in worktree from another terminal

Use `cloister path` to get the host directory for any cloister:

```bash
$ cd $(cloister path my-api-feature-auth)
$ git log --oneline -3
# See agent's commits
```

This works whether the cloister is running or not, making it easy to explore worktree directories.

### Cleanup

Worktree commands require project context (from current directory or `-p`):

```bash
# From project directory
$ cd ~/repos/my-api
$ cloister worktree remove feature-auth
Error: Worktree has uncommitted changes. Commit, stash, or use -f to force.

$ cloister worktree remove feature-auth -f
Stopping cloister my-api-feature-auth...
Removing worktree: feature-auth

# Or with explicit project
$ cloister worktree remove -p my-api feature-auth -f
```

---

## Shell Completion

Cloister provides shell completion for commands, flags, and dynamic values (cloister names, project names, worktree names).

### Setup

Generate and install a completion script for your shell:

```bash
# Bash (Linux)
cloister completion bash > ~/.local/share/bash-completion/completions/cloister

# Bash (macOS with Homebrew)
cloister completion bash > $(brew --prefix)/etc/bash_completion.d/cloister

# Zsh (add to ~/.zshrc)
eval "$(cloister completion zsh)"

# Fish
cloister completion fish > ~/.config/fish/completions/cloister.fish
```

### What Gets Completed

| Context | Completions |
|---------|-------------|
| `cloister <TAB>` | Subcommands: start, stop, list, path, guardian, project, worktree, ... |
| `cloister stop <TAB>` | Running cloister names |
| `cloister path <TAB>` | All cloister names (running + registered) |
| `cloister project show <TAB>` | Project names |
| `cloister worktree list -p <TAB>` | Project names |
| `cloister start -<TAB>` | Flags: -b, -d, -p, ... |

### Implementation Notes

Static completions (subcommands, flags) are embedded in the generated script. Dynamic completions call back into the CLI:

```bash
# Internal command used by completion scripts
cloister __complete path my-a
# Returns: my-api, my-api-feature-auth
```

Dynamic completion sources:
- **Cloister names**: Project/worktree registry + running containers
- **Project names**: `~/.config/cloister/projects/*.yaml`
- **Worktree names**: Per-project worktree registry (requires project context)

---

## Design Decisions

1. **Agent selection:** Auto-detect from config. If only one agent configured, use it. If multiple, require `default.agent` config setting or error. `--agent` flag overrides.

2. **Lifecycle:** `stop` = stop and remove container. Containers are ephemeral; valuable state lives in `/work` (bind mount). May add `--keep` later if needed.

3. **Detached start:** `start -d` creates/starts without entering. Useful for entering from a different terminal.

4. **Command structure:** Top-level commands (`start`, `stop`, `list`) operate on cloisters. Namespaced commands (`guardian *`, `project *`, `worktree *`) operate on those resources. The binary is `cloister`, so the default noun is implicit.

5. **Project naming:** Directory basename by default. If basename collides with existing project, error and require explicit name via `-p`. No auto-disambiguation.

6. **Cloister naming:** `<project>` for main checkout, `<project>-<branch>` for worktrees. Explicit name as positional arg overrides.

7. **Worktree storage:** `~/.local/share/cloister/worktrees/<project>/<branch>/`. Cloister manages these directories.

8. **Manual worktrees:** Git worktrees created via `git worktree add` are treated as independent projects (named by their directory basename). Only worktrees created via `-b` are managed by cloister and inherit project config.

9. **Cleanup safety:** `worktree remove` refuses if uncommitted changes exist (matching `git worktree remove` behavior). Use `-f` to force.

---

## Open Questions

(None currently — add questions here as scenarios reveal them.)
