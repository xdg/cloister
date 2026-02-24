# Phase 5: Worktree Support Implementation Plan

Adds `cloister start -b <branch>` to create managed git worktrees with
dedicated cloister containers. Includes a cloister registry for authoritative
name resolution, worktree lifecycle management, and new CLI commands
(`worktree list`, `worktree remove`, `path`).

## Testing Philosophy

- **Unit tests for all new packages and functions**: Registry persistence,
  worktree git operations, name resolution, path helpers
- **Use `t.TempDir()` and `testutil.IsolateXDGDirs`** to avoid touching real
  user config during tests
- **Git operations under test**: Create real (local) git repos in temp dirs;
  do NOT mock git. These are fast and give real guarantees.
- **Container/guardian operations**: Mock via existing interfaces
  (`ContainerManager`, `GuardianManager`, `ConfigLoader`) for unit tests.
  Integration tests (Docker-required) are out of scope unless container
  creation logic materially changes.
- **Go tests**: Use `testing` package with table-driven tests where appropriate
- **CLI tests**: Use cobra's `Execute()` with captured output for command tests

## Verification Checklist

Before marking a phase complete and committing:

1. `make fmt` passes
2. `make lint` passes
3. `make test` passes (unit tests, no Docker required)
4. No `//nolint` directives added
5. No changes to `.golangci.yml`

When verification of a subphase is complete, commit all relevant
newly-created and modified files.

## Dependencies Between Phases

```
Phase 1 (Test fixtures & helpers)
       |
       v
Phase 2 (Cloister registry)
       |
       v
Phase 3 (Worktree storage & git operations)
       |
       v
Phase 4 (Orchestration: start -b, naming, stop)
       |
       +---> Phase 5 (CLI: start -b, path)
       |
       +---> Phase 6 (CLI: worktree list, worktree remove)
                    |
                    v
              Phase 7 (Polish & edge cases)
```

Phases 5 and 6 can proceed in parallel after Phase 4.

---

## Phase 1: Test Fixtures and Helpers

Build test infrastructure for worktree-related operations. All subsequent
phases depend on this. The goal is to make it trivial to create test git
repos with branches, worktrees, and known state.

### 1.1 Git test helpers in `internal/testutil/`

- [x] Add `CreateTestRepo(t *testing.T) string` — creates a bare-minimum git
      repo in `t.TempDir()` with an initial commit. Returns the repo path.
      (Needs: `git init`, create a file, `git add`, `git commit`.)
- [x] Add `CreateTestBranch(t *testing.T, repoPath, branchName string)` —
      creates a branch in the test repo (does NOT check it out). Useful for
      testing "branch already exists" vs "branch needs creation" paths.
- [x] Add `CreateTestWorktree(t *testing.T, repoPath, worktreePath, branch string)` —
      runs `git worktree add` to create a real worktree. Returns the worktree
      path. Useful for verifying worktree detection and cleanup.
- [x] Add `DirtyWorktree(t *testing.T, worktreePath string)` — creates an
      uncommitted file in the worktree (for testing dirty-check refusal).
- [x] Add `CommitFile(t *testing.T, repoPath, filename, content string)` —
      commits a file to the repo. Useful for advancing history.
- [x] **Test**: Verify each helper works in isolation: create repo, create
      branch, create worktree from that branch, dirty it, confirm
      `git status --porcelain` shows changes.

---

## Phase 2: Cloister Registry

A new persistent registry mapping cloister names to their metadata. This is
the authoritative source for resolving ambiguous cloister names (since
`ParseCloisterName` cannot distinguish `my-app` (project) from `my` + branch
`app`).

### 2.1 Registry types and persistence (`internal/cloister/registry.go`)

- [x] Define `RegistryEntry` struct: `CloisterName`, `ProjectName`, `Branch`
      (empty for main checkout), `HostPath` (absolute path on host),
      `IsWorktree` (bool), `CreatedAt` (time.Time).
- [x] Define `Registry` struct wrapping `[]RegistryEntry` with a `Clock`
      interface (same pattern as `project.Registry`).
- [x] Define `RegistryPath()` — returns path under `config.Dir()` (e.g.
      `~/.config/cloister/cloisters.yaml`). Respects `XDG_CONFIG_HOME` via
      existing `config.Dir()`.
