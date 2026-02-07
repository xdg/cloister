## Phase 7: Approval Persistence via Isolated Directory

Domain approvals clicked in the web UI (Save to Project, Save to Global) currently don't persist to disk because the guardian container has a read-only config mount. Instead of making the config mount writable (which would allow guardian to corrupt static config files), create an isolated `~/.config/cloister/approvals/` directory with read-write mount for approval persistence only.

## Testing Philosophy

- **Unit tests for approval file I/O**: Round-trip tests for LoadGlobalApprovals/WriteGlobalApprovals with t.TempDir()
- **Unit tests for ConfigPersisterImpl**: Verify writes go to approval files, not config files
- **Unit tests for allowlist merging**: Verify static config + approvals merge correctly
- **E2E test for persistence**: Verify actual guardian container persists approvals to disk
- **Manual testing**: Verify static config files remain unchanged after approvals

## Verification Checklist

Before marking a phase complete and committing it:

1. `make test` passes (unit tests, no Docker required)
2. `make lint` passes
3. `make fmt` has been run
4. `make test-e2e` passes (guardian persistence test)
5. Manual testing checklist completed (see plan file)

When verification of a phase or subphase is complete, commit all
relevant newly-created and modified files.

## Dependencies Between Phases

```
7.1 (Approval File I/O) ─── foundation for all other subphases
       │
       ├──► 7.2 (ConfigPersisterImpl Update)
       │          │
       │          ▼
       │    7.3 (Guardian Approval Mount) ─── connects container to approval directory
       │
       └──► 7.4 (Allowlist Loading)
                  │
                  ▼
            7.5 (E2E Test) ─── verifies end-to-end persistence
                  │
                  ▼
            7.6 (Documentation)
```

7.2, 7.3, and 7.4 depend on 7.1.
7.5 requires all prior subphases.

---

## Phase 7.1: Approval File I/O

Create `internal/config/approvals.go` with functions for reading/writing approval YAML files separate from static config files.

### 7.1.1 Approval directory structure

- [ ] **Directory layout**:
  ```
  ~/.config/cloister/approvals/
  ├── global.yaml                   # Global approved domains/patterns
  └── projects/
      └── <project>.yaml            # Project approved domains/patterns
  ```
- [ ] **Precedent**: Similar to existing `~/.config/cloister/tokens/` directory
- [ ] **Benefits**: Guardian can write approvals without access to static config files

### 7.1.2 Config package additions

- [ ] Create `internal/config/approvals.go`
- [ ] Define `Approvals` struct:
  ```go
  type Approvals struct {
      Domains  []string `yaml:"domains,omitempty"`
      Patterns []string `yaml:"patterns,omitempty"`
  }
  ```
- [ ] Implement `ApprovalDir() string`:
  - Check `CLOISTER_APPROVAL_DIR` env var first (container context)
  - Fall back to `ConfigDir() + "approvals"` (host context)
- [ ] Implement `GlobalApprovalPath() string` → `ApprovalDir() + "/global.yaml"`
- [ ] Implement `ProjectApprovalPath(project string) string` → `ApprovalDir() + "/projects/" + project + ".yaml"`
- [ ] Implement `LoadGlobalApprovals() (*Approvals, error)`:
  - Return empty `Approvals{}` if file doesn't exist (not error)
  - Return error if file exists but has invalid YAML
- [ ] Implement `LoadProjectApprovals(project string) (*Approvals, error)`:
  - Return empty `Approvals{}` if file doesn't exist (not error)
  - Return error if file exists but has invalid YAML
- [ ] Implement `WriteGlobalApprovals(approvals *Approvals) error`:
  - Create directory if missing (with 0700 permissions)
  - Write YAML atomically (temp file + rename)
- [ ] Implement `WriteProjectApprovals(project string, approvals *Approvals) error`:
  - Create `approvals/projects/` directory if missing (with 0700 permissions)
  - Write YAML atomically (temp file + rename)
- [ ] **Test**: Round-trip tests with XDG_CONFIG_HOME override to t.TempDir()
- [ ] **Test**: Directory creation (approvals/, approvals/projects/)
- [ ] **Test**: Empty file handling (return empty Approvals{}, not error)
- [ ] **Test**: Invalid YAML handling (return error)

---

## Phase 7.2: Update ConfigPersisterImpl

Modify `internal/guardian/config_persister.go` to write to approval files instead of config files.

### 7.2.1 Change all persistence methods

- [ ] Modify `AddDomainToProject(project, domain string) error`:
  - Load existing approvals via `LoadProjectApprovals(project)`
  - Check for duplicates in `approvals.Domains`
  - Append domain to `approvals.Domains`
  - Write via `WriteProjectApprovals(project, approvals)`
  - Call `ReloadNotifier` if not nil
- [ ] Modify `AddDomainToGlobal(domain string) error`:
  - Load existing approvals via `LoadGlobalApprovals()`
  - Check for duplicates in `approvals.Domains`
  - Append domain to `approvals.Domains`
  - Write via `WriteGlobalApprovals(approvals)`
  - Call `ReloadNotifier` if not nil
- [ ] Modify `AddPatternToProject(project, pattern string) error`:
  - Load existing approvals via `LoadProjectApprovals(project)`
  - Check for duplicates in `approvals.Patterns`
  - Append pattern to `approvals.Patterns`
  - Write via `WriteProjectApprovals(project, approvals)`
  - Call `ReloadNotifier` if not nil
