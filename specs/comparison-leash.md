# Leash vs. Cloister: Comprehensive Comparison

## Executive Summary

Both are AI coding agent sandboxing systems written in Go that use containers and network proxies to isolate AI agents. They share the same threat model (Simon Willison's "Lethal Trifecta") but differ substantially in **enforcement mechanism**, **policy language**, **architecture philosophy**, and **platform scope**.

| Dimension | Leash | Cloister |
|-----------|-------|----------|
| **Primary enforcement** | eBPF LSM (kernel-level) | Container isolation + domain allowlist proxy |
| **Policy language** | Cedar (declarative, auditable) | YAML regex patterns |
| **macOS support** | Native mode via system extensions | Container-only (no native) |
| **MCP awareness** | Yes (intercepts, logs, enforces) | No |
| **Command approval model** | Cedar policies (no human-in-loop by default) | Human approval web UI with auto-approve patterns |
| **Project/worktree model** | Per-directory config | Local-path-based project identity, first-class worktree support |
| **Secret handling** | Header injection at proxy boundary | Not specified (blocks credential mounts) |

---

## 1. Design Philosophy

### Leash
- **Kernel-first enforcement**: Uses eBPF LSM hooks to intercept syscalls (file_open, bprm_check_security, socket_connect) before they complete. Policy is enforced at the kernel boundary.
- **Observability-centric**: Designed around comprehensive telemetry—ring buffers stream all events to userspace. Three operational modes (Record/Shadow/Enforce) allow gradual policy rollout.
- **Policy as code**: Cedar policies are human-readable, version-controllable, and transpiled to internal representation. Policies are **declarative** rather than imperative.
- **Single-agent focus**: Each `leash` invocation wraps one agent in one container with one policy file.

### Cloister
- **Defense-in-depth via isolation**: Relies on Docker's `--internal` network (no gateway) plus an allowlist proxy. No kernel-level enforcement.
- **Human-in-the-loop**: Explicit approval workflow for commands that need host execution. The `hostexec` pattern assumes humans will review non-trivial operations.
- **Project-centric model**: Configuration is organized around git repositories (identified by local filesystem path), with worktrees as first-class citizens. Permissions apply uniformly across all worktrees of a project.
- **Multi-cloister architecture**: Single `cloister-guardian` daemon serves multiple concurrent cloisters, with per-project permissions.

---

## 2. Architecture

### Leash Architecture
```
┌─────────────────────────────────────────────────────────────┐
│                        Host                                 │
│  ┌───────────────────────────────────────────────────────┐  │
│  │              Manager Container (privileged)           │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌──────────────┐   │  │
│  │  │ eBPF LSM    │  │ MITM Proxy  │  │ Control UI   │   │  │
│  │  │ Programs    │  │ (L7 aware)  │  │ (:18080)     │   │  │
│  │  └─────────────┘  └─────────────┘  └──────────────┘   │  │
│  └───────────────────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────────────────┐  │
│  │              Agent Container (governed)               │  │
│  │  - Project bind-mounted                               │  │
│  │  - Cgroup-scoped enforcement                          │  │
│  │  - CA cert installed for MITM                         │  │
│  └───────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

**Key components:**
- **eBPF LSM programs**: `lsm_open.bpf.c`, `lsm_exec.bpf.c`, `lsm_connect.bpf.c`
- **MITM proxy**: TLS interception, header injection, L7 policy enforcement
- **Bootstrap protocol (BOOT)**: Coordinated startup sequence ensures CA installed before enforcement begins
- **Shared volumes**: `/leash` (public), `/leash-private` (manager-only, contains CA private key)

### Cloister Architecture
```
┌─────────────────────────────────────────────────────────────┐
│                        Host                                 │
│  ┌───────────────────────────────────────────────────────┐  │
│  │     cloister-guardian container + host executor       │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌──────────────┐   │  │
│  │  │ HTTP Proxy  │  │ Request Srv │  │ Approval Srv │   │  │
│  │  │ :3128       │  │ :9998       │  │ :9999 (local)│   │  │
│  │  └─────────────┘  └─────────────┘  └──────────────┘   │  │
│  └───────────────────────────────────────────────────────┘  │
│  ┌─────────────────────────────────────────────────────────┐│
│  │              cloister-net (--internal)                  ││
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐   ││
│  │  │ project-a    │  │ project-b    │  │ project-c    │   ││
│  │  │ (claude)     │  │ (codex)      │  │ (gemini)     │   ││
│  │  └──────────────┘  └──────────────┘  └──────────────┘   ││
│  └─────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

**Key components:**
- **Guardian**: Hybrid architecture—container on two networks (cloister-net + bridge) plus host process for command execution via TCP
- **Internal network**: Docker `--internal` flag prevents direct egress
- **hostexec**: In-container binary that sends commands to guardian for approval/execution
- **Per-cloister logs**: Audit trails organized by project/branch

---

## 3. Enforcement Mechanisms

### Leash: Kernel-Level (eBPF LSM)

| Operation | LSM Hook | Enforcement |
|-----------|----------|-------------|
| File access | `file_open` | Allow/deny based on path patterns, read vs write |
| Process execution | `bprm_check_security` | Allow/deny based on executable path |
| Network connect | `socket_connect` | Allow/deny based on host:port |

**Advantages:**
- Cannot be bypassed by the agent (kernel enforces)
- Sub-millisecond overhead (~100ns per hook)
- Sees all operations regardless of how invoked
- Cgroup-scoped (only affects target container)

**Requirements:**
- Linux kernel 5.7+ (for LSM BPF)
- Privileged manager container
- Not available on macOS containers (separate native mode)

### Cloister: Container + Proxy

| Operation | Mechanism | Enforcement |
|-----------|-----------|-------------|
| Network egress | `--internal` network | No direct route to internet |
| Allowed domains | HTTP CONNECT proxy | Domain allowlist |
| File access | Container isolation | Only /work (rw), /refs (ro) mounted |
| Host commands | `hostexec` → approval server | Pattern matching + human approval |

**Advantages:**
- Simpler to understand and audit
- Works on any Docker/Podman-capable system
- Human approval provides explicit control
- No kernel version requirements

**Limitations:**
- Agent could potentially bypass proxy if container escape occurred
- No visibility into file operations within container
- DNS exfiltration theoretically possible (mitigated by proxy handling DNS)

---

## 4. Policy Language

### Leash: Cedar

```cedar
// Allow read-only access to /var/app/
permit(principal, action == FileOpenReadOnly, resource)
  when { resource in Dir::"/var/app/" };

// Block writes to secrets
forbid(principal, action == FileOpenReadWrite, resource)
  when { resource in Dir::"/var/app/secrets/" };

// Allow API calls
permit(principal, action == NetworkConnect, resource)
  when { resource == Host::"api.anthropic.com" };

// Inject auth header
permit(principal, action == HttpRewrite, resource)
  when {
    resource == Host::"api.example.com" &&
    context.inject_header == "Authorization"
  };
```

**Characteristics:**
- Declarative, auditable, version-controllable
- First-match-wins evaluation
- Resource types: `Dir::`, `File::`, `Host::`, `MCP::Server::`, `MCP::Tool::`
- Actions: FileOpen*, ProcessExec, NetworkConnect, HttpRewrite, McpCall
- Hot-reloadable without process restart

### Cloister: YAML Patterns

```yaml
proxy:
  allow:
    - domain: "api.anthropic.com"
    - domain: "pkg.go.dev"

hostexec:
  auto_approve:
    - pattern: "^npm install$"
    - pattern: "^go mod tidy$"
  manual_approve:
    - pattern: "^npm install .+$"
  deny:
    - pattern: "^curl .*"
    - pattern: "^git .*"
```

**Characteristics:**
- Regex-based command matching
- Domain-only network allowlist (no path-level filtering)
- Three tiers: auto-approve, manual-approve, deny
- Per-project config files merged with global defaults

---

## 5. Network Control

### Leash
- **L7 MITM proxy**: Full visibility into HTTP methods, paths, headers, bodies
- **Secret injection**: Real credentials injected at proxy boundary; fake tokens in container
- **MCP interception**: JSON-RPC/SSE traffic inspected and logged
- **Cedar policies**: Can match on hostname, potentially path (via HttpRewrite context)

### Cloister
- **HTTP CONNECT proxy**: Domain-based allowlist only
- **No MITM by default**: Proxy handles CONNECT but doesn't decrypt TLS
- **Rate limiting**: Configurable requests/minute per cloister
- **No secret injection specified**: Relies on blocking credential mounts instead

---

## 6. Command Execution Model

### Leash
- Cedar policies can permit/forbid `ProcessExec` by path
- No explicit human-approval workflow
- The agent either can or cannot run a command based on policy
- Control UI provides visibility but not interactive approval

### Cloister
- **hostexec workflow**: Container cannot run host commands directly
- Commands sent to guardian's request server
- Three possible outcomes:
  1. **Auto-approve**: Matches safe pattern, executes immediately
  2. **Manual-approve**: Queued in web UI for human review
  3. **Deny**: Matches deny pattern, rejected immediately
- Approval server binds to `127.0.0.1:9999` (host-only)

---

## 7. Project & Configuration Model

### Leash
- **Per-directory config**: `~/.config/leash/config.toml`
- **Project sections**: `[projects."/path/to/project"]`
- **Volume prompts**: Interactive prompts for mounting agent config dirs (~/.claude, etc.)
- **Policy file**: Separate Cedar file, path specified via `--policy` or env var

### Cloister
- **Path-based identity**: Projects identified by local directory path
- **Auto-registration**: First use of a repo creates project config
- **Worktree-native**: `cloister start -b feature-auth` creates managed worktrees
- **Config hierarchy**:
  - `~/.config/cloister/config.yaml` (global)
  - `~/.config/cloister/projects/<name>.yaml` (per-project)
  - `~/.config/cloister/approvals/` (web UI domain approvals)
  - `.cloister.yaml` in repo (bootstrap template, never read at runtime)

---

## 8. Devcontainer Support

### Leash
- Not explicitly documented
- Uses custom `Dockerfile.coder` as base
- Users can extend with custom images

### Cloister
- **First-class devcontainer.json support**
- Discovery order: explicit path → `.devcontainer/cloister/` → `.devcontainer/`
- **Build-time vs runtime separation**: Full network during build, proxy-only at runtime
- **Security overrides**: Blocked mounts (~/.ssh, ~/.aws, etc.) regardless of devcontainer.json
- **Feature allowlist**: Trusted sources auto-allowed, unknown sources warn
- **Dual-container workflow**: Human devcontainer (full privileges) + cloister (restricted) share same project mount

---

## 9. macOS Support

### Leash
- **Native mode via system extensions** (macOS 14+):
  - Endpoint Security extension for file/exec monitoring
  - Network Extension for per-directory network policy
- **No Docker required** in native mode
- Requires administrator approval for extensions
- **Limitations**: No HTTP header injection, no MCP logging, no CIDR matching

### Cloister
- **Container-only**: Requires Docker/OrbStack on macOS
- No native macOS sandboxing planned

---

## 10. MCP (Model Context Protocol) Support

### Leash
- **MCP observer**: Intercepts JSON-RPC/SSE traffic
- **Tool-level policies**: `MCP::Tool::"name"` resources in Cedar
- **Server-level policies**: `MCP::Server::"host"` resources
- **Current state**: Forbid rules enforced; permit rules informational only
- **Autocomplete**: Monaco editor integration with MCP tool suggestions

### Cloister
- **Not specified**: No MCP-specific features documented

---

## 11. Observability & Telemetry

### Leash
- **Ring buffers**: eBPF programs emit events to userspace
- **Control UI**: Web interface at `:18080` with real-time event stream
- **Monaco editor**: Cedar policy editing with autocomplete
- **Statsig telemetry**: Optional anonymized usage stats (two events per run)
- **Structured logging**: Per-operation events with full context

### Cloister
- **Unified audit log**: `~/.local/share/cloister/audit.log`
- **Per-cloister logs**: `~/.local/share/cloister/logs/<cloister-name>.log`
- **Log format**: Timestamped entries with project/branch/cloister tags
- **Approval UI**: Web interface for pending requests and log viewing
- **No external telemetry specified**

---

## 12. Security Model Comparison

| Threat | Leash Mitigation | Cloister Mitigation |
|--------|------------------|---------------------|
| Arbitrary file access | eBPF LSM `file_open` hook | Container mount restrictions |
| Process execution | eBPF LSM `bprm_check_security` | Not enforced within container |
| Network exfiltration | LSM + MITM proxy | `--internal` network + proxy allowlist |
| Credential theft | Blocked mounts + secret injection | Blocked mounts (no injection) |
| DNS exfiltration | Proxy handles DNS | Proxy handles DNS |
| Container escape | Out of scope | Out of scope |
| Prompt injection | Policy enforcement | Network isolation + human approval |
| Malicious devcontainer | Not specified | Feature allowlist + mount blocking |

---

## 13. Operational Differences

| Aspect | Leash | Cloister |
|--------|-------|----------|
| **Startup** | Single command: `leash claude` | `cloister start` (auto-starts guardian) |
| **Multi-agent** | Separate invocations | Single guardian serves all |
| **Policy changes** | Hot-reload | Config file edit + guardian restart (assumed) |
| **Approval flow** | Policy-based auto | Human-in-loop with patterns |
| **Session continuity** | Per-invocation | Named cloisters can be rejoined |

---

## 14. Technology Stack

| Component | Leash | Cloister |
|-----------|-------|----------|
| **Language** | Go | Go (planned) |
| **Policy engine** | Cedar (transpiled to IR) | Regex patterns |
| **Kernel enforcement** | eBPF LSM | None |
| **Container runtime** | Docker/Podman/OrbStack | Docker/Podman |
| **Network isolation** | iptables + MITM proxy | `--internal` + HTTP CONNECT proxy |
| **UI** | Web (React, Monaco) | Web (simpler approval UI) |
| **Distribution** | npm, brew, binary releases | Planned: single binary |

---

## 15. Feature Matrix

| Feature | Leash | Cloister |
|---------|-------|----------|
| File access control | ✓ (kernel) | ✗ (container only) |
| Process execution control | ✓ (kernel) | ✗ |
| Network allowlist | ✓ | ✓ |
| HTTPS inspection | ✓ (MITM) | ✗ |
| Secret injection | ✓ | ✗ |
| MCP enforcement | ✓ | ✗ |
| Human approval workflow | ✗ | ✓ |
| Devcontainer integration | Partial | ✓ (full) |
| Git worktree management | ✗ | ✓ |
| Multi-agent daemon | ✗ | ✓ |
| macOS native mode | ✓ | ✗ |
| Policy hot-reload | ✓ | Not specified |
| Cedar policy language | ✓ | ✗ |
| Audit logging | ✓ | ✓ |
| Web control UI | ✓ | ✓ |

---

## 16. Target Audience & Use Cases

### Leash
- Teams wanting **fine-grained kernel-level control**
- Environments where **policy-as-code** is important
- Use cases requiring **MCP governance**
- macOS users wanting **native sandboxing without Docker**
- Organizations with **compliance/audit requirements**

### Cloister
- Developers wanting **explicit human oversight** of agent actions
- Teams with **existing devcontainer workflows**
- Users working on **multiple branches/worktrees simultaneously**
- Environments where **simplicity** is valued over maximum enforcement
- Use cases where **human approval** is a feature, not a limitation

---

This comparison is factual and does not express preference for either approach. Both tools address the same fundamental problem with different trade-offs in complexity, enforcement depth, and operational model.
