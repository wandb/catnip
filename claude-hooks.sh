#!/bin/bash

# Claude Code Hook Script for Catnip Activity Tracking
# This script should be installed at ~/.claude/hooks/hook.sh
# It will be called by Claude Code on various events with environment variables

# Exit early if we don't have the required environment variables
if [[ -z "$EVENT_TYPE" || -z "$PWD" ]]; then
    exit 0
fi

# Only handle the events we care about for activity tracking
case "$EVENT_TYPE" in
    "UserPromptSubmit"|"Stop")
        # Good, we want to track these events
        ;;
    *)
        # For other events, exit silently
        exit 0
        ;;
esac

# Default to localhost:8080 for the catnip server
CATNIP_HOST="${CATNIP_HOST:-localhost:8080}"

# Build the JSON payload
JSON_PAYLOAD=$(cat <<EOF
{
    "event_type": "$EVENT_TYPE",
    "working_directory": "$PWD"
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