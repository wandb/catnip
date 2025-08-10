#!/bin/bash

# Claude Code Hook Script for Catnip Activity Tracking
# This script should be installed at ~/.claude/hooks/hook.sh
# It will be called by Claude Code with JSON data via stdin

# Read JSON input from stdin
INPUT_JSON=$(cat)

# Exit early if we don't have any input
if [[ -z "$INPUT_JSON" ]]; then
    exit 0
fi

# Extract hook_event_name and cwd from the JSON input
HOOK_EVENT_NAME=$(echo "$INPUT_JSON" | jq -r '.hook_event_name // empty')
CWD=$(echo "$INPUT_JSON" | jq -r '.cwd // empty')

# Only handle the events we care about for activity tracking
case "$HOOK_EVENT_NAME" in
    "UserPromptSubmit"|"Stop")
        # Good, we want to track these events
        ;;
    *)
        # For other events, exit silently
        exit 0
        ;;
esac

# Exit if we don't have required fields
if [[ -z "$HOOK_EVENT_NAME" || -z "$CWD" ]]; then
    exit 0
fi

# Default to localhost:8080 for the catnip server
CATNIP_HOST="${CATNIP_HOST:-localhost:8080}"

# Build the JSON payload for catnip
JSON_PAYLOAD=$(cat <<EOF
{
    "event_type": "$HOOK_EVENT_NAME",
    "working_directory": "$CWD"
}
EOF
)

# Send the hook event to catnip server
curl -s -X POST \
    -H "Content-Type: application/json" \
    -d "$JSON_PAYLOAD" \
    "http://$CATNIP_HOST/v1/claude/hooks" \
    >/dev/null 2>&1

# Exit successfully regardless of curl result to avoid breaking Claude
exit 0