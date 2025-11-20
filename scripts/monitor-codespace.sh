#!/bin/bash
set -euo pipefail

# Usage: ./monitor-codespace.sh <codespace-name>
# Example: ./monitor-codespace.sh reimagined-parakeet-qgv7wcx6p5

CODESPACE_NAME="${1:-}"

if [ -z "$CODESPACE_NAME" ]; then
    echo "Usage: $0 <codespace-name>"
    echo "Example: $0 reimagined-parakeet-qgv7wcx6p5"
    exit 1
fi

echo "🔍 Monitoring codespace: $CODESPACE_NAME"
echo "Will check every 30 seconds until state changes from 'Available'"
echo "Press Ctrl+C to stop"
echo ""

# Track iterations
ITERATION=0

while true; do
    ITERATION=$((ITERATION + 1))

    # Get current timestamp (ISO 8601 format with timezone)
    TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S %z')

    # Fetch codespace status
    echo "[$TIMESTAMP] Check #$ITERATION"

    JSON_RESPONSE=$(gh cs view -c "$CODESPACE_NAME" --json state,lastUsedAt,gitStatus 2>&1)

    # Check if command succeeded
    if [ $? -ne 0 ]; then
        echo "❌ Failed to fetch codespace status:"
        echo "$JSON_RESPONSE"
        exit 1
    fi

    # Parse state using jq (fallback to grep if jq not available)
    if command -v jq &> /dev/null; then
        STATE=$(echo "$JSON_RESPONSE" | jq -r '.state')
        LAST_USED=$(echo "$JSON_RESPONSE" | jq -r '.lastUsedAt')

        echo "   State: $STATE"
        echo "   Last used: $LAST_USED"
        echo "   Full response: $JSON_RESPONSE"
    else
        # Fallback if jq is not available
        STATE=$(echo "$JSON_RESPONSE" | grep -o '"state":"[^"]*"' | cut -d'"' -f4)
        echo "   State: $STATE"
        echo "   Full response: $JSON_RESPONSE"
    fi

    # Check if state changed from Available
    if [ "$STATE" != "Available" ]; then
        echo ""
        echo "🎯 STATE CHANGED!"
        echo "⏰ Detected at: $TIMESTAMP"
        echo "📊 New state: $STATE"
        echo ""
        echo "Full response:"
        echo "$JSON_RESPONSE"
        exit 0
    fi

    echo ""

    # Wait 30 seconds before next check
    sleep 30
done
