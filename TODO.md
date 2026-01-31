# Phase 4: Host Execution (hostexec)

Goal: Agents can request host commands via approval workflow. A request server receives commands from containers, auto-approves or queues for human review, and executes via a host process communicating over a Unix socket.

## Testing Philosophy

Tests are split into tiers based on what they require:

| Tier | Command | What it tests | Requirements |
|------|---------|---------------|--------------|
| **Unit** | `make test` | Request/response parsing, pattern matching, approval logic | None (sandbox-safe) |
| **Integration** | `make test-integration` | Socket communication, container↔guardian flow | Docker daemon |
| **Manual** | Human verification | Approval UI interaction, real command execution | Human + browser |

**Design for testability:**
- Request server uses `http.Handler` interface for `httptest`-based testing
- Executor interface abstracts command execution (mock for tests, real `exec.Command` for production)
- Pattern matching is pure logic, fully testable without I/O
- SSE testing uses mock `http.ResponseWriter` with flusher support
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
- [ ] Create `internal/guardian/approval/queue.go` with:
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
- [ ] Implement `Add(request) string` (returns ID)
- [ ] Implement `Get(id) (*PendingRequest, bool)`
- [ ] Implement `Remove(id)`
- [ ] Implement `List() []PendingRequest` (for UI)
- [ ] Generate request IDs with `crypto/rand` (8 bytes hex)
- [ ] **Test (unit)**: Add/Get/Remove/List operations work correctly
- [ ] **Test (unit)**: Concurrent access is safe (use `-race`)

### 4.3.2 Add timeout handling
- [ ] Start timeout goroutine when request is added
- [ ] On timeout: remove from queue, send timeout response on channel
- [ ] Cancel timeout when request is approved/denied
- [ ] Default timeout: 5 minutes (configurable in config)
- [ ] **Test (unit)**: Request times out → timeout response sent
- [ ] **Test (unit)**: Approved before timeout → no timeout response

### 4.3.3 Integrate queue into request handler
- [ ] For ManualApprove matches:
  - Create response channel
  - Add to queue with metadata from token lookup
  - Block waiting on response channel
  - Return response to client
- [ ] **Test (unit)**: Handler blocks until approval received
- [ ] **Test (unit)**: Handler returns timeout response after timeout

---

## Phase 4.4: Host Executor

Implement the host-side process that executes approved commands.

### 4.4.1 Define executor interface
- [ ] Create `internal/executor/executor.go` with:
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
- [ ] **Test (unit)**: Types compile and serialize correctly

### 4.4.2 Implement command executor
- [ ] Create `RealExecutor` that uses `os/exec`:
  - Parse command string into executable + args (shell-free)
  - Set working directory
  - Merge environment variables
  - Capture stdout/stderr
  - Respect timeout via context
- [ ] Return appropriate error for executable not found
- [ ] **Test (unit)**: Execute `echo hello` → stdout contains "hello"
- [ ] **Test (unit)**: Execute nonexistent command → error response
- [ ] **Test (unit)**: Timeout → partial output + timeout status

### 4.4.3 Implement Unix socket listener
- [ ] Create `internal/executor/socket.go` with socket server:
  - Listen on `~/.local/share/cloister/hostexec.sock`
  - Create parent directory if needed
  - Set socket permissions (0600)
  - Accept connections, spawn goroutine per connection
  - Read newline-delimited JSON request
  - Validate shared secret
  - Validate token against registry
  - Validate workdir matches token's registered worktree
  - Execute command
  - Write JSON response
  - Close connection
- [ ] **Test (unit)**: Mock socket, verify request/response flow
- [ ] **Test (integration)**: Real socket communication works

### 4.4.4 Implement shared secret validation
- [ ] Generate 32-byte secret at guardian start
- [ ] Pass to executor process via environment variable
- [ ] Pass to guardian container via environment variable
- [ ] Verify secret on every request
- [ ] Reject with "invalid secret" if mismatch
- [ ] **Test (unit)**: Wrong secret → rejected
- [ ] **Test (unit)**: Correct secret → proceeds

### 4.4.5 Implement workdir validation
- [ ] Look up token in registry to get registered worktree path
- [ ] Compare request workdir against registered path
- [ ] Reject with "workdir mismatch" if different
- [ ] **Test (unit)**: Matching workdir → proceeds
- [ ] **Test (unit)**: Mismatched workdir → rejected with clear error

