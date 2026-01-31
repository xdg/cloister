# Phase 3: Claude Code Integration

Goal: Claude Code works inside cloister with no manual setup. A wizard handles credential configuration, and credentials are injected from config rather than host environment variables.

## Testing Philosophy

Tests are split into tiers based on what they require:

| Tier | Command | What it tests | Requirements |
|------|---------|---------------|--------------|
| **Unit** | `make test` | Pure logic, mocked I/O, config parsing | None (sandbox-safe) |
| **Integration** | `make test-integration` | Container creation, env var injection | Docker daemon |
| **Manual** | Human verification | Real TTY input, actual Claude API calls | Human + credentials |

**Design for testability:**
- Factor interactive I/O through interfaces (`io.Reader`/`io.Writer`) for unit testing
- Use `t.TempDir()` for config file tests (avoids touching real `~/.config`)
- Mock `container.Manager` interface to verify env vars without Docker
- Integration tests use `//go:build integration` tag

## Verification Checklist

Before marking Phase 3 complete:

**Automated (`make test`):**
1. All unit tests pass
2. `make build` produces working binary
3. `make lint` passes
4. `make test-race` passes

**Automated (`make test-integration`):**
5. Integration tests pass (credential injection, file creation, ~/.claude/ settings copy)

**Manual — test each auth method:**

*Option 1: Existing login (macOS):*
6. Run `cloister setup claude`, select "Use existing Claude login"
7. Verify keychain extraction succeeds (or errors clearly if not logged in)
8. `cloister start` → `cat /home/cloister/.claude/.credentials.json` → credentials present
9. Run `claude` command → API call succeeds (token refresh works)

*Option 1: Existing login (Linux):*
10. Verify setup detects `~/.claude/.credentials.json` (or errors if missing)
11. `cloister start` → credentials file copied to container

*Option 2: Long-lived OAuth token:*
12. Run `cloister setup claude`, select "Long-lived OAuth token"
13. Paste token from `claude setup-token` (verify hidden input)
14. `cloister start` → `echo $CLAUDE_CODE_OAUTH_TOKEN` → token visible
15. Run `claude` command → API call succeeds

*Option 3: API key:*
16. Run `cloister setup claude`, select "API key"
17. Paste API key (verify hidden input)
18. `cloister start` → `echo $ANTHROPIC_API_KEY` → key visible
19. Run `claude` command → API call succeeds

*Common verification:*
20. Run `ls -la /home/cloister/.claude/` → user settings present (minus .credentials.json on macOS)
21. Run `cat /home/cloister/.claude.json` → onboarding config present (generated)
22. Run `type claude` → shows alias with `--dangerously-skip-permissions`

## Dependencies Between Phases

```
Phase 1 (MVP + Guardian) ✓
       │
       ▼
Phase 2 (Config) ✓
       │
       ▼
Phase 3 (Claude Integration) ◄── CURRENT
       │
       ├─► Phase 4 (hostexec) [parallel path from Phase 2]
       │
       ▼
Phase 5 (Worktrees)
       │
       ▼
Phase 6 (Domain Approval)
       │
       ▼
Phase 7 (Polish)
```

---

## Phase 3.1: Config Schema Updates

Extend the configuration types to support credential storage. **All tests sandbox-safe.**

### 3.1.1 Add credential fields to AgentConfig
- [x] In `internal/config/types.go`, extend `AgentConfig` with credential fields:
  ```go
  type AgentConfig struct {
      Command    string   `yaml:"command,omitempty"`
      Env        []string `yaml:"env,omitempty"`
      AuthMethod string   `yaml:"auth_method,omitempty"`  // "existing", "token", or "api_key"
      Token      string   `yaml:"token,omitempty"`        // long-lived OAuth token
      APIKey     string   `yaml:"api_key,omitempty"`      // Anthropic API key
      SkipPerms  *bool    `yaml:"skip_permissions,omitempty"` // default true
  }
  ```
