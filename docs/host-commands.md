# Host Commands

Cloisters are isolated from the host system. When an AI agent needs to run commands that affect the host (like `git push` or `docker`), it uses **hostexec** — a controlled escape hatch with human approval.

## Why Hostexec Exists

Inside a cloister:
- No SSH keys or git credentials (can't push directly)
- No Docker socket (can't manage host containers)
- No access to host commands or filesystems

This is by design. When legitimate host access is needed, `hostexec` routes the request through the guardian for approval.

## Using Hostexec

From inside a cloister:

```bash
cloister:my-app:/work$ hostexec git push origin main
```

What happens:
1. Request sent to guardian
2. Guardian checks auto-approve patterns
3. If no match, request appears in approval UI
4. Human approves or denies
5. Command runs on host (in the project directory)
6. Output returned to container

## The Approval UI

Open http://localhost:9999 to see pending requests:

```
┌─────────────────────────────────────────────────────────┐
│  Pending Requests                                        │
├─────────────────────────────────────────────────────────┤
│  my-app │ git push origin feature-branch                │
│         │ [Approve] [Deny] [Approve & Save Pattern]     │
│                                                          │
│  frontend │ docker compose up -d                        │
│           │ [Approve] [Deny] [Approve & Save Pattern]   │
└─────────────────────────────────────────────────────────┘
```

### Approval Options

- **Approve** — Run this command once
- **Deny** — Reject this request
- **Approve & Save Pattern** — Run and remember (prompts for scope)

### Pattern Scopes

When saving a pattern:
- **Session** — Valid until cloister stops
- **Project** — Saved to project config
- **Global** — Saved to global config

## Auto-Approve Patterns

Configure patterns to approve automatically without UI interaction:

```yaml
# ~/.config/cloister/config.yaml
hostexec:
  auto_approve:
    - "^go mod tidy$"
    - "^npm install$"
    - "^git status$"
    - "^git diff"
```

Patterns are Go regular expressions matched against the command string.

### Pattern Examples

```yaml
auto_approve:
  # Exact command only
  - "^go mod tidy$"

  # Command with any arguments
  - "^npm install"

  # Specific docker compose commands
  - "^docker compose (up|down|ps|logs)"

  # Git operations on specific remote
  - "^git push origin"

  # Allow any git command (use with caution)
  - "^git "
```

## Auto-Deny Patterns

Block dangerous commands without prompting:

```yaml
hostexec:
  auto_deny:
    - "^rm -rf /$"
    - "^rm -rf /home$"
    - "^chmod -R 777"
    - "^curl .* | bash$"
```

## Common Use Cases

### Git Push

```bash
cloister:my-app:/work$ hostexec git push origin feature-branch
```

Git credentials are on the host, so push must go through hostexec.

### Docker Commands

```bash
cloister:my-app:/work$ hostexec docker compose up -d
cloister:my-app:/work$ hostexec docker compose logs api
```

Docker socket isn't in the container; docker commands run on host.

### Package Managers (Host-level)

```bash
cloister:my-app:/work$ hostexec brew install jq
```

Installing tools on the host (not in container).

## Execution Context

Host commands run:
- In the project's worktree directory on the host
- With the host user's environment
- With access to host credentials and tools

## Timeouts

Hostexec requests timeout after 5 minutes of waiting for approval. The AI agent receives a timeout error.

Long-running commands (after approval) have a separate execution timeout.

## Security Considerations

- Commands execute with your host user's full permissions
- Review requests carefully, especially:
  - Commands with pipes or redirects
  - Commands accessing files outside the project
  - Network-related commands (curl, wget)
- Use auto-deny for patterns you never want to run

## Troubleshooting

### Request not appearing in UI

1. Verify guardian is running: `cloister guardian status`
2. Check the cloister token is registered
3. Ensure you're using `hostexec`, not running directly

### Command denied unexpectedly

Check auto-deny patterns in your config:

```bash
cloister config show
```

### Slow approval

If the UI is slow to update, check browser console for SSE connection issues. Refresh the page if needed.

## Next Steps

- [Configuration](configuration.md) — Setting up patterns
- [Command Reference](command-reference.md) — Full CLI documentation
