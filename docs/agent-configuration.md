# Agent Configuration

This document covers how to configure AI agents to work inside cloisters.

Each agent has its own authentication mechanism and configuration requirements. Cloister needs to inject the right credentials and config into the container without exposing them to the filesystem in ways the AI could exfiltrate.

---

## Claude Code

### Authentication

Claude Code supports two authentication modes. The `cloister setup claude` command handles both interactively.

#### Interactive Setup

```bash
$ cloister setup claude
# Prompts for:
#   1. Authentication method:
#      - OAuth token (default) — For Claude Pro/Max subscriptions
#      - API key — For pay-per-use via Anthropic API
#   2. Credentials (hidden input)
#   3. Whether to skip Claude's permission system (default: yes)
```

#### OAuth Token (Recommended for Pro/Max users)

For users with Claude Pro or Max subscriptions:

1. Run `claude setup-token` on host to start OAuth flow
2. Copy the token (valid for 1 year)
3. Run `cloister setup claude` and select OAuth (default)
4. Paste the token when prompted

Token is stored in `~/.config/cloister/config.yaml` under `agents.claude.token`.

#### API Key

For users paying via Anthropic API:

1. Get your API key from console.anthropic.com
2. Run `cloister setup claude` and select API key
3. Paste the key when prompted

Alternatively, set `ANTHROPIC_API_KEY` in your host environment — cloister will pass it through to the container.

### Container Configuration

**What cloister does at container launch:**

1. **Injects authentication:**
   - OAuth: Sets `CLAUDE_CODE_OAUTH_TOKEN=<token>` in container environment
   - API key: Sets `ANTHROPIC_API_KEY=<key>` in container environment

2. **Creates `~/.claude.json` inside the container:**
   ```json
   {
     "hasCompletedOnboarding": true
   }
   ```
   This skips Claude's interactive onboarding flow.

3. **If permission skipping is enabled, creates a shell alias:**
   ```bash
   alias claude='claude --dangerously-skip-permissions'
   ```
   This is added to `~/.bashrc` in the container.

**Why skip Claude's permissions?**

Claude Code has its own permission system that prompts before file edits, shell commands, etc. Inside a cloister, this is redundant — the cloister *is* the sandbox. Disabling Claude's prompts allows uninterrupted operation while cloister enforces the actual security boundary.

There is no config file option for `--dangerously-skip-permissions`, so we use a shell alias.

**Implementation notes:**

- Credentials stored on host in `~/.config/cloister/config.yaml` under `agents.claude.*`
- The `~/.claude.json` is created fresh in each container (not bind-mounted from host)
- The `~/.claude/` directory (Claude Code's working state) is separate from `~/.claude.json` (config)

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

1. **Token rotation:** OAuth tokens expire after 1 year. How do we remind users to refresh? Detect expiry and prompt?

2. **Multiple auth methods:** What if a user has both API key and OAuth token? Which takes precedence?

3. **Copilot CLI:** Uses GitHub's OAuth via `gh auth`. May need to pass through auth tokens or handle differently.

---
