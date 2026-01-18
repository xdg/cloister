# Implementation Phases

This document outlines the phased implementation plan for cloister.

---

## Phase 1: Core Infrastructure

1. **cloister binary**
   - Guardian mode (`cloister guardian`):
     - HTTP CONNECT proxy with domain allowlist
     - Approval server with web UI
     - Multi-cloister support (identification, logging)
   - CLI mode (default):
     - Container lifecycle management
     - Guardian daemon management (`cloister guardian start/stop/status`)
     - Project auto-detection and registration
     - Worktree creation and management
     - Basic agent configuration
   - Unified configuration loading

2. **Default container image**
   - Base Ubuntu 24.04
   - Go, Node, Python runtimes
   - Claude code pre-installed
   - hostexec wrapper

3. **Per-project config**
   - Auto-registration from git remote
   - Project config file management (`~/.config/cloister/projects/`)
   - Config import from `.cloister.yaml` templates
   - Per-project proxy and command allowlists

---

## Phase 2: Devcontainer Integration

1. **Devcontainer parsing**
   - Read devcontainer.json
   - Feature fetching and installation
   - Lifecycle command execution

2. **Security overrides**
   - Mount filtering
   - Network enforcement
   - Feature allowlist

3. **Build caching**
   - Hash-based image caching
   - Incremental rebuilds

---

## Phase 3: Polish

1. **Web UI improvements**
   - Per-project grouping (with branch/cloister details)
   - Approval history
   - "Always allow" rules per session
   - Real-time log streaming

2. **Developer experience**
   - Shell completion
   - Status bar integration (tmux)
   - Better error messages
   - Agent auto-detection

---

## Phase 4: Extensions

1. **Additional agents** — Add configs for other common CLI tools
2. **Path-level proxy filtering** — If domain-only proves insufficient
3. **Metrics and dashboards** — Token usage, request patterns, etc.
