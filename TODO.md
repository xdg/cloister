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

- [x] **Directory layout**:
  ```
  ~/.config/cloister/approvals/
  ├── global.yaml                   # Global approved domains/patterns
  └── projects/
      └── <project>.yaml            # Project approved domains/patterns
  ```
- [x] **Precedent**: Similar to existing `~/.config/cloister/tokens/` directory
- [x] **Benefits**: Guardian can write approvals without access to static config files

### 7.1.2 Config package additions

- [x] Create `internal/config/approvals.go`
- [x] Define `Approvals` struct:
  ```go
  type Approvals struct {
      Domains  []string `yaml:"domains,omitempty"`
      Patterns []string `yaml:"patterns,omitempty"`
  }
  ```
- [x] Implement `ApprovalDir() string`:
  - Check `CLOISTER_APPROVAL_DIR` env var first (container context)
  - Fall back to `ConfigDir() + "approvals"` (host context)
- [x] Implement `GlobalApprovalPath() string` → `ApprovalDir() + "/global.yaml"`
- [x] Implement `ProjectApprovalPath(project string) string` → `ApprovalDir() + "/projects/" + project + ".yaml"`
- [x] Implement `LoadGlobalApprovals() (*Approvals, error)`:
  - Return empty `Approvals{}` if file doesn't exist (not error)
  - Return error if file exists but has invalid YAML
- [x] Implement `LoadProjectApprovals(project string) (*Approvals, error)`:
  - Return empty `Approvals{}` if file doesn't exist (not error)
  - Return error if file exists but has invalid YAML
- [x] Implement `WriteGlobalApprovals(approvals *Approvals) error`:
  - Create directory if missing (with 0700 permissions)
  - Write YAML atomically (temp file + rename)
- [x] Implement `WriteProjectApprovals(project string, approvals *Approvals) error`:
  - Create `approvals/projects/` directory if missing (with 0700 permissions)
  - Write YAML atomically (temp file + rename)
- [x] **Test**: Round-trip tests with XDG_CONFIG_HOME override to t.TempDir()
- [x] **Test**: Directory creation (approvals/, approvals/projects/)
- [x] **Test**: Empty file handling (return empty Approvals{}, not error)
- [x] **Test**: Invalid YAML handling (return error)

---

## Phase 7.2: Update ConfigPersisterImpl

Modify `internal/guardian/config_persister.go` to write to approval files instead of config files.

### 7.2.1 Change all persistence methods

- [x] Modify `AddDomainToProject(project, domain string) error`:
  - Load existing approvals via `LoadProjectApprovals(project)`
  - Check for duplicates in `approvals.Domains`
  - Append domain to `approvals.Domains`
  - Write via `WriteProjectApprovals(project, approvals)`
  - Call `ReloadNotifier` if not nil
- [x] Modify `AddDomainToGlobal(domain string) error`:
  - Load existing approvals via `LoadGlobalApprovals()`
  - Check for duplicates in `approvals.Domains`
  - Append domain to `approvals.Domains`
  - Write via `WriteGlobalApprovals(approvals)`
  - Call `ReloadNotifier` if not nil
- [x] Modify `AddPatternToProject(project, pattern string) error`:
  - Load existing approvals via `LoadProjectApprovals(project)`
  - Check for duplicates in `approvals.Patterns`
  - Append pattern to `approvals.Patterns`
  - Write via `WriteProjectApprovals(project, approvals)`
  - Call `ReloadNotifier` if not nil
- [x] Modify `AddPatternToGlobal(pattern string) error`:
  - Load existing approvals via `LoadGlobalApprovals()`
  - Check for duplicates in `approvals.Patterns`
  - Append pattern to `approvals.Patterns`
  - Write via `WriteGlobalApprovals(approvals)`
  - Call `ReloadNotifier` if not nil
- [x] **Test**: Update `config_persister_test.go` to verify approval files written (not config files)
- [x] **Test**: Verify static config files remain unchanged after persistence operations

---

## Phase 7.3: Add Guardian Approval Directory Mount

Add a read-write mount for `~/.config/cloister/approvals` to the guardian container, keeping the main config directory read-only.

