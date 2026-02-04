# Phase 6: Domain Approval Flow

When a cloister container requests an unlisted domain, instead of immediately returning 403, the proxy holds the connection and creates an approval request visible in the web UI. The user can approve (session/project/global scope) or deny. Timeout defaults to 60s.

## Testing Philosophy

- **Automated tests for proxy/queue logic**: Unit tests for domain queue, session allowlist, proxy hold-and-wait behavior
- **Automated tests for approval server handlers**: Use `httptest` for new domain approval endpoints (`/pending-domains`, `/approve-domain/{id}`, `/deny-domain/{id}`)
- **Automated tests for config persistence**: Write/read round-trip tests for adding domains to project and global configs
- **Manual tests for UI interactions**: Browser testing for the domain approval section in the web UI (SSE updates, htmx swaps, scope selector)
- **Factor code for testability**: Use interfaces for config writers and allowlist mutators so proxy tests don't touch the filesystem
- **Go tests**: Use `testing` with `httptest` for handlers, `t.TempDir()` for config persistence, channel-based tests for hold-and-wait

## Verification Checklist

Before marking a phase complete and committing it:

1. `make test` passes (unit tests, no Docker required)
2. `make lint` passes
3. `make fmt` has been run
4. Playwright MCP for approval UI verification (SSE updates, htmx swaps, scope buttons)
5. Code reviewed for obvious issues

When verification of a phase or subphase is complete, commit all
relevant newly-created and modified files.

## Dependencies Between Phases

```
6.1 (Domain Queue) ─── foundation for all other subphases
       │
       ├──► 6.2 (Session Allowlist)
       │          │
       │          ▼
       │    6.3 (Proxy Hold-and-Wait) ─── connects proxy to queue + session allowlist
       │
       ├──► 6.4 (Approval Server Endpoints)
       │
       ├──► 6.5 (Domain Approval Templates)
       │
       └──► 6.6 (Config Persistence)
                  │
                  ▼
            6.7 (Guardian Wiring) ─── integrates all components
                  │
                  ▼
            6.8 (Audit Logging)
```

6.2, 6.4, 6.5, and 6.6 can proceed in parallel after 6.1.
6.3 requires 6.1 + 6.2.
6.7 requires all prior subphases.

---

## Phase 6.1: Domain Approval Queue

Create a separate queue for domain approval requests, distinct from the existing hostexec command queue. Domain requests have different fields (domain instead of cmd) and different response semantics (scope-based approval).

### 6.1.1 DomainRequest and DomainQueue types
- [x] Create `internal/guardian/approval/domain_queue.go`
- [x] Define `DomainRequest` struct: `ID`, `Cloister`, `Project`, `Domain`, `Timestamp`, `ExpiresAt`, `Response chan<- DomainResponse`
- [x] Define `DomainResponse` struct: `Status` (approved/denied/timeout), `Scope` (session/project/global), `Reason`
- [x] Implement `DomainQueue` with same pattern as existing `Queue`: thread-safe map, timeout goroutines, cancel functions
- [x] `Add(req *DomainRequest) (string, error)` — generate ID, start timeout, broadcast SSE event
- [x] `Get(id string) (*DomainRequest, bool)`
- [x] `Remove(id string)`
- [x] `List() []DomainRequest` (omit Response channel in copies)
- [x] `Len() int`
- [x] Wire `EventHub` for domain-specific SSE events via `SetEventHub`
- [x] **Test**: Unit test for Add/Get/Remove/List lifecycle
- [x] **Test**: Unit test for timeout — request times out, response channel receives timeout status
- [x] **Test**: Unit test for cancel — approve before timeout, timeout goroutine is a no-op

---

## Phase 6.2: Session Allowlist

Track domains approved with "session" scope in memory. These are per-project, ephemeral (lost on guardian restart), and checked by the proxy before consulting the persistent allowlist.

### 6.2.1 SessionAllowlist type
- [x] Create `internal/guardian/session_allowlist.go`
- [x] Define `SessionAllowlist` struct: thread-safe map of `projectName -> set of domains`
- [x] `Add(project, domain string)` — add domain to project's session set
- [x] `IsAllowed(project, domain string) bool` — check if domain is in project's session set
- [x] `Clear(project string)` — remove all session domains for a project (for cloister stop)
- [x] `ClearAll()` — remove all session domains (for guardian restart)
- [x] **Test**: Unit test for Add/IsAllowed with multiple projects isolated from each other
- [x] **Test**: Unit test for Clear per-project without affecting other projects

---

## Phase 6.3: Proxy Hold-and-Wait

Modify `handleConnect` in the proxy to hold blocked requests instead of returning 403 immediately. The proxy checks session allowlist first, then submits to the domain queue and waits for a response.

