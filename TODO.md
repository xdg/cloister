# PolicyEngine Refactor: Unified Proxy Domain Access Control

Replace the multi-layer callback-heavy proxy approval/denial architecture
(AllowlistCache, CacheReloader, MemorySessionAllowlist, MemorySessionDenylist,
ConfigPersisterImpl, closure-based reloading) with a single PolicyEngine that
owns all domain access policy state.

## Testing Philosophy

- **Unit tests for all policy logic**: DomainSet membership, ProxyPolicy evaluation, PolicyEngine.Check precedence
- **Table-driven tests**: Precedence edge cases (deny overrides allow, session vs static, etc.)
- **In-process tests only**: No Docker, no real guardian. Use `t.TempDir()` for config/decision files
- **Go tests**: Use `testing` with `httptest` for proxy handler tests
- **Preserve existing proxy_test.go patterns**: The proxy tests use mock interfaces; PolicyEngine becomes one more mock-able interface
- **Test what changed, not what didn't**: PatternCache, hostexec, approval queue are untouched — skip their tests

## Verification Checklist

Before marking a phase complete and committing it:

1. `make test` passes
2. `make lint` passes
3. `make fmt` produces no changes
4. `make build` succeeds
5. No dead code left behind (unused types, functions, imports)

When verification of a phase or subphase is complete, commit all
relevant newly-created and modified files.

## Dependencies Between Phases

```
Phase 1 (DomainSet) ─► Phase 2 (ProxyPolicy + PolicyEngine)
                                    │
                                    ├─► Phase 3 (Proxy integration)
                                    │         │
                                    │         ▼
                                    │   Phase 4 (Approval integration)
                                    │         │
                                    │         ▼
                                    │   Phase 5 (SIGHUP + guardian.go wiring)
                                    │         │
                                    │         ▼
                                    │   Phase 6 (Delete dead code + cleanup)
                                    │
                                    └─► Phase 7 (Smoke test full flow)
```

---

## Phase 1: Extract DomainSet from Allowlist

Rename/refactor the core domain matching logic into a `DomainSet` type that
has clear semantics (set membership, not "allow" or "deny" — just "contains").
Keep the `Allowlist` type temporarily as a thin wrapper or type alias so
existing code still compiles during the transition.

### 1.1 Create DomainSet type

- [x] Create `internal/guardian/domain_set.go` with `DomainSet` struct:
  - `domains map[string]struct{}` (exact matches)
  - `patterns []string` (wildcards like `*.example.com`)
  - `Contains(domain string) bool` — strips port, checks exact then patterns
  - `Add(domain string)` — adds exact domain
  - `AddPattern(pattern string)` — adds wildcard pattern (validates first)
  - `NewDomainSet(domains []string, patterns []string) *DomainSet`
  - `NewDomainSetFromConfig(entries []config.AllowEntry) *DomainSet`
- [x] Move `stripPort`, `matchPattern`, `IsValidPattern` into `domain_set.go` (or keep shared)
- [x] `DomainSet` needs its own `sync.RWMutex` for thread-safe `Add`/`AddPattern` (session-tier mutations happen concurrently with reads)
- [x] **Test**: `domain_set_test.go` — exact match, pattern match, port stripping, Add/AddPattern, concurrent access

### 1.2 Alias Allowlist to DomainSet

- [x] Redefine `Allowlist` in `allowlist.go` as a thin wrapper: `type Allowlist = DomainSet` or delegate all methods
- [x] Verify `make test` passes with no changes to callers
- [x] **Test**: Existing `allowlist_test.go` and `allowlist_pattern_test.go` still pass unchanged

---

## Phase 2: PolicyEngine Core

Implement the PolicyEngine with `Check`, `RecordDecision`, `ReloadGlobal`,
`ReloadProject`, and `RevokeToken`. No integration with proxy or approval
server yet — pure policy logic with disk I/O for reload.

### 2.1 Define ProxyPolicy and Decision types

- [x] Create `internal/guardian/policy_engine.go`
- [x] `type Decision int` with `Allow`, `Deny`, `AskHuman` constants
- [x] `type ProxyPolicy struct { Allow *DomainSet; Deny *DomainSet }` — nil-safe (nil means empty)
- [x] `func (p *ProxyPolicy) IsAllowed(domain string) bool`
- [x] `func (p *ProxyPolicy) IsDenied(domain string) bool`

### 2.2 Implement PolicyEngine

- [x] `type PolicyEngine struct` with `global ProxyPolicy`, `projects map[string]*ProxyPolicy`, `tokens map[string]*ProxyPolicy`, `sync.RWMutex`
- [x] `Check(token, project, domain string) Decision` — deny pass (global→project→token), allow pass (global→project→token), fallback AskHuman
- [x] `Check` must handle missing project/token gracefully (skip that tier)
- [x] Constructor: `NewPolicyEngine(cfg *config.GlobalConfig, globalDecisions *config.Decisions, projectLister ProjectLister) (*PolicyEngine, error)`
  - Builds global ProxyPolicy from config + decisions
  - Eagerly loads all projects from `projectLister.List()` via `loadProjectPolicy(name)`