### 7.3.1 Container mount modifications

- [x] Add to `internal/guardian/container.go` constants:
  ```go
  // ContainerApprovalDir is the path inside the guardian container where approvals are mounted.
  ContainerApprovalDir = "/var/lib/cloister/approvals"
  ```
- [x] Add helper function `HostApprovalDir() (string, error)`:
  - Use existing `hostCloisterPath("approvals")` helper
  - Returns `~/.config/cloister/approvals`
- [x] Modify `StartWithOptions()` to add approval mount (after token mount at line 251):
  - Get `hostApprovalDir` via `HostApprovalDir()`
  - Create directory with `os.MkdirAll(hostApprovalDir, 0700)` if missing
  - Add to args: `"-v", hostApprovalDir + ":/var/lib/cloister/approvals"`
  - Add to args: `"-e", "CLOISTER_APPROVAL_DIR=/var/lib/cloister/approvals"`
- [x] **Result**: Guardian can read config.yaml (RO), write approvals (RW), but cannot write config.yaml
- [x] **Test**: Integration test verifies mount permissions (config RO, approvals RW) — covered by E2E test in 7.5

---

## Phase 7.4: Update Allowlist Loading

Merge approval files with static config when loading allowlists in the guardian.

### 7.4.1 Guardian startup modifications

- [x] Modify `internal/cmd/guardian.go` in `runGuardianProxy()`:
  - After loading global config, load global approvals via `config.LoadGlobalApprovals()`
  - Create helper `approvalsToAllowEntries(approvals *config.Approvals) []config.AllowEntry`
  - Merge static allowlist + approval entries: `append(cfg.Proxy.Allow, approvalsToAllowEntries(globalApprovals)...)`
  - Update log message: "loaded global allowlist: %d static + %d approved = %d total"
- [x] Modify `loadProjectAllowlist` function:
  - After loading project config, load project approvals via `config.LoadProjectApprovals(projectName)`
  - Merge: global config + project config + global approvals + project approvals
  - Use `config.MergeAllowlists()` with all four sources
  - Update log message to show total domain count
- [x] Implement helper function:
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
- [x] **Reload behavior**: When `ReloadNotifier` is called (approval written), allowlist cache is cleared and reloaded with new approvals
- [x] **Test**: Unit test for `approvalsToAllowEntries` helper
- [x] **Test**: Verify allowlist merging with various combinations of static config and approvals

---

## Phase 7.5: Add E2E Test for Guardian Persistence

Create end-to-end test that verifies the guardian container can persist approvals to disk.

### 7.5.1 E2E test implementation

- [x] Create `test/e2e/domain_approval_persistence_test.go` with build tag `//go:build e2e`
- [x] Test flow:
  1. Start guardian via `guardian.EnsureRunning()`
  2. Submit domain approval request via approval server API
  3. Approve with project scope via `POST /approve-domain/{id}`
  4. Verify approval file written to `~/.config/cloister/approvals/projects/<project>.yaml`
  5. Verify static config `~/.config/cloister/projects/<project>/config.yaml` unchanged
  6. Verify subsequent requests to same domain don't require re-approval
  7. Repeat for global scope → verify `~/.config/cloister/approvals/global.yaml`
  8. Verify static config `~/.config/cloister/config.yaml` unchanged
- [x] **Requirements**: Requires Docker, uses TestMain guardian instance
- [x] **Test**: Run `make test-e2e` to verify guardian container persistence

---

## Phase 7.6: Update Documentation

The new `approvals/` directory changes the config directory structure. Multiple docs reference the old structure where approvals wrote directly to `config.yaml` and `projects/<name>.yaml`. All references need updating, and the rationale for the split needs documenting.

### Why approvals are separate from static config

The guardian container handles potentially compromised AI-generated proxy requests. Giving it write access to `config.yaml` would let a compromised guardian corrupt the entire config: allowlists, hostexec patterns, agent credentials. The `approvals/` directory scopes the guardian's write access to just approval data.

This also means static config is human-authored and machine-readable, while approval files are machine-authored. Users may wish to periodically review accumulated approvals and consolidate them into static config (moving entries from `approvals/global.yaml` into `config.yaml`, or from `approvals/projects/<name>.yaml` into `projects/<name>.yaml`), then clear the approval files. This is optional — both sources are merged at load time.

