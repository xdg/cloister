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
2. `make test-all` passes (use hostexec when inside sandbox)
3. Manual browser testing for UI interactions
4. No console errors in browser (check DevTools)
5. Code reviewed for obvious issues
6. Audit log entries appear correctly for new denial events

When inside a cloister, use `hostexec` to run `make test-integration`, `make
test-e2e` or `make test-all`.

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

- [x] In `internal/guardian/approval/server.go`, update `POST /approve-domain/{id}` handler
- [x] Add `"once"` to valid scope options (currently: session, project, global)
- [x] Handle scope="once" by forwarding request without persistence
- [x] **Test**: Handler test - approve with scope="once" returns correct response
- [x] **Test**: Handler test - approve with scope="once" does not write to config

---

## Phase 3: Web UI Updates

Update web UI to support persistent denials with scope buttons and simplified layout.

### 3.1 Update approval server HTML templates

- [x] In `internal/guardian/approval/templates/index.html`, remove "Active Cloisters" section
- [x] Remove tabs for "Commands" vs "Domains" - show single chronological list
- [x] Add connection status banner area at top (initially hidden, shown on SSE disconnect)
- [x] Update pending requests section to show mixed command/domain cards chronologically
- [x] Add card type indicators: 🔧 for commands, 🌐 for domains
- [x] **Test**: Manual - Load UI, verify single list layout with no tabs

### 3.2 Update domain request card template

- [x] In `internal/guardian/approval/templates/domain_request.html`, update button layout:
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
- [x] Add CSS classes `.allow-section` (green) and `.deny-section` (red)
- [x] **Test**: Manual - Domain request shows 8 buttons + wildcard checkbox

### 3.3 Update JavaScript event handlers

- [x] In `internal/guardian/approval/templates/index.html` or separate JS file:
- [x] Add click handlers for deny buttons that send POST to `/deny-domain/{id}` with scope and wildcard
- [x] Update approve button handlers to support scope="once"
- [x] Add SSE disconnect handler to show connection status banner
- [x] Add SSE reconnect handler to hide banner when connection restored
- [x] Update confirmation message to not show undo/view-effective buttons (removed from spec)
- [x] **Test**: Manual - Click "Deny → Project" sends correct request
- [x] **Test**: Manual - Click "Allow → Once" forwards request without persistence
- [x] **Test**: Manual - Disconnect guardian, verify yellow banner appears
- [x] **Test**: Manual - Reconnect guardian, verify banner disappears

### 3.4 Update CSS for new layout

- [x] Add styles for `.allow-section` (green background/border)
- [x] Add styles for `.deny-section` (red background/border)
- [x] Add styles for connection status banner (yellow for reconnecting, red for offline)
- [x] Update card styles to handle 🔧/🌐 icons and mixed list
- [x] **Test**: Manual - Buttons styled correctly with green/red sections
- [x] **Test**: Manual - Connection banner styled correctly (yellow/red)

---

## Phase 4: Proxy Domain Precedence Logic

Update proxy to evaluate denials before allowlists with correct precedence.

### 4.1 Update allowlist cache to include denylists

- [x] In `internal/guardian/allowlist_cache.go`, add methods to `AllowlistCache` for denylist support
- [x] Update cache to store both allowed and denied domains/patterns per project
- [x] Add `IsBlocked(projectName, domain string) bool` method to cache
- [x] Update cache loading to parse `denied_domains` and `denied_patterns` from decisions files
- [x] **Test**: Unit test - cache loads denied_domains from decisions file
- [x] **Test**: Unit test - cache.IsBlocked() returns true for denied domain

### 4.2 Update proxy evaluation order

- [x] In `internal/guardian/proxy.go`, update `handleConnect()` evaluation order:
  1. Check static denylist (exact domains)
  2. Check static denylist (patterns)
  3. Check session denylist
  4. Check static allowlist (exact domains)
  5. Check static allowlist (patterns)
  6. Check session allowlist
  7. Queue for human approval
- [x] Ensure denials take precedence over approvals at all scope levels
- [x] **Test**: Integration test - denied domain in global config blocks request even if in project allowlist
- [x] **Test**: Integration test - session denied domain blocks request
- [x] **Test**: Integration test - denied pattern blocks matching subdomain

### 4.3 Add audit log events for denials

