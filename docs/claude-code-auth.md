# Claude Code Docker Authentication: Keychain Credential Extraction

## Problem

Claude Code on macOS stores OAuth credentials in the system Keychain, not on disk. Claude Code on Linux expects credentials in `~/.claude/.credentials.json`. When running Claude Code in a Docker container (Linux) from a macOS host, we need to bridge this gap.

## Solution

Extract credentials from macOS Keychain and write them to the container's filesystem in the format Linux Claude Code expects.

## Keychain Entry Details

- **Service name**: `Claude Code-credentials`
- **Account**: User's macOS username (e.g., `xdg`)
- **Extraction command**:
  ```bash
  security find-generic-password -s 'Claude Code-credentials' -a "$(whoami)" -w
  ```

## Credential Format

The keychain stores a JSON blob with this structure:

```json
{
  "claudeAiOauth": {
    "accessToken": "sk-ant-oat01-...",
    "refreshToken": "sk-ant-ort01-...",
    "expiresAt": 1769753311584,
    "scopes": ["user:inference", "user:mcp_servers", "user:profile", "user:sessions:claude_code"],
    "subscriptionType": "max",
    "rateLimitTier": "default_claude_max_5x"
  }
}
```

## Target File Location

Linux Claude Code reads credentials from:
```
~/.claude/.credentials.json
```

Note: The filename has a leading dot (`.credentials.json`), inside the `.claude` directory.

## Important Notes

1. **Don't share `~/.claude` directly between Mac and container**: Mac's Claude Code deletes `.credentials.json` because it uses Keychain instead. Use a separate directory for container credentials.

2. **Token refresh**: The `accessToken` is short-lived but Claude Code should auto-refresh using the `refreshToken`. If refresh fails, re-extract from Keychain.

3. **Alternative approach**: `claude setup-token` generates a long-lived token for CI/CD use, passed via `CLAUDE_CODE_OAUTH_TOKEN` env var. This is simpler but requires manual token generation and doesn't reuse existing login.

## Error Handling

- If `security find-generic-password` fails, the user likely hasn't logged into Claude Code on Mac
- Exit with clear error message directing user to run `claude login` first
