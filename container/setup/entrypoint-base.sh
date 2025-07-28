#!/bin/bash
set -e

# Handle custom username if provided
if [ -n "$CATNIP_USERNAME" ] && [ "$CATNIP_USERNAME" != "catnip" ]; then
    # Change the username but keep the same UID and home directory
    usermod -l "$CATNIP_USERNAME" catnip 2>/dev/null || true
    # Update the group name too
    groupmod -n "$CATNIP_USERNAME" catnip 2>/dev/null || true
    # Unlock the account for SSH (set password to * which allows key auth only)
    usermod -p '*' "$CATNIP_USERNAME" 2>/dev/null || true
    # Export for tools
    export USER="$CATNIP_USERNAME"
    export USERNAME="$CATNIP_USERNAME"
fi

# Configure git for the user
if [ -n "$CATNIP_USERNAME" ]; then
    GIT_USERNAME="$CATNIP_USERNAME"
    GIT_EMAIL="${CATNIP_USERNAME}@catnip.run"
else
    GIT_USERNAME="catnip"
    GIT_EMAIL="catnip@catnip.run"
fi

# Override email if CATNIP_EMAIL is provided
if [ -n "$CATNIP_EMAIL" ]; then
    GIT_EMAIL="$CATNIP_EMAIL"
fi

# Set git config for the catnip user (not root) and mark as safe repo
gosu 1000:1000 git config --global user.name "$GIT_USERNAME"
gosu 1000:1000 git config --global user.email "$GIT_EMAIL"
gosu 1000:1000 git config --global init.defaultBranch main
gosu 1000:1000 git config --global --add safe.directory /workspace

# Ensure workspace has proper ownership
chown -R 1000:1000 "${WORKSPACE}" 2>/dev/null || true

# Change to catnip user if running as root
if [ "$EUID" -eq 0 ]; then
    # Use gosu for clean user switching without job control issues
    cd "${WORKSPACE}"
    # Use the actual username (which might have been changed)
    ACTUAL_USER=$(id -un 1000 2>/dev/null || echo "catnip")
    exec gosu "$ACTUAL_USER" "$@"
fi

# Execute the command
exec "$@"