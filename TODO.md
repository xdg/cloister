# Cloister Phase 2: Configuration System

Global and per-project config controls allowlists, project registry, and configurable settings. Transforms Phase 1's hardcoded behavior into a flexible configuration-driven system.

## Testing Philosophy

- **Unit tests for core logic**: Config parsing, merging, validation, allowlist expansion
- **Integration tests for config loading**: File discovery, environment variable expansion, YAML parsing
- **Manual tests for CLI workflows**: Config editing, project registration, allowlist behavior
- **Go tests**: Use `testing` package; `os.MkdirTemp` for isolated config directories
- **Table-driven tests**: Config merge precedence, allowlist domain matching with wildcards

## Reference Documentation

Before implementing, read these spec files in `./docs`:

- **[config-reference.md](docs/config-reference.md)** — Full YAML schemas for global and project configs (primary reference for 2.1-2.5)
- **[cloister-spec.md](docs/cloister-spec.md)** — Architecture, file layout (`~/.config/cloister/` structure), security model
- **[cli-workflows.md](docs/cli-workflows.md)** — CLI command examples and expected behaviors (reference for 2.7)

Provied relevant excerpts to subagents referenced in this doc if the excerpt is small, or tell subagents to read an entire doc.  Avoid using too much of your own context citing specs to subagents.

## Verification Checklist

Before marking Phase 2 complete:

1. `make test` passes
2. `make build` produces working binary
3. `make lint` passes
4. Manual verification of all "Verification" items from spec:
   - [ ] Add domain to project config; cloister for that project allows it
   - [ ] Same domain blocked for different project without that config
   - [ ] `project list` shows registered projects
   - [ ] Config edit opens in `$EDITOR`
   - [ ] Guardian restart preserves token associations (already done in Phase 1)
5. No race conditions (`make test-race`)

## Dependencies Between Phases

```
2.1 Config Schema & Parsing
       │
       ▼
2.2 Global Config Loading
       │
       ├─► 2.3 Project Detection & Registry (parallel)
       │              │
       │              ▼
       │         2.4 Per-Project Config
       │              │
       └──────────────┤
                      ▼
              2.5 Config Merging
                      │
                      ▼
              2.6 Guardian Integration
                      │
                      ▼
              2.7 CLI Commands
                      │
                      ▼
              2.8 Integration & Polish
```

---

## Phase 2.1: Config Schema & Parsing

Define Go types for configuration and implement YAML parsing.

### 2.1.1 Config types
- [x] Create `internal/config/types.go`
- [x] Define `GlobalConfig` struct matching `config-reference.md` schema
- [x] Define `ProjectConfig` struct for per-project settings
- [x] Define `ProxyConfig`, `AllowEntry`, `CommandPattern` sub-structs
- [x] Use struct tags for YAML field mapping
- [x] **Test**: Struct tags correctly map to YAML field names

### 2.1.2 Config parsing
- [x] Create `internal/config/parse.go`
- [x] Implement `ParseGlobalConfig([]byte) (*GlobalConfig, error)`
- [x] Implement `ParseProjectConfig([]byte) (*ProjectConfig, error)`
- [x] Handle missing optional fields with sensible defaults
- [x] Return clear errors for invalid YAML or unknown fields
- [x] **Test**: Parse valid config, parse config with defaults, parse invalid config

### 2.1.3 Config validation
- [x] Create `internal/config/validate.go`
- [x] Validate required fields present
- [x] Validate port numbers in valid range
- [x] Validate duration strings parseable
- [x] Validate regex patterns in command patterns compile
- [x] **Test**: Valid config passes, various invalid configs rejected with clear messages

---

## Phase 2.2: Global Config Loading

Load global configuration from `~/.config/cloister/config.yaml`.

### 2.2.1 Config directory management
- [x] Create `internal/config/paths.go`
- [x] Implement `ConfigDir() string` returning `~/.config/cloister/`
- [x] Implement `EnsureConfigDir() error` creating directory if missing
- [x] Handle `XDG_CONFIG_HOME` override
- [x] **Test**: Path resolution with/without XDG override

### 2.2.2 Default config generation
- [x] Create `internal/config/defaults.go`
- [x] Define `DefaultGlobalConfig()` with full default allowlist from spec
- [x] Include all documentation sites, package registries, AI APIs
- [x] Set sensible defaults for timeouts, rate limits, etc.
- [x] **Test**: Default config is valid, contains expected domains

