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

# Go (latest stable)
ARG GO_VERSION=1.23.5
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${TARGETARCH}.tar.gz" \
    | tar -C /usr/local -xz
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

# Cloister binary (for guardian mode inside the container)
COPY cloister /usr/local/bin/cloister
RUN chmod +x /usr/local/bin/cloister

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
RUN echo '{"hasCompletedOnboarding": true}' > /home/cloister/.claude.json

# 2. Add alias to skip permission prompts (cloister is the sandbox, not Claude)
RUN echo "alias claude='claude --dangerously-skip-permissions'" >> /home/cloister/.bashrc

# Working directory for projects
WORKDIR /work

# Proxy and guardian env vars are set at runtime by cloister

CMD ["/bin/bash"]
