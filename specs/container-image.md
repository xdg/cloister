# Container Image Specification

This document describes the default cloister container image used when no devcontainer.json is present.

For devcontainer-based images, see [devcontainer-integration.md](devcontainer-integration.md).

---

## Filesystem Layout

| Path | Mode | Source |
|------|------|--------|
| `/work` | read-write | Project directory |
| `/refs` | read-only | Reference materials (other repos, docs) |
| `/home/cloister` | read-write | Container-local home |
| `/home/cloister/<AI config>` | read-write | Copied from host |

AI config folder/files are copied from the host because several tools read and write session data to them. Copying isolates the original from the host while allowing the agent access to the config directory as needed.

---

## Security Hardening

```bash
--network cloister-net  # internal only
```

Docker's default capability set is used (drops SYS_ADMIN, SYS_PTRACE, etc. while keeping SETUID/SETGID for sudo e.g. for apt). Cloister's security relies on network isolation and filesystem restrictions.

No access to:
- Docker socket
- Host SSH keys (`~/.ssh`)
- Cloud credentials (`~/.aws`, `~/.config/gcloud`)
- Host config (`~/.config`, `~/.local`)
- GPG keys (`~/.gnupg`)

---

## Default Image Contents

When no devcontainer.json is present:

- **Core:** git, curl, wget, ripgrep, jq, build-essential, etc.
- **Go:** Latest stable
- **Node:** LTS version
- **Python:** 3.11+
- **Common AI CLIs:** Pre-installed for convenience

---

## Default Dockerfile

```dockerfile
FROM ubuntu:24.04

ARG TARGETARCH

ENV DEBIAN_FRONTEND=noninteractive

# Core tools
RUN apt-get update && apt-get install -y \
    git \
    curl \
    wget \
    ripgrep \
    fd-find \
    jq \
    build-essential \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Go
ARG GO_VERSION=1.25.5
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${TARGETARCH}.tar.gz" \
    | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:${PATH}"

# Node via NodeSource
ARG NODE_MAJOR=20
RUN curl -fsSL https://deb.nodesource.com/setup_${NODE_MAJOR}.x | bash - \
    && apt-get install -y nodejs \
    && rm -rf /var/lib/apt/lists/*

# Python
RUN apt-get update && apt-get install -y \
    python3 \
    python3-pip \
    python3-venv \
    && rm -rf /var/lib/apt/lists/*

# Create unprivileged user
RUN useradd -m -s /bin/bash -u 1000 cloister

# AI CLIs (installed globally)
RUN npm install -g @anthropic-ai/claude-code

# hostexec wrapper
COPY hostexec /usr/local/bin/hostexec
RUN chmod +x /usr/local/bin/hostexec

# Switch to unprivileged user
USER cloister
WORKDIR /work

# Proxy and guardian env vars are set at runtime by cloister

CMD ["/bin/bash"]
```

---

## Runtime Environment Variables

When a cloister container starts, the launcher sets these environment variables:

| Variable | Purpose |
|----------|---------|
| `CLOISTER_TOKEN` | Authentication token for guardian proxy and hostexec requests |
| `CLOISTER_GUARDIAN_HOST` | Guardian container hostname (default: `cloister-guardian`) |
| `HTTP_PROXY` / `http_proxy` | Proxy URL with embedded credentials for HTTP traffic |
| `HTTPS_PROXY` / `https_proxy` | Proxy URL for HTTPS traffic (same as HTTP_PROXY) |

The proxy URL format is: `http://token:$CLOISTER_TOKEN@$CLOISTER_GUARDIAN_HOST:3128`

Both uppercase and lowercase proxy variables are set for maximum compatibility with different tools.

---

## Agent Configuration at Launch

When a cloister starts, the launcher configures agent-specific settings by writing to the container's home directory. This happens after container creation but before the user's shell starts.

**For Claude Code:**

1. Creates `~/.claude.json` with `{"hasCompletedOnboarding": true, "bypassPermissionsModeAccepted": true}`
2. Appends alias to `~/.bashrc`:
   ```bash
   alias claude='claude --dangerously-skip-permissions'
   ```

The alias is necessary because Claude Code's permission system is redundant inside a cloister — the cloister enforces the security boundary, so Claude's internal prompts just add friction. There is no config file option to disable permissions, so we use a shell alias.

See [agent-configuration.md](agent-configuration.md) for full details on each supported agent.

---

## hostexec Wrapper

The `hostexec` binary allows commands to be executed on the host with human approval. It sends requests to the guardian's request server and blocks until approval/denial.

**Execution flow:**
1. `hostexec` in cloister sends HTTP POST to guardian container (port 9998)
2. Guardian checks command against auto-approve patterns; if matched, proceeds to step 4
3. If manual approval required, guardian presents request in approval UI and waits
4. Guardian forwards approved command to host executor via Unix socket (`~/.local/share/cloister/hostexec.sock`)
5. Host executor executes command and returns stdout/stderr/exit code
6. Guardian returns result to `hostexec`

```bash
#!/bin/bash
# /usr/local/bin/hostexec
# Sends command to cloister-guardian for approval and execution

set -e

if [ -z "$CLOISTER_GUARDIAN_HOST" ]; then
    echo "Error: CLOISTER_GUARDIAN_HOST not set" >&2
    exit 1
fi

if [ -z "$CLOISTER_TOKEN" ]; then
    echo "Error: CLOISTER_TOKEN not set" >&2
    exit 1
fi

if [ $# -eq 0 ]; then
    echo "Usage: hostexec <command> [args...]" >&2
    exit 1
fi

# Build JSON request with args array only.
# The guardian reconstructs the canonical command string from args.
# Using jq ensures proper JSON escaping of arguments.
ARGS_JSON=$(printf '%s\n' "$@" | jq -R . | jq -s .)

# Send request to request server and wait for response
response=$(curl -s --noproxy "*" -X POST "http://${CLOISTER_GUARDIAN_HOST}:9998/request" \
    -H "Content-Type: application/json" \
    -H "X-Cloister-Token: ${CLOISTER_TOKEN}" \
    -d "{\"args\": ${ARGS_JSON}}" \
    --max-time 300)

status=$(echo "$response" | jq -r '.status // "error"')

case "$status" in
    "approved"|"auto_approved")
        exit_code=$(echo "$response" | jq -r '.exit_code // 1')
        stdout=$(echo "$response" | jq -r '.stdout // ""')
        stderr=$(echo "$response" | jq -r '.stderr // ""')

        [ -n "$stdout" ] && echo "$stdout"
        [ -n "$stderr" ] && echo "$stderr" >&2
        exit "$exit_code"
        ;;
    "denied")
        reason=$(echo "$response" | jq -r '.reason // "No reason given"')
        echo "Command denied: $reason" >&2
        exit 1
        ;;
    "timeout")
        echo "Command timed out waiting for approval" >&2
        exit 1
        ;;
    *)
        echo "Unexpected response: $response" >&2
        exit 1
        ;;
esac
```
