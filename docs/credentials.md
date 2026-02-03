# Credentials

Cloister needs AI agent credentials to run tools like Claude Code inside containers. This guide covers authentication setup for supported agents.

## Claude Code

Claude Code supports three authentication methods. Cloister's setup wizard handles the configuration.

### Running the Setup Wizard

```bash
cloister setup claude
```

The wizard will:
1. Detect any existing Claude authentication on the host
2. Present a menu of authentication methods
3. Store credentials in config
4. Configure injection for container startup

### Authentication Methods

#### 1. OAuth Token (Recommended for Pro/Max)

Best for Claude Pro or Max subscribers:

```bash
# First, get a token from Claude Code CLI
claude setup-token
# This opens a browser for OAuth and displays a token

# Then configure Cloister
cloister setup claude
# Choose "Long-lived OAuth token" and paste the token
```

Tokens are valid for approximately one year.

#### 2. Existing Login Session

If you're already logged into Claude Code on the host:

```bash
cloister setup claude
# Choose "Use existing login"
```

Cloister extracts credentials and injects them into the container at startup.

**Note:** Session files may expire. Use OAuth tokens for long-running setups.

#### 3. API Key

For API access (requires Anthropic API account):

```bash
cloister setup claude
# Choose "API key"
# Paste your sk-ant-... key
```

API usage is billed separately from Claude Pro/Max subscriptions.

## Credential Storage

Credentials are stored in the global config file:

```
~/.config/cloister/
└── config.yaml          # Contains agent credentials under agents.claude
```

Example config structure:

```yaml
agents:
  claude:
    auth_method: token  # or "existing" or "api_key"
    token: "..."        # if using OAuth token
    api_key: "..."      # if using API key
```

### Security Notes

- Host credential files (`~/.anthropic/`, `~/.claude/`) are not bind-mounted into containers
- For "existing" auth, credentials are extracted and written into the container at startup
- For token/API key auth, values are injected via environment or config files
- Config file permissions should be restricted (readable only by owner)

## Container Environment

Inside the cloister, Claude Code is pre-configured:

```bash
cloister@container:/work$ claude --version
# Claude Code runs with injected credentials
```

By default, Claude is aliased to include `--dangerously-skip-permissions` since the sandbox provides containment.

### Disabling the Alias

If you prefer Claude's normal permission prompts inside the cloister:

```yaml
# ~/.config/cloister/config.yaml
agents:
  claude:
    skip_permissions: false
```

## Refreshing Credentials

To update credentials (e.g., after token expiration):

```bash
cloister setup claude
# Re-run the wizard
```

For running cloisters, restart to pick up new credentials:

```bash
cloister stop my-app
cloister start
```

## Codex CLI

Codex CLI uses OpenAI API key authentication.

### Running the Setup Wizard

```bash
cloister setup codex
```

The wizard will:
1. Prompt for your OpenAI API key
2. Ask about full-auto mode preference
3. Store credentials in config
4. Configure injection for container startup

### Authentication

Get your API key from [platform.openai.com/api-keys](https://platform.openai.com/api-keys):

```bash
cloister setup codex
# Paste your sk-... key when prompted
```

API key is stored in `~/.config/cloister/config.yaml` under `agents.codex.api_key` and injected via `OPENAI_API_KEY` env var.

### Credential Storage

```yaml
agents:
  codex:
    api_key: "sk-..."
    skip_permissions: true  # enables full-auto mode
```

### Container Environment

Inside the cloister, Codex CLI is pre-configured:

```bash
cloister@container:/work$ codex --version
# Codex runs with injected credentials
```

By default, Codex is aliased to include `--approval-mode full-auto` since the sandbox provides containment.

### Disabling Full-Auto Mode

If you prefer Codex's normal approval prompts inside the cloister:

```yaml
# ~/.config/cloister/config.yaml
agents:
  codex:
    skip_permissions: false
```

### Refreshing Credentials

To update credentials:

```bash
cloister setup codex
# Re-run the wizard
```

For running cloisters, restart to pick up new credentials:

```bash
cloister stop my-app
cloister start
```

## Other Agents

Support for additional AI coding agents is planned:

- **Gemini CLI** — Coming soon

The setup pattern is the same:

```bash
cloister setup <agent>
```

## Troubleshooting

### Claude: "Authentication failed" inside cloister

1. Verify credentials are configured: `cloister setup claude`
2. Check token hasn't expired (OAuth tokens last ~1 year)
3. Restart the cloister after updating credentials

### Claude prompts for login inside container

The credential injection may have failed. Re-run setup:

```bash
cloister setup claude
cloister stop
cloister start
```

### Codex: "Authentication failed" inside cloister

1. Verify credentials are configured: `cloister setup codex`
2. Verify API key is valid at platform.openai.com
3. Restart the cloister after updating credentials

### Codex prompts for API key inside container

The credential injection may have failed. Re-run setup:

```bash
cloister setup codex
cloister stop
cloister start
```

## Next Steps

- [Getting Started](getting-started.md) — First cloister walkthrough
- [Configuration](configuration.md) — Global and per-project settings
