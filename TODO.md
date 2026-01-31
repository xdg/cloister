# Phase 4: Host Execution (hostexec)

Goal: Agents can request host commands via approval workflow. A request server receives commands from containers, auto-approves or queues for human review, and executes via a host process communicating over a Unix socket.

## Testing Philosophy

Tests are split into three tiers based on what they require:

| Tier | Command | Docker | Guardian | What It Tests |
|------|---------|--------|----------|---------------|
| Unit | `make test` | Mocked/self-skip | In-process/mocked | Logic, handlers, protocol |
| Integration | `make test-integration` | Real | Self-managed | Lifecycle, container ops |
| E2E | `make test-e2e` | Real | TestMain-managed | Workflows assuming stable guardian |

**Build tags:**
- Tests in `*_integration_test.go` files use `//go:build integration` — lifecycle tests that manage their own guardian
- Tests in `test/e2e/` use `//go:build e2e` — workflow tests that share a guardian via TestMain
- All other tests are sandbox-safe (use httptest, t.TempDir(), mock interfaces)

**Shared test helpers:** `internal/testutil/` provides `RequireDocker`, `RequireGuardian`, `CleanupContainer`, and unique name generators.

**Design for testability:**
- Request server uses `http.Handler` interface for `httptest`-based testing
- Executor interface abstracts command execution (mock for tests, real `exec.Command` for production)
- Pattern matching is pure logic, fully testable without I/O
- Approval state machine testable in isolation

## Verification Checklist

Before marking Phase 4 complete:

**Automated (`make test`):**
1. All unit tests pass
2. `make build` produces working binary
3. `make lint` passes
4. `make test-race` passes

**Automated (`make test-integration`):**
5. Request server integration tests pass
6. Guardian↔executor socket communication tests pass
7. hostexec wrapper tests pass

**Manual — test approval flow:**
8. `hostexec docker compose ps` → auto-approves (if configured), output returned
9. `hostexec docker compose up -d` → appears in approval UI
10. Click Approve → command executes, output returned to container
11. Click Deny → container receives denial message
12. Wait 5 minutes without action → request times out

**Manual — test security:**
13. Request with invalid token → rejected
14. Request with mismatched workdir → rejected
15. Verify socket permissions (not world-writable)

DO NOT PROCEED TO THE NEXT ITEM IF MANUAL TESTS ARE NOT COMPLETE.

After each subphase is complete and all of its tests pass, commit relevant new
and changed files as a single, atomic commit.

## Dependencies Between Phases

```
Phase 1 (MVP + Guardian) ✓
       │
       ▼
Phase 2 (Config) ✓
       │
       ├─► Phase 3 (Claude Integration) ✓
       │
       └─► Phase 4 (hostexec) ◄── CURRENT
               │
               ▼
         Phase 5 (Worktrees)
               │
               ▼
         Phase 6 (Domain Approval)
               │
               ▼
         Phase 7 (Polish)
```

---

## Phase 4.1: Request Server Foundation

Add the request server (:9998) to the guardian container for receiving hostexec commands.

### 4.1.1 Define request/response types
- [x] Create `internal/guardian/request/types.go` with request and response structs:
  ```go
  type CommandRequest struct {
      Cmd string `json:"cmd"`
  }
  type CommandResponse struct {
      Status   string `json:"status"`              // "approved", "auto_approved", "denied", "timeout", "error"
      Pattern  string `json:"pattern,omitempty"`   // Matched pattern (for auto_approved)
      Reason   string `json:"reason,omitempty"`    // Denial/timeout reason
      ExitCode int    `json:"exit_code,omitempty"` // Command exit code
      Stdout   string `json:"stdout,omitempty"`
      Stderr   string `json:"stderr,omitempty"`
  }
  ```
- [x] **Test (unit)**: JSON marshal/unmarshal round-trip for all response variants

### 4.1.2 Implement token-based authentication middleware
- [x] Create `internal/guardian/request/auth.go` with middleware that:
  - Extracts `X-Cloister-Token` header
  - Looks up token in guardian's token registry
  - Returns 401 if missing/invalid
  - Attaches cloister metadata to request context
- [x] **Test (unit)**: Missing header → 401; invalid token → 401; valid token → context populated

