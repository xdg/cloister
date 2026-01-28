# Cloister Quality Improvements

Code quality refactoring following Phase 2 completion. Eliminates duplication, improves testability, and simplifies APIs without changing external behavior.

## Testing Philosophy

- **Unit tests for refactored code**: All extracted helpers must have unit tests
- **Regression tests**: Existing tests must continue passing after each change
- **No behavior changes**: These are internal refactors; external APIs remain stable
- **Go tests**: Use `testing` package; verify coverage doesn't decrease
- **Incremental commits**: One logical change per commit for easy bisection

## Verification Checklist

Before marking Quality phase complete:

1. `make test` passes
2. `make build` produces working binary
3. `make lint` passes
4. `make test-race` passes (no race conditions)
5. Code coverage does not decrease from baseline
6. Manual smoke test: `cloister start` / `cloister stop` workflow works

## Dependencies Between Phases

```
Q.1 Foundation (Duplicate Code Elimination)
       │
       ▼
Q.2 Shared Utilities
       │
       ▼
Q.3 API Simplification
       │
       ▼
Q.4 Dependency Injection & Testability
       │
       ▼
Q.5 Testing & Polish
```

---

## Phase Q.1: Foundation — Duplicate Code Elimination

Extract helpers from duplicated code patterns. No API changes.

### Q.1.1 Consolidate docker strict/non-strict function pairs
- [x] In `internal/docker/docker.go`, consolidate `RunJSON` and `RunJSONStrict` into single function
- [x] Add `strict bool` parameter or use options pattern
- [x] Same for `RunJSONLines` and `RunJSONLinesStrict`
- [x] Extract repeated `cmdName` extraction (lines 54-57, 121-124, 181-184, 207-210) into helper
- [x] **Test**: Existing tests pass; add test for strict vs non-strict behavior

### Q.1.2 Extract container creation helper
- [x] In `internal/container/manager.go`, extract shared logic from `Start()` and `Create()`
- [x] Lines 45-78 and 86-119 are 95% identical
- [x] Create private `createContainer()` helper
- [x] **Test**: Existing container tests pass

### Q.1.3 Extract token store helper in cloister
- [x] In `internal/cloister/cloister.go`, extract token store creation pattern
- [x] `Start()` (lines 57-67) and `Stop()` (lines 189-195) both create tokenDir/store
- [x] Create `getTokenStore() (*token.Store, error)` helper
- [x] **Test**: Existing cloister tests pass

### Q.1.4 Extract cmd error helpers
- [x] Create `internal/cmd/errors.go`
- [x] Extract `dockerNotRunningError()` — used in 6 locations across start.go, stop.go, list.go, guardian.go
- [x] Extract `gitDetectionError(err error) error` — handles ErrNotGitRepo/ErrGitNotInstalled consistently
- [x] Update all call sites to use helpers
- [x] **Test**: Add unit tests for error helpers

### Q.1.5 Extract guardian client HTTP helper
- [x] In `internal/guardian/client.go`, extract shared HTTP request execution
- [x] Lines 55-72, 84-106, 118-135 all repeat: get client, do request, check status, decode error
- [x] Create private `doRequest(method, path string, body, result any) error` helper
- [x] **Test**: Existing client tests pass

### Q.1.6 Extract guardian container call pattern
- [x] In `internal/guardian/container.go`, extract guardian client call pattern
- [x] `RegisterToken`, `RevokeToken`, `ListTokens` all: check IsRunning(), create client, call method
- [x] Create helper that handles IsRunning check and client creation
- [x] **Test**: Existing guardian tests pass

---

## Phase Q.2: Shared Utilities

Create shared packages for code duplicated across multiple packages.

### Q.2.1 Create internal/pathutil package
- [x] Create `internal/pathutil/home.go`
- [x] Move `expandHome()` from `internal/config/paths.go` (lines 46-64)
- [x] Remove duplicate from `internal/project/registry.go` (lines 97-115)
- [x] Update both packages to import from pathutil
- [x] **Test**: Add unit tests for expandHome edge cases

### Q.2.2 Extract docker exact-match filtering
- [x] In `internal/docker/docker.go`, add `FindContainerByExactName(name string) (*ContainerInfo, error)`
- [x] Identical filtering pattern exists in:
  - `internal/container/manager.go` (lines 244-263)
  - `internal/guardian/container.go` (lines 206-223)
  - `internal/docker/network.go` (lines 34-51) — network pattern left as-is (different API)
- [x] Update all three locations to use shared helper
- [x] **Test**: Add unit test for exact-match vs substring behavior

### Q.2.3 Extract HostDir helper in guardian
- [x] In `internal/guardian/container.go`, `HostTokenDir()` and `HostConfigDir()` have identical structure
- [x] Extract `hostCloisterPath(subdir string) (string, error)` helper
- [x] Both get home dir, join with `.config/cloister/<subdir>`
- [x] **Test**: Existing tests pass

---

## Phase Q.3: API Simplification

Consolidate redundant API methods. May require updating callers.

