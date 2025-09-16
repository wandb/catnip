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
# TODO: we'll likely want to forward other env vars here
export ANTHROPIC_API_KEY="${ANTHROPIC_API_KEY:-}"
export OPENAI_API_KEY="${OPENAI_API_KEY:-}"
EOF