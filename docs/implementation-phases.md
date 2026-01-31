# Cloister Implementation Plan

## Design Principle: Dogfood Early

Each phase produces a working (if limited) system. Phase 1 enables basic sandboxed development. Later phases add capabilities without breaking what works.

---

## Phase 1: Minimal Viable Cloister with Guardian (COMPLETE)

**Goal:** `cloister start` launches a sandboxed container with guardian-proxied networking.

**Delivers:**
- Docker networks: `cloister-net` (internal), guardian connected to both `cloister-net` and `bridge`
- Guardian container with HTTP CONNECT proxy (:3128)
- Minimal hardcoded allowlist: `api.anthropic.com`, `api.openai.com`, `generativelanguage.googleapis.com`
- Per-cloister tokens (generated at start, passed via env)
- Token persistence (`~/.config/cloister/tokens/`) to survive guardian restarts
- Proxy authentication via `Proxy-Authorization` header
- Basic container launch with project bind-mounted at `/work`
- Default container image (Ubuntu + Go/Node/Python/Claude CLI)
- CLI: `start`, `stop`, `list`, `guardian start/stop/status`
- Guardian auto-start from `cloister start`
- Pass-through `CLAUDE_CODE_OAUTH_TOKEN` and `ANTHROPIC_API_KEY` from host env (temporary dogfooding support)
- Create `~/.claude.json` with onboarding skip, add `--dangerously-skip-permissions` alias

**Verification:**
- `cloister start` → guardian starts if needed → container starts → user gets shell at `/work`
- `curl -x $HTTP_PROXY https://api.anthropic.com` succeeds
- `curl -x $HTTP_PROXY https://github.com` fails (not in allowlist)
- Start 2 cloisters; each authenticated with own token
- `cloister stop` cleans up container
- `guardian stop` warns about running cloisters
- With `CLAUDE_CODE_OAUTH_TOKEN` set: `claude` command works inside container

**Not yet:** Config files, hostexec, projects, worktrees, approval UI, token persistence, credential wizard

---

## Phase 2: Configuration System (COMPLETE)

**Goal:** Global and per-project config controls allowlists.

**Delivers:**
- Config file loading (`~/.config/cloister/config.yaml`, `projects/<name>.yaml`)
- Full default allowlist in global config (AI APIs, package registries, documentation sites)
- Project auto-detection from git repos (name from directory basename)
- Project registry (maps names to paths)
- Per-project allowlist merging with global
- CLI: `project list/show/edit/remove`, `config` command for settings

**Verification:**
- Add domain to project config; cloister for that project allows it
- Same domain blocked for different project without that config
- `project list` shows registered projects
- Config edit opens in `$EDITOR`
- Guardian restart preserves token associations

---

## Phase 3: Claude Code Integration (COMPLETE)

**Goal:** Claude Code works inside cloister with no manual setup.

**Delivers:**
- `cloister setup claude` wizard with three auth options:
  1. **Existing login** — extracts from macOS Keychain or verifies Linux `~/.claude/.credentials.json`
  2. **Long-lived OAuth token** — user runs `claude setup-token`, pastes result
  3. **API key** — user pastes key from console.anthropic.com
- Platform-aware credential injection:
  - macOS: extract from Keychain (`security find-generic-password`), write `.credentials.json` to container
  - Linux: copy existing `~/.claude/.credentials.json` to container
  - Token/API key: inject via environment variable
- Copy host `~/.claude/` settings (excluding `.credentials.json` on macOS) into container
- Generate `~/.claude.json` in container (onboarding skip, bypass-permissions acceptance)
- Remove reliance on host env vars for credentials (Phase 1 workaround)

**Supersedes from Phase 1:**
- Host env var pass-through (`CLAUDE_CODE_OAUTH_TOKEN`, `ANTHROPIC_API_KEY`) replaced by config-based credential injection
- Hardcoded `~/.claude.json` creation in Phase 1 replaced by config-driven generation

**Verification:**
- Run `cloister setup claude`, select "existing login" → credentials extracted/verified
- `cloister start` → `claude` command works without additional auth
- Claude can reach api.anthropic.com, edit files in `/work`
- User settings from host `~/.claude/` available in container
- `~/.claude.json` contains onboarding skip flags
- Token refresh works (for existing login method)

---

## Phase 4: Host Execution (hostexec)

**Goal:** Agents can request host commands via approval workflow.

**Delivers:**
- Request server (:9998) in guardian container
- Approval server (:9999) bound to localhost with htmx web UI
- Host executor process communicating via Unix socket
- Guardian↔executor shared secret (ephemeral, not persisted)
- `hostexec` wrapper script in container
- Auto-approve and manual-approve patterns in config
- SSE for real-time request updates

**Verification:**
- `hostexec docker compose ps` auto-approves (if configured)
- `hostexec docker compose up -d` appears in approval UI
- Approve → command runs on host, output returned to container
- Deny → container gets denial message
- Timeout after 5 minutes

---

## Phase 5: Worktree Support

**Goal:** `cloister start -b <branch>` creates managed worktrees.

