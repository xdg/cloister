FROM ubuntu:24.04

ARG TARGETARCH

ENV DEBIAN_FRONTEND=noninteractive

# Force apt to use IPv4 (BuildKit's network has flaky IPv6 to Ubuntu mirrors)
RUN echo 'Acquire::ForceIPv4 "true";' > /etc/apt/apt.conf.d/99force-ipv4

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
    tini \
    && rm -rf /var/lib/apt/lists/*

# Go (latest stable)
RUN set -eux; \
    RELEASE=$(curl -fsSL 'https://go.dev/dl/?mode=json' | \
      jq -r '[.[] | select(.stable == true)][0]'); \
    GO_VERSION=$(echo "$RELEASE" | jq -r '.version' | sed 's/^go//'); \
    SHA256=$(echo "$RELEASE" | jq -r --arg arch "go${GO_VERSION}.linux-${TARGETARCH}.tar.gz" \
      '.files[] | select(.filename == $arch) | .sha256'); \
    [ -n "$GO_VERSION" ] && [ -n "$SHA256" ]; \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${TARGETARCH}.tar.gz" -o /tmp/go.tar.gz; \
    echo "${SHA256}  /tmp/go.tar.gz" | sha256sum -c -; \
    tar -C /usr/local -xzf /tmp/go.tar.gz; \
    rm /tmp/go.tar.gz
ENV PATH="/usr/local/go/bin:${PATH}"

# Node.js LTS via NodeSource
ARG NODE_MAJOR=22
RUN curl -fsSL https://deb.nodesource.com/setup_${NODE_MAJOR}.x | bash - \
    && apt-get install -y nodejs \
    && rm -rf /var/lib/apt/lists/*

# Python
RUN apt-get update && apt-get install -y \
    python3 \
    python3-pip \
    python3-venv \
    && rm -rf /var/lib/apt/lists/*

# Create unprivileged user (UID 1000)
# Remove existing user with UID 1000 if present (ubuntu base image has 'ubuntu' user)
RUN if id -u 1000 >/dev/null 2>&1; then userdel -r $(getent passwd 1000 | cut -d: -f1); fi \
    && useradd -m -s /bin/bash -u 1000 cloister

# Build cloister binary from source (for guardian mode inside the container)
WORKDIR /tmp/cloister-build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN go build -o /usr/local/bin/cloister ./cmd/cloister \
    && rm -rf /tmp/cloister-build

# hostexec wrapper for host command execution
COPY hostexec /usr/local/bin/hostexec
RUN chmod +x /usr/local/bin/hostexec

# Switch to unprivileged user for Claude Code installation
USER cloister
WORKDIR /home/cloister

# Install Claude Code via native installer
# Installer symlinks claude to ~/.local/bin/claude
RUN mkdir -p /home/cloister/.local/bin \
    && curl -fsSL https://claude.ai/install.sh | bash

# Add Claude binary to PATH
ENV PATH="/home/cloister/.local/bin:${PATH}"

# Configure Claude Code for cloister operation
# 1. Skip onboarding prompts
RUN echo '{"hasCompletedOnboarding": true, "bypassPermissionsModeAccepted": true}' > /home/cloister/.claude.json

# 2. Add alias to skip permission prompts (cloister is the sandbox, not Claude)
RUN echo "alias claude='claude --dangerously-skip-permissions'" >> /home/cloister/.bashrc

# 3. Clean up backup files created during install/config
RUN rm -f /home/cloister/.claude.json.backup.*

# Working directory for projects
WORKDIR /work

# Proxy and guardian env vars are set at runtime by cloister

# Use tini as init to handle signals properly (enables fast container shutdown)
ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["/bin/bash"]
