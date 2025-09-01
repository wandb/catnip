#!/bin/bash

# This script is run at the end of the container creation process and would be cached
# in pre-builds.

WORKDIR="${1:-$PWD}"
USER="${_REMOTE_USER:-${USERNAME:-vscode}}"
OPT_DIR="/opt/catnip"

# Add directories needed by catnip
mkdir -p /home/vscode/.catnip/volume
sudo mkdir -p /opt/catnip
sudo chown vscode:vscode /opt/catnip

# ensure mounted volumes have proper permissions
sudo install -d -o "$USER" -g "$USER" "$WORKDIR/node_modules"
# Ensure /go/pkg exists and fix ownership only where needed
sudo install -d -o "$USER" -g "$USER" "/go/pkg"

# Only fix permissions if they're actually wrong (much faster than chmod -R)
if [ "$(stat -c '%U:%G' /go/pkg 2>/dev/null)" != "$USER:$USER" ]; then
  echo "Fixing /go/pkg permissions..."
  sudo chown "$USER:$USER" /go/pkg
fi

# Fix subdirectories only if they exist and have wrong ownership
if [ -d "/go/pkg/mod" ] && [ "$(stat -c '%U:%G' /go/pkg/mod 2>/dev/null)" != "$USER:$USER" ]; then
  echo "Fixing /go/pkg/mod permissions..."
  sudo chown -R "$USER:$USER" /go/pkg/mod
fi
sudo install -d -o "$USER" -g "$USER" "/home/vscode/.catnip/volume"

# Run the main setup script (it will handle installing pnpm and just if needed)
cd /workspaces/catnip && bash setup.sh