**Delivers:**
- Worktree creation at `~/.local/share/cloister/worktrees/<project>/<branch>/`
- Cloister naming: `<project>-<branch>`
- Branch creation if not exists
- Worktree cleanup protection (refuse if uncommitted changes)
- CLI: `worktree list/remove`, `cloister path <name>`

**Verification:**
- `cloister start -b feature-x` creates worktree and cloister
- Both cloisters (main and worktree) run concurrently
- `cloister path my-api-feature-x` returns worktree directory
- `worktree remove feature-x` refuses without `-f` if dirty

---

## Phase 6: Domain Approval Flow

**Goal:** Unlisted domains can be approved interactively with persistence options.

**Delivers:**
- Proxy holds connection for unlisted domains (60s timeout)
- Approval UI shows pending domain requests
- Approval scopes: session (memory only), project (persisted), global (persisted)
- Deny option for requests

**Verification:**
- Request to unlisted domain → appears in approval UI
- "Allow (session)" → request succeeds, subsequent requests auto-allowed until stop
- "Save to project" → persisted to project config, survives restart
- "Deny" → request fails with 403
- Timeout → request fails with 403

---

## Phase 7: Polish

**Goal:** Production-ready UX and observability.

**Delivers:**
- Image distribution:
  - Default image: `ghcr.io/xdg/cloister:latest`
  - `CLOISTER_IMAGE` env var overrides default (for local dev and CI/CD)
  - Precedence: `CLOISTER_IMAGE` > `config.default_image` > built-in default
  - Auto-pull on `cloister start` if remote image, rate-limited to once per day
  - `--no-pull` flag to skip update check (offline use)
  - `cloister update` command to explicitly pull latest image
  - First-run UX: progress indicator when pulling image
- Custom image configuration:
  - Global config: `default_image` overrides built-in default
  - Per-project config: `image` field overrides global default
  - Users build custom images extending the default (e.g., Rust+WASM toolchain, Python ML stack)
- Multi-arch container images:
  - GitHub Action builds linux/amd64 + linux/arm64
  - Tagged commits → `ghcr.io/xdg/cloister:vX.Y.Z` + update `:latest`
  - Merges to main → `ghcr.io/xdg/cloister:edge`
- Shell completion (bash, zsh, fish)
- Read-only reference mounts (`/refs` for other repos, configured per-project)
- Audit logging (unified + per-cloister)
- Improved error messages with actionable suggestions
- `cloister start -d` (detached mode)
- Non-git directory support with `--allow-no-git`
- Guardian API versioning (CLI checks compatibility with container image)
- Structured logging: warnings/errors to stderr, debug diagnostics to file (default location or `--debuglog=<path>`)

**Verification:**
- `CLOISTER_IMAGE=cloister:latest cloister start` → uses local dev image
- `cloister start` with no local image → pulls from ghcr.io with progress
- `cloister start --no-pull` → skips update check
- `cloister update` → pulls latest image
- Set `default_image: my-rust-wasm:latest` in global config → all cloisters use it
- Set `image: my-python-ml:latest` in project config → overrides global for that project
- Tab completion works for commands, cloister names, project names
- Project with refs config → ref directories mounted read-only at `/refs/`
- Logs capture all proxy and hostexec events
- Clear error when starting from non-git directory without flag
- Tagged release triggers GH Action → multi-arch image appears in ghcr.io
- Merge to main triggers GH Action → `:edge` image updated

---

## Future: Devcontainer Integration

**Goal:** Use project's `.devcontainer/devcontainer.json` with security overrides.

**Delivers:**
- Devcontainer.json discovery (cloister-specific → standard location)
- Image building from devcontainer spec
- Feature installation (with allowlist)
- Security overrides (blocked mounts, network, capabilities)
- Lifecycle command execution (postCreateCommand, etc.)
- Build caching by config hash
- Signal handling: inject `--init` flag for images without tini (ensures fast shutdown)

**Verification:**
- Project with devcontainer.json uses custom image
- Mount request for `~/.ssh` logged and blocked
- `postCreateCommand` runs via proxy
- Rebuild only when devcontainer.json changes

---

## Dependency Graph

```
Phase 1 (MVP + Guardian)
    ↓
Phase 2 (Config)
    ↓
Phase 3 (Claude) ←──┬── Phase 4 (hostexec)
    ↓               │
Phase 5 (Worktrees) │
    ↓               ↓
Phase 6 (Domain Approval)
    ↓
Phase 7 (Polish)
    ↓
Future (Devcontainer)
```

Phases 3-4 can proceed in parallel after Phase 2.

---

## Dogfooding Milestones

| After Phase | What You Can Do |
|-------------|-----------------|
| 1 | Run Claude Code in sandbox (with manual env var), reach AI APIs |
| 2 | Configure per-project allowlists, persist tokens |
| 3 | One-command Claude setup (no manual env var) |
| 4 | Run docker, gh, aws commands with approval |
| 5 | Work on feature branches in parallel |
| 6 | Dynamically allow new domains |
| 7 | Full production UX |
| Future | Use existing devcontainer.json setups |
