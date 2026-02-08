# Persistent Domain Denials & Decisions Directory Migration

Implement persistent domain denial feature with scope options (once/session/project/global) and migrate the storage directory from `approvals/` to `decisions/` to reflect that both approvals and denials are persisted.

## Testing Philosophy

- **Automated tests for business logic**: Config loading/saving, domain precedence evaluation, API handlers
- **Integration tests for guardian services**: Domain approval flow with persistence, session allowlist behavior
- **Manual tests for UI interactions**: Web UI button clicks, SSE updates, connection status banners
- **Factor code for testability**: Config persistence should be testable independently of HTTP handlers
- **Go tests**: Use `testing` package with `httptest` for HTTP handlers, table-driven tests for precedence logic
- **Frontend tests**: Manual browser testing for UI workflows (no frontend test framework currently)

## Verification Checklist

Before marking a phase complete and committing:

1. `make test` passes (unit tests, sandbox-safe)
2. `make test-integration` passes (requires Docker) - only when changing Docker/container code
3. Manual browser testing for UI interactions
4. No console errors in browser (check DevTools)
5. Code reviewed for obvious issues
6. Audit log entries appear correctly for new denial events

When verification of a phase or subphase is complete, commit all relevant newly-created and modified files.

## Dependencies Between Phases

```
Phase 1 (Config/Backend Foundation)
       │
       ├─► Phase 2 (API Endpoints - parallel track)
       │         │
       │         ▼
       │   Phase 3 (Web UI Updates)
       │
       └─► Phase 4 (Integration & Testing)
               │
               ▼
         Phase 5 (Documentation & Cleanup)
```

