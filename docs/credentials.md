# Credentials

Cloister needs AI agent credentials to run tools like Claude Code inside containers. This guide covers authentication setup for supported agents.

## Claude Code

Claude Code supports three authentication methods. Cloister's setup wizard handles the configuration.

### Running the Setup Wizard

```bash
cloister setup claude
```

The wizard will:
1. Detect any existing Claude authentication
2. Prompt for your preferred method
3. Store credentials securely
4. Configure the container environment

### Authentication Methods

#### 1. OAuth Token (Recommended for Pro/Max)

Best for Claude Pro or Max subscribers:

```bash
# First, get a token from Claude
claude setup-token
# This opens a browser for OAuth and displays a token

# Then configure Cloister
cloister setup claude
# Choose "OAuth token" and paste the token
```

Tokens are valid for approximately one year.

#### 2. Existing Login Session

If you're already logged into Claude Code on the host:

```bash
cloister setup claude
# Choose "Use existing login"
```

Cloister copies the necessary session files into the container.

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

Credentials are stored in `~/.config/cloister/`:

```
~/.config/cloister/
├── config.yaml          # General config
├── credentials/
│   └── claude.yaml      # Claude credentials (encrypted)
└── projects/
    └── ...
```

### Security Notes

- Credentials are **not** mounted directly into containers
- The container receives only the authentication token/session needed
- Host credential files (`~/.anthropic/`, etc.) are never exposed
- Credentials in `~/.config/cloister/credentials/` have restricted permissions

## Container Environment

Inside the cloister, Claude Code is pre-configured:

```bash
cloister:my-app:/work$ claude --version
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

## Other Agents

<!-- TODO: Document additional agents as they're implemented -->

Support for additional AI coding agents is planned:

- **OpenAI Codex** — Coming soon
- **Gemini CLI** — Coming soon
- **Aider** — Coming soon

The setup pattern will be similar:

```bash
cloister setup <agent>
```

## Troubleshooting

### "Authentication failed" inside cloister

1. Verify credentials are configured: `cloister setup claude`
2. Check token hasn't expired
3. Restart the cloister after updating credentials

### Claude prompts for login inside container

The credential injection may have failed. Check:

```bash
# Inside cloister
echo $CLAUDE_AUTH_TOKEN
# Should show a value if using OAuth token
```

Re-run setup if needed.

## Next Steps

- [Getting Started](getting-started.md) — First cloister walkthrough
- [Configuration](configuration.md) — Global and per-project settings
