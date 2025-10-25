#!/usr/bin/env bash
set -Eeuo pipefail

log() { printf "[catnip] %s\n" "$*"; }
warn() { printf "[catnip] ⚠️  %s\n" "$*" >&2; }
ok() { printf "[catnip] ✅ %s\n" "$*"; }

OPT_DIR="/opt/catnip"

# Ensure catnip is in PATH for upgrade command
export PATH="$OPT_DIR/bin:$HOME/.local/bin:$PATH"

# 1. Capture current environment to /etc/default/catnip
log "Updating catnip service environment..."
sudo tee -a /etc/default/catnip >/dev/null <<EOF

# Updated with current codespace environment ($(date))
$(printenv | sed 's/^/export /')
EOF
ok "Environment captured"

# 2. Attempt upgrade with timeout (non-blocking)
log "Checking for catnip updates..."
if timeout 10s catnip upgrade --yes; then
  ok "Upgrade check completed"
else
  EXIT_CODE=$?
  if [ $EXIT_CODE -eq 124 ]; then
    warn "Upgrade check timed out after 10s, proceeding with existing version"
  else
    warn "Upgrade check failed (exit $EXIT_CODE), proceeding with existing version"
  fi
fi

# 3. Start service (always runs regardless of upgrade outcome)
log "Starting catnip service..."
service catnip start