Phase 1 and 2 can proceed in parallel (config changes don't block API work). Phase 3 depends on Phase 2 (needs API endpoints). Phase 4 requires Phases 1-3 complete. Phase 5 is final polish.

---

## Phase 1: Config & Storage Migration

Migrate from `approvals/` to `decisions/` directory and add denylist support to config schema.

### 1.1 Update config types and constants

- [x] Rename `internal/config/approvals.go` to `internal/config/decisions.go`
- [x] Rename type `Approvals` to `Decisions` in `decisions.go`
- [x] Add new fields to `Decisions` struct:
  ```go
  type Decisions struct {
      Domains        []string `yaml:"domains,omitempty"`
      Patterns       []string `yaml:"patterns,omitempty"`
      DeniedDomains  []string `yaml:"denied_domains,omitempty"`
      DeniedPatterns []string `yaml:"denied_patterns,omitempty"`
  }
  ```
- [x] Update `ApprovalDir()` function to `DecisionDir()` returning `~/.config/cloister/decisions`
- [x] Rename `GlobalApprovalPath()` to `GlobalDecisionPath()` (update path to `decisions/global.yaml`)
- [x] Rename `ProjectApprovalPath()` to `ProjectDecisionPath()` (update path to `decisions/projects/<name>.yaml`)
- [x] **Test**: Unit test for `DecisionDir()` returns correct path
- [x] **Test**: Unit test for `GlobalDecisionPath()` returns `decisions/global.yaml`
- [x] **Test**: Unit test for `ProjectDecisionPath("my-api")` returns `decisions/projects/my-api.yaml`

### 1.2 Update config load/save functions

- [x] Rename `LoadGlobalApprovals()` to `LoadGlobalDecisions()`
- [x] Rename `LoadProjectApprovals()` to `LoadProjectDecisions()`
- [x] Rename `WriteGlobalApprovals()` to `WriteGlobalDecisions()`
- [x] Rename `WriteProjectApprovals()` to `WriteProjectDecisions()`
- [x] Update internal helper `loadApprovals()` to `loadDecisions()`
- [x] Update internal helper `writeApprovalsAtomic()` to `writeDecisionsAtomic()`
- [x] **Test**: Unit test for loading decisions file with all 4 fields (domains, patterns, denied_domains, denied_patterns)
- [x] **Test**: Unit test for writing decisions atomically
- [x] **Test**: Unit test for loading empty/missing decisions file returns empty struct

### 1.3 Update test files

- [x] Rename `internal/config/approvals_test.go` to `internal/config/decisions_test.go`
- [x] Update all test function names (`TestLoadGlobalApprovals` → `TestLoadGlobalDecisions`, etc.)
- [x] Update test assertions to use new function names and paths
- [x] Add test cases for denied_domains and denied_patterns fields
- [x] **Test**: All tests in `decisions_test.go` pass with new paths and field names

### 1.4 Update guardian command integration

- [x] In `internal/cmd/guardian.go`, rename `approvalsToAllowEntries()` to `decisionsToAllowEntries()`
- [x] Update `decisionsToAllowEntries()` to handle both allowlist and denylist fields
- [x] Update all calls to `LoadGlobalApprovals()` → `LoadGlobalDecisions()`
- [x] Update all calls to `LoadProjectApprovals()` → `LoadProjectDecisions()`
- [x] Update comments referencing "approvals" to "decisions"
- [x] **Test**: Integration test - guardian loads decisions from new path correctly
- [x] **Test**: Integration test - global decisions merge with project decisions

---

## Phase 2: API Endpoints for Persistent Denials

Add scope support to `/deny-domain/{id}` endpoint and update domain approver to handle denials with scope.

### 2.1 Update domain queue response types

- [x] In `internal/guardian/approval/domain_queue.go`, update `DomainResponse` struct to include `Scope` field for denials
- [x] Ensure `DomainResponse.Status` can be "approved", "denied", or "timeout"
- [x] **Test**: Unit test for DomainResponse JSON marshaling with all fields

### 2.2 Update approval server deny endpoint

- [x] In `internal/guardian/approval/server.go`, update `POST /deny-domain/{id}` handler
- [x] Parse request body with schema:
  ```go
  type DenyDomainRequest struct {
      Scope    string `json:"scope"`    // "once", "session", "project", "global"
      Wildcard bool   `json:"wildcard"` // default false
  }
  ```
- [x] Validate `scope` is one of: "once", "session", "project", "global"
- [x] Send `DomainResponse` with `Status: "denied"`, `Scope: <scope>`, and optional `Pattern: <wildcard>`
- [x] **Test**: Handler test - deny with scope="once" returns correct response
- [x] **Test**: Handler test - deny with scope="project" returns correct response
- [x] **Test**: Handler test - deny with wildcard=true returns pattern in response
- [x] **Test**: Handler test - invalid scope returns 400 Bad Request

### 2.3 Update domain approver to persist denials

- [x] In `internal/guardian/domain_approver.go`, update `RequestApproval()` to handle denial responses with scope
- [x] For scope="session": Add domain to `SessionDenylist` (new interface, similar to SessionAllowlist)
- [x] For scope="project": Load project decisions, append to `denied_domains` or `denied_patterns`, write back
- [x] For scope="global": Load global decisions, append to `denied_domains` or `denied_patterns`, write back
- [x] For scope="once": No persistence, just return denial
- [x] Add wildcard logic: if `wildcard=true`, convert `api.example.com` → `*.example.com` before persisting
- [x] **Test**: Unit test - denial with scope="session" adds to session denylist
- [x] **Test**: Unit test - denial with scope="project" writes to project decisions file
- [x] **Test**: Unit test - denial with scope="global" writes to global decisions file
- [x] **Test**: Unit test - denial with wildcard creates correct pattern

### 2.4 Create SessionDenylist interface and implementation

- [x] Define `SessionDenylist` interface in `internal/guardian/proxy.go` (parallel to `SessionAllowlist`)
  ```go
  type SessionDenylist interface {
      IsBlocked(token, domain string) bool
      Add(token, domain string) error
      Clear(token string)
  }
  ```
- [x] Implement `SessionDenylistImpl` in `internal/guardian/session_allowlist.go` (or new file)
- [x] Add SessionDenylist field to `ProxyServer` struct
- [x] Update proxy request evaluation to check session denylist before allowlist
- [x] **Test**: Unit test for SessionDenylist.Add and IsBlocked
- [x] **Test**: Unit test for SessionDenylist.Clear removes all entries for token

### 2.5 Update approve endpoint for consistency

- [ ] In `internal/guardian/approval/server.go`, update `POST /approve-domain/{id}` handler
- [ ] Add `"once"` to valid scope options (currently: session, project, global)
- [ ] Handle scope="once" by forwarding request without persistence
- [ ] **Test**: Handler test - approve with scope="once" returns correct response
- [ ] **Test**: Handler test - approve with scope="once" does not write to config

---

## Phase 3: Web UI Updates

Update web UI to support persistent denials with scope buttons and simplified layout.

### 3.1 Update approval server HTML templates

- [ ] In `internal/guardian/approval/templates/index.html`, remove "Active Cloisters" section
- [ ] Remove tabs for "Commands" vs "Domains" - show single chronological list
- [ ] Add connection status banner area at top (initially hidden, shown on SSE disconnect)
- [ ] Update pending requests section to show mixed command/domain cards chronologically
- [ ] Add card type indicators: 🔧 for commands, 🌐 for domains
- [ ] **Test**: Manual - Load UI, verify single list layout with no tabs

### 3.2 Update domain request card template

- [ ] In `internal/guardian/approval/templates/domain_request.html`, update button layout:
  ```html
  <div class="allow-section">
    <button data-scope="once">Once</button>
    <button data-scope="session">Session</button>
    <button data-scope="project">Project</button>
    <button data-scope="global">Global</button>
  </div>
  <div class="deny-section">
    <button data-scope="once">Once</button>
    <button data-scope="session">Session</button>
    <button data-scope="project">Project</button>
    <button data-scope="global">Global</button>
  </div>
  <label>
    <input type="checkbox" class="wildcard-checkbox">
    Apply to wildcard pattern: *.{{.Domain}}
  </label>
  ```
- [ ] Add CSS classes `.allow-section` (green) and `.deny-section` (red)
- [ ] **Test**: Manual - Domain request shows 8 buttons + wildcard checkbox

### 3.3 Update JavaScript event handlers

- [ ] In `internal/guardian/approval/templates/index.html` or separate JS file:
- [ ] Add click handlers for deny buttons that send POST to `/deny-domain/{id}` with scope and wildcard
- [ ] Update approve button handlers to support scope="once"
- [ ] Add SSE disconnect handler to show connection status banner
- [ ] Add SSE reconnect handler to hide banner when connection restored
- [ ] Update confirmation message to not show undo/view-effective buttons (removed from spec)
- [ ] **Test**: Manual - Click "Deny → Project" sends correct request
- [ ] **Test**: Manual - Click "Allow → Once" forwards request without persistence
- [ ] **Test**: Manual - Disconnect guardian, verify yellow banner appears
- [ ] **Test**: Manual - Reconnect guardian, verify banner disappears

### 3.4 Update CSS for new layout

- [ ] Add styles for `.allow-section` (green background/border)
- [ ] Add styles for `.deny-section` (red background/border)
- [ ] Add styles for connection status banner (yellow for reconnecting, red for offline)
- [ ] Update card styles to handle 🔧/🌐 icons and mixed list
- [ ] **Test**: Manual - Buttons styled correctly with green/red sections
- [ ] **Test**: Manual - Connection banner styled correctly (yellow/red)

---

## Phase 4: Proxy Domain Precedence Logic

Update proxy to evaluate denials before allowlists with correct precedence.

### 4.1 Update allowlist cache to include denylists

- [ ] In `internal/guardian/allowlist_cache.go`, add methods to `AllowlistCache` for denylist support
- [ ] Update cache to store both allowed and denied domains/patterns per project
- [ ] Add `IsBlocked(projectName, domain string) bool` method to cache
- [ ] Update cache loading to parse `denied_domains` and `denied_patterns` from decisions files
- [ ] **Test**: Unit test - cache loads denied_domains from decisions file
- [ ] **Test**: Unit test - cache.IsBlocked() returns true for denied domain

### 4.2 Update proxy evaluation order

- [ ] In `internal/guardian/proxy.go`, update `handleConnect()` evaluation order:
  1. Check static denylist (exact domains)
  2. Check static denylist (patterns)
  3. Check session denylist
  4. Check static allowlist (exact domains)
  5. Check static allowlist (patterns)
  6. Check session allowlist
  7. Queue for human approval
- [ ] Ensure denials take precedence over approvals at all scope levels
- [ ] **Test**: Integration test - denied domain in global config blocks request even if in project allowlist
- [ ] **Test**: Integration test - session denied domain blocks request
- [ ] **Test**: Integration test - denied pattern blocks matching subdomain

### 4.3 Add audit log events for denials

- [ ] In `internal/guardian/domain_approver.go`, log denial events to audit log
- [ ] Log format: `PROXY DENY project=X cloister=Y domain=Z scope=S user=U`
- [ ] Include `pattern=P` if wildcard was used
- [ ] **Test**: Integration test - denial with scope="project" logs audit event
- [ ] **Test**: Integration test - audit log includes pattern field for wildcard denials

---

## Phase 5: Integration Testing & Documentation

Verify end-to-end workflows and update remaining documentation.

### 5.1 End-to-end workflow tests

- [ ] **Test**: E2E - Deny domain with scope="global", verify future requests blocked
- [ ] **Test**: E2E - Deny domain with scope="session", stop cloister, verify denial forgotten
- [ ] **Test**: E2E - Approve domain with scope="once", verify subsequent requests re-prompt
- [ ] **Test**: E2E - Deny with wildcard, verify all subdomains blocked
- [ ] **Test**: E2E - Domain in both allowlist and denylist, verify blocked (deny wins)
- [ ] **Test**: E2E - Load decisions from file on guardian startup, verify applied correctly

### 5.2 Update CLI help text and examples

- [ ] Update `internal/cmd/guardian.go` help text to reference "decisions" directory
- [ ] Update example commands in CLI output
- [ ] **Test**: Manual - `cloister guardian --help` shows updated text

### 5.3 Update internal documentation comments

- [ ] Update package comments in `internal/config/decisions.go`
- [ ] Update function comments in `internal/guardian/domain_approver.go`
- [ ] Update comments in `internal/guardian/proxy.go` for evaluation order
- [ ] **Test**: Manual - `go doc` output shows updated terminology

### 5.4 Migration path for existing users

- [ ] Add check in `DecisionDir()` to detect if old `approvals/` directory exists
- [ ] If `approvals/` exists and `decisions/` does not, automatically migrate (rename directory)
- [ ] Log migration: "Migrated approvals/ to decisions/ directory"
- [ ] **Test**: Integration test - create `approvals/` directory, start guardian, verify migrated to `decisions/`
- [ ] **Test**: Integration test - verify contents preserved during migration

---

## Future Phases (Deferred)

### Undo functionality
- Add DELETE endpoint to remove domain from decision file
- Add undo button to UI confirmation messages
- Time-limited undo window (30 seconds)

### Config file management via UI
- Add "Edit decisions file" buttons to UI
- Add "Reload config" button to trigger SIGHUP
- Show diff of changes before/after reload

### View effective allowlist/denylist
- Add endpoint to compute merged allowlist/denylist across all scopes
- Add UI view showing which domains allowed/denied and from which source
- Export as YAML/JSON options

### Bulk import/export
- CLI command to export all decisions to YAML
- CLI command to import decisions from YAML (with merge/replace options)

### Domain history tracking
- Track when domains were added/removed from decisions files
- Show decision history in UI (who approved/denied, when, which scope)
