#!/usr/bin/env bash
set -Eeuo pipefail

VSIX_PATH="/opt/catnip/catnip-sidebar.vsix"

# Try the normal CLI if it's already on PATH
if command -v code >/dev/null 2>&1; then
  code --install-extension "$VSIX_PATH" --force
  exit 0
fi

# Fallback: locate the per-build VS Code server's CLI (works in Codespaces/VS Code)
CODE_BIN="$(find ~/.vscode-server -type f -path '*/bin/*/bin/code' -print -quit 2>/dev/null || true)"
if [ -n "${CODE_BIN:-}" ]; then
  "$CODE_BIN" --install-extension "$VSIX_PATH" --force
  exit 0
fi

echo "VS Code CLI not available yet; will try again next attach."