### 2.2.3 Config file loading
- [x] Create `internal/config/load.go`
- [x] Implement `LoadGlobalConfig() (*GlobalConfig, error)`
- [x] Return default config if file doesn't exist
- [x] Expand `~` in paths to actual home directory
- [x] Log config file path used for debugging
- [x] **Test**: Load existing config, load missing config (use defaults), handle corrupt file

### 2.2.4 Config file creation
- [x] Implement `WriteDefaultConfig() error` to create initial config.yaml
- [x] Write commented YAML with documentation
- [x] Only create if file doesn't exist (don't overwrite)
- [x] **Test**: Creates file with expected content, doesn't overwrite existing

---

## Phase 2.3: Project Detection & Registry

Identify projects and maintain a registry mapping names to locations.

### 2.3.1 Git repository detection
- [x] Create `internal/project/detect.go`
- [x] Implement `DetectProject(path string) (*ProjectInfo, error)`
- [x] Walk up directory tree to find `.git`
- [x] Extract remote URL from git config (`git remote get-url origin`)
- [x] Derive project name from directory basename (already done in Phase 1, consolidate here)
- [x] Handle detached HEAD, bare repos gracefully
- [x] **Test**: Detect project in repo root, subdirectory, handle non-repo

### 2.3.2 Project registry storage
- [x] Create `internal/project/registry.go`
- [x] Define `Registry` struct with project map
- [x] Store in `~/.config/cloister/projects.yaml` (list of known projects)
- [x] Fields: name, remote URL, root path, last used timestamp
- [x] **Test**: Load registry, save registry, round-trip

### 2.3.3 Project registration
- [x] Implement `Register(info *ProjectInfo) error`
- [x] Auto-register on first `cloister start` in a directory
- [x] Update existing entry if same name but different path (warn user)
- [x] Handle name collisions (different remotes, same basename) - suggest rename or use full name
- [x] **Test**: Register new project, update existing, handle collision

### 2.3.4 Project lookup
- [x] Implement `Lookup(name string) (*ProjectInfo, error)`
- [x] Implement `LookupByPath(path string) (*ProjectInfo, error)`
- [x] Implement `List() []*ProjectInfo`
- [x] **Test**: Lookup by name, by path, list all

---

## Phase 2.4: Per-Project Configuration

Load and manage project-specific configuration files.

### 2.4.1 Project config paths
- [x] Project configs stored at `~/.config/cloister/projects/<name>.yaml`
- [x] Implement `ProjectConfigPath(name string) string`
- [x] Ensure projects directory exists on first use
- [x] **Test**: Path generation is consistent

### 2.4.2 Project config loading
- [x] Implement `LoadProjectConfig(name string) (*ProjectConfig, error)`
- [x] Return empty/default config if file doesn't exist
- [x] Validate remote URL matches registered project (warn on mismatch)
- [x] **Test**: Load existing config, load missing config, handle corrupt file

### 2.4.3 Project config creation
- [x] Implement `InitProjectConfig(info *ProjectInfo) error`
- [x] Create minimal project config with remote URL and root path
- [x] Don't overwrite existing config
- [x] **Test**: Creates file, preserves existing

### 2.4.4 Project config editing
- [x] Implement `EditProjectConfig(name string) error`
- [x] Open config file in `$EDITOR` (fall back to `vi`)
- [x] Create minimal config first if doesn't exist
- [x] Validate config after edit, warn on errors
- [x] **Test**: Manual - opens in editor, warns on invalid (deferred to Phase 2.7 when CLI available)

---

## Phase 2.5: Config Merging

Merge global and project configs into effective configuration.

### 2.5.1 Allowlist merging
- [x] Create `internal/config/merge.go`
- [x] Project allowlist entries add to (not replace) global allowlist
- [x] Implement `MergeAllowlists(global, project []AllowEntry) []AllowEntry`
- [x] Deduplicate domains
- [x] **Test**: Merge with overlap, merge with additions only

### 2.5.2 Command pattern merging
- [x] Project auto_approve patterns add to global patterns
- [x] Project manual_approve patterns add to global patterns
- [x] Implement `MergeCommandPatterns(global, project *CommandConfig) *CommandConfig`
- [x] **Test**: Pattern merging, no duplicates