### 4.4.6 Start executor with guardian
- [ ] Add `cloister guardian start` to spawn executor process
- [ ] Create socket before starting guardian container
- [ ] Bind-mount socket into guardian container at `/var/run/hostexec.sock`
- [ ] Pass shared secret to both processes
- [ ] Graceful shutdown: stop executor when guardian stops
- [ ] **Test (integration)**: `guardian start` creates socket and executor runs
- [ ] **Test (integration)**: `guardian stop` cleans up socket and executor

---

## Phase 4.5: Guardian↔Executor Communication

Connect the guardian container to the host executor via the Unix socket.

### 4.5.1 Create executor client in guardian
- [ ] Create `internal/guardian/executor/client.go`:
  - Connect to socket at `/var/run/hostexec.sock`
  - Send execute request as JSON
  - Read response
  - Close connection
- [ ] Handle connection errors gracefully
- [ ] **Test (unit)**: Mock socket, verify wire format
- [ ] **Test (integration)**: Guardian can execute command via socket

### 4.5.2 Wire executor client into request flow
- [ ] After approval (auto or manual), call executor client
- [ ] Map executor response to command response
- [ ] Return to waiting request handler
- [ ] **Test (unit)**: Approved request → executor called → response returned

### 4.5.3 Handle executor errors
- [ ] Connection refused → return error response
- [ ] Timeout → return timeout response with partial output
- [ ] Command failed → return response with exit code and stderr
- [ ] **Test (unit)**: Each error case produces correct response

---

## Phase 4.6: Approval Server and Web UI

Implement the approval server (:9999) with htmx-based UI for human review.

### 4.6.1 Create approval server skeleton
- [ ] Create `internal/guardian/approval/server.go`:
  - `GET /` → serve HTML UI
  - `GET /pending` → list pending requests (JSON)
  - `POST /approve/{id}` → approve request
  - `POST /deny/{id}` → deny request with optional reason
- [ ] Bind to `127.0.0.1:9999` (localhost only)
- [ ] Wire into guardian startup
- [ ] **Test (unit)**: Endpoints respond correctly

### 4.6.2 Implement pending requests list
- [ ] `GET /pending` returns JSON array of pending requests
- [ ] Include: id, cloister, project, branch, agent, cmd, timestamp
- [ ] **Test (unit)**: Queue with requests → JSON contains all fields

### 4.6.3 Implement approve endpoint
- [ ] `POST /approve/{id}`:
  - Look up request in queue
  - Return 404 if not found
  - Send approved response on request's channel
  - Remove from queue
  - Return success JSON
- [ ] **Test (unit)**: Approve existing request → response sent, removed from queue
- [ ] **Test (unit)**: Approve nonexistent → 404

### 4.6.4 Implement deny endpoint
- [ ] `POST /deny/{id}`:
  - Accept optional `reason` in request body
  - Send denied response on request's channel
  - Remove from queue
  - Return success JSON
- [ ] Default reason: "Denied by user"
- [ ] **Test (unit)**: Deny with reason → reason in response
- [ ] **Test (unit)**: Deny without reason → default reason used

### 4.6.5 Create HTML templates
- [ ] Create `internal/guardian/approval/templates/` with embedded templates:
  - `index.html` — main page with pending requests list
  - `request.html` — single request partial (for htmx updates)
- [ ] Use `embed.FS` for single-binary distribution
- [ ] Style with minimal inline CSS (no build step)
- [ ] **Test (unit)**: Templates parse without error

### 4.6.6 Integrate htmx
- [ ] Embed htmx.min.js (~14kb) via `embed.FS`
- [ ] Serve at `/static/htmx.min.js`
- [ ] Include in `index.html` template
- [ ] **Test (unit)**: Static file served correctly

### 4.6.7 Implement approve/deny buttons
- [ ] Add htmx buttons to request template:
  - Approve: `hx-post="/approve/{id}" hx-swap="outerHTML"`
  - Deny: `hx-post="/deny/{id}" hx-swap="outerHTML"`
- [ ] Return updated HTML partial showing result
- [ ] **Test (manual)**: Click Approve → request disappears, shows "Approved"
- [ ] **Test (manual)**: Click Deny → request disappears, shows "Denied"

