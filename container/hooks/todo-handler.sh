#!/bin/bash

# todo-handler.sh - Handle todo completion and progress updates
# This script is called by Claude when todo items are processed

# Read the todo data from stdin (jq output)
TODO_DATA=$(cat)

# Log the received data for debugging
echo "$(date): Processing todo data: $TODO_DATA" >> ~/.claude/todo-handler.log

# Parse the todo data using jq
TODO_STATUS=$(echo "$TODO_DATA" | jq -r '.tool_input.todos[]?.status // empty' | head -1)
TODO_CONTENT=$(echo "$TODO_DATA" | jq -r '.tool_input.todos[]?.content // empty' | head -1)

# If no todo data found, try alternative parsing
if [ -z "$TODO_STATUS" ] || [ -z "$TODO_CONTENT" ]; then
    # Alternative parsing for different data structures
    TODO_STATUS=$(echo "$TODO_DATA" | jq -r '.status // empty')
    TODO_CONTENT=$(echo "$TODO_DATA" | jq -r '.content // empty')
fi

# Get current git branch
CURRENT_BRANCH=$(git branch --show-current 2>/dev/null || echo "unknown")

# Get current workspace (current directory)
WORKSPACE=$(pwd)

echo "$(date): Todo status: $TODO_STATUS, Content: $TODO_CONTENT, Branch: $CURRENT_BRANCH" >> ~/.claude/todo-handler.log

# Handle different todo statuses
case "$TODO_STATUS" in
    "completed")
        echo "$(date): Processing completed todo: $TODO_CONTENT" >> ~/.claude/todo-handler.log
        
        # Add all changes to git
        git add .
        
        # Commit with the completed todo message
        git commit -m "completed: $TODO_CONTENT"
        
        if [ $? -eq 0 ]; then
            echo "$(date): Successfully committed completed todo: $TODO_CONTENT" >> ~/.claude/todo-handler.log
        else
            echo "$(date): Failed to commit completed todo: $TODO_CONTENT" >> ~/.claude/todo-handler.log
        fi
        ;;
        
    "in_progress")
        echo "$(date): Processing in-progress todo: $TODO_CONTENT" >> ~/.claude/todo-handler.log
        
        # Get workspace name for the API call
        WORKSPACE_NAME=$(basename "$WORKSPACE")
        
        # Make API call to UpdateSessionStatus endpoint (no need to look up session ID)
        API_RESPONSE=$(curl -s -X POST "http://localhost:8080/v1/sessions/workspace/${WORKSPACE_NAME}/status" \
            -H "Content-Type: application/json" \
            -d "{\"branch\": \"$CURRENT_BRANCH\", \"todo\": \"$TODO_CONTENT\", \"status\": \"in_progress\"}")
        
        if [ $? -eq 0 ]; then
            echo "$(date): Successfully updated session status - Response: $API_RESPONSE" >> ~/.claude/todo-handler.log
        else
            echo "$(date): Failed to update session status for todo: $TODO_CONTENT" >> ~/.claude/todo-handler.log
        fi
        ;;
        
    *)
        echo "$(date): Unknown or empty todo status: $TODO_STATUS" >> ~/.claude/todo-handler.log
        ;;
esac

# Always log the original command for reference
echo "$TODO_DATA" >> ~/.claude/bash-command-log.txt 