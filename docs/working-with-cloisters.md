# Working with Cloisters

A cloister is a Docker container running on an isolated network with your project mounted at `/work`. This guide covers managing cloister lifecycle and workflows.

## Starting a Cloister

From any git repository:

```bash
cd ~/projects/my-app
cloister start
```

This creates (or enters) a cloister and drops you into an interactive shell.

### Entering a Running Cloister

```bash
# From the project directory
cloister start
```

If the cloister is already running, this opens a new shell session inside it.

## Listing Cloisters

```bash
cloister list
```

Output:
```
NAME      PROJECT    BRANCH    UPTIME    STATUS
my-app    my-app     main      2h 15m    running
frontend  frontend   dev       45m       running
```

## Stopping Cloisters

```bash
# Stop cloister for current directory
cloister stop

# Stop by name
cloister stop my-app
```

Stopping removes the container. Your project files (in `/work`) are unaffected since they're bind-mounted from the host.

## The Container Environment

Inside a cloister, the environment is configured for sandboxed development:

### Working Directory

Your project is mounted at `/work`:

```bash
cloister@container:/work$ pwd
/work
cloister@container:/work$ ls
README.md  src/  go.mod  ...
```

### Proxy Configuration

Network traffic routes through the guardian proxy:

```bash
cloister@container:/work$ echo $HTTPS_PROXY
http://token:TOKEN@cloister-guardian:3128
```

Programs respecting `HTTPS_PROXY` automatically use the allowlisted proxy.

### Pre-installed Tools

The default container image includes:
- Go, Node.js, Python
- Git, curl, common CLI tools
- AI agents: Claude Code, Codex CLI (configured with your credentials)

## Managing the Guardian

The guardian is a background service handling the proxy and approval UI.

```bash
# Check guardian status
cloister guardian status

# Manually start (usually auto-starts)
cloister guardian start

# Stop guardian (also stops all cloisters)
cloister guardian stop
```

## Multiple Cloisters

You can run multiple cloisters simultaneously — each is an independent container.

```bash
# Terminal 1
cd ~/projects/api
cloister start

# Terminal 2
cd ~/projects/frontend
cloister start

# List both
cloister list
```

Each cloister:
- Has its own shell sessions
- Uses the shared guardian proxy
- Can have different project configurations

## Project Detection

Cloister auto-detects projects from the git repository:

```bash
cd ~/projects/my-app
cloister start
# Creates cloister named "my-app" (from directory basename)
```

Projects are registered in `~/.config/cloister/projects.yaml`.

### Managing Projects

```bash
# List registered projects
cloister project list

# Show project details
cloister project show my-app

# Edit project config
cloister project edit my-app

# Remove project registration
cloister project remove my-app
```

## Next Steps

- [Host Commands](host-commands.md) — Running commands outside the container
- [Configuration](configuration.md) — Per-project settings
- [Troubleshooting](troubleshooting.md) — Common issues