- [x] **Test (unit)**: YAML round-trip: marshal config with new fields, unmarshal, verify values

### 3.1.2 Add defaults for agents.claude in default config
- [x] In `internal/config/defaults.go`, add default `agents.claude` entry with `skip_permissions: true`
- [x] Ensure default config creates agents map if missing
- [x] **Test (unit)**: `DefaultGlobalConfig()` returns config with `agents["claude"].SkipPerms == true`

### 3.1.3 Add config validation for agent credentials
- [x] In `internal/config/validate.go`, add validation:
  - Error if `auth_method` set but required field missing (e.g., `token` for "token" method)
  - Warn if `auth_method` not set and no host env vars (no auth configured)
  - Validate `auth_method` is one of: "existing", "token", "api_key"
- [x] **Test (unit)**: Validation returns expected errors/warnings for each case

---

## Phase 3.2: Setup Command

Create the interactive setup wizard for Claude credentials.

### 3.2.1 Create setup command structure
- [x] Create `internal/cmd/setup.go` with `setup` parent command
- [x] Create `internal/cmd/setup_claude.go` with `setup claude` subcommand
- [x] Wire up to root command: `cloister setup claude`
- [x] **Test (unit)**: `rootCmd.Commands()` includes setup; setup has claude subcommand

### 3.2.2 Implement auth method prompt
- [x] Define `Prompter` interface: `Prompt(prompt string, options []string, defaultIdx int) (int, error)`
- [x] Implement `StdinPrompter` for real use, `MockPrompter` for tests
- [x] Prompt user to select authentication method:
  ```
  Select authentication method:
    1. Use existing Claude login (recommended)
    2. Long-lived OAuth token (from `claude setup-token`)
    3. API key (from console.anthropic.com)
  ```
- [x] Default to option 1 if user just presses Enter
- [x] **Test (unit)**: Mock prompter returns expected selection; default works

### 3.2.3 Implement "existing login" option (option 1)
- [x] Create `internal/claude/credentials.go` package for credential extraction
- [x] Detect platform: `runtime.GOOS`
- [x] **macOS path:**
  - [x] Run `security find-generic-password -s 'Claude Code-credentials' -a "$(whoami)" -w`
  - [x] Parse JSON response to extract credentials
  - [x] Store extracted JSON in config at `agents.claude.credentials_json` (or separate file)
  - [x] Error if command fails → "Run `claude login` first"
- [x] **Linux path:**
  - [x] Check if `~/.claude/.credentials.json` exists
  - [x] Store path reference in config (will be copied at container start)
  - [x] Error if file missing → "Run `claude login` first"
- [x] **Test (unit)**: Mock exec.Command for keychain; mock filesystem for Linux
- [x] **Test (manual)**: macOS keychain extraction works; Linux file detection works

### 3.2.4 Implement token/API key options (options 2 & 3)
- [x] Define `CredentialReader` interface: `ReadCredential(prompt string) (string, error)`
- [x] Implement `TerminalCredentialReader` using `golang.org/x/term` for hidden input
- [x] Implement `MockCredentialReader` that reads from provided string
- [x] For OAuth token: "Paste your OAuth token (from `claude setup-token`):"
- [x] For API key: "Paste your API key (from console.anthropic.com):"
- [x] **Test (unit)**: Mock reader returns provided credential
- [x] **Test (manual)**: Real terminal hides input while typing

### 3.2.5 Implement skip-permissions prompt
- [x] Prompt: "Skip Claude's built-in permission prompts? (recommended inside cloister) [Y/n]:"
- [x] Default to yes (Y) if user just presses Enter
- [x] **Test (unit)**: Empty input → true; "n" → false; "N" → false; "y" → true

### 3.2.6 Save credentials to config
- [x] Load existing global config (or create default)
- [x] Based on auth method, update appropriate config field:
  - Option 1: `agents.claude.auth_method: "existing"` (credentials extracted at start)
  - Option 2: `agents.claude.auth_method: "token"`, `agents.claude.token: "..."`
  - Option 3: `agents.claude.auth_method: "api_key"`, `agents.claude.api_key: "..."`
