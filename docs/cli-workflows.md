# Cloister CLI Workflows

This document defines cloister CLI commands through concrete scenarios.

---

## Prerequisites

- Docker or OrbStack installed and running
- `cloister` binary downloaded and in `$PATH`

---

## One-Time Setup

Before using cloister, configure your AI agent credentials.

### Claude Code (Pro/Max subscription)

```bash
# Run Claude's OAuth flow (opens browser)
$ claude setup-token
# Displays a token valid for 1 year — copy it

# Store the token in cloister config
$ cloister setup claude
# Prompts for token (hidden input)
```

By default, this will alias `claude` to run with `--dangerously-skip-permissions` within the cloister container, as the cloister provides a safe sandbox.

See [agent-configuration.md](agent-configuration.md) for other agents and authentication methods.

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
2. Project auto-detected from git remote, registered as `my-project`
3. Container created on internal network
4. User dropped into shell at `/work` (bind-mounted project directory)
5. Proxy environment variables set so network goes through guardian
6. `claude` aliased with `--dangerously-skip-permissions` if so configured

```
Starting guardian: use http://localhost:9999/ to monitor activity
Detected project: my-project (from git@github.com:user/my-project.git)
Creating cloister: my-project-main (from main branch)

Entering cloister my-project-main. Type 'exit' to leave.
cloister:my-project-main:/work$
```

Open `http://localhost:9999` to monitor agent requests for exceptional domains
or to run commands on the host.

**Inside the cloister:**

```bash
cloister:my-project-main:/work$ claude
# Claude Code starts, can edit files in /work, network proxied through guardian
# When done:
cloister:my-project-main:/work$ exit
```

After exiting, the cloister container is still running. You can get a new shell inside the cloister or you can stop and clean it up.

**Start a new shell in the container:**

```
$ cloister start
Entering cloister my-project-main. Type 'exit' to leave.
cloister:my-project-main:/work$
```

**Stop and remove the cloister:**

```
$ cloister stop
Cloister my-project-main stopped.
```

**Start detached (to enter from another terminal):**

```bash
# Terminal 1: start without entering
$ cloister start -d
Starting guardian: use http://localhost:9999/ to monitor activity
Cloister my-project-main running (detached).
Run 'cloister start' to open a shell.

# Terminal 2: enter the running cloister
$ cloister start
Entering cloister my-project-main.
cloister:my-project-main:/work$
```

**Reducing output verbosity:**

To suppress the startup messages on subsequent runs:

```bash
$ cloister config default.verbose false
```

---

## Managing the Guardian

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

## Managing Projects

Projects are auto-registered on first use. You can view and edit their configuration.

```bash
# List registered projects
$ cloister project list
PROJECT        REMOTE                                  CLOISTERS
my-api         git@github.com:user/my-api.git          1 running
frontend       git@github.com:user/frontend.git        0
shared-lib     git@github.com:user/shared-lib.git      0

# Show project details
$ cloister project show my-api
Project: my-api
Remote: git@github.com:user/my-api.git
Config: ~/.config/cloister/projects/my-api.yaml

Worktrees:
  main         ~/repos/my-api (main checkout)
  feature-auth ~/.local/share/cloister/worktrees/my-api/feature-auth

Running cloisters:
  my-api-main

# Edit project config (opens in $EDITOR)
$ cloister project edit my-api

# Remove project registration (keeps files, stops any running cloisters)
$ cloister project remove my-api
Stop 1 running cloister? [y/N] y
Cloister my-api-main stopped.
Project my-api removed.
```

---

## Managing Cloisters

Top-level commands operate on cloisters. When run from a git repo, they target the cloister for that project/branch.

```bash
# List all running cloisters
$ cloister list
CLOISTER              PROJECT      BRANCH    STATUS    UPTIME
my-api-main           my-api       main      running   2h 15m
frontend-main         frontend     main      running   45m

# Start/enter cloister (from repo directory)
$ cd ~/repos/my-api
$ cloister start

# Start detached
$ cloister start -d

# Stop cloister for current repo
$ cloister stop

# Stop specific cloister by name (from anywhere)
$ cloister stop my-api-main

# Stop all cloisters
$ cloister stop --all
```

---

## Design Decisions

1. **Agent selection:** Auto-detect from config. If only one agent configured, use it. If multiple, require `default.agent` config setting or error. `--agent` flag overrides.

2. **Lifecycle:** `stop` = stop and remove container. Containers are ephemeral; valuable state lives in `/work` (bind mount). May add `--keep` later if needed.

3. **Detached start:** `start -d` creates/starts without entering. Useful for entering from a different terminal.

4. **Command structure:** Top-level commands (`start`, `stop`, `list`) operate on cloisters. Namespaced commands (`guardian *`, `project *`) operate on those resources. The binary is `cloister`, so the default noun is implicit.

## Open Questions

(None currently — add questions here as scenarios reveal them.)

---

## Scenarios To Define

- [x] Quick start
- [x] Managing guardian, projects, cloisters
- [ ] Working on multiple branches (worktrees)
- [ ] Multiple projects simultaneously
- [ ] Using devcontainer.json
- [ ] Team shared config (.cloister.yaml)
- [ ] Troubleshooting / debugging

---
