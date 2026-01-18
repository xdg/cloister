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
--cap-drop=ALL
--security-opt=no-new-privileges
--network cloister-net  # internal only
```

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

# Proxy configuration (set by launcher)
ENV HTTP_PROXY=""
ENV HTTPS_PROXY=""
ENV NO_PROXY="localhost,127.0.0.1"

# Guardian connection (set by launcher)
ENV CLOISTER_GUARDIAN_HOST=""
ENV CLOISTER_TOKEN=""

CMD ["/bin/bash"]
```

---

## hostexec Wrapper

The `hostexec` binary allows commands to be executed on the host with human approval. It sends requests to the guardian's request server and blocks until approval/denial.

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

COMMAND="$*"

# Send request to request server and wait for response
# Token header is authoritative; body fields are informational for logging
response=$(curl -s -X POST "http://${CLOISTER_GUARDIAN_HOST}:9998/request" \
    -H "Content-Type: application/json" \
    -H "X-Cloister-Token: ${CLOISTER_TOKEN}" \
    -d "{\"cmd\": \"${COMMAND}\"}" \
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