- [x] Update `agents.claude.skip_permissions`
- [x] Write config back with `config.WriteGlobalConfig()`
- [x] Print success message with config path
- [x] **Test (unit)**: Use `t.TempDir()` for config path; verify YAML contains expected values

### 3.2.7 Handle existing credentials
- [x] Check if credentials already exist in config
- [x] If yes, prompt: "Credentials already configured. Replace? [y/N]:"
- [x] Default to no (N) to prevent accidental overwrite
- [x] **Test (unit)**: Existing creds + "N" input → config unchanged; "y" → replaced

---

## Phase 3.3: Credential Injection

Platform-aware credential injection into containers.

### 3.3.1 Create credential injection interface
- [x] In `internal/claude/inject.go`, define credential injection logic
- [x] Three injection modes based on `auth_method`:
  - `"existing"`: Write `.credentials.json` file to container
  - `"token"`: Set `CLAUDE_CODE_OAUTH_TOKEN` env var
  - `"api_key"`: Set `ANTHROPIC_API_KEY` env var
- [x] **Test (unit)**: Each mode produces correct injection config

### 3.3.2 Implement "existing login" injection
- [x] **macOS:** Re-extract from Keychain at container start (credentials may have refreshed)
- [x] **Linux:** Read `~/.claude/.credentials.json` from host
- [x] Write credentials to container at `/home/cloister/.claude/.credentials.json`
- [x] Use Docker copy or volume mount (prefer copy to avoid host file mutation issues)
- [x] **Test (unit)**: Mock keychain/filesystem, verify correct JSON produced
- [x] **Test (integration)**: Container has valid `.credentials.json`

### 3.3.3 Implement token/API key injection
- [x] For token: set `CLAUDE_CODE_OAUTH_TOKEN` env var on container
- [x] For API key: set `ANTHROPIC_API_KEY` env var on container
- [x] **Test (unit)**: Verify correct env vars passed to container.Manager
- [x] **Test (integration)**: `printenv` inside container shows expected var

### 3.3.4 Update container start to use new injection
- [x] In `internal/cloister/cloister.go`, load global config
- [x] Call `claude.InjectCredentials()` to get injection config
- [x] Pass to container manager (env vars and/or files to create)
- [x] **Test (unit)**: Mock `container.Manager`, verify correct injection config passed

### 3.3.5 Deprecate host env var passthrough
- [x] If no config credentials and host env vars present, use as fallback
- [x] Print deprecation warning:
  ```
  Warning: Using ANTHROPIC_API_KEY from environment.
  Run 'cloister setup claude' to store credentials in config.
  ```
- [x] Only warn once per `cloister start` invocation
- [x] **Test (unit)**: Capture stderr, verify warning present when using env fallback

### 3.3.6 Handle credential refresh errors
- [x] If macOS keychain extraction fails at start, error with clear message
- [x] If Linux `.credentials.json` missing, error with clear message
- [x] Suggest running `claude login` or `cloister setup claude` again
- [x] **Test (unit)**: Verify error messages for each failure mode

---

## Phase 3.4: Container Configuration

Ensure Claude Code works correctly inside the container. **All tests require Docker.**

