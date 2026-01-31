# Agent Configuration

This document covers how to configure AI agents to work inside cloisters.

Each agent has its own authentication mechanism and configuration requirements. Cloister needs to inject the right credentials and config into the container without exposing them to the filesystem in ways the AI could exfiltrate.

---

## Claude Code

### Authentication

Claude Code supports three authentication methods. The `cloister setup claude` command handles all interactively.

#### Interactive Setup

```bash
$ cloister setup claude
# Prompts for:
#   1. Authentication method:
#      - Use existing Claude login (recommended) — Reuses your host login
#      - Long-lived OAuth token — From `claude setup-token`
#      - API key — For pay-per-use via Anthropic API
#   2. Credentials (extracted automatically or hidden input)
#   3. Whether to skip Claude's permission system (default: yes)
```

#### Option 1: Use Existing Login (Recommended)

If you've already run `claude login` on your host machine, cloister can reuse those credentials:

**macOS:**
- Credentials are extracted from the system Keychain (service: `Claude Code-credentials`)
- Extracted at container start, so token refreshes are picked up automatically
- If extraction fails, you'll be prompted to run `claude login` first

**Linux:**
- Credentials are read from `~/.claude/.credentials.json`
- File is copied into the container at start
- If file is missing, you'll be prompted to run `claude login` first

This is the recommended method because it requires no additional setup if you're already using Claude Code on your host.

#### Option 2: Long-lived OAuth Token

For users who prefer explicit token management or CI/CD scenarios:

1. Run `claude setup-token` on host to generate a long-lived token (valid for 1 year)
2. Run `cloister setup claude` and select "Long-lived OAuth token"
3. Paste the token when prompted

Token is stored in `~/.config/cloister/config.yaml` under `agents.claude.token` and injected via `CLAUDE_CODE_OAUTH_TOKEN` env var.

#### Option 3: API Key

For users paying via Anthropic API (not Pro/Max subscription):

1. Get your API key from console.anthropic.com
2. Run `cloister setup claude` and select "API key"
3. Paste the key when prompted

Key is stored in `~/.config/cloister/config.yaml` under `agents.claude.api_key` and injected via `ANTHROPIC_API_KEY` env var.

#### Legacy: Environment Variable Fallback

If no credentials are configured via `cloister setup claude`, cloister will fall back to host environment variables (`CLAUDE_CODE_OAUTH_TOKEN` or `ANTHROPIC_API_KEY`). This is deprecated and will show a warning. Run `cloister setup claude` to migrate.

**Priority order:** Config credentials (from `cloister setup claude`) always take priority over environment variables. If both are present, the config value is used.

### Container Configuration

**What cloister does at container launch:**

1. **Injects authentication (based on configured method):**
   - *Existing login:* Writes `~/.claude/.credentials.json` to container
     - macOS: Extracted fresh from Keychain (picks up token refreshes)
     - Linux: Copied from host `~/.claude/.credentials.json`
   - *Long-lived token:* Sets `CLAUDE_CODE_OAUTH_TOKEN` env var
   - *API key:* Sets `ANTHROPIC_API_KEY` env var

2. **Copies `~/.claude/` settings from host:**
   - Contains user settings (but NOT `.credentials.json` on macOS—see note below)
   - One-way copy (host → container); changes don't persist back to host
   - Missing directory is handled gracefully (first-time users)

3. **Generates `~/.claude.json` inside the container:**

   This file is *generated* (not simply copied from host) to ensure consistent behavior.
   Cloister merges forced values with select fields from the host's `~/.claude.json`:

   | Field | Source | Purpose |
   |-------|--------|---------|
   | `hasCompletedOnboarding` | Set to `true` | Skip onboarding prompts |
   | `bypassPermissionsModeAccepted` | Set to `true` | Accept bypass-permissions mode |
   | `installMethod` | Set to `"native"` | Match container's install method |
   | `userID` | Copied from host | Preserve stable user identity hash |
   | `lastOnboardingVersion` | Copied from host | Avoid "new version" upgrade prompts |
   | `oauthAccount` | Copied from host (Option 1 only) | Account info tied to credentials |

   **Fields NOT copied:**
   - `projects` — Contains host-specific paths (e.g., `/Users/xdg/git/...`) that don't exist in the container
   - `numStartups`, caches, tips history — Machine-local state that regenerates naturally

   Example generated file:
   ```json
   {
     "hasCompletedOnboarding": true,
     "bypassPermissionsModeAccepted": true,
     "installMethod": "native",
     "userID": "66fae89a7697d69d2a7773fe6714e73439141570901d7b104829a4a061317d79",
     "lastOnboardingVersion": "2.1.25",
     "oauthAccount": {
       "accountUuid": "aa22cae9-...",
       "emailAddress": "user@example.com",
       "organizationUuid": "e0d2c770-..."
     }
   }
   ```

   Note: `oauthAccount` is only included when using "existing" auth (Option 1), since it's tied to the credentials being injected. For token/API key auth, this field is omitted.