### 4.1.3 Create request server skeleton
- [x] Create `internal/guardian/request/server.go` with:
  - `NewServer(tokenRegistry, patternMatcher, executor)` constructor
  - `POST /request` handler (returns placeholder response for now)
  - Bind to port 9998 on `cloister-net`
- [x] Wire into guardian startup in `internal/guardian/guardian.go`
- [x] **Test (unit)**: Server starts, `/request` endpoint responds
- [x] **Test (integration)**: Container can reach request server via `cloister-guardian:9998`

---

## Phase 4.2: Pattern Matching and Auto-Approval

Implement command pattern matching for auto-approve and manual-approve decisions.

### 4.2.1 Define pattern matcher interface
- [x] Create `internal/guardian/patterns/matcher.go` with:
  ```go
  type Matcher interface {
      Match(cmd string) MatchResult
  }
  type MatchResult struct {
      Action  Action  // AutoApprove, ManualApprove, Deny
      Pattern string  // The matched pattern (if any)
  }
  type Action int
  const (
      Deny Action = iota
      AutoApprove
      ManualApprove
  )
  ```
- [x] **Test (unit)**: Interface compiles, MatchResult struct works

### 4.2.2 Implement regex-based pattern matcher
- [x] Create `RegexMatcher` that:
  - Compiles patterns from config at construction time
  - Checks auto_approve patterns first (return AutoApprove on match)
  - Checks manual_approve patterns second (return ManualApprove on match)
  - Returns Deny if no pattern matches
- [x] Handle regex compilation errors gracefully (log and skip invalid patterns)
- [x] **Test (unit)**: `docker compose ps` matches `^docker compose ps$` → AutoApprove
- [x] **Test (unit)**: `docker compose up -d` matches `^docker compose (up|down|restart|build).*$` → ManualApprove
- [x] **Test (unit)**: `rm -rf /` matches nothing → Deny
- [x] **Test (unit)**: Invalid regex pattern → logged, skipped

### 4.2.3 Load patterns from config
- [x] Add config parsing for `approval.auto_approve` and `approval.manual_approve` in `internal/config`
- [x] Pass patterns to `RegexMatcher` during guardian initialization
- [x] Support per-project pattern additions (merged with global)
- [x] **Test (unit)**: Config with patterns → matcher initialized correctly
- [x] **Test (unit)**: Project patterns merge with global patterns

### 4.2.4 Integrate pattern matcher into request handler
- [x] In `/request` handler, call `matcher.Match(cmd)`
- [x] If AutoApprove: proceed to execution immediately
- [x] If ManualApprove: queue for approval (Phase 4.3)
- [x] If Deny: return denial response immediately
- [x] **Test (unit)**: Auto-approve pattern → executes without approval queue
- [x] **Test (unit)**: Manual-approve pattern → queued (mocked)
- [x] **Test (unit)**: No match → denied immediately

---

## Phase 4.3: Approval Queue and Pending Requests

Implement the in-memory queue for pending approval requests.

### 4.3.1 Create approval queue data structure
- [x] Create `internal/guardian/approval/queue.go` with:
  ```go
  type PendingRequest struct {
      ID        string
      Cloister  string
      Project   string
      Branch    string
      Agent     string
      Cmd       string
      Timestamp time.Time
      Response  chan<- CommandResponse  // Channel to send result back
  }
  type Queue struct {
      // Thread-safe queue operations
  }
  ```
- [x] Implement `Add(request) string` (returns ID)
- [x] Implement `Get(id) (*PendingRequest, bool)`
- [x] Implement `Remove(id)`
- [x] Implement `List() []PendingRequest` (for UI)
- [x] Generate request IDs with `crypto/rand` (8 bytes hex)
- [x] **Test (unit)**: Add/Get/Remove/List operations work correctly
- [x] **Test (unit)**: Concurrent access is safe (use `-race`)

### 4.3.2 Add timeout handling
- [x] Start timeout goroutine when request is added
- [x] On timeout: remove from queue, send timeout response on channel
- [x] Cancel timeout when request is approved/denied
- [x] Default timeout: 5 minutes (configurable in config)
- [x] **Test (unit)**: Request times out → timeout response sent
- [x] **Test (unit)**: Approved before timeout → no timeout response