- [x] In `internal/guardian/domain_approver.go`, log denial events to audit log
- [x] Log format: `PROXY DENY project=X cloister=Y domain=Z scope=S user=U`
- [x] Include `pattern=P` if wildcard was used
- [x] **Test**: Integration test - denial with scope="project" logs audit event
- [x] **Test**: Integration test - audit log includes pattern field for wildcard denials

---

## Phase 5: Integration Testing & Documentation

Verify end-to-end workflows and update remaining documentation.

### 5.1 End-to-end workflow tests

- [x] **Test**: E2E - Deny domain with scope="global", verify future requests blocked
- [x] **Test**: E2E - Deny domain with scope="session", stop cloister, verify denial forgotten
- [x] **Test**: E2E - Approve domain with scope="once", verify subsequent requests re-prompt
- [x] **Test**: E2E - Deny with wildcard, verify all subdomains blocked
- [x] **Test**: E2E - Domain in both allowlist and denylist, verify blocked (deny wins)
- [x] **Test**: E2E - Load decisions from file on guardian startup, verify applied correctly

### 5.2 Update CLI help text and examples

- [x] Update `internal/cmd/guardian.go` help text to reference "decisions" directory
- [x] Update example commands in CLI output
- [x] **Test**: Manual - `cloister guardian --help` shows updated text

### 5.3 Update internal documentation comments

- [x] Update package comments in `internal/config/decisions.go`
- [x] Update function comments in `internal/guardian/domain_approver.go`
- [x] Update comments in `internal/guardian/proxy.go` for evaluation order
- [ ] **Test**: Manual - `go doc` output shows updated terminology

### 5.4 Migration path for existing users

- [x] Add check in `DecisionDir()` to detect if old `approvals/` directory exists
- [x] If `approvals/` exists and `decisions/` does not, automatically migrate (rename directory)
- [x] Log migration: "Migrated approvals/ to decisions/ directory"
- [x] **Test**: Integration test - create `approvals/` directory, start guardian, verify migrated to `decisions/`
- [x] **Test**: Integration test - verify contents preserved during migration

---

## Phase 6: Config Naming & Structure Consistency

Unify naming between global and project configs, unify the structure of static config
and decision files, add deny support to static config, and fix the `MergeAllowlists`
dedup bug for pattern entries.

### Summary of changes

**Naming:** Project config `commands` → `hostexec` (matching global config field name).

**Structure:** Decision files currently use flat top-level lists (`domains`, `patterns`,
`denied_domains`, `denied_patterns`). Restructure to mirror static config's nested
`proxy.allow`/`proxy.deny` format using `AllowEntry` objects with `domain`/`pattern`
discrimination. This means a user moving entries between static config and decision
files doesn't need to restructure the data.

**New capability:** Static config files (global and project) gain `proxy.deny` support,
symmetric with `proxy.allow`. Previously, deny rules could only be expressed in
decision files.

**Bug fix:** `MergeAllowlists` deduplicates by `Domain` field only; pattern-only
`AllowEntry` values all collide on empty string. Fix the key function.

### 6.1 Rename project `commands` → `hostexec`

- [x] In `internal/config/types.go`: rename `ProjectCommandsConfig` → `ProjectHostexecConfig`
- [x] In `internal/config/types.go`: rename `ProjectConfig.Commands` field → `Hostexec` with YAML tag `hostexec`
- [x] Update `internal/config/merge.go` `ResolveConfig()`: change `project.Commands.*` → `project.Hostexec.*`
- [x] Update `internal/config/validate.go` `ValidateProjectConfig()`: change `cfg.Commands.*` → `cfg.Hostexec.*`, update error prefix strings from `"commands.*"` → `"hostexec.*"`
- [x] Update all YAML literals in test files (`commands:` → `hostexec:`) in:
  - `internal/config/types_test.go`
  - `internal/config/parse_test.go`
  - `internal/config/load_test.go`
  - `internal/config/merge_test.go`
  - `internal/config/validate_test.go`
  - `internal/config/defaults_test.go`
- [x] Update all Go references (`cfg.Commands.*` → `cfg.Hostexec.*`, `ProjectCommandsConfig` → `ProjectHostexecConfig`) in the same test files
- [x] **Test**: `make test` passes with all renames

### 6.2 Add `proxy.deny` to static config types

