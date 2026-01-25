# Cloister Phase 1: Minimal Viable Cloister with Guardian

Launch a sandboxed container with guardian-proxied networking. Produces a working (if limited) system that enables basic sandboxed development with Claude Code.

## Testing Philosophy

- **Unit tests for core logic**: Token generation, allowlist matching, container naming
- **Integration tests for guardian**: Proxy authentication, domain filtering
- **Manual tests for end-to-end flows**: Container lifecycle, Claude Code operation
- **Go tests**: Use `testing` package; `httptest` for proxy handler tests
- **Table-driven tests**: Prefer table-driven tests for allowlist matching edge cases

## Verification Checklist

Before marking Phase 1 complete:

1. `go test ./...` passes
2. `go build ./cmd/cloister` produces working binary
3. Manual verification of all "Verification" items from spec:
   - [ ] `cloister start` → guardian starts if needed → container starts → shell at `/work`
   - [ ] `curl -x $HTTP_PROXY https://api.anthropic.com` succeeds
   - [ ] `curl -x $HTTP_PROXY https://github.com` fails (not in allowlist)
   - [ ] Start 2 cloisters; each authenticated with own token
   - [ ] `cloister stop` cleans up container
   - [ ] `guardian stop` warns about running cloisters
   - [ ] With `CLAUDE_CODE_OAUTH_TOKEN` set: `claude` command works inside container
4. No race conditions (`go test -race ./...`)

## Dependencies Between Phases

```
1.1 Project Scaffolding
       │
       ▼
1.2 Docker Network Setup
       │
       ▼
1.3 Guardian Proxy ◄── 1.4 Token System (parallel)
       │                    │
       └────────┬───────────┘
                ▼
1.5 Container Launch
       │
       ▼
1.6 CLI Commands
       │
       ▼
1.7 Claude Code Bootstrap
       │
       ▼
1.8 Integration & Polish
```

---

## Phase 1.1: Project Scaffolding

Set up Go module structure and build infrastructure.

### 1.1.1 Go module initialization
- [x] Initialize Go module (`go mod init github.com/xdg/cloister`)
- [x] Create `cmd/cloister/main.go` with stub main
- [x] Add `.gitignore` for Go binaries, test artifacts

### 1.1.2 CLI framework setup
- [x] Add cobra dependency for CLI
- [x] Create root command with version flag
- [x] Set up subcommand structure: `start`, `stop`, `list`, `guardian`
- [x] **Test**: Root command prints help without error

### 1.1.3 Build infrastructure
- [x] Create Makefile with `build`, `test`, `lint` targets
- [x] Add `golangci-lint` configuration (`.golangci.yml`)
- [x] Verify `make build` produces binary

---

## Phase 1.2: Docker Network Setup

Create and manage the `cloister-net` internal network.

### 1.2.1 Docker CLI integration
- [x] Create `internal/docker/docker.go` with CLI wrapper functions
- [x] Use `docker` CLI with `--format '{{json .}}'` for parseable output
- [x] Handle Docker daemon not running (check via `docker info`)
- [x] Works with Docker Desktop, OrbStack, Colima, Podman, etc.

### 1.2.2 Network management
- [x] Implement `EnsureNetwork(name string, internal bool)` function
- [x] Create `cloister-net` as internal network (no external access)
- [x] Implement `NetworkExists(name string)` check
- [x] **Test**: Create network, verify internal flag, cleanup

### 1.2.3 Network cleanup
- [x] Implement `RemoveNetworkIfEmpty(name string)` function
- [x] Handle "network in use" errors appropriately
- [x] **Test**: Removal blocked when container attached

---

## Phase 1.3: Guardian HTTP CONNECT Proxy

Implement the allowlist-enforcing HTTP CONNECT proxy.

### 1.3.1 Proxy server skeleton
- [x] Create `internal/guardian/proxy.go`
- [x] Implement basic HTTP server on :3128
- [x] Handle CONNECT method requests
- [x] Return 405 for non-CONNECT methods

### 1.3.2 Allowlist enforcement
- [x] Create `internal/guardian/allowlist.go`
- [x] Hardcode initial allowlist: `api.anthropic.com`, `api.openai.com`, `generativelanguage.googleapis.com`
- [x] Implement domain matching (exact match, no wildcards yet)
- [x] Return 403 Forbidden for non-allowed domains
- [x] **Test**: Table-driven tests for allowed/denied domains

### 1.3.3 CONNECT tunneling
- [x] Establish upstream TLS connection on allowed requests
- [x] Respond with `200 Connection Established`
- [x] Bidirectional copy between client and upstream
- [x] Handle connection timeouts and errors
- [x] **Test**: Mock upstream, verify tunnel establishment

### 1.3.4 Proxy authentication
- [x] Parse `Proxy-Authorization` header
- [x] Validate token against registered tokens (from 1.4)
- [x] Return 407 Proxy Authentication Required on missing/invalid token
- [x] Log authentication failures with source IP
- [x] **Test**: Request with valid token succeeds, invalid fails

---

## Phase 1.4: Token System

Generate and validate per-cloister authentication tokens.

### 1.4.1 Token generation
- [x] Create `internal/token/token.go`
- [x] Implement `Generate() string` using crypto/rand (32 bytes, hex encoded)
- [x] **Test**: Generated tokens are 64 hex characters, unique

### 1.4.2 Token registry
- [x] Create `internal/token/registry.go`
- [x] Implement in-memory token→cloister-name map
- [x] `Register(token, cloisterName)` and `Validate(token) (cloisterName, bool)`
- [x] `Revoke(token)` for cleanup on container stop
- [x] Thread-safe with mutex
- [x] **Test**: Register, validate, revoke lifecycle

