#!/bin/bash
# Pre-commit hook: runs 'make test-all' before any bash command that
# includes 'git commit'.  Blocks the tool call if tests fail.
#
# Receives JSON on stdin with tool_input.command containing the bash command.

set -euo pipefail

# If jq is not available, ask the user so they know the safety hook is broken.
if ! command -v jq &>/dev/null; then
    echo '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","permissionDecisionReason":"pre-commit-tests.sh: jq not found - test gate hook cannot function"}}'
    exit 0
fi

INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')
if [[ -z "$COMMAND" ]]; then
    exit 0
fi

# Check if the command contains 'git commit' anywhere (handles chained
# commands like "git add . && git commit -m ...").
if ! echo "$COMMAND" | grep -qE '\bgit\s+commit\b'; then
    exit 0
fi

# Check both staged and unstaged diffs for lint-cheating.
# Unstaged changes matter because "git add . && git commit" will stage them
# before committing, but this hook runs before the command executes.
ALL_DIRTY=$({ git diff --cached --name-only; git diff --name-only; } 2>/dev/null | sort -u)
if echo "$ALL_DIRTY" | grep -qE '(^|/)\.golangci\.(yml|yaml|toml)$'; then
    jq -n '{
        hookSpecificOutput: {
            hookEventName: "PreToolUse",
            permissionDecision: "deny",
            permissionDecisionReason: "Uncommitted changes to golangci-lint config detected — only humans may commit lint rule changes"
        }
    }'
    exit 0
fi

NOLINT_ADDS=$({ git diff --cached --diff-filter=AM -U0 -- '*.go'; git diff --diff-filter=AM -U0 -- '*.go'; } 2>/dev/null | grep -E '^\+.*//\s*nolint' | sort -u || true)
if [[ -n "$NOLINT_ADDS" ]]; then
    jq -n --arg lines "$NOLINT_ADDS" '{
        hookSpecificOutput: {
            hookEventName: "PreToolUse",
            permissionDecision: "deny",
            permissionDecisionReason: "Uncommitted changes add //nolint directives — fix the lint issues instead",
            additionalContext: ("Added nolint lines:\n" + $lines)
        }
    }'
    exit 0
fi

# Run fmt, lint, then tests — stop at first failure.
for target in fmt lint test-all; do
    if ! STEP_OUTPUT=$(make "$target" 2>&1); then
        jq -n --arg target "$target" --arg output "$STEP_OUTPUT" '{
            hookSpecificOutput: {
                hookEventName: "PreToolUse",
                permissionDecision: "deny",
                permissionDecisionReason: ("make " + $target + " failed - fix before committing"),
                additionalContext: ("Output:\n" + $output)
            }
        }'
        exit 0
    fi
done
