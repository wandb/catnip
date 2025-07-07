#!/bin/bash
set -e

# Source the catnip environment
source /etc/profile.d/catnip.sh

# Handle custom username if provided
if [ -n "$CATNIP_USERNAME" ] && [ "$CATNIP_USERNAME" != "catnip" ]; then
    # Change the username but keep the same UID and home directory
    usermod -l "$CATNIP_USERNAME" catnip 2>/dev/null || true
    # Update the group name too
    groupmod -n "$CATNIP_USERNAME" catnip 2>/dev/null || true
    # Export for starship and other tools
    export USER="$CATNIP_USERNAME"
    export USERNAME="$CATNIP_USERNAME"
fi

# Configure git for the user
if [ -n "$CATNIP_USERNAME" ]; then
    GIT_USERNAME="$CATNIP_USERNAME"
    GIT_EMAIL="${CATNIP_USERNAME}@catnip.run"
else
    GIT_USERNAME="whiskers"
    GIT_EMAIL="whiskers@catnip.run"
fi

# Override email if CATNIP_EMAIL is provided
if [ -n "$CATNIP_EMAIL" ]; then
    GIT_EMAIL="$CATNIP_EMAIL"
fi

# Set git config globally
git config --global user.name "$GIT_USERNAME"
git config --global user.email "$GIT_EMAIL"
git config --global init.defaultBranch main

# Install specific versions if requested via environment variables
if [ -n "$CATNIP_NODE_VERSION" ]; then
    echo "Installing Node.js version: $CATNIP_NODE_VERSION"
    source "$NVM_DIR/nvm.sh" && nvm install "$CATNIP_NODE_VERSION" && nvm use "$CATNIP_NODE_VERSION"
fi

if [ -n "$CATNIP_PYTHON_VERSION" ]; then
    if [ "$CATNIP_PYTHON_VERSION" != "system" ]; then
        echo "Installing Python version: $CATNIP_PYTHON_VERSION"
        # Use uv to install and manage Python versions
        uv python install "$CATNIP_PYTHON_VERSION"
        uv python pin "$CATNIP_PYTHON_VERSION"
    else
        echo "Using system Python: $(python3 --version)"
    fi
fi

if [ -n "$CATNIP_RUST_VERSION" ]; then
    echo "Installing Rust version: $CATNIP_RUST_VERSION"
    rustup install "$CATNIP_RUST_VERSION" && rustup default "$CATNIP_RUST_VERSION"
fi

if [ -n "$CATNIP_GO_VERSION" ]; then
    echo "Installing Go version: $CATNIP_GO_VERSION"
    # Would need to download and install different Go version
    echo "Note: Go version switching not yet implemented"
fi

# Initialize workspace directories
mkdir -p "${GOPATH}/bin" "${GOPATH}/src" "${GOPATH}/pkg"
mkdir -p "${WORKSPACE}/projects"

# Initialize volume directory for persistent data
if [ -d "/volume" ]; then
    echo "📁 Setting up persistent volume..."
    # Set permissions so catnip user can write to entire volume
    sudo chown -R 1000:1000 /volume 2>/dev/null || true
    sudo chmod -R 755 /volume 2>/dev/null || true
fi

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