### 7.6.1 specs/cloister-spec.md

- [ ] Update "Domain approval flow" section (lines ~228-238):
  - "Save to project" persists to `~/.config/cloister/approvals/projects/<name>.yaml` (not `projects/<name>.yaml`)
  - "Save to global" persists to `~/.config/cloister/approvals/global.yaml` (not `config.yaml`)
- [ ] Update "File Structure" diagram (lines ~266-294):
  - Add `approvals/` directory with `global.yaml` and `projects/` subdirectory
  - Add comment distinguishing static config (human-authored, RO mount) from approvals (machine-authored, RW mount)
- [ ] Update "Configuration" section (lines ~308-312):
  - Mention approvals directory as third config source
  - Explain merge order: global config + project config + global approvals + project approvals

### 7.6.2 specs/guardian-api.md

- [ ] Update `POST /approve-domain/{id}` scope options (lines ~362-367):
  - `"project"` saves to `~/.config/cloister/approvals/projects/<name>.yaml` (not `projects/<name>.yaml`)
  - `"global"` saves to `~/.config/cloister/approvals/global.yaml` (not `config.yaml`)

### 7.6.3 docs/configuration.md

- [ ] Add `approvals/` directory to "Configuration File Locations" table
- [ ] Add new section "Approved Domains" explaining:
  - Domains approved via the web UI are stored separately from static config
  - Why: guardian write access is scoped to approval files only (security)
  - Merge behavior: static config + approval files are combined at load time
  - How to consolidate: move entries from approval files into static config, then delete the approval file
- [ ] Add example showing the approval file format

### 7.6.4 README.md

- [ ] Update "Configuration" section (lines ~142-145):
  - Add `approvals/` directory as third bullet
  - Brief explanation: "Web UI domain approvals (persisted separately from static config)"

### 7.6.5 CLAUDE.md

- [ ] Update `internal/config` package description:
  - Mention approval file I/O alongside config parsing
- [ ] Update `internal/guardian` package description:
  - Mention per-project allowlist caching now includes approval files

### 7.6.6 specs/implementation-phases.md

- [ ] Update Phase 6 & 7 description to mention the approval persistence split
- [ ] Update Phase 7 verification bullets:
  - "Save to project" and "Save to global" persist to approval files, not static config files

### 7.6.7 docs/troubleshooting.md

- [ ] Update "Domain not in allowlist" section (lines ~96-118):
  - Existing guidance (manually adding to config files) is still valid for static config
  - Add note: domains approved via web UI are stored in `~/.config/cloister/approvals/` and are merged automatically

### 7.6.8 Review remaining files (no changes expected)

These files reference `~/.config/cloister/` but only in contexts unaffected by the approval split (agent credentials, comparison docs, operational details). Verify no stale references:

- [ ] `specs/agent-configuration.md` — only references `config.yaml` for agent credentials (correct, no change)
- [ ] `specs/comparison-leash.md` — config hierarchy mention (verify still accurate)
- [ ] `specs/comparison-claude-sandbox.md` — config location mention (verify still accurate)
- [ ] `docs/credentials.md` — credential storage in `config.yaml` (correct, no change)
- [ ] `docs/getting-started.md` — no config structure references
- [ ] `docs/command-reference.md` — no config structure references
- [ ] `docs/working-with-cloisters.md` — no config structure references
- [ ] `docs/host-commands.md` — config references are about hostexec patterns (correct, no change)
- [ ] `specs/container-image.md` — no config structure references
- [ ] `specs/devcontainer-integration.md` — no config structure references
- [ ] `specs/brand-guidelines.md` — no config structure references
- [ ] `specs/cli-workflows.md` — no config structure references
- [ ] `specs/config-reference.md` — documents static config schema (correct, but consider adding approval file schema)

### 7.6.9 Optional: Update specs/config-reference.md

- [ ] Add "Approval File Schema" section documenting the `Approvals` struct format
- [ ] Explain relationship between approval files and static config (merge behavior, consolidation)

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