- [ ] Modify `AddPatternToGlobal(pattern string) error`:
  - Load existing approvals via `LoadGlobalApprovals()`
  - Check for duplicates in `approvals.Patterns`
  - Append pattern to `approvals.Patterns`
  - Write via `WriteGlobalApprovals(approvals)`
  - Call `ReloadNotifier` if not nil
- [ ] **Test**: Update `config_persister_test.go` to verify approval files written (not config files)
- [ ] **Test**: Verify static config files remain unchanged after persistence operations

---

## Phase 7.3: Add Guardian Approval Directory Mount

Add a read-write mount for `~/.config/cloister/approvals` to the guardian container, keeping the main config directory read-only.

### 7.3.1 Container mount modifications

- [ ] Add to `internal/guardian/container.go` constants:
  ```go
  // ContainerApprovalDir is the path inside the guardian container where approvals are mounted.
  ContainerApprovalDir = "/var/lib/cloister/approvals"
  ```
- [ ] Add helper function `HostApprovalDir() (string, error)`:
  - Use existing `hostCloisterPath("approvals")` helper
  - Returns `~/.config/cloister/approvals`
- [ ] Modify `StartWithOptions()` to add approval mount (after token mount at line 251):
  - Get `hostApprovalDir` via `HostApprovalDir()`
  - Create directory with `os.MkdirAll(hostApprovalDir, 0700)` if missing
  - Add to args: `"-v", hostApprovalDir + ":/var/lib/cloister/approvals"`
  - Add to args: `"-e", "CLOISTER_APPROVAL_DIR=/var/lib/cloister/approvals"`
- [ ] **Result**: Guardian can read config.yaml (RO), write approvals (RW), but cannot write config.yaml
- [ ] **Test**: Integration test verifies mount permissions (config RO, approvals RW)

---

## Phase 7.4: Update Allowlist Loading

Merge approval files with static config when loading allowlists in the guardian.

### 7.4.1 Guardian startup modifications

- [ ] Modify `internal/cmd/guardian.go` in `runGuardianProxy()`:
  - After loading global config, load global approvals via `config.LoadGlobalApprovals()`
  - Create helper `approvalsToAllowEntries(approvals *config.Approvals) []config.AllowEntry`
  - Merge static allowlist + approval entries: `append(cfg.Proxy.Allow, approvalsToAllowEntries(globalApprovals)...)`
  - Update log message: "loaded global allowlist: %d static + %d approved = %d total"
- [ ] Modify `loadProjectAllowlist` function:
  - After loading project config, load project approvals via `config.LoadProjectApprovals(projectName)`
  - Merge: global config + project config + global approvals + project approvals
  - Use `config.MergeAllowlists()` with all four sources
  - Update log message to show total domain count
- [ ] Implement helper function:
  ```go
  func approvalsToAllowEntries(approvals *config.Approvals) []config.AllowEntry {
      entries := make([]config.AllowEntry, 0, len(approvals.Domains)+len(approvals.Patterns))
      for _, d := range approvals.Domains {
          entries = append(entries, config.AllowEntry{Domain: d})
      }
      for _, p := range approvals.Patterns {
          entries = append(entries, config.AllowEntry{Pattern: p})
      }
      return entries
  }
  ```
- [ ] **Reload behavior**: When `ReloadNotifier` is called (approval written), allowlist cache is cleared and reloaded with new approvals
- [ ] **Test**: Unit test for `approvalsToAllowEntries` helper
- [ ] **Test**: Verify allowlist merging with various combinations of static config and approvals

---

## Phase 7.5: Add E2E Test for Guardian Persistence

Create end-to-end test that verifies the guardian container can persist approvals to disk.

### 7.5.1 E2E test implementation

- [ ] Create `test/e2e/domain_approval_persistence_test.go` with build tag `//go:build e2e`
- [ ] Test flow:
  1. Start guardian via `guardian.EnsureRunning()`
  2. Submit domain approval request via approval server API
  3. Approve with project scope via `POST /approve-domain/{id}`
  4. Verify approval file written to `~/.config/cloister/approvals/projects/<project>.yaml`
  5. Verify static config `~/.config/cloister/projects/<project>/config.yaml` unchanged
  6. Verify subsequent requests to same domain don't require re-approval
  7. Repeat for global scope → verify `~/.config/cloister/approvals/global.yaml`
  8. Verify static config `~/.config/cloister/config.yaml` unchanged
- [ ] **Requirements**: Requires Docker, uses TestMain guardian instance
- [ ] **Test**: Run `make test-e2e` to verify guardian container persistence

---

## Phase 7.6: Update Documentation

Document the approval directory architecture and mount structure.

### 7.6.1 Specification updates

- [ ] Update `specs/cloister-spec.md`:
  - Document guardian mounts three directories from ~/.config/cloister
  - Config files (config.yaml, projects/*/config.yaml) are read-only
  - Tokens directory is read-write for token persistence
  - Approvals directory is read-write for domain approval persistence
  - Explain separation prevents guardian from corrupting static configuration
  - Add diagram showing directory structure
- [ ] **Review**: Verify documentation accurately reflects implementation

---

## Future Phases (Deferred)

### Phase 5: Worktree Support (Skipped)
- `cloister start -b <branch>` creates managed worktrees
- Worktree naming: `<project>-<branch>`
- Worktree cleanup protection
- CLI: `worktree list/remove`, `cloister path <name>`

### Phase 8: Polish
- Image distribution and auto-pull
- Custom image configuration
- Multi-arch container images
- Shell completion
- Read-only reference mounts
- Audit logging improvements
- Detached mode, non-git support
- Guardian API versioning
- Structured logging