- [x] Implement `LoadRegistry() (*Registry, error)` — reads from disk, returns
      empty registry if file absent.
- [x] Implement `SaveRegistry(r *Registry) error` — writes YAML with 0600
      permissions.
- [x] Implement `Register(entry RegistryEntry) error` — upserts by cloister
      name. Error if name collision with different project.
- [x] Implement `FindByName(name string) *RegistryEntry`.
- [x] Implement `FindByProject(projectName string) []RegistryEntry` — returns
      all cloisters (main + worktrees) for a project.
- [x] Implement `Remove(cloisterName string) error`.
- [x] Implement `List() []RegistryEntry`.
- [x] **Test**: Round-trip: register entries, save, load, verify all fields
      preserved. Test upsert behavior. Test FindByProject returns correct
      subset. Test Remove.

### 2.2 Registry integration with Start/Stop

- [x] In `cloister.Start()`: after creating the cloister, register it in the
      cloister registry. Include project name, branch, host path, and whether
      it's a worktree.
- [x] In `cloister.Stop()`: remove the entry from the cloister registry.
- [x] Add registry dependency injection via a new `Option` (e.g.
      `WithCloisterRegistry`) to allow testing without touching disk.
- [x] Backfill: existing `cmd/start.go` flow (main checkout) must register
      in the cloister registry. Branch is the detected branch, `IsWorktree`
      is false, `HostPath` is the git root.
- [x] **Test**: Mock-based unit test: `Start` registers entry, `Stop` removes
      it. Verify correct fields are passed.
- [x] **Test**: Verify that existing main-checkout start flow still works
      (no regression).

---

## Phase 3: Worktree Storage and Git Operations

Manage the on-disk worktree directory and git worktree create/remove
operations.

### 3.1 Worktree directory helpers

- [x] Add `WorktreeBaseDir() (string, error)` — returns
      `$XDG_DATA_HOME/cloister/worktrees/` (defaulting to
      `~/.local/share/cloister/worktrees/` per XDG spec). Create the dir
      if it doesn't exist.
- [x] Add `WorktreeDir(projectName, branch string) (string, error)` — returns
      `WorktreeBaseDir()/<project>/<branch>/`. Does NOT create it (git worktree
      add does that).
- [x] Decide on package location: likely `internal/worktree/` as a new package,
      since this is a new domain with its own git operations and state.
- [x] **Test**: Verify paths are constructed correctly. Verify XDG_DATA_HOME
      override works (use `t.Setenv`).

### 3.2 Git worktree operations

- [x] Implement `Create(repoRoot, worktreePath, branch string) error` —
      wraps `git worktree add <path> <branch>`. If branch doesn't exist,
      creates it first (from HEAD). Should detect whether branch exists and
      handle both cases.
- [x] Implement `Remove(worktreePath string, force bool) error` — wraps
      `git worktree remove <path>`. If `!force`, check for uncommitted changes
      first and return a descriptive error.
- [x] Implement `IsDirty(worktreePath string) (bool, error)` — runs
      `git -C <path> status --porcelain` and checks for non-empty output.
- [x] Implement `IsWorktree(path string) bool` — detects whether a path is
      a git worktree (vs main checkout). Uses `git -C <path> rev-parse
      --git-common-dir` vs `--git-dir` comparison.
- [x] **Test** (using Phase 1 helpers): Create a test repo, create a worktree
      via `Create`, verify the directory exists and is on the correct branch.
      Dirty the worktree, verify `IsDirty` returns true. Verify `Remove`
      refuses without force. Verify `Remove` with force succeeds. Verify
      `IsWorktree` correctly identifies worktrees vs main checkouts.

### 3.3 Branch resolution

- [x] Implement `ResolveBranch(repoRoot, branch string) (existed bool, err error)` —
      checks if branch exists locally. If not, checks if a tracking remote
      exists. If neither, creates from HEAD. Returns whether the branch
      already existed (for user messaging).
- [x] **Test**: Branch exists locally -> returns (true, nil). Branch doesn't
      exist, no remote -> creates from HEAD, returns (false, nil). Verify
      branch was actually created.