- [x] **Test**: `policy_engine_test.go` — Check precedence: deny beats allow, global deny beats project allow, token deny beats everything, AskHuman when nothing matches

### 2.3 Implement targeted reload and record

- [x] `ReloadGlobal() error` — re-reads global config + global decisions, rebuilds `pe.global`
- [x] `ReloadProject(name string) error` — re-reads project config + project decisions, rebuilds `pe.projects[name]`
- [x] `RecordDecision(token, project, domain string, scope Scope, allowed bool, isPattern bool) error`
  - `scope == "session"`: mutate `pe.tokens[token]` in-memory only
  - `scope == "project"`: persist to project decisions file, then `ReloadProject(project)`
  - `scope == "global"`: persist to global decisions file, then `ReloadGlobal()`
  - `scope == "once"`: no-op (caller already has the allow/deny decision)
- [x] `RevokeToken(token string)` — `delete(pe.tokens, token)`
- [x] `ReloadAll() error` — for SIGHUP: reload global + all projects
- [x] Persistence logic: absorb `ConfigPersisterImpl`'s load-check-dedup-append-write pattern for allow entries; absorb `DomainApproverImpl.persistDenial` pattern for deny entries
- [x] **Test**: RecordDecision session scope — adds to token policy, Check reflects it immediately
- [x] **Test**: RecordDecision project scope — persists to temp dir, ReloadProject picks it up
- [x] **Test**: RecordDecision global scope — persists to temp dir, ReloadGlobal picks it up
- [x] **Test**: RevokeToken — Check no longer sees session decisions for that token
- [x] **Test**: ReloadAll — rebuilds global + all project policies

---

## Phase 3: Proxy Integration

Replace ProxyServer's AllowlistCache, SessionAllowlist, SessionDenylist, and
TokenLookup fields with a single PolicyEngine field. Rewrite
`checkDomainAccess` to delegate to `PolicyEngine.Check`.

### 3.1 Add PolicyEngine to ProxyServer

- [x] Add `PolicyEngine *PolicyEngine` field to `ProxyServer`
- [x] Keep `TokenLookup` (still needed by `resolveRequest` to map token→project)
- [x] Rewrite `checkDomainAccess` to call `pe.Check(token, project, domain)` and map `Decision` to allow/deny/requestApproval
- [x] Remove `AllowlistCache`, `SessionAllowlist`, `SessionDenylist` fields from ProxyServer
- [x] Update `resolveRequest` — no longer needs to call `AllowlistCache.GetProject()` to get a per-project allowlist; just needs token→project mapping
- [x] The `resolvedRequest` struct loses the `Allowlist` field; keeps `ProjectName`, `CloisterName`, `Token`
- [x] **Test**: Update `proxy_test.go` — replace AllowlistCache/Session mocks with PolicyEngine (or mock PolicyEngine interface)

### 3.2 Define PolicyChecker interface for testability

- [x] Extract `PolicyChecker` interface: `Check(token, project, domain string) Decision`
- [x] ProxyServer takes `PolicyChecker` (interface), PolicyEngine implements it
- [x] This lets proxy tests use a simple mock without building a full PolicyEngine
- [x] **Test**: Verify proxy tests still pass with mock PolicyChecker

---

## Phase 4: Approval Integration

Rewire `DomainApproverImpl` and `approval.Server` to use
`PolicyEngine.RecordDecision` instead of separate session/persist/cache paths.

### 4.1 Simplify DomainApproverImpl

- [x] Replace `sessionAllowlist`, `sessionDenylist`, `allowlistCache` fields with `PolicyEngine` (or `PolicyChecker` + `DecisionRecorder` interface)
- [x] `handleApproval` → call `pe.RecordDecision(token, project, domain, scope, true, false)`
- [x] `handleDenial` → call `pe.RecordDecision(token, project, domain, scope, false, isPattern)`
- [x] Delete `persistDenial`, `updateDenylistCache`, `containsDenyEntry` methods
- [x] Update `NewDomainApprover` signature
- [x] **Test**: Update `domain_approver_test.go` — mock PolicyEngine instead of mocking SessionAllowlist/Denylist/AllowlistCache

### 4.2 Replace ConfigPersister on approval.Server

- [x] The `approval.Server.ConfigPersister` interface is used for project/global scope approvals
- [x] Option A: PolicyEngine implements `approval.ConfigPersister` interface directly (4 methods map to RecordDecision calls) — skipped in favor of Option B
- [x] Option B: Thin adapter that wraps PolicyEngine and implements ConfigPersister — implemented as `PolicyConfigPersister` in `policy_config_persister.go`
- [x] Either way, `ConfigPersisterImpl` type is eliminated — deferred to Phase 6; `ConfigPersisterImpl` still used by legacy code in `cmd/guardian.go`
- [x] **Test**: Update `approval/server_test.go` mock to match new interface (or keep existing interface if using adapter) — interface unchanged; `approval/server_test.go` unaffected; adapter tested in `policy_config_persister_test.go`

