# Claude Code Native Sandbox vs. Cloister: Comparison

## Executive Summary

Both systems sandbox AI coding agents to prevent filesystem destruction and data exfiltration. They share the same threat model (Simon Willison's "Lethal Trifecta") and both use proxy-based network filtering, but differ fundamentally in **isolation mechanism**, **agent coupling**, **credential handling**, and **human oversight model**.

| Dimension | Claude Code Native Sandbox | Cloister |
|-----------|---------------------------|----------|
| **Isolation mechanism** | OS primitives (Seatbelt/bubblewrap) — no container | Docker container with `--internal` network |
| **Agent coupling** | Tight integration with Claude Code | Agent-agnostic (any CLI tool) |
| **Credential handling** | Blocked via deny rules; web version uses scoped git proxy | Not mounted into container; git runs on host |
| **Human approval model** | Prompts on boundary violations; no general command approval | Explicit hostexec workflow with web UI |
| **Git push safety** | Local: requires manual `ask` rules; Web: proxy-validated | Requires hostexec approval (no credentials in container) |
| **Worktree support** | Per-directory config only | First-class worktree management |
| **Devcontainer support** | Not specified | Full integration with security overrides |
| **Platform requirements** | macOS or Linux (no Docker needed) | Docker/Podman on any OS |

---

## 1. Fundamental Design Difference

### Claude Code Native Sandbox
Uses **process-level isolation** via OS security primitives:
- **macOS**: `sandbox-exec` with dynamically generated Seatbelt profiles
- **Linux**: `bubblewrap` for filesystem/network namespace isolation

No container is required. The sandbox wraps the bash tool directly, enforcing restrictions at the syscall level. This is lightweight (~100ns overhead per operation) and doesn't require Docker.

### Cloister
Uses **container-level isolation** via Docker:
- Containers run on a Docker `--internal` network with no gateway
- All network traffic routes through a guardian proxy on a separate network
- Project directory is bind-mounted at `/work`

This requires Docker. The AI agent cannot access host resources that aren't explicitly mounted into the container.

---

## 2. Architecture

### Claude Code Native Sandbox
```
┌──────────────────────────────────────────────────────┐
│                     Host Machine                      │
│  ┌────────────────────────────────────────────────┐  │
│  │              Claude Code Process               │  │
│  │  ┌──────────────────────────────────────────┐  │  │
│  │  │     Sandboxed Bash Tool (srt wrapper)    │  │  │
│  │  │  • Seatbelt profile (macOS)              │  │  │
│  │  │  • bubblewrap namespace (Linux)          │  │  │
│  │  │  • HTTP/SOCKS proxy for network          │  │  │
│  │  └──────────────────────────────────────────┘  │  │
│  │  ┌──────────────────────────────────────────┐  │  │
│  │  │     Proxy Servers (on host)              │  │  │
│  │  │  • Domain allowlist enforcement          │  │  │
│  │  │  • User prompts for new domains          │  │  │
│  │  └──────────────────────────────────────────┘  │  │
│  └────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────┘
```

The sandbox runtime is an npm package (`@anthropic-ai/sandbox-runtime`) that wraps commands. It's tightly integrated with Claude Code's permission system — when a boundary is crossed, Claude Code's UI prompts the user.

### Cloister
```
┌──────────────────────────────────────────────────────────┐
│                        Host                               │
│  ┌─────────────────────────────────────────────────────┐ │
│  │    cloister-guardian (bridge + cloister-net)        │ │
│  │  ┌────────────┐ ┌────────────┐ ┌───────────────┐    │ │
│  │  │ HTTP Proxy │ │ Request Srv│ │ Approval UI   │    │ │
│  │  │ :3128      │ │ :9998      │ │ :9999 (local) │    │ │
│  │  └────────────┘ └────────────┘ └───────────────┘    │ │
│  └─────────────────────────────────────────────────────┘ │
│  ┌─────────────────────────────────────────────────────┐ │
│  │          cloister-net (--internal, no gateway)      │ │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  │ │
│  │  │ my-api      │  │ my-api-feat │  │ frontend    │  │ │
│  │  │ (claude)    │  │ (claude)    │  │ (codex)     │  │ │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  │ │
│  └─────────────────────────────────────────────────────┘ │
│  ┌─────────────────────────────────────────────────────┐ │
│  │    Host Executor (Unix socket)                      │ │
│  │    Executes approved commands outside container     │ │
│  └─────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────┘
```

Multiple cloisters share a single guardian. Each cloister authenticates with a cryptographic token. The guardian bridges between the isolated network and the internet.

---

## 3. Credential and Secret Handling

This is the most significant architectural difference.

### Claude Code Native Sandbox
Credentials exist on the host filesystem. The sandbox prevents access via:
- **Deny rules**: `Read(~/.ssh/**)`, `Read(~/.aws/**)`, etc.
- **Default write restrictions**: Only CWD is writable by default

However, the sandbox runs as the user's process. If a deny rule is misconfigured or missing, credentials are accessible. The sandbox-runtime docs explicitly warn:

> "Filesystem Permission Escalation: Overly broad filesystem write permissions can enable privilege escalation attacks."

**Claude Code on the Web** handles this differently: credentials never enter the sandbox. A custom git proxy validates operations (branch restrictions, repo destinations) and injects authentication tokens server-side.

### Cloister
Credentials **are not mounted into the container**:
- `~/.ssh`, `~/.aws`, `~/.config/gcloud`, `~/.gnupg` are explicitly omitted
- Git operations requiring authentication must go through `hostexec` (executed on host by human approval)

The container cannot access credentials because they don't exist in its filesystem namespace. However, this requires that the container configuration correctly omits these mounts — a misconfigured Dockerfile or compose file could reintroduce them.

---

## 4. Example: The Git Push Problem

### Claude Code Native Sandbox (Local)
This scenario illustrates the difference between action control and scope control:

1. If `autoAllowBashIfSandboxed: true` is set
2. And `github.com` is in `allowedHosts` (common for Go modules, npm, etc.)
3. And no explicit `ask` rule exists for `git push`

Then `git push` executes without prompting. The sandbox sees: network allowed ✓, filesystem allowed ✓ — done.

**Mitigation requires manual configuration**:
```json
{
  "permissions": {
    "ask": ["Bash(git push:*)", "Bash(git push)"]
  }
}
```

**However, `ask` rules are bypassable.** The rules are string pattern matching on the command Claude requests, not enforcement of what actually executes. If the AI invokes git push indirectly, the rule doesn't match:

- `perl -e 'system("git push")'`
- `python -c 'import os; os.system("git push")'`
- `bash -c "git push"`
- `env git push`
- Write a script, then execute it: `echo 'git push' > /tmp/x.sh && sh /tmp/x.sh`

Once a command is approved (or auto-allowed because it fits sandbox boundaries), subprocesses inherit the same sandbox but receive no further permission checks. The sandbox enforces filesystem and network restrictions at the OS level, but `ask` rules are advisory — they rely on the command string matching the pattern.

To close this loophole via action control, you'd need deny rules for every possible interpreter and indirection mechanism — a game of whack-a-mole that's inherently incomplete.

### Claude Code on the Web
The git proxy solves this elegantly:
- Git client inside sandbox authenticates to proxy with scoped credential
- Proxy validates: correct repo? correct branch? authorized operation?
- Proxy attaches real GitHub token only after validation

Unauthorized pushes are blocked server-side.

### Cloister
Git push is **inherently blocked**:
- The container has no route to GitHub (only the guardian can reach external networks)
- The container has no git credentials (never mounted)
- To push, the AI must request `hostexec git push origin feature-branch`
- This goes through the approval workflow — human reviews and approves

No configuration required for safety, but this also means legitimate pushes require human approval every time — increased friction compared to Claude Code on the Web's scoped proxy approach.

---

## 5. Network Isolation

### Claude Code Native Sandbox
- All traffic routes through HTTP/SOCKS proxies running on the host
- Proxies enforce domain allowlist
- New domains trigger user prompts
- **Linux**: Network namespace removed; traffic must go through Unix socket to proxy
- **macOS**: Seatbelt profile allows only localhost proxy port

**Limitation**: Environment variable-based (`HTTP_PROXY`, `HTTPS_PROXY`). Programs that ignore these variables get blocked entirely rather than proxied.

### Cloister
- Containers on `--internal` network have no gateway — no direct internet route exists
- Guardian container bridges `cloister-net` (internal) and `bridge` (internet)
- Proxy handles DNS resolution (containers can't reach external DNS)

Programs that ignore proxy environment variables are blocked (no route exists). This is functionally equivalent to Claude Code's Linux implementation, which also removes network namespace access. The mechanisms differ (Docker network topology vs. bubblewrap namespace) but the outcome is the same: processes cannot bypass the proxy.

---

## 6. Human Approval Model

### Claude Code Native Sandbox
- **Boundary violations trigger prompts**: New domain? Prompt. Write outside CWD? Prompt.
- **No general command approval**: If a command fits within boundaries, it executes
- **Permission rules are declarative**: `allow`, `ask`, `deny` lists in config
- **No persistent approval UI**: Prompts appear inline in Claude Code interface

This works well for the "well-meaning but overeager AI" threat model. The AI can work autonomously within boundaries; humans intervene only at boundaries.

### Cloister
- **Explicit hostexec workflow**: Commands requiring host resources go through approval server
- **Web UI for review**: Pending requests queue in browser interface
- **Three persistence levels**: Session-only, project config, global config
- **Pattern-based auto-approval**: `^go mod tidy$` can auto-approve; `^git .*` can auto-deny

This treats host command execution as a privilege requiring explicit grant, not a default with exceptions.

---

## 7. Project and Worktree Model

### Claude Code Native Sandbox
- **Per-directory configuration**: Settings in `.claude/settings.json` or `~/.claude/settings.json`
- **No worktree awareness**: Each directory is independent
- **Session-based**: Sandbox state doesn't persist across Claude Code restarts

### Cloister
- **Project identity**: Git repos identified by path, named by basename
- **Worktree-native**: `cloister start -b feature-auth` creates managed worktree + cloister
- **Shared permissions**: All worktrees of a project inherit project permissions
- **Named sessions**: Cloisters can be rejoined (`cloister start` re-enters existing cloister)

---

## 8. Devcontainer Integration

### Claude Code Native Sandbox
Not documented. The sandbox wraps bash commands regardless of environment.

### Cloister
First-class support:
- Discovers `.devcontainer/devcontainer.json` automatically
- **Build-time**: Full network access for `npm install`, etc.
- **Runtime**: Security overrides applied regardless of devcontainer.json requests
- **Blocked mounts**: `~/.ssh`, `~/.aws`, etc. blocked even if devcontainer requests them
- **Feature allowlist**: Trusted feature sources auto-allowed; unknown sources warn

---

## 9. Security Model Comparison

| Threat | Claude Code Sandbox | Cloister |
|--------|--------------------| ---------|
| Arbitrary file read | Deny rules block specific paths | Paths not mounted don't exist |
| Arbitrary file write | Default: CWD only; configurable | Default: `/work` only |
| Network exfiltration | Proxy allowlist + prompts | `--internal` network + proxy |
| Credential theft | Deny rules block paths | Credentials not mounted |
| DNS exfiltration | Proxy handles DNS | No route to external DNS |
| Git push (unintended) | Requires manual `ask` rule | Requires hostexec approval |
| Container/sandbox escape | Out of scope (OS primitives) | Out of scope (Docker) |
| Prompt injection | Boundaries contain blast radius | Boundaries + human approval |

---

## 10. Operational Differences

| Aspect | Claude Code Sandbox | Cloister |
|--------|--------------------| ---------|
| **Startup** | `/sandbox` command in Claude Code | `cloister start` (auto-starts guardian) |
| **Dependencies** | None (macOS); bubblewrap, socat (Linux) | Docker |
| **Multi-agent** | Per-Claude-Code-instance | Single guardian serves all cloisters |
| **Config location** | `~/.srt-settings.json` or Claude Code settings | `~/.config/cloister/` |
| **Violation visibility** | macOS: system log; Linux: strace | Per-cloister audit logs |
| **Session persistence** | None | Named cloisters persist until stopped |

---

## 11. When to Use Which

### Claude Code Native Sandbox
Best for:
- **Claude Code users** who want reduced permission prompts
- **macOS/Linux native workflows** without Docker overhead
- **Lightweight isolation** where credential exposure is managed via config
- **Rapid iteration** — no container startup time

Requires:
- Manually configure `ask` rules for dangerous operations
- Audit allowlists to prevent accidental exfiltration vectors
- Correct deny rules for credential protection

### Cloister
Best for:
- **Long-running autonomous sessions** where human oversight is explicit
- **Multi-agent workflows** (different tools in different cloisters)
- **Existing devcontainer setups** that should be security-hardened
- **Users who prefer credential isolation via omission** rather than deny rules

Requires:
- Docker installed and running
- Comfort with container-based workflows
- Acceptance that host operations require explicit approval
- Correct container configuration (misconfigured mounts could expose credentials)

---

## 12. Key Insight: Scope Control Philosophy

Both systems embrace **scope control over action control** — limiting where damage can occur rather than enumerating forbidden actions. But they implement it differently:

**Claude Code Sandbox**: The scope is the user's machine, restricted by policies. The AI runs as a sandboxed subprocess of the user. Credentials exist on the host filesystem but are blocked by deny rules.

**Cloister**: The scope is a container with only the project directory. The AI is a guest in a limited environment. Credentials aren't mounted into the container. Host operations are an exception requiring explicit approval.

The Claude Code approach is more ergonomic for interactive use and has lower startup overhead. The Cloister approach requires Docker and explicit approval workflows but provides credential isolation through omission rather than policy.

---

*This comparison is factual and does not express preference. Both tools address the same threat model with different trade-offs in isolation strength, operational complexity, and integration depth.*
