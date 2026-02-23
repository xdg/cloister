#!/bin/bash
# PreToolUse hook for Edit/Write: prevents the agent from bypassing lint rules.
#
# Blocks:
#   - Any edit/write that introduces "nolint" directives
#
# Receives JSON on stdin with tool_input fields (file_path, new_string, content).

set -euo pipefail

if ! command -v jq &>/dev/null; then
    echo '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","permissionDecisionReason":"no-lint-cheating.sh: jq not found - lint guard hook cannot function"}}'
    exit 0
fi

INPUT=$(cat)
TOOL=$(echo "$INPUT" | jq -r '.tool_name // empty')

# Extract the file path being edited/written.
if [[ "$TOOL" == "Edit" ]]; then
    FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')
    NEW_CONTENT=$(echo "$INPUT" | jq -r '.tool_input.new_string // empty')
elif [[ "$TOOL" == "Write" ]]; then
    FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')
    NEW_CONTENT=$(echo "$INPUT" | jq -r '.tool_input.content // empty')
else
    exit 0
fi

# Block additions of nolint directives in Go files.
if echo "$FILE_PATH" | grep -qE '\.go$'; then
    if echo "$NEW_CONTENT" | grep -qE '//\s*nolint'; then
        jq -n '{
            hookSpecificOutput: {
                hookEventName: "PreToolUse",
                permissionDecision: "deny",
                permissionDecisionReason: "Adding //nolint directives is not allowed — fix the lint issue instead"
            }
        }'
        exit 0
    fi
fi