- [x] Add `Deny []AllowEntry` field to `ProxyConfig` with YAML tag `deny,omitempty`
- [x] Add `Deny []AllowEntry` field to `ProjectProxyConfig` with YAML tag `deny,omitempty`
- [x] Add `MergeDenylists` function in `internal/config/merge.go` (parallel to `MergeAllowlists`)
- [x] Update `EffectiveConfig` in `merge.go`: add `Deny []AllowEntry` field
- [x] Update `ResolveConfig()` in `merge.go`: merge deny lists from global + project config
- [x] **Test**: Unit test for `MergeDenylists` merging global and project deny entries
- [x] **Test**: Unit test for `ResolveConfig` populating `Deny` field

### 6.3 Fix `MergeAllowlists` / `MergeDenylists` dedup key

The current key function `func(e AllowEntry) string { return e.Domain }` causes all
pattern-only entries (where Domain is "") to collide. Fix both merge functions.

- [x] Change `MergeAllowlists` key function to `func(e AllowEntry) string { if e.Pattern != "" { return "p:" + e.Pattern }; return "d:" + e.Domain }`
- [x] Use the same key function in `MergeDenylists`
- [x] **Test**: Unit test — merge two lists each containing pattern-only entries; verify all patterns survive
- [x] **Test**: Unit test — merge lists with mix of domain and pattern entries; verify correct dedup

### 6.4 Restructure `Decisions` type to mirror static config

Replace the four flat string lists with nested `proxy.allow` / `proxy.deny` using
`AllowEntry` objects.

Old format:
```yaml
domains: [example.com]
patterns: ["*.example.com"]
denied_domains: [evil.com]
denied_patterns: ["*.evil.com"]
```

New format:
```yaml
proxy:
  allow:
    - domain: example.com
    - pattern: "*.example.com"
  deny:
    - domain: evil.com
    - pattern: "*.evil.com"
```

- [x] Redefine `Decisions` struct in `internal/config/decisions.go`:
  ```go
  type Decisions struct {
      Proxy DecisionsProxy `yaml:"proxy,omitempty"`
  }
  type DecisionsProxy struct {
      Allow []AllowEntry `yaml:"allow,omitempty"`
      Deny  []AllowEntry `yaml:"deny,omitempty"`
  }
  ```
- [x] Update `LoadGlobalDecisions` / `LoadProjectDecisions` — these use `strictUnmarshal` so the new struct just works
- [x] Update `WriteGlobalDecisions` / `WriteProjectDecisions` — same, `yaml.Marshal` handles it
- [x] Update all decision-reading code in `internal/config/decisions_test.go`:
  - Replace `decisions.Domains` → access via `decisions.Proxy.Allow` filtering by Domain
  - Replace `decisions.Patterns` → access via `decisions.Proxy.Allow` filtering by Pattern
  - Replace `decisions.DeniedDomains` → access via `decisions.Proxy.Deny` filtering by Domain
  - Replace `decisions.DeniedPatterns` → access via `decisions.Proxy.Deny` filtering by Pattern
- [x] Add helper methods on `Decisions` for convenience (optional but reduces churn):
  ```go
  func (d *Decisions) AllowedDomains() []string   // extract domain strings from Proxy.Allow
  func (d *Decisions) AllowedPatterns() []string   // extract pattern strings from Proxy.Allow
  func (d *Decisions) DeniedDomains() []string     // extract domain strings from Proxy.Deny
  func (d *Decisions) DeniedPatterns() []string    // extract pattern strings from Proxy.Deny
  ```
- [x] **Test**: Unit test — round-trip marshal/unmarshal of new `Decisions` format
- [x] **Test**: Unit test — empty `Decisions` marshals to empty YAML (no spurious keys)
- [x] **Test**: Unit test — `AllowedDomains()` / `DeniedPatterns()` helpers return correct values

### 6.5 Update guardian startup (`internal/cmd/guardian.go`)

The `decisionsToAllowEntries` function becomes trivial (decisions already contain
`AllowEntry` slices), but the call sites need updating.

- [x] Replace `decisionsToAllowEntries()` with direct access to `decisions.Proxy.Allow` and `decisions.Proxy.Deny`
- [x] Update global allowlist construction: `globalDecisions.Proxy.Allow` instead of iterating `Domains`/`Patterns`
- [x] Update global denylist construction: `globalDecisions.Proxy.Deny` instead of iterating `DeniedDomains`/`DeniedPatterns`
- [x] Update `loadProjectAllowlist` closure: use `projectDecisions.Proxy.Allow`
- [x] Update `loadProjectDenylist` closure: use `projectDecisions.Proxy.Deny`
- [x] Update config reloader (SIGHUP handler): same changes as above
- [x] Update `ConfigPersisterImpl.ReloadNotifier` closure: same changes
- [x] Delete the now-unnecessary `decisionsToAllowEntries()` function
- [x] **Test**: `make test` passes (existing `guardian_helpers_test.go` for `decisionsToAllowEntries` either removed or updated)