### 4.6.8 Implement SSE for real-time updates
- [ ] Create `GET /events` SSE endpoint
- [ ] Broadcast events when:
  - New request added to queue
  - Request approved/denied/timed out
- [ ] Client subscribes on page load
- [ ] Use htmx SSE extension for updates
- [ ] **Test (unit)**: SSE endpoint sends correctly formatted events
- [ ] **Test (manual)**: New request appears without page refresh

---

## Phase 4.7: hostexec Wrapper

Create the in-container wrapper script that communicates with the request server.

### 4.7.1 Create hostexec script
- [ ] Create `docker/hostexec` bash script (per container-image.md spec):
  - Validate `CLOISTER_GUARDIAN_HOST` and `CLOISTER_TOKEN` are set
  - Validate at least one argument provided
  - Send POST to `http://${CLOISTER_GUARDIAN_HOST}:9998/request`
  - Include `X-Cloister-Token` header
  - Use `--max-time 300` for 5-minute timeout
  - Parse JSON response with jq
  - Print stdout/stderr appropriately
  - Exit with command's exit code
- [ ] **Test (unit)**: Script syntax check (`bash -n`)

### 4.7.2 Handle response statuses
- [ ] `approved` or `auto_approved`: print output, exit with exit_code
- [ ] `denied`: print reason to stderr, exit 1
- [ ] `timeout`: print timeout message to stderr, exit 1
- [ ] `error`: print error to stderr, exit 1
- [ ] **Test (integration)**: Each status handled correctly

### 4.7.3 Add to container image
- [ ] Copy `hostexec` to `/usr/local/bin/hostexec` in Dockerfile
- [ ] Set executable permissions
- [ ] **Test (integration)**: `hostexec` available in container

### 4.7.4 Set environment variables at container start
- [ ] Set `CLOISTER_GUARDIAN_HOST=cloister-guardian` in container
- [ ] `CLOISTER_TOKEN` already set (from Phase 1)
- [ ] **Test (integration)**: Environment variables present in container

---

## Phase 4.8: Audit Logging

Add logging for all hostexec requests and responses.

### 4.8.1 Define audit log format
- [ ] Request event: `HOSTEXEC REQUEST project=X branch=Y cloister=Z cmd="..."`
- [ ] Auto-approve event: `HOSTEXEC AUTO_APPROVE ... pattern="^..."`
- [ ] Approve event: `HOSTEXEC APPROVE ... user="..."` (user from approval UI)
- [ ] Deny event: `HOSTEXEC DENY ... reason="..."`
- [ ] Complete event: `HOSTEXEC COMPLETE ... exit=N duration=Xs`
- [ ] Timeout event: `HOSTEXEC TIMEOUT ...`
- [ ] Follow existing log format from config-reference.md

### 4.8.2 Integrate with existing logging
- [ ] Use existing guardian logger
- [ ] Log to unified audit log (`~/.local/share/cloister/audit.log`)
- [ ] Log to per-cloister log if enabled
- [ ] **Test (unit)**: Mock logger, verify correct format

### 4.8.3 Add structured fields
- [ ] Include all relevant metadata: project, branch, cloister, agent
- [ ] Include execution duration for completed commands
- [ ] **Test (unit)**: All fields present in log output

---

## Phase 4.9: Documentation and Integration

### 4.9.1 Update ~/.claude/rules/cloister.md
- [ ] Review and update the rules file created in Phase 3.4.6
- [ ] Ensure hostexec usage instructions are accurate:
  - When to use hostexec (git push, docker build, etc.)
  - Common patterns (`hostexec git push`, `hostexec docker compose up -d`)
  - What happens when approval is needed
  - How to check if command was approved
- [ ] **Test (manual)**: Rules file content is helpful for Claude

### 4.9.2 Update docs/guardian-api.md if needed
- [ ] Verify API documentation matches implementation
- [ ] Add any undocumented endpoints or fields
- [ ] **Test (manual)**: API docs accurate

### 4.9.3 Update docs/container-image.md if needed
- [ ] Verify hostexec script documentation matches implementation
- [ ] Document environment variables
- [ ] **Test (manual)**: Container image docs accurate

### 4.9.4 End-to-end verification
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
