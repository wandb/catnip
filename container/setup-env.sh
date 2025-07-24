#!/bin/bash

# Parse PORTZ JSON array and set VITE_PORT to the first port if available
if [ -n "$PORTZ" ]; then
    # Use jq to parse the JSON array and get the first element
    FIRST_PORT=$(echo "$PORTZ" | jq -r '.[0] // empty')
    if [ -n "$FIRST_PORT" ] && [ "$FIRST_PORT" != "null" ]; then
        export VITE_PORT="$FIRST_PORT"
        echo "🌐 Set VITE_PORT to $VITE_PORT from PORTZ array"
    fi
fi

# Pass the PORT environment variable to the Go app
if [ -n "$PORT" ]; then
    echo "🔗 Using PORT=$PORT for Go server"
fi

# Execute the original command with all arguments
exec "$@"