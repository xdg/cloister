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

# Run the full test suite, capturing output for the deny response.
if TEST_OUTPUT=$(make test-all 2>&1); then
    exit 0
fi

# Tests failed - deny the commit with test output as context.
jq -n --arg output "$TEST_OUTPUT" '{
    hookSpecificOutput: {
        hookEventName: "PreToolUse",
        permissionDecision: "deny",
        permissionDecisionReason: "make test-all failed - fix tests before committing",
        additionalContext: ("Test output:\n" + $output)
    }
}'
