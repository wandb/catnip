#!/bin/bash
set -Eeuo pipefail

# This script is run whenever the container is started ensuring the latest catnip code
# is installed and running.

# Update /etc/default/catnip with current environment variables
echo "Updating catnip service environment..."
sudo tee -a /etc/default/catnip >/dev/null <<EOF

# Updated with current codespace environment ($(date))
export GITHUB_TOKEN="${GITHUB_TOKEN:-}"
export GITHUB_USER="${GITHUB_USER:-}"
export CODESPACE_NAME="${CODESPACE_NAME:-}"
export GITHUB_REPOSITORY="${GITHUB_REPOSITORY:-}"
export CODESPACES="${CODESPACES:-}"
EOF

sudo service catnip stop
rm /home/vscode/.local/bin/catnip

echo "Installing latest catnip binary"
cd /workspaces/catnip/container && just install

echo "Restarting catnip service"
sudo service catnip start