### 6.3.1 Add DomainApprover interface and proxy integration
- [x] Define `DomainApprover` interface in `internal/guardian/proxy.go`:
  ```go
  type DomainApprover interface {
      RequestApproval(project, cloister, domain string) (DomainApprovalResult, error)
  }
  type DomainApprovalResult struct {
      Approved bool
      Scope    string // "session", "project", "global"
  }
  ```
- [x] Add `DomainApprover` field to `ProxyServer` struct (nil = reject immediately, preserving current behavior)
- [x] Add `SessionAllowlist` field to `ProxyServer` struct (nil = skip session check)
- [x] Modify `handleConnect`: when domain is not in persistent allowlist:
  1. Check `SessionAllowlist.IsAllowed(project, domain)` — if allowed, proceed
  2. If `DomainApprover` is nil, return 403 (backward-compatible)
  3. Call `DomainApprover.RequestApproval(...)` (blocks up to timeout)
  4. If approved, proceed to dial upstream
  5. If denied/timeout, return 403
- [x] Extract token and project from request for session allowlist and approval context
- [x] **Test**: Unit test — nil DomainApprover returns 403 (backward-compatible)
- [x] **Test**: Unit test — DomainApprover approves, connection proceeds
- [x] **Test**: Unit test — DomainApprover denies, returns 403
- [x] **Test**: Unit test — session allowlist hit bypasses DomainApprover entirely

---

## Phase 6.4: Approval Server Domain Endpoints

Add endpoints to the approval server for listing, approving, and denying domain requests. These endpoints mirror the existing hostexec pattern but add a `scope` parameter.

### 6.4.1 Domain queue field and new HTTP handlers
- [x] Add `DomainQueue *DomainQueue` field to approval `Server` struct
- [x] Add `ConfigPersister` interface field (for saving approved domains to config files):
  ```go
  type ConfigPersister interface {
      AddDomainToProject(project, domain string) error
      AddDomainToGlobal(domain string) error
  }
  ```
- [x] Register new routes in `Start()`:
  - `GET /pending-domains` — list pending domain requests as JSON
  - `POST /approve-domain/{id}` — approve with `{"scope": "session|project|global"}`
  - `POST /deny-domain/{id}` — deny with optional reason
- [x] `handlePendingDomains`: serialize `DomainQueue.List()` to JSON
- [x] `handleApproveDomain`: parse scope from body, send `DomainResponse` on channel, persist if scope is project/global via `ConfigPersister`, broadcast SSE removal
- [x] `handleDenyDomain`: send denied `DomainResponse`, broadcast SSE removal
- [x] **Test**: Handler test for `GET /pending-domains` — returns JSON array
- [x] **Test**: Handler test for `POST /approve-domain/{id}` with scope "session" — no config persistence, response sent
- [x] **Test**: Handler test for `POST /approve-domain/{id}` with scope "project" — calls `ConfigPersister.AddDomainToProject`
- [x] **Test**: Handler test for `POST /deny-domain/{id}` — response sent, removed from queue
- [x] **Test**: Handler test for approve/deny with unknown ID — returns 404

---

## Phase 6.5: Domain Approval Templates

Add HTML templates for domain approval requests in the web UI. Domain requests appear in a separate section from hostexec commands, with scope selection buttons.

### 6.5.1 New HTML templates for domain requests
- [x] Create `internal/guardian/approval/templates/domain_request.html`:
  - Display domain, cloister, project, timestamp
  - Three approve buttons: "Allow (Session)", "Save to Project", "Save to Global"
  - One deny button
  - Use htmx: `hx-post="/approve-domain/{id}"` with `hx-vals='{"scope":"session"}'` etc.
- [x] Create `internal/guardian/approval/templates/domain_result.html`:
  - Show approved/denied status with domain and scope
- [x] Update `index.html`:
  - Add "Domain Requests" section below existing "Command Requests"
  - Render initial domain requests from server data
  - Add SSE handlers for `domain-request-added` and `domain-request-removed` events
  - Notifications for new domain requests
- [x] Add new SSE event types: `EventDomainRequestAdded`, `EventDomainRequestRemoved`
- [x] Add `BroadcastDomainRequestAdded` and `BroadcastDomainRequestRemoved` to `EventHub`
- [x] Update `handleIndex` to pass both command and domain request lists to template
- [x] Add `domainTemplateRequest` struct for template rendering (Domain field instead of Cmd)
- [x] **Test**: Template rendering test — `domain_request.html` renders without error with sample data
- [x] **Test**: SSE format test — domain events serialize correctly

---

## Phase 6.6: Config Persistence

