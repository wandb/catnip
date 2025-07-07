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