### 4.3 Wire token revocation

- [x] When a token is revoked (API server `handleRevoke`), call `pe.RevokeToken(token)`
- [x] Currently `api.SessionAllowlist.Clear(token)` is called — replace with `pe.RevokeToken(token)`
- [x] Remove `SessionAllowlist` field from APIServer
- [x] **Test**: Verify token revocation clears session policy

---

## Phase 5: SIGHUP and guardian.go Wiring

Simplify the guardian startup wiring now that PolicyEngine owns all state.

### 5.1 Simplify guardianState

- [ ] Replace `allowlistCache` and `reloader` fields with `policyEngine *guardian.PolicyEngine`
- [ ] Remove `setupAllowlistCache` function entirely
- [ ] Remove `setupConfigReloader` function entirely (SIGHUP calls `pe.ReloadAll()` + `patternCache.Clear()`)
- [ ] Simplify `setupDomainApproval` — no longer creates `sessionAllowlist`/`sessionDenylist` objects
- [ ] `ConfigPersisterImpl` instantiation gone (replaced by PolicyEngine or adapter)
- [ ] Remove `ReloadNotifier` callback wiring

### 5.2 Rewrite SIGHUP handler

- [ ] ProxyServer's SIGHUP handler calls `pe.ReloadAll()` instead of the closure chain
- [ ] Also reload hostexec PatternCache (unrelated, keep as-is)
- [ ] Remove `ConfigReloader` type from proxy.go
- [ ] Remove `SetConfigReloader` method from ProxyServer
- [ ] **Test**: Verify SIGHUP-triggered reload picks up new config/decisions (unit test with temp files)

### 5.3 Simplify proxy constructor

- [ ] `NewProxyServerWithConfig` no longer needs an initial Allowlist parameter — it gets a PolicyEngine
- [ ] Update or replace with simpler constructor
- [ ] **Test**: Verify proxy starts and checks domains correctly with PolicyEngine

---

## Phase 6: Delete Dead Code and Cleanup

Remove all replaced types, files, and their tests.

### 6.1 Delete replaced files

- [ ] Delete `internal/guardian/allowlist_cache.go` + `allowlist_cache_test.go`
- [ ] Delete `internal/guardian/cache_reloader.go` + `cache_reloader_test.go`
- [ ] Delete `internal/guardian/session_allowlist.go` + `session_allowlist_test.go`
- [ ] Delete `internal/guardian/config_persister.go` + `config_persister_test.go` + `config_persister_validation_test.go`

### 6.2 Clean up allowlist.go

- [ ] If `Allowlist` is now just an alias for `DomainSet`, decide: keep alias for external compatibility or rename all usages
- [ ] Remove `Allowlist` type entirely if nothing outside guardian uses it
- [ ] Move `DefaultAllowedDomains` to `policy_engine.go` or `defaults.go`
- [ ] Delete `allowlist.go` if fully absorbed into `domain_set.go`
- [ ] Update `allowlist_test.go` / `allowlist_pattern_test.go` → rename or merge into `domain_set_test.go`

### 6.3 Clean up proxy.go interfaces

- [ ] Remove `SessionAllowlist` interface from proxy.go
- [ ] Remove `SessionDenylist` interface from proxy.go
- [ ] Remove `ProjectAllowlistLoader`, `ProjectDenylistLoader`, `TokenLookupFunc` from `allowlist_cache.go` (already deleted)
- [ ] Remove `ConfigReloader` type and `SetConfigReloader` from proxy.go
- [ ] Remove `ConfigError` type if no longer needed (or keep if proxy still returns 502 for config errors)
- [ ] **Test**: `make test` passes, `make lint` clean, no unused imports

---

## Phase 7: End-to-End Validation

Validate the full flow works with the real guardian (requires Docker).

### 7.1 Integration smoke test

- [ ] `make build` succeeds
- [ ] `make test` passes (all unit tests)
- [ ] `make lint` passes
- [ ] Manual or integration test: start guardian, register token, proxy request to allowed domain → allowed
- [ ] Manual or integration test: proxy request to denied domain → denied
- [ ] Manual or integration test: proxy request to unlisted domain → approval prompt appears
- [ ] Manual or integration test: approve with session scope → subsequent requests allowed for that token only
- [ ] Manual or integration test: approve with project scope → persisted to decisions file, survives reload
- [ ] Manual or integration test: SIGHUP → config changes take effect

---

## Future Phases (Deferred)

### PolicyEngine Enhancements
- Time-based decision expiry ("allow for 1 hour")
- Per-domain rate limiting
- Decision audit log (queryable history of all Check calls)
- Policy dry-run mode (log what would happen without enforcing)

### Config Simplification
- Merge static config and decisions into a single file per scope (eliminate the config/decisions split)
- Config file watching (inotify) instead of SIGHUP
