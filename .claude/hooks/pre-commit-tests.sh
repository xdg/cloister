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