### 6.6 Update guardian domain approver and config persister

The `persistDenial` method in `domain_approver.go` writes to flat fields on `Decisions`.
The `ConfigPersisterImpl` methods write to `Decisions.Domains` / `Decisions.Patterns`.
Both need to write to the new nested structure.

- [x] Update `domain_approver.go` `persistDenial()`:
  - Replace `decisions.DeniedDomains = appendUnique(...)` → append `AllowEntry{Domain: target}` to `decisions.Proxy.Deny`
  - Replace `decisions.DeniedPatterns = appendUnique(...)` → append `AllowEntry{Pattern: target}` to `decisions.Proxy.Deny`
  - Add dedup check against existing `decisions.Proxy.Deny` entries
- [x] Update `config_persister.go` `AddDomainToProject` / `AddDomainToGlobal`:
  - Replace `approvals.Domains = append(...)` → append `AllowEntry{Domain: domain}` to `approvals.Proxy.Allow`
  - Dedup check against `approvals.Proxy.Allow`
- [x] Update `config_persister.go` `AddPatternToProject` / `AddPatternToGlobal`:
  - Replace `approvals.Patterns = append(...)` → append `AllowEntry{Pattern: pattern}` to `approvals.Proxy.Allow`
  - Dedup check against `approvals.Proxy.Allow`
- [x] Update `domain_approver_test.go`: change assertions from `decisions.DeniedDomains` to `decisions.Proxy.Deny` entries
- [x] Update `config_persister_test.go` and `config_persister_validation_test.go`: change assertions from `approvals.Domains`/`approvals.Patterns` to `approvals.Proxy.Allow` entries
- [x] **Test**: `make test` passes with all domain approver and config persister tests

### 6.7 Update E2E tests

- [ ] Update `test/e2e/domain_denial_test.go`:
  - Replace `decisions.DeniedDomains` → check `decisions.Proxy.Deny` for domain entries
  - Replace `decisions.DeniedPatterns` → check `decisions.Proxy.Deny` for pattern entries
  - Replace `config.Decisions{DeniedDomains: ...}` → `config.Decisions{Proxy: config.DecisionsProxy{Deny: ...}}`
- [ ] Update `test/e2e/domain_approval_persistence_test.go`:
  - Replace `approvals.Domains` → check `approvals.Proxy.Allow` for domain entries
- [ ] **Test**: `make test-e2e` passes (requires Docker + guardian)

### 6.8 Wire static deny config into guardian startup

Now that static config has `proxy.deny`, the guardian needs to load it alongside
decision-file denylists.

- [ ] In `runGuardianProxy()`: build global denylist from both `cfg.Proxy.Deny` (static) and `globalDecisions.Proxy.Deny` (decisions)
- [ ] In `loadProjectDenylist`: merge `projectCfg.Proxy.Deny` (static) with `projectDecisions.Proxy.Deny` (decisions)
- [ ] In config reloader: same merging for deny
- [ ] **Test**: Unit test — global static deny + decision deny are both loaded
- [ ] **Test**: Unit test — project static deny merges with project decision deny

---

## Phase 7: Documentation & Spec Updates

Update all documentation to reflect the config consistency changes.

### 7.1 Update config-reference.md

- [ ] Change project config example: `commands:` → `hostexec:`
- [ ] Add `proxy.deny` to both global and project config schemas
- [ ] Update decision file schema to new `proxy.allow`/`proxy.deny` format
- [ ] Update the "Consolidation" section to reflect that moving entries between decision files and static config no longer requires restructuring
- [ ] Update precedence rules if needed (static deny should merge with decision deny)

### 7.2 Update CLAUDE.md

- [ ] Update the "Internal Packages" table if any package descriptions changed
- [ ] Ensure config field references are accurate

### 7.3 Update other spec files

- [ ] Grep specs/ for references to `commands:` in project config context → update to `hostexec:`
- [ ] Grep specs/ for references to old decisions format (`denied_domains`, `denied_patterns`, flat `domains`/`patterns`) → update to new nested format
- [ ] **Test**: Manual review of all spec files for consistency

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
