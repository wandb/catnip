#!/bin/bash

# Setup Script for Claude Code Hooks Integration with Catnip
# This script installs the hook script that enables improved activity tracking

set -e

HOOKS_DIR="$HOME/.claude/hooks"
HOOK_SCRIPT="$HOOKS_DIR/hook.sh"

echo "🔧 Setting up Claude Code hooks for improved activity tracking..."

# Create hooks directory if it doesn't exist
if [[ ! -d "$HOOKS_DIR" ]]; then
    echo "📁 Creating hooks directory: $HOOKS_DIR"
    mkdir -p "$HOOKS_DIR"
fi

# Copy hook script
echo "📋 Installing hook script..."
cp "$(dirname "$0")/claude-hooks.sh" "$HOOK_SCRIPT"

# Make it executable
chmod +x "$HOOK_SCRIPT"

echo "✅ Claude hooks setup complete!"
echo ""
echo "📝 The hook script has been installed at: $HOOK_SCRIPT"
echo "🚀 Claude Code will now send activity events to catnip for improved status tracking"
echo ""
echo "💡 To customize the catnip server address, set the CATNIP_HOST environment variable:"
echo "   export CATNIP_HOST=your-server:8080"
echo ""
echo "🔍 To verify the installation, check that the file exists and is executable:"
echo "   ls -la '$HOOK_SCRIPT'"