Implement `ConfigPersister` that adds approved domains to project or global config files and triggers proxy allowlist reload.

### 6.6.1 ConfigPersister implementation
- [x] Create `internal/guardian/config_persister.go`
- [x] Implement `ConfigPersister` interface:
  - `AddDomainToProject(project, domain string) error`:
    1. Load project config via `config.LoadProjectConfig(project)`
    2. Append `AllowEntry{Domain: domain}` if not already present
    3. Write via `config.WriteProjectConfig(project, cfg, true)`
  - `AddDomainToGlobal(domain string) error`:
    1. Load global config via `config.LoadGlobalConfig()`
    2. Append `AllowEntry{Domain: domain}` if not already present
    3. Write via `config.WriteGlobalConfig(cfg)`
- [x] Add `ReloadNotifier func()` field — called after config write to signal proxy reload (will be wired to SIGHUP or cache clear)
- [x] **Test**: `AddDomainToProject` — writes domain, reload round-trips correctly
- [x] **Test**: `AddDomainToProject` with existing domain — no duplicate added
- [x] **Test**: `AddDomainToGlobal` — writes domain, round-trips correctly

---

## Phase 6.7: Guardian Wiring

Wire all Phase 6 components together in the guardian startup (`internal/cmd/guardian.go`). This is the integration point where domain queue, session allowlist, config persister, and proxy all connect.

### 6.7.1 Wire domain approval into guardian startup
- [ ] In `runGuardianProxy()`:
  1. Create `DomainQueue` with timeout from config (`approval_timeout`, default 60s)
  2. Create `SessionAllowlist`
  3. Create `ConfigPersister` with `ReloadNotifier` that clears `AllowlistCache` and reloads
  4. Create `DomainApproverImpl` that bridges `DomainQueue` (submits request, waits on channel)
  5. Set `proxy.DomainApprover` and `proxy.SessionAllowlist`
  6. Set `approvalServer.DomainQueue` and `approvalServer.ConfigPersister`
  7. Wire `DomainQueue.SetEventHub` to share the existing `EventHub`
- [ ] Parse `unlisted_domain_behavior` from config: if `"reject"`, leave `DomainApprover` nil (backward-compatible)
- [ ] Parse `approval_timeout` from config (default 60s) for `DomainQueue` timeout
- [ ] On session allowlist approval, also add domain to the project's cached `Allowlist` so subsequent requests from the same project don't re-prompt
- [ ] **Test**: Integration test (unit-level, no Docker) — create all components, submit domain request through proxy mock, approve via server endpoint, verify domain is allowed on next request

### 6.7.2 DomainApproverImpl
- [ ] Create `internal/guardian/domain_approver.go`
- [ ] Implement `DomainApprover` interface using `DomainQueue`:
  - Create `DomainRequest` with response channel
  - Call `DomainQueue.Add(req)`
  - Block on response channel
  - Return `DomainApprovalResult` based on response
- [ ] On "session" scope approval, call `SessionAllowlist.Add(project, domain)` and add to `AllowlistCache` project entry
- [ ] On "project"/"global" scope approval, `ConfigPersister` handles persistence (already called from server handler), then clear+reload relevant `AllowlistCache` entry
- [ ] **Test**: Unit test — submit request, approve on channel, verify result
- [ ] **Test**: Unit test — submit request, timeout, verify timeout result

---

## Phase 6.8: Audit Logging

Add audit log entries for domain approval events, consistent with existing hostexec audit logging.

### 6.8.1 Domain audit events
- [ ] Add domain-specific audit methods to `audit.Logger`:
  - `LogDomainRequest(project, cloister, domain string) error`
  - `LogDomainApprove(project, cloister, domain, scope, actor string) error`
  - `LogDomainDeny(project, cloister, domain, reason string) error`
  - `LogDomainTimeout(project, cloister, domain string) error`
- [ ] Call `LogDomainRequest` when domain request is added to queue
- [ ] Call `LogDomainApprove` from `handleApproveDomain`
- [ ] Call `LogDomainDeny` from `handleDenyDomain`
- [ ] Call `LogDomainTimeout` from `DomainQueue` timeout handler
- [ ] **Test**: Unit test — verify audit log output format for each event type

---

## Future Phases (Deferred)

### Phase 5: Worktree Support (Skipped)
- `cloister start -b <branch>` creates managed worktrees
- Worktree naming: `<project>-<branch>`
- Worktree cleanup protection
- CLI: `worktree list/remove`, `cloister path <name>`

### Phase 7: Polish
- Image distribution and auto-pull
- Custom image configuration
- Multi-arch container images
- Shell completion
- Read-only reference mounts
- Audit logging improvements
- Detached mode, non-git support
- Guardian API versioning
- Structured logging
