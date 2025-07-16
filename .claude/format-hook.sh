#!/usr/bin/env bash
set -euo pipefail

# Debug: Log that hook is being executed
echo "🔧 Claude format hook executing..." >&2

# Extract file path from Claude input
FILE_PATH=$(echo "$CLAUDE_INPUT" | jq -r '.file_path // .path // empty' 2>/dev/null || true)

# Debug: Log the file path
echo "🔧 Hook processing file: $FILE_PATH" >&2

# Exit if no file path found
if [ -z "$FILE_PATH" ]; then
    echo "🔧 No file path found, exiting" >&2
    exit 0
fi

# Convert relative path to absolute if needed
if [[ "$FILE_PATH" != /* ]]; then
    PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
    FILE_PATH="$PROJECT_ROOT/$FILE_PATH"
fi

# Check if file exists
if [ ! -f "$FILE_PATH" ]; then
    echo "🔧 File does not exist: $FILE_PATH, exiting" >&2
    exit 0
fi

# Get file extension
EXT=$(echo "$FILE_PATH" | grep -oE '\.[^.]+$' || true)

# Get the project root
PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || dirname "$FILE_PATH")

# Format based on file type
case "$EXT" in
    .ts|.tsx|.js|.jsx|.json|.css|.md)
        # Check if we're in the project with pnpm/prettier
        if [ -f "$PROJECT_ROOT/package.json" ] && command -v pnpm >/dev/null 2>&1; then
            echo "🎨 Auto-formatting $FILE_PATH with Prettier (via pnpm)..."
            (cd "$PROJECT_ROOT" && pnpm prettier --write "$FILE_PATH" 2>/dev/null) || true
        elif command -v prettier >/dev/null 2>&1; then
            echo "🎨 Auto-formatting $FILE_PATH with Prettier (global)..."
            prettier --write "$FILE_PATH" 2>/dev/null || true
        else
            echo "⚠️  Prettier not found, skipping formatting for $FILE_PATH"
        fi
        ;;
    .go)
        # gofmt comes with Go installation, but let's check anyway
        if command -v gofmt >/dev/null 2>&1; then
            echo "🎨 Auto-formatting $FILE_PATH with gofmt..."
            gofmt -w -s "$FILE_PATH" 2>/dev/null || true
        elif command -v go >/dev/null 2>&1; then
            # Try using go fmt as fallback (doesn't support -s flag though)
            echo "🎨 Auto-formatting $FILE_PATH with go fmt..."
            go fmt "$FILE_PATH" 2>/dev/null || true
        else
            echo "⚠️  Go toolchain not found, skipping formatting for $FILE_PATH"
        fi
        ;;
    *)
        # No formatting for other file types
        ;;
esac