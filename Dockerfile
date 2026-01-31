# Build stage: compile cloister binary
ARG GO_VERSION=1.25
FROM golang:${GO_VERSION} AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 go build -o cloister ./cmd/cloister

# Runtime stage
FROM ubuntu:24.04

ARG TARGETARCH

ENV DEBIAN_FRONTEND=noninteractive

# Force apt to use IPv4 (BuildKit's network has flaky IPv6 to Ubuntu mirrors)
RUN echo 'Acquire::ForceIPv4 "true";' > /etc/apt/apt.conf.d/99force-ipv4

# Core tools
RUN apt-get update && apt-get install -y \
    build-essential \
    ca-certificates \
    cmake \
    curl \
    dnsutils \
    fd-find \
    file \
    git \
    htop \
    jq \
    less \
    locales \
    man-db \
    nano \
    netcat-openbsd \
    openssh-client \
    pkg-config \
    procps \
    ripgrep \
    sqlite3 \
    tini \
    tree \
    unzip \
    vim \
    wget \
    xz-utils \
    zip \
    && locale-gen en_US.UTF-8 \
    && rm -rf /var/lib/apt/lists/*

ENV LANG=en_US.UTF-8

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

# Create unprivileged user (UID 1000) with passwordless sudo
# Remove existing user with UID 1000 if present (ubuntu base image has 'ubuntu' user)
RUN apt-get update && apt-get install -y sudo \
    && rm -rf /var/lib/apt/lists/* \
    && if id -u 1000 >/dev/null 2>&1; then userdel -r $(getent passwd 1000 | cut -d: -f1); fi \
    && useradd -m -s /bin/bash -u 1000 cloister \
    && echo 'cloister ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/cloister \
    && chmod 0440 /etc/sudoers.d/cloister

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

# 2. Skip-permissions alias is added dynamically by ClaudeAgent.Setup based on config

# 3. Clean up backup files created during install/config
RUN rm -f /home/cloister/.claude.json.backup.*

# hostexec wrapper for host command execution (rarely changes, so cache-friendly here)
USER root
COPY hostexec /usr/local/bin/hostexec
RUN chmod +x /usr/local/bin/hostexec

# Copy cloister binary from builder (this layer changes on source updates)
COPY --from=builder /build/cloister /usr/local/bin/cloister

# Switch back to unprivileged user
USER cloister

# Working directory for projects
WORKDIR /work

# Proxy and guardian env vars are set at runtime by cloister

# Use tini as init to handle signals properly (enables fast container shutdown)
ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["/bin/bash"]