### 1.4.3 Token injection
- [x] Pass token to container via `CLOISTER_TOKEN` env var
- [x] Pass proxy address via `HTTP_PROXY` and `HTTPS_PROXY` env vars
- [x] Format: `http://token:$CLOISTER_TOKEN@guardian:3128`

---

## Phase 1.5: Container Launch

Launch cloister containers with proper security settings.

### 1.5.1 Container configuration
- [x] Create `internal/container/config.go`
- [x] Define container create options struct
- [x] Set container name format: `cloister-<project>-<branch>`
- [x] Mount project directory at `/work` (read-write)
- [x] Set working directory to `/work`

### 1.5.2 Security hardening
- [x] Add `--cap-drop=ALL`
- [x] Add `--security-opt=no-new-privileges`
- [x] Connect only to `cloister-net` (no bridge network)
- [x] Run as non-root user (UID 1000)

### 1.5.3 Container lifecycle
- [x] Create `internal/container/manager.go`
- [x] Implement `Start(projectPath, branchName string) (containerID, error)`
- [x] Implement `Stop(containerName string) error`
- [x] Implement `List() ([]ContainerInfo, error)`
- [x] Handle container already exists (return error or attach)
- [x] **Test**: Start container, verify settings, stop, verify removal

### 1.5.4 Interactive shell attachment
- [x] Attach stdin/stdout/stderr to container
- [x] Allocate TTY for interactive use
- [x] Handle Ctrl+C gracefully (detach, don't kill)
- [x] Return exit code from shell session

---

## Phase 1.6: CLI Commands

Wire up the CLI commands to the container and guardian systems.

### 1.6.1 `cloister start` command
- [x] Detect project from current directory (git root)
- [x] Detect branch from git HEAD
- [x] Ensure guardian is running (auto-start if not)
- [x] Generate token and register with guardian
- [x] Start container with token injected
- [x] Attach interactive shell
- [x] On shell exit, leave container running (user runs `cloister stop` explicitly)

### 1.6.2 `cloister stop` command
- [x] Accept container name argument (or default to current project)
- [x] Revoke token from guardian
- [x] Stop and remove container
- [x] Print confirmation message

### 1.6.3 `cloister list` command
- [x] List all running cloister containers
- [x] Show: name, project, branch, uptime, status
- [x] Format as table

### 1.6.4 `cloister guardian` subcommands
- [x] `guardian start`: Start guardian container if not running
- [x] `guardian stop`: Stop guardian (warn if cloisters running)
- [x] `guardian status`: Show guardian status, uptime, active token count
- [x] Guardian container connected to both `cloister-net` and bridge

---

## Phase 1.7: Claude Code Bootstrap

Configure containers for Claude Code operation.

### 1.7.1 Default container image
- [x] Create `Dockerfile` for default image
- [x] Base: Ubuntu 24.04
- [x] Install: Go, Node.js, Python, curl, git
- [x] Install: Claude Code via native installer (`curl -fsSL https://claude.ai/install.sh | bash`)
- [x] Verify `claude` is in PATH (likely `~/.claude/bin`)
- [x] Create non-root user (UID 1000)
- [x] Build and tag as `cloister-default:latest`

### 1.7.2 User settings injection
- [x] Copy host `~/.claude/` into container at creation time (one-way snapshot)
- [x] Agent inherits user's settings, skills, memory, CLAUDE.md
- [x] Writes inside container are isolated (no modification of host config)
- [x] Handle missing `~/.claude/` gracefully (fresh install)

### 1.7.3 Credential injection
- [ ] Pass `ANTHROPIC_API_KEY` from host if set
- [ ] Pass `CLAUDE_CODE_OAUTH_TOKEN` from host if set
- [ ] Document this is temporary (replaced in Phase 3)

### 1.7.4 Claude Code configuration
- [ ] Create `~/.claude.json` in container with onboarding skipped
- [ ] Set up `claude --dangerously-skip-permissions` alias in bashrc
- [ ] Ensure proxy env vars visible to Claude process

---

## Phase 1.8: Integration and Polish

End-to-end testing and cleanup.

### 1.8.1 Guardian container setup
- [ ] Create guardian Dockerfile (or use same binary in guardian mode)
- [ ] `cloister guardian` runs binary with `guardian` subcommand
- [ ] Guardian container exposes :3128 to `cloister-net`
- [ ] Guardian container name: `cloister-guardian`

### 1.8.2 End-to-end integration
- [ ] **Test**: Full `cloister start` → shell → `curl` test → `exit` → `cloister stop`
- [ ] **Test**: Two concurrent cloisters with different tokens
- [ ] **Test**: Guardian restart while cloister running (should work or fail gracefully)

### 1.8.3 Error handling polish
- [ ] Clear error when Docker not running
- [ ] Clear error when not in git repository
- [ ] Clear error when guardian fails to start
- [ ] Timeout handling for proxy connections

### 1.8.4 Documentation
- [ ] Update README with Phase 1 quick-start
- [ ] Document env var requirements (`ANTHROPIC_API_KEY` or `CLAUDE_CODE_OAUTH_TOKEN`)
- [ ] Document known limitations (hardcoded allowlist, no persistence)

---

## Not In Scope (Deferred to Later Phases)

### Phase 2: Configuration System
- Config file loading and merging
- Project registry and auto-detection
- Configurable allowlists
- Token persistence across guardian restarts

### Phase 3: Claude Code Integration
- `cloister setup claude` wizard
- Credential storage in config
- Remove host env var dependency

### Phase 4: Host Execution
- hostexec wrapper
- Request and approval servers
- Approval web UI

### Future
- Worktree support
- Domain approval flow
- Devcontainer integration