### 3.4.1 Copy ~/.claude/ settings from host
- [x] Copy host `~/.claude/` directory into container at `/home/cloister/.claude/`
- [x] Exclude machine-local files (debug/, statsig/, history, etc.) via rsync patterns
- [x] Handle missing directory gracefully (first-time users won't have it)
- [x] Copy is one-way (host → container); changes inside container don't persist
- [x] **Test (unit)**: `TestInjectUserSettings_MissingClaudeDir` verifies nil return when dir missing
- [x] **Test (integration)**: `TestInjectUserSettings_IntegrationWithContainer` verifies copy and ownership

### 3.4.2 Generate ~/.claude.json in container
- [x] Generate `/home/cloister/.claude.json` (separate from `.claude/` directory):
  ```json
  {
    "hasCompletedOnboarding": true,
    "bypassPermissionsModeAccepted": true
  }
  ```
- [x] File must exist before user shell starts
- [x] This is generated, not copied (we control onboarding behavior)
- [x] **Test (integration)**: Start container, `cat /home/cloister/.claude.json`, verify JSON content

### 3.4.3 Create claude alias for --dangerously-skip-permissions
- [x] Pass `skip_permissions` setting to container (env var or mount)
- [x] If enabled (default), entrypoint adds to `/home/cloister/.bashrc`:
  ```bash
  alias claude='claude --dangerously-skip-permissions'
  ```
- [x] Only add if not already present (idempotent)
- [x] **Test (integration)**: Start container, run `bash -ic 'type claude'`, verify alias shown

### 3.4.4 Handle skip_permissions=false case
- [x] If user sets `skip_permissions: false` in config, don't create alias
- [x] Claude will use its normal permission prompts
- [x] **Test (integration)**: Start container with skip_permissions=false, verify no alias

### 3.4.5 End-to-end Claude verification
- [x] **Test (manual)**: Inside container, run `claude --version` → prints version
- [x] **Test (manual)**: Run `claude "say hello"` → API call succeeds, response shown
- [x] **Test (manual)**: Verify user settings from host `~/.claude/` are respected

### 3.4.6 Generate cloister rules file for Claude
- [ ] After copying `~/.claude/`, write `/home/cloister/.claude/rules/cloister.md`
- [ ] Content explains cloister environment to Claude:
  - Running in sandboxed container with `/work` as project directory
  - No direct network access; proxy allowlists documentation + package registries
  - `hostexec <command>` for operations requiring host access (git push, docker, etc.)
  - Common patterns: `hostexec git push`, `hostexec docker build`
- [ ] Create `rules/` directory if it doesn't exist
- [ ] Overwrites any existing `cloister.md` (cloister-controlled file)
- [ ] **Test (integration)**: Start container, verify `/home/cloister/.claude/rules/cloister.md` exists with expected content

---

## Phase 3.5: Documentation and Cleanup

### 3.5.1 Update README with new setup flow
- [ ] Replace manual env var instructions with `cloister setup claude`
- [ ] Document both OAuth and API key flows
- [ ] Remove "Phase 1 limitation" notes about manual credential setup

### 3.5.2 Update docs/agent-configuration.md
- [ ] Add complete setup wizard documentation
- [ ] Document credential priority order (config > env vars)
- [ ] Document skip_permissions option and why it defaults to true
- [ ] Note env var fallback is deprecated

### 3.5.3 Remove Phase 1 workaround comments
- [ ] Remove "TEMPORARY: Phase 1 workaround" comments from `internal/token/credentials.go`
- [ ] Update function docs to reflect new behavior
- [ ] **Test (unit)**: `make lint` passes

### 3.5.4 Add migration notes to CHANGELOG or docs
- [ ] Document how to migrate from env vars to config: just run `cloister setup claude`
- [ ] Note that env vars still work as fallback (deprecated, removal in Phase 7)

---

## Not In Scope (Deferred to Later Phases)

### Phase 4: Host Execution
- hostexec wrapper
- Request server (:9998) and approval server (:9999)
- Approval web UI with htmx
- Auto-approve and manual-approve pattern execution
- Review and update `~/.claude/rules/cloister.md` content (from 3.4.6) with accurate hostexec usage

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
- Remove env var fallback (require config-based credentials)
- Multi-arch Docker image builds (linux/amd64, linux/arm64)

### Future: Devcontainer Integration
- Devcontainer.json discovery and image building
- Security overrides for mounts and capabilities

### Other Agents
- `cloister setup codex` (OpenAI)
- `cloister setup gemini` (Google)
- Generic agent credential framework