### 4.3.3 Integrate queue into request handler
- [x] For ManualApprove matches:
  - Create response channel
  - Add to queue with metadata from token lookup
  - Block waiting on response channel
  - Return response to client
- [x] **Test (unit)**: Handler blocks until approval received
- [x] **Test (unit)**: Handler returns timeout response after timeout

---

## Phase 4.4: Host Executor

Implement the host-side process that executes approved commands.

### 4.4.1 Define executor interface
- [x] Create `internal/executor/executor.go` with:
  ```go
  type Executor interface {
      Execute(ctx context.Context, req ExecuteRequest) ExecuteResponse
  }
  type ExecuteRequest struct {
      Token     string
      Command   string
      Args      []string
      Workdir   string
      Env       map[string]string
      TimeoutMs int
  }
  type ExecuteResponse struct {
      Status   string // "completed", "timeout", "error"
      ExitCode int
      Stdout   string
      Stderr   string
      Error    string
  }
  ```
- [x] **Test (unit)**: Types compile and serialize correctly

### 4.4.2 Implement command executor
- [x] Create `RealExecutor` that uses `os/exec`:
  - Parse command string into executable + args (shell-free)
  - Set working directory
  - Merge environment variables
  - Capture stdout/stderr
  - Respect timeout via context
- [x] Return appropriate error for executable not found
- [x] **Test (unit)**: Execute `echo hello` → stdout contains "hello"
- [x] **Test (unit)**: Execute nonexistent command → error response
- [x] **Test (unit)**: Timeout → partial output + timeout status

### 4.4.3 Implement Unix socket listener
- [x] Create `internal/executor/socket.go` with socket server:
  - Listen on `~/.local/share/cloister/hostexec.sock`
  - Create parent directory if needed
  - Set socket permissions (0600)
  - Accept connections, spawn goroutine per connection
  - Read newline-delimited JSON request
  - Validate shared secret
  - Validate token against registry (via injected TokenValidator)
  - Validate workdir matches token's registered worktree (via injected WorkdirValidator)
  - Execute command
  - Write JSON response
  - Close connection
- [x] **Test (unit)**: Mock socket, verify request/response flow
- [x] **Test (integration)**: Real socket communication works

### 4.4.4 Implement shared secret validation
- [x] Verify secret on every request (socket.go handleConnection)
- [x] Reject with "invalid secret" if mismatch
- [x] **Test (unit)**: Wrong secret → rejected (TestSocketServerInvalidSecret)
- [x] **Test (unit)**: Correct secret → proceeds (TestSocketServerValidRequest)

### 4.4.5 Implement workdir validation
- [x] Compare request workdir against registered path (via injected WorkdirValidator)
- [x] Reject with "workdir mismatch" if different
- [x] **Test (unit)**: Matching workdir → proceeds (TestSocketServerWorkdirValidation)
- [x] **Test (unit)**: Mismatched workdir → rejected with clear error

### 4.4.6 Start executor with guardian
- [x] Generate 32-byte secret at guardian start (use token.Generate())
- [x] Add `cloister guardian start` to spawn executor process
- [x] Create socket before starting guardian container
- [x] Bind-mount socket into guardian container at `/var/run/hostexec.sock`
- [x] Pass shared secret to executor process via environment variable
- [x] Pass shared secret to guardian container via environment variable
- [x] Wire TokenValidator to look up token in registry for worktree path
- [x] Wire WorkdirValidator to compare workdir against registered worktree
- [x] Graceful shutdown: stop executor when guardian stops
- [x] **Test (integration)**: `guardian start` creates socket and executor runs
- [x] **Test (integration)**: `guardian stop` cleans up socket and executor

---

## Phase 4.5: Guardian↔Executor Communication

Connect the guardian container to the host executor via the Unix socket.

### 4.5.1 Create executor client in guardian
- [x] Create `internal/guardian/executor/client.go`:
  - Connect to socket at `/var/run/hostexec.sock`
  - Send execute request as JSON
  - Read response
  - Close connection
- [x] Handle connection errors gracefully
- [x] **Test (unit)**: Mock socket, verify wire format
- [x] **Test (integration)**: Guardian can execute command via socket

### 4.5.2 Wire executor client into request flow
- [x] After approval (auto or manual), call executor client
- [x] Map executor response to command response
- [x] Return to waiting request handler
- [x] **Test (unit)**: Approved request → executor called → response returned