4. **If permission skipping is enabled, creates a shell alias:**
   ```bash
   alias claude='claude --dangerously-skip-permissions'
   ```
   This is added to `~/.bashrc` in the container.

**Why skip Claude's permissions?**

Claude Code has its own permission system that prompts before file edits, shell commands, etc. Inside a cloister, this is redundant — the cloister *is* the sandbox. Disabling Claude's prompts allows uninterrupted operation while cloister enforces the actual security boundary.

There is no config file option for `--dangerously-skip-permissions`, so we use a shell alias.

**Configuration:**

Permission skipping is controlled by `agents.claude.skip_permissions` in `~/.config/cloister/config.yaml`:

```yaml
agents:
  claude:
    skip_permissions: true  # default
```

This defaults to `true`. Set to `false` to omit the alias, allowing Claude to use its normal permission prompts inside the container.

**Important: macOS credential handling**

On macOS, Claude Code stores credentials in the system Keychain, not on disk. The `~/.claude/.credentials.json` file doesn't exist (or is deleted by Claude Code if created). Therefore:

- Don't bind-mount `~/.claude/` directly from macOS host to container
- Instead, cloister extracts credentials from Keychain and writes them to the container
- User settings from `~/.claude/` are copied separately (excluding `.credentials.json`)

**Technical details: Keychain extraction**

Keychain entry:
- **Service name**: `Claude Code-credentials`
- **Account**: User's macOS username (e.g., `xdg`)

Extraction command:
```bash
security find-generic-password -s 'Claude Code-credentials' -a "$(whoami)" -w
```

The command outputs a JSON blob that cloister writes to `~/.claude/.credentials.json` in the container:

```json
{
  "claudeAiOauth": {
    "accessToken": "sk-ant-oat01-...",
    "refreshToken": "sk-ant-ort01-...",
    "expiresAt": 1769753311584,
    "scopes": ["user:inference", "user:profile", "..."],
    "subscriptionType": "max"
  }
}
```

Note: The `accessToken` is short-lived, but Claude Code auto-refreshes using the `refreshToken`. By extracting fresh from Keychain at each container start, cloister picks up any token refreshes that occurred on the host.

**Implementation notes:**

- Auth method stored in `~/.config/cloister/config.yaml` under `agents.claude.auth_method`
- For token/API key methods, credentials stored under `agents.claude.token` or `agents.claude.api_key`
- `~/.claude/` settings copied from host (one-way snapshot, excluding machine-local files)
- `~/.claude.json` generated at container start:
  - Forced fields: `hasCompletedOnboarding`, `bypassPermissionsModeAccepted`, `installMethod`
  - Copied fields: `userID`, `lastOnboardingVersion`
  - Conditional: `oauthAccount` (only for "existing" auth method)

---

## Other Agents

TODO: Document as we add support.

| Agent | Auth Mechanism | Config Location | Notes |
|-------|---------------|-----------------|-------|
| Codex (OpenAI) | API key | `OPENAI_API_KEY` | |
| Gemini CLI | API key | `GOOGLE_API_KEY` | |
| GitHub Copilot CLI | gh auth | `~/.config/gh/` | May need special handling |
| Aider | API keys | Various | Supports multiple providers |

---

## Open Questions

1. **Token rotation:** Long-lived OAuth tokens (from `claude setup-token`) expire after 1 year. How do we remind users to refresh? Detect expiry and prompt? (Note: "existing login" method auto-refreshes, so this only affects option 2.)

2. **Copilot CLI:** Uses GitHub's OAuth via `gh auth`. May need to pass through auth tokens or handle differently.

---