---

## Phase 4: Orchestration Layer Updates

Wire worktree creation into the cloister start/stop lifecycle.

### 4.1 Update `cloister.StartOptions`

- [x] Add `IsWorktree bool` field to `StartOptions` — indicates this is a
      managed worktree (vs main checkout).
- [x] When `IsWorktree` is true, `Start()` should use
      `container.GenerateWorktreeCloisterName(project, branch)` instead of
      `GenerateCloisterName(project)`.
- [x] Update `container.Config.ContainerName()` to use branch when present:
      if `Branch != ""`, use `GenerateWorktreeCloisterName`; otherwise
      `GenerateCloisterName`. Remove the "Phase 1" comments.
- [x] Update `registerCloisterToken` to pass the worktree path (not the main
      project path) when starting a worktree cloister.
- [x] **Test**: Mock-based: `Start` with `IsWorktree=true` creates a container
      named `cloister-<project>-<branch>`. Token registered with worktree path.

### 4.2 Update `cloister.DetectName()`

- [x] Currently only returns main-checkout cloister name. Update to check the
      cloister registry first: look up by current working directory path to
      find the matching cloister entry (could be main or worktree).
- [x] Fallback: if not in registry, use the existing project-name-based
      detection (backward compatible).
- [x] **Test**: When cwd is inside a registered worktree path, returns the
      worktree cloister name. When cwd is inside main checkout, returns
      main cloister name. When not registered, falls back to project detection.

### 4.3 Worktree start orchestration

- [x] Create a new function (e.g. `StartWorktree(opts StartOptions) (...)`)
      or extend `Start` to handle the worktree creation flow:
      1. Resolve the project from current directory (same as main checkout)
      2. Call `worktree.ResolveBranch` to ensure the branch exists
      3. Call `worktree.WorktreeDir` to get the target path
      4. Call `worktree.Create` to create the git worktree
      5. Set `opts.ProjectPath` to the worktree path, `opts.IsWorktree = true`
      6. Call existing `Start` (reuse all token/guardian/agent setup)
      7. Register in cloister registry with `IsWorktree: true`
- [x] Handle the "already exists" case: if the worktree dir already exists and
      the cloister container exists, attach to it (same as main checkout
      re-enter behavior).
- [x] **Test**: Mock-based: full flow creates worktree, starts container with
      correct name and path, registers in cloister registry.

### 4.4 Worktree stop and cleanup

- [x] When stopping a worktree cloister, the worktree directory is NOT removed
      (user might want to inspect it). Only the container and token are cleaned
      up, same as main checkout.