### 4.5.3 Handle executor errors
- [x] Connection refused → return error response
- [x] Timeout → return timeout response with partial output
- [x] Command failed → return response with exit code and stderr
- [x] **Test (unit)**: Each error case produces correct response

---

## Phase 4.6: Approval Server and Web UI

Implement the approval server (:9999) with htmx-based UI for human review.

### 4.6.1 Create approval server skeleton
- [x] Create `internal/guardian/approval/server.go`:
  - `GET /` → serve HTML UI
  - `GET /pending` → list pending requests (JSON)
  - `POST /approve/{id}` → approve request
  - `POST /deny/{id}` → deny request with optional reason
- [x] Bind to `127.0.0.1:9999` (localhost only)
- [x] Wire into guardian startup
- [x] **Test (unit)**: Endpoints respond correctly

### 4.6.2 Implement pending requests list
- [x] `GET /pending` returns JSON array of pending requests
- [x] Include: id, cloister, project, branch, agent, cmd, timestamp
- [x] **Test (unit)**: Queue with requests → JSON contains all fields

### 4.6.3 Implement approve endpoint
- [x] `POST /approve/{id}`:
  - Look up request in queue
  - Return 404 if not found
  - Send approved response on request's channel
  - Remove from queue
  - Return success JSON
- [x] **Test (unit)**: Approve existing request → response sent, removed from queue
- [x] **Test (unit)**: Approve nonexistent → 404

### 4.6.4 Implement deny endpoint
- [x] `POST /deny/{id}`:
  - Accept optional `reason` in request body
  - Send denied response on request's channel
  - Remove from queue
  - Return success JSON
- [x] Default reason: "Denied by user"
- [x] **Test (unit)**: Deny with reason → reason in response
- [x] **Test (unit)**: Deny without reason → default reason used

### 4.6.5 Create HTML templates
- [x] Create `internal/guardian/approval/templates/` with embedded templates:
  - `index.html` — main page with pending requests list
  - `request.html` — single request partial (for htmx updates)
- [x] Use `embed.FS` for single-binary distribution
- [x] Style with minimal inline CSS (no build step)
- [x] **Test (unit)**: Templates parse without error

### 4.6.6 Integrate htmx
- [x] Embed htmx.min.js (~14kb) via `embed.FS`
- [x] Serve at `/static/htmx.min.js`
- [x] Include in `index.html` template
- [x] **Test (unit)**: Static file served correctly

### 4.6.7 Implement approve/deny buttons
- [x] Add htmx buttons to request template:
  - Approve: `hx-post="/approve/{id}" hx-swap="outerHTML"`
  - Deny: `hx-post="/deny/{id}" hx-swap="outerHTML"`
- [x] Return updated HTML partial showing result
- [x] **Test (manual)**: Click Approve → request disappears, shows "Approved"
- [x] **Test (manual)**: Click Deny → request disappears, shows "Denied"

---

## Phase 4.7: hostexec Wrapper

Create the in-container wrapper script that communicates with the request server.

### 4.7.1 Validate hostexec script
- [x] Verify `hostexec` script in repo root matches spec (per container-image.md):
  - Validates `CLOISTER_GUARDIAN_HOST` and `CLOISTER_TOKEN` are set
  - Validates at least one argument provided
  - Sends POST to `http://${CLOISTER_GUARDIAN_HOST}:9998/request`
  - Includes `X-Cloister-Token` header
  - Uses `--max-time 300` for 5-minute timeout
  - Parses JSON response with jq
  - Prints stdout/stderr appropriately
  - Exits with command's exit code
- [x] **Test (unit)**: Script syntax check (`bash -n`)

### 4.7.2 Handle response statuses
- [x] Verify script handles all statuses:
  - `approved` or `auto_approved`: print output, exit with exit_code
  - `denied`: print reason to stderr, exit 1
  - `timeout`: print timeout message to stderr, exit 1
  - `error`: print error to stderr, exit 1
- [x] **Test (integration)**: Each status handled correctly

### 4.7.3 Add to container image
- [x] Copy `hostexec` (from repo root) to `/usr/local/bin/hostexec` in Dockerfile
- [x] Set executable permissions
- [x] **Test (integration)**: `hostexec` available in container