### Q.3.1 Consolidate token.Registry methods
- [ ] In `internal/token/registry.go`, consolidate lookup methods
- [ ] Replace `Lookup()`, `LookupProject()`, `LookupInfo()` with single `Lookup() (TokenInfo, bool)`
- [ ] Callers use only the fields they need from TokenInfo
- [ ] Consolidate `List()` and `ListInfo()` into single `List() map[string]TokenInfo`
- [ ] Mark old `Register()` as deprecated, prefer `RegisterWithProject()`
- [ ] Update callers in `internal/guardian/` and `internal/cmd/`
- [ ] **Test**: Update tests to use new signatures

### Q.3.2 Consolidate config merge functions
- [ ] In `internal/config/merge.go`, extract generic merge helper
- [ ] `MergeAllowlists()` and `MergeCommandPatterns()` have identical dedup logic
- [ ] Create generic `mergeSlice[T comparable](a, b []T, key func(T) string) []T`
- [ ] Or use simpler approach: extract dedup logic to helper
- [ ] **Test**: Existing merge tests pass

### Q.3.3 Consolidate container existence checks
- [ ] In `internal/container/manager.go`, consolidate `containerExists()` and `IsRunning()`
- [ ] Both query same Docker data; return `(exists bool, running bool, err error)`
- [ ] Single Docker call per check instead of separate calls
- [ ] **Test**: Add test verifying single Docker call

---

## Phase Q.4: Dependency Injection & Testability

Add interfaces and injection points to enable unit testing without Docker.

### Q.4.1 Inject container.Manager in cloister
- [ ] In `internal/cloister/cloister.go`, add `Manager` field to package or use functional options
- [ ] Replace inline `container.NewManager()` calls with injected dependency
- [ ] Default to `container.NewManager()` when not injected
- [ ] **Test**: Add unit test using mock Manager

### Q.4.2 Add DockerRunner interface in container
- [ ] In `internal/container/manager.go`, define `DockerRunner` interface
- [ ] Interface wraps `docker.Run`, `docker.RunJSONLines` calls
- [ ] Add `Runner` field to `Manager` struct, default to real implementation
- [ ] **Test**: Add unit test using mock DockerRunner

### Q.4.3 Add Clock interface in project.Registry
- [ ] In `internal/project/registry.go`, inject time source
- [ ] Replace direct `time.Now()` calls (lines 123, 181) with clock interface
- [ ] Default to real time when not injected
- [ ] **Test**: Add test verifying LastUsed timestamp updates

---

## Phase Q.5: Testing & Polish

Add missing tests and file organization improvements.

### Q.5.1 Add cmd package tests
- [ ] Create `internal/cmd/start_test.go` — test runStart logic
- [ ] Create `internal/cmd/stop_test.go` — test runStop logic
- [ ] Create `internal/cmd/list_test.go` — test runList logic
- [ ] Focus on error handling paths (Docker not running, git detection, etc.)
- [ ] **Test**: New tests achieve >70% coverage for these files

### Q.5.2 Add token package tests
- [ ] Add test for `DefaultTokenDir()` — currently 0% coverage
- [ ] Add test for `Store.Dir()` — currently 0% coverage
- [ ] **Test**: Token package coverage increases to >90%

### Q.5.3 Refactor os.Exit in cmd handlers
- [ ] In `internal/cmd/start.go`, replace `os.Exit(exitCode)` (lines 152, 196) with typed error
- [ ] Define `ExitCodeError` type that carries exit code
- [ ] Handle in root command to call `os.Exit` with code
- [ ] **Test**: Test that exit code propagates correctly

### Q.5.4 File organization
- [ ] Move name generation functions from `internal/container/config.go` to `internal/container/names.go`
- [ ] Move `AllowlistCache` from `internal/guardian/proxy.go` to `internal/guardian/allowlist_cache.go`
- [ ] Move credential env var logic from `internal/token/env.go` to `internal/token/credentials.go`
- [ ] **Test**: No behavior change, existing tests pass

### Q.5.5 Code style consistency
- [ ] Use `filepath.Join` consistently instead of string concatenation in `internal/project/registry.go`
- [ ] Rename shadowing variable in `internal/cmd/list.go` line 86 (`project` shadows package)
- [ ] Group sentinel errors together in `internal/docker/docker.go`
- [ ] **Test**: `make lint` passes

---

## Not In Scope (Deferred to Later Phases)

### Phase 3: Claude Code Integration
- `cloister setup claude` wizard
- Credential storage in config
- Remove host env var dependency

### Phase 4: Host Execution
- hostexec wrapper
- Request server (:9998) and approval server (:9999)
- Approval web UI with htmx
- Auto-approve and manual-approve pattern execution

### Phase 5: Worktree Support
- `cloister start -b <branch>` creates managed worktrees
- Worktree cleanup and management

### Phase 6: Domain Approval Flow
- Proxy holds connection for unlisted domains
- Interactive domain approval with persistence options

### Phase 7: Polish
- Shell completion
- Read-only reference mounts
- Audit logging
- Detached mode, non-git support
- `project show` and `project edit` should infer project name from current directory
- Suppress log output in user-facing commands (or redirect to stderr with --verbose flag)
- Don't show usage on config load errors (just show error message)
- Multi-arch Docker image builds (linux/amd64, linux/arm64)

### Future: Devcontainer Integration
- Devcontainer.json discovery and image building
- Security overrides for mounts and capabilities