- [x] Worktree directory removal is explicit via `worktree remove` command.
- [x] Verify `Stop` works correctly with worktree cloister names (the existing
      flow should work since it's keyed by container name, but verify).
- [x] **Test**: Stop a worktree cloister, verify container stopped, token
      revoked, registry entry removed. Verify worktree directory still exists.

---

## Phase 5: CLI — `start -b` and `path` Commands

### 5.1 Add `-b` flag to `start` command

- [x] Add `--branch` / `-b` flag to `cmd/start.go`.
- [x] When `-b` is provided: detect project from cwd, then delegate to
      worktree start orchestration (Phase 4.3) instead of the main checkout
      flow.
- [x] Print appropriate messages: "Creating worktree: <path>",
      "Starting cloister: <project>-<branch>".
- [x] Handle re-entry: if worktree cloister already exists, attach to it.
- [x] **Test**: Cobra command test with `-b feature-x` flag — verify the
      correct orchestration path is taken (mock the orchestration layer).

### 5.2 Add `path` command

- [x] Add `cloister path <name>` command that prints the host path for a
      cloister.
- [x] Look up `<name>` in the cloister registry. Print the `HostPath` field.
- [x] If no argument, print path for the cloister detected from cwd.
- [x] Error if cloister not found in registry.
- [x] Output just the path (no decoration) so it's usable in
      `cd $(cloister path ...)`.
- [x] **Test**: Register a cloister, run `path <name>`, verify output matches
      registered path.

---

## Phase 6: CLI — `worktree` Subcommands

### 6.1 Add `worktree list` command

- [x] Add `cloister worktree list` subcommand.
- [x] Detect project from cwd (or accept `-p <project>` flag).
- [x] Query cloister registry for all entries matching the project.
- [x] Display table: WORKTREE (branch or "(main)"), PATH, CLOISTER (name +
      running status).
- [x] Running status: check container existence via `ContainerManager`.
- [x] **Test**: Register multiple cloisters for a project (main + 2 worktrees),
      verify `worktree list` output shows all three with correct columns.

### 6.2 Add `worktree remove` command

- [ ] Add `cloister worktree remove <branch>` subcommand.
- [ ] Detect project from cwd (or accept `-p <project>` flag).
- [ ] Resolve the cloister name from project + branch.
- [ ] Check `worktree.IsDirty`: if dirty and no `-f` flag, error with
      "Worktree has uncommitted changes. Commit, stash, or use -f to force."
- [ ] If cloister container is running, stop it first (with user confirmation
      unless `-f`).
- [ ] Call `worktree.Remove` to remove the git worktree.
- [ ] Remove the cloister registry entry.
- [ ] Clean up the worktree directory under `WorktreeBaseDir` if empty.
- [ ] **Test**: Remove a clean worktree — succeeds. Remove a dirty worktree
      without `-f` — fails with correct error. Remove with `-f` — succeeds.
      Verify container stopped, registry entry removed, git worktree removed.

---

## Phase 7: Polish and Edge Cases

### 7.1 `list` command update

- [ ] Update `cmd/list.go` to use the cloister registry for project/branch
      resolution instead of `ParseCloisterName`. This eliminates the ambiguity
      problem with hyphenated project names.
- [ ] Fallback: if a running container isn't in the cloister registry (e.g.
      created before Phase 5), still show it using `ParseCloisterName`.
- [ ] **Test**: List with a hyphenated project name (e.g. `my-api`) correctly
      shows project="my-api" branch="" instead of project="my" branch="api".

### 7.2 `stop` command update

- [ ] Update `cmd/stop.go` to accept worktree cloister names (should already
      work since `stop` takes a cloister name, but verify).
- [ ] When stopping from within a worktree directory, `DetectName` should
      return the correct worktree cloister name (Phase 4.2).
- [ ] **Test**: `cloister stop` from within a worktree directory stops the
      correct worktree cloister (not the main checkout).

### 7.3 `project show` update

- [ ] Update `cmd/project.go` `project show` to list managed worktrees.
      Query the cloister registry for all entries with the project name and
      display them.
- [ ] **Test**: `project show my-api` lists main checkout and worktrees with
      paths.

### 7.4 Edge cases

- [ ] Branch names with slashes (e.g. `feature/auth`): verify `SanitizeName`
      handles them correctly in cloister names and worktree directory names.
      The git worktree path must use the sanitized name, not the raw branch
      name (slashes would create subdirectories).
- [ ] Long branch names: verify truncation in `SanitizeName` doesn't collide
      with other cloister names.
- [ ] Starting a worktree cloister when the main checkout cloister is not
      running: should work (worktrees are independent containers).
- [ ] Starting a worktree cloister when no main checkout cloister has ever
      been created: should work (auto-register project as part of the flow).
- [ ] `worktree remove` when the worktree was created by `git worktree add`
      manually (not by cloister): should refuse with a clear error ("not a
      cloister-managed worktree").
- [ ] **Test**: Branch name `feature/auth` produces cloister name
      `my-api-feature-auth` and worktree dir uses sanitized name.
- [ ] **Test**: Start worktree cloister without prior main checkout — project
      gets auto-registered, worktree created, cloister starts.

---

## Future (Deferred)

### Shell completion for worktree names
- Tab completion for `worktree remove <TAB>` showing managed worktree branches
- Tab completion for `path <TAB>` showing all registered cloister names
- Tab completion for `stop <TAB>` updated to use cloister registry

### Worktree garbage collection
- `cloister gc` to clean up orphaned worktrees (worktree dir exists but
  no cloister registry entry) and stale registry entries (entry exists but
  worktree dir missing)

### Worktree-specific configuration
- Per-worktree config overrides (beyond project config inheritance)
- Different agent or image per worktree