### 4.7.4 Set environment variables at container start
- [x] Set `CLOISTER_GUARDIAN_HOST=cloister-guardian` in container
- [x] `CLOISTER_TOKEN` already set (from Phase 1)
- [x] **Test (integration)**: Environment variables present in container

---

## Phase 4.8: SSE for Real-Time Updates

Complete the approval UI with real-time updates via Server-Sent Events.

### 4.8.1 Implement SSE endpoint
- [ ] Create `GET /events` SSE endpoint
- [ ] Broadcast events when:
  - New request added to queue
  - Request approved/denied/timed out
- [ ] **Test (unit)**: SSE endpoint sends correctly formatted events

### 4.8.2 Integrate SSE into approval UI
- [ ] Client subscribes on page load
- [ ] Use htmx SSE extension or native EventSource for updates
- [ ] **Test (manual)**: New request appears without page refresh

---

## Phase 4.9: Audit Logging

Add logging for all hostexec requests and responses.

### 4.9.1 Define audit log format
- [ ] Request event: `HOSTEXEC REQUEST project=X branch=Y cloister=Z cmd="..."`
- [ ] Auto-approve event: `HOSTEXEC AUTO_APPROVE ... pattern="^..."`
- [ ] Approve event: `HOSTEXEC APPROVE ... user="..."` (user from approval UI)
- [ ] Deny event: `HOSTEXEC DENY ... reason="..."`
- [ ] Complete event: `HOSTEXEC COMPLETE ... exit=N duration=Xs`
- [ ] Timeout event: `HOSTEXEC TIMEOUT ...`
- [ ] Follow existing log format from config-reference.md

### 4.9.2 Integrate with existing logging
- [ ] Use existing guardian logger
- [ ] Log to unified audit log (`~/.local/share/cloister/audit.log`)
- [ ] Log to per-cloister log if enabled
- [ ] **Test (unit)**: Mock logger, verify correct format

### 4.9.3 Add structured fields
- [ ] Include all relevant metadata: project, branch, cloister, agent
- [ ] Include execution duration for completed commands
- [ ] **Test (unit)**: All fields present in log output

---

## Phase 4.10: Documentation and Integration

### 4.10.1 Update ~/.claude/rules/cloister.md
- [ ] Review and update the rules file created in Phase 3.4.6
- [ ] Ensure hostexec usage instructions are accurate:
  - When to use hostexec (git push, docker build, etc.)
  - Common patterns (`hostexec git push`, `hostexec docker compose up -d`)
  - What happens when approval is needed
  - How to check if command was approved
- [ ] **Test (manual)**: Rules file content is helpful for Claude

### 4.10.2 Update docs/guardian-api.md if needed
- [ ] Verify API documentation matches implementation
- [ ] Add any undocumented endpoints or fields
- [ ] **Test (manual)**: API docs accurate

### 4.10.3 Update docs/container-image.md if needed
- [ ] Verify hostexec script documentation matches implementation
- [ ] Document environment variables
- [ ] **Test (manual)**: Container image docs accurate

### 4.10.4 End-to-end verification
- [ ] **Test (manual)**: Full flow from Claude → hostexec → approval UI → execution → output
- [ ] **Test (manual)**: Auto-approve flow works without UI interaction
- [ ] **Test (manual)**: Denial flow shows clear message in container

---

## Not In Scope (Deferred to Later Phases)

### Phase 5: Worktree Support
- `cloister start -b <branch>` creates managed worktrees
- Worktree cleanup and management
- Multiple cloisters for same project on different branches

### Phase 6: Domain Approval Flow
- Proxy holds connection for unlisted domains
- Interactive domain approval with persistence options (session/project/global)
- Pending domain requests in approval UI

### Phase 7: Polish
- Shell completion (bash, zsh, fish)
- Read-only reference mounts (`/refs`)
- Audit logging improvements (currently basic in Phase 4.8)
- Detached mode (`cloister start -d`)
- Non-git directory support with `--allow-no-git`
- Multi-arch Docker image builds (linux/amd64, linux/arm64)
- Guardian API versioning
- Remove env var credential fallback

### Future: Devcontainer Integration
- Devcontainer.json discovery and image building
- Security overrides for mounts and capabilities
- Feature allowlisting