### 2.5.3 Effective config resolution
- [x] Implement `ResolveConfig(project string) (*EffectiveConfig, error)`
- [x] Load global config
- [x] Load project config (if project provided)
- [x] Merge allowlists and patterns
- [x] Return merged effective config
- [x] **Test**: Resolution with project, resolution without project (global only)

---

## Phase 2.6: Guardian Integration

Wire configuration into the guardian proxy.

### 2.6.1 Allowlist from config
- [x] Modify `NewAllowlist` to accept config-derived domain list
- [x] Update `guardian.Server` to receive allowlist from config
- [x] Remove hardcoded `DefaultAllowedDomains` (or keep as fallback)
- [x] **Test**: Proxy uses config-derived allowlist

### 2.6.2 Dynamic allowlist updates
- [x] Implement mechanism for guardian to receive updated allowlist
- [x] Option A: Guardian reloads config on SIGHUP
- [x] Option B: Guardian exposes endpoint to update allowlist
- [x] For Phase 2, prefer Option A (simpler, no API versioning concerns)
- [x] **Test**: Config change + SIGHUP → allowlist updated

### 2.6.3 Per-cloister allowlist
- [x] Guardian stores per-cloister allowlist (project-specific additions)
- [x] Token registration includes project name
- [x] Proxy looks up cloister's project, uses merged allowlist
- [x] **Test**: Two cloisters, different projects, different allowlists

---

## Phase 2.7: CLI Commands

Implement configuration and project management commands.

### 2.7.1 `cloister config` command
- [ ] Create `internal/cmd/config.go`
- [ ] `config show`: Print current effective global config
- [ ] `config edit`: Open global config in `$EDITOR`
- [ ] `config path`: Print path to config file
- [ ] `config init`: Create default config if missing
- [ ] **Test**: Manual - commands work as expected

### 2.7.2 `cloister project list` command
- [ ] Create `internal/cmd/project.go`
- [ ] List all registered projects
- [ ] Show: name, root path, remote URL, last used
- [ ] Format as table
- [ ] **Test**: Manual - list shows registered projects

### 2.7.3 `cloister project show` command
- [ ] `project show <name>`: Show project details
- [ ] Display: name, root, remote, effective allowlist additions
- [ ] Show config file path
- [ ] **Test**: Manual - show displays details

### 2.7.4 `cloister project edit` command
- [ ] `project edit <name>`: Open project config in `$EDITOR`
- [ ] Auto-complete project names
- [ ] **Test**: Manual - opens correct file

### 2.7.5 `cloister project remove` command
- [ ] `project remove <name>`: Remove project from registry
- [ ] Optionally remove project config file (`--config` flag)
- [ ] Don't remove if cloisters running for that project
- [ ] **Test**: Manual - removes from registry, handles running cloisters

### 2.7.6 Update `cloister start`
- [ ] Auto-register project on first start
- [ ] Load and apply project-specific allowlist
- [ ] Pass project name to guardian on token registration
- [ ] **Test**: Manual - start registers project, uses project allowlist

---

## Phase 2.8: Integration & Polish

End-to-end testing and documentation.

### 2.8.1 End-to-end integration
- [ ] **Test**: Fresh install → default config created → cloister starts
- [ ] **Test**: Add domain to project config → restart cloister → domain allowed
- [ ] **Test**: Same domain denied for different project
- [ ] **Test**: `project list` → `project show` → `project edit` workflow
- [ ] **Test**: Config validation errors shown clearly

### 2.8.2 Migration from Phase 1
- [ ] Existing cloisters continue to work (backward compatible)
- [ ] First run after upgrade creates default config
- [ ] Token persistence (Phase 1.8.2) continues to work with config
- [ ] Document upgrade path (none needed, automatic)

### 2.8.3 Error handling improvements
- [ ] Clear error when config file has syntax errors
- [ ] Clear error when project not found in registry
- [ ] Suggest `project list` when project lookup fails
- [ ] Warn when config has unknown fields (typo detection)

### 2.8.4 Documentation
- [ ] Update README with config system overview
- [ ] Update config-reference.md with any schema changes
- [ ] Add examples for common configurations
- [ ] Document allowlist best practices

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

### Future: Devcontainer Integration
- Devcontainer.json discovery and image building
- Security overrides for mounts and capabilities
