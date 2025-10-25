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

# Create a temporary file with properly quoted exports
TEMP_ENV=$(mktemp)
trap "rm -f $TEMP_ENV" EXIT

# Use bash's declare -px to get properly quoted environment exports
# This handles all special characters, quotes, newlines, etc. correctly
{
  echo ""
  echo "# ==================== RUNTIME ENVIRONMENT ===================="
  echo "# Updated from codespace environment at $(date)"
  echo "# This section is regenerated on each startup"
  echo ""

  # Export all current environment variables with proper shell quoting
  # declare -px outputs variables in a format safe for re-sourcing
  declare -px

} > "$TEMP_ENV"

# Remove any previous runtime environment section and append the new one
# This preserves the template but prevents duplicates
sudo sed -i '/^# ==================== RUNTIME ENVIRONMENT ====================/,$d' /etc/default/catnip
sudo cat "$TEMP_ENV" | sudo tee -a /etc/default/catnip >/dev/null
sudo chmod 644 /etc/default/catnip

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
