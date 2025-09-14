#!/usr/bin/env bash
set -Eeuo pipefail

# Update /etc/default/catnip with current environment variables
echo "Updating catnip service environment..."
sudo tee -a /etc/default/catnip >/dev/null <<EOF

# Updated with current codespace environment ($(date))
export GITHUB_TOKEN="${GITHUB_TOKEN:-}"
export GITHUB_USER="${GITHUB_USER:-}"
export CODESPACE_NAME="${CODESPACE_NAME:-}"
export GITHUB_REPOSITORY="${GITHUB_REPOSITORY:-}"
export CODESPACES="${CODESPACES:-}"
export PATH="${PATH:-}"
EOF