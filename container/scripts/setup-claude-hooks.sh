#!/bin/bash

# Setup Script for Claude Code Hooks Integration with Catnip
# This script installs the hook script and configures Claude Code to use it

set -e

CLAUDE_DIR="$HOME/.claude"
SETTINGS_FILE="$CLAUDE_DIR/settings.json"
HOOK_SCRIPT="$CLAUDE_DIR/catnip-activity-hook.sh"

echo "🔧 Setting up Claude Code hooks for improved activity tracking..."

# Create .claude directory if it doesn't exist
if [[ ! -d "$CLAUDE_DIR" ]]; then
    echo "📁 Creating .claude directory: $CLAUDE_DIR"
    mkdir -p "$CLAUDE_DIR"
fi

# Copy hook script to .claude directory
echo "📋 Installing hook script..."
cp "$(dirname "$0")/claude-hooks.sh" "$HOOK_SCRIPT"

# Make it executable
chmod +x "$HOOK_SCRIPT"

# Create or update settings.json
echo "⚙️ Configuring Claude Code settings..."

# Check if settings.json exists
if [[ -f "$SETTINGS_FILE" ]]; then
    echo "📝 Found existing settings.json, backing up..."
    cp "$SETTINGS_FILE" "$SETTINGS_FILE.backup"
    
    # Read existing settings
    EXISTING_SETTINGS=$(cat "$SETTINGS_FILE")
else
    echo "📝 Creating new settings.json..."
    EXISTING_SETTINGS="{}"
fi

# Create new settings with hooks configuration
NEW_SETTINGS=$(echo "$EXISTING_SETTINGS" | jq --arg hook_path "$HOOK_SCRIPT" '
    .hooks = {
        "UserPromptSubmit": [
            {
                "matcher": "*",
                "hooks": [
                    {
                        "type": "command",
                        "command": $hook_path
                    }
                ]
            }
        ],
        "Stop": [
            {
                "matcher": "*", 
                "hooks": [
                    {
                        "type": "command",
                        "command": $hook_path
                    }
                ]
            }
        ]
    }
')

# Write the updated settings
echo "$NEW_SETTINGS" > "$SETTINGS_FILE"

echo "✅ Claude hooks setup complete!"
echo ""
echo "📝 Hook script installed at: $HOOK_SCRIPT"
echo "⚙️ Settings configured in: $SETTINGS_FILE"
echo "🚀 Claude Code will now send activity events to catnip for improved status tracking"
echo ""
echo "💡 To customize the catnip server address, set the CATNIP_HOST environment variable:"
echo "   export CATNIP_HOST=your-server:6369"
echo ""
echo "🔍 To verify the installation:"
echo "   - Hook script: ls -la '$HOOK_SCRIPT'"
echo "   - Settings: cat '$SETTINGS_FILE' | jq .hooks"