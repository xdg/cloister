# Extract Business Logic from internal/cmd

Move business logic out of CLI command handlers into library packages so
commands become thin shims. This improves testability and reuse.

## Testing Philosophy

- **Unit tests for all extracted functions**: Each moved function gets tests in its destination package
- **Table-driven tests**: Use table-driven style for functions with multiple input cases (name parsing, token lookup)
- **In-process tests only**: No Docker, no real guardian. Use `t.TempDir()`, mock interfaces, `httptest`
- **Go tests**: Use `testing` package; mock `container.Manager` via interfaces where needed
- **Test what moved, not what stayed**: Cmd handlers that become one-line delegations don't need new tests
- **Preserve existing test coverage**: Existing tests in destination packages must keep passing

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
Phase 1 (container names) ─────────────────────────┐
Phase 2 (token lookup) ────────────────────────────┐│
Phase 3 (project auto-register) ──────────────────┐││
Phase 4 (cloister detect + attach) ──────────────┐│││
Phase 5 (container running check) ──────────────┐││││
                                                 │││││
                                                 ▼▼▼▼▼
                                          Phase 6 (cmd shims)
                                                 │
                                                 ▼
                                          Phase 7 (guardian server)
                                                 │
                                                 ▼
                                          Phase 8 (validation)
```

Phases 1-5 are independent and can be done in any order. Phase 6 updates all
cmd files to use the new functions. Phase 7 is the largest extraction (guardian
server startup). Phase 8 validates everything end-to-end.

---

## Phase 1: Extract ParseCloisterName to container package

Move name-parsing logic to where the other name functions live.

### 1.1 Add ParseCloisterName to container/names.go

- [x] Add `ParseCloisterName(name string) (project, branch string)` to `internal/container/names.go`
- [x] Logic: split on last hyphen (same as current `cmd/list.go:parseCloisterName`)
- [x] **Test**: `names_test.go` — "foo-main" → ("foo","main"), "foo-bar-feature" → ("foo-bar","feature"), "foo" → ("foo","")

### 1.2 Update cmd/list.go

- [x] Replace `parseCloisterName()` call with `container.ParseCloisterName()`
- [x] Delete the local `parseCloisterName` function

---

## Phase 2: Extract token reverse lookup to guardian package

Deduplicate `findTokenForCloister` (stop.go) and `tokenForContainer` (shutdown.go).

### 2.1 Add FindTokenForContainer to guardian

- [x] Add `FindTokenForContainer(containerName string) string` to `internal/guardian/` (e.g. `client.go` or new `helpers.go`)
- [x] Implementation: call `ListTokens()`, iterate map, return matching token or ""
- [x] Returns "" on any error (best-effort, matches current behavior)
- [x] **Test**: Unit test with mock HTTP server returning a token map, verify correct container matched

### 2.2 Update cmd/stop.go and cmd/shutdown.go

- [x] Replace `findTokenForCloister()` with `guardian.FindTokenForContainer()`
- [x] Replace `tokenForContainer()` usage in shutdown.go: call `guardian.FindTokenForContainer()` per container (simpler API; guardian is local so N+1 cost is negligible)
- [x] Delete both `findTokenForCloister` and `tokenForContainer` local functions

---

## Phase 3: Extract auto-register to project package

Move the load-register-save pattern into a single library call.

### 3.1 Add AutoRegister to project/registry.go

- [x] Add `AutoRegister(name, root, remote, branch string) error` to `internal/project/registry.go`
- [x] Implementation: LoadRegistry, build Info, Register, SaveRegistry. Log warnings for collision errors, return nil on collision (best-effort semantics matching current behavior)
- [x] **Test**: `registry_test.go` — successful registration writes to temp dir; collision returns nil; load failure returns error

### 3.2 Update cmd/start.go

- [x] Replace `autoRegisterProject()` call with `project.AutoRegister()`
- [x] Delete the local `autoRegisterProject` function

---

## Phase 4: Extract cloister detection and attach-existing

Move cloister name detection and attach-or-start logic to library packages.

### 4.1 Add DetectName to cloister package

- [x] Add `DetectName() (string, error)` to `internal/cloister/` (new file `detect.go` or add to existing)
- [x] Implementation: `project.DetectGitRoot(".")` → `project.Name(root)` → `container.GenerateCloisterName(name)`
- [x] **Test**: Unit test with injected git detection (use functional options or interface to mock project detection)

### 4.2 Add AttachExisting to cloister package

- [x] Add `AttachExisting(containerName string, opts ...Option) (int, error)` to `internal/cloister/`
- [x] Implementation: check if running via Manager, start if stopped, call Attach. Return exit code.
- [x] The function does NOT print output — cmd handler does that
- [x] **Test**: Unit test with mock ContainerManager — test stopped-then-started path, already-running path, start-fails path

### 4.3 Update cmd/stop.go and cmd/start.go

- [x] Replace `detectCloisterName()` with `cloister.DetectName()`
- [x] Replace `attachToExisting()` business logic with `cloister.AttachExisting()`, keep output formatting in cmd
- [x] Delete local functions

---

## Phase 5: Extract running-cloister check to container package

Move the "are there running cloisters for project X?" check.

### 5.1 Add HasRunningCloister to container.Manager

- [ ] Add `HasRunningCloister(projectName string) (string, error)` to `container.Manager`
- [ ] Returns the name of a running cloister matching the project, or "" if none
- [ ] Skips guardian container and non-running containers
- [ ] **Test**: Unit test with mock DockerRunner — returns running container matching prefix, returns "" when none match

### 5.2 Update cmd/project.go

- [ ] Replace `checkNoRunningCloisters()` with `container.NewManager().HasRunningCloister(name)`
- [ ] Keep the error message formatting in cmd
- [ ] Delete local `checkNoRunningCloisters` function

---

## Phase 6: Thin out remaining cmd handlers

With phases 1-5 done, update all cmd files to use the extracted functions.
This phase is a sweep to ensure each handler is a thin shim.

### 6.1 Audit and simplify cmd handlers

- [ ] Review each `run*` function in cmd/ to confirm it's now delegation + output + error wrapping
- [ ] Remove any remaining helper functions that were made redundant by phases 1-5
- [ ] Verify no unused imports remain
- [ ] **Test**: `make test` passes, `make lint` clean

---

## Phase 7: Extract guardian server startup

The largest extraction: move `runGuardianProxy` and its ~10 setup helpers from
`cmd/guardian.go` into `internal/guardian/server.go`.

### 7.1 Define Server type

- [ ] Create `internal/guardian/server.go` with `Server` struct
- [ ] Fields: registry, config, policyEngine, patternCache, auditLogger, and the 4 stoppable servers
- [ ] Constructor: `NewServer(registry *token.Registry, cfg *config.GlobalConfig, decisions *config.Decisions) (*Server, error)`
- [ ] Move `setupPolicyEngine`, `setupProxyServer`, `setupPatternCache`, `setupAuditLogger`, `setupDomainApproval`, `setupExecutorClient` into `Server` methods or constructor

### 7.2 Add Run and Shutdown methods

- [ ] `func (s *Server) Run() error` — starts all servers, blocks on signal, shuts down
- [ ] Move `startAllServers`, `awaitShutdownSignal`, `shutdownAllServers` as Server methods
- [ ] Move `stoppable` interface into `server.go`
- [ ] Move `domainApprovalResult` type and `extractPatterns` helper

### 7.3 Move supporting functions

- [ ] Move `loadPersistedTokens` → Server method or standalone in guardian package
- [ ] Move `loadGuardianConfig`, `loadGuardianDecisions` → standalone funcs in guardian package (they just wrap config.Load with fallback)
- [ ] Move `formatDuration`, `getGuardianUptime` — `formatDuration` is a utility (could go in a `util` or stay in guardian); `getGuardianUptime` uses `docker.Run` so it stays in guardian or cmd

### 7.4 Update cmd/guardian.go

- [ ] `runGuardianProxy` becomes: load config, create Server, call `server.Run()`
- [ ] Delete all moved helper functions and types (`guardianState`, `domainApprovalResult`, `stoppable`, `setup*`, etc.)
- [ ] Keep `getGuardianUptime` and `formatDuration` in cmd if they're only used for `guardian status` output

### 7.5 Test Server construction

- [ ] **Test**: `server_test.go` — `NewServer` with default config creates valid server (no panic, components initialized)
- [ ] **Test**: Verify PolicyEngine is wired correctly (check a default-allowed domain)
- [ ] **Test**: `extractPatterns` (if moved) — simple mapping test

---

## Phase 8: End-to-end validation

### 8.1 Automated checks

- [ ] `make build` succeeds
- [ ] `make test` passes (all unit tests)
- [ ] `make lint` passes
- [ ] `make fmt` produces no changes

### 8.2 Integration validation (requires Docker)

- [ ] `make test-integration` passes
- [ ] `make test-e2e` passes
- [ ] Manual: `cloister start` in a project, verify attach works
- [ ] Manual: `cloister stop`, verify token revoked
- [ ] Manual: `cloister guardian status` shows correct info
- [ ] Manual: `cloister shutdown` stops everything cleanly

---

## Future Phases (Deferred)

### Setup wizard extraction
- Move `setup_claude.go` / `setup_codex.go` credential-save logic to `config.SetAgentConfig()`
- Keep interactive prompting in cmd (inherently UI-layer)

### Guardian status extraction
- Move `getGuardianUptime` + `formatDuration` to guardian package if other consumers emerge
- Currently only used by `guardian status` command — not worth extracting yet

### Cmd test cleanup
- `guardian_helpers_test.go` tests config merge patterns, not guardian helpers — rename or move to `config` package
