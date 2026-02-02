# Agent Configuration

This document covers how to configure AI agents to work inside cloisters.

Each agent has its own authentication mechanism and configuration requirements. Cloister needs to inject the right credentials and config into the container without exposing them to the filesystem in ways the AI could exfiltrate.

---

## Claude Code

### Authentication

Claude Code supports two authentication methods. The `cloister setup claude` command handles all interactively.

#### Interactive Setup

```bash
$ cloister setup claude
# Prompts for:
#   1. Authentication method:
#      - Long-lived OAuth token (recommended) — From `claude setup-token`
#      - API key — For pay-per-use via Anthropic API
#   2. Credentials (hidden input)
#   3. Whether to skip Claude's permission system (default: yes)
```

#### Option 1: Long-lived OAuth Token (Recommended)

For Claude Pro/Max subscribers who want explicit token management:

1. Run `claude setup-token` on host to generate a long-lived token (valid for 1 year)
2. Run `cloister setup claude` and select "Long-lived OAuth token"
3. Paste the token when prompted

Token is stored in `~/.config/cloister/config.yaml` under `agents.claude.token` and injected via `CLAUDE_CODE_OAUTH_TOKEN` env var.

#### Option 2: API Key

For users paying via Anthropic API (not Pro/Max subscription):

1. Get your API key from console.anthropic.com
2. Run `cloister setup claude` and select "API key"
3. Paste the key when prompted

Key is stored in `~/.config/cloister/config.yaml` under `agents.claude.api_key` and injected via `ANTHROPIC_API_KEY` env var.

#### Legacy: Environment Variable Fallback

If no credentials are configured via `cloister setup claude`, cloister will fall back to host environment variables (`CLAUDE_CODE_OAUTH_TOKEN` or `ANTHROPIC_API_KEY`). This fallback is deprecated and will be removed in a future release. A warning is shown when env vars are used. Run `cloister setup claude` to migrate.

**Priority order:** Config credentials (from `cloister setup claude`) always take priority over environment variables. If both are present, the config value is used.

### Container Configuration

**What cloister does at container launch:**

1. **Injects authentication (based on configured method):**
   - *Long-lived token:* Sets `CLAUDE_CODE_OAUTH_TOKEN` env var
   - *API key:* Sets `ANTHROPIC_API_KEY` env var

2. **Copies `~/.claude/` settings from host:**
   - Contains user settings
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

   **Fields NOT copied:**
   - `projects` — Contains host-specific paths (e.g., `/Users/xdg/git/...`) that don't exist in the container
   - `numStartups`, caches, tips history — Machine-local state that regenerates naturally
   - `oauthAccount` — Tied to credentials that are not extracted from host

   Example generated file:
   ```json
   {
     "hasCompletedOnboarding": true,
     "bypassPermissionsModeAccepted": true,
     "installMethod": "native",
     "userID": "66fae89a7697d69d2a7773fe6714e73439141570901d7b104829a4a061317d79",
     "lastOnboardingVersion": "2.1.25"
   }
   ```

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

**Implementation notes:**

- Auth method stored in `~/.config/cloister/config.yaml` under `agents.claude.auth_method`
- For token/API key methods, credentials stored under `agents.claude.token` or `agents.claude.api_key`
- `~/.claude/` settings copied from host (one-way snapshot, excluding machine-local files)
- `~/.claude.json` generated at container start:
  - Forced fields: `hasCompletedOnboarding`, `bypassPermissionsModeAccepted`, `installMethod`
  - Copied fields: `userID`, `lastOnboardingVersion`

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

1. **Token rotation:** Long-lived OAuth tokens (from `claude setup-token`) expire after 1 year. How do we remind users to refresh? Detect expiry and prompt?

2. **Copilot CLI:** Uses GitHub's OAuth via `gh auth`. May need to pass through auth tokens or handle differently.

---
