#!/bin/bash

# Check for git lock files in current repo and worktrees

echo "Checking for git lock files..."

# Check local .git directory
if [ -f ".git/index.lock" ]; then
    echo "❌ Found local index lock: .git/index.lock"
else
    echo "✅ No local index lock found"
fi

# Check worktree lock files
if [ -f "/live/catnip/.git/worktrees/teleport-quasar/index.lock" ]; then
    echo "❌ Found worktree index lock: /live/catnip/.git/worktrees/teleport-quasar/index.lock"
else
    echo "✅ No worktree index lock found"
fi

# Check for any other lock files in .git directory
lock_files=$(find .git -name "*.lock" -type f 2>/dev/null)
if [ -n "$lock_files" ]; then
    echo "❌ Other lock files found:"
    echo "$lock_files"
else
    echo "✅ No other lock files found"
fi

echo "Done checking git locks."