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

  # If upgrade failed/timed-out, check if it left us with a backup but no binary
  # This can happen if timeout occurs after backup creation but before install
  log "Checking for orphaned backup files..."
  for BACKUP_PATH in "$HOME/.local/bin/catnip.backup" "$OPT_DIR/bin/catnip.backup"; do
    if [ -f "$BACKUP_PATH" ]; then
      BINARY_PATH="${BACKUP_PATH%.backup}"
      if [ ! -f "$BINARY_PATH" ]; then
        warn "Found backup at $BACKUP_PATH but no binary at $BINARY_PATH - restoring backup"
        mv "$BACKUP_PATH" "$BINARY_PATH"
        chmod +x "$BINARY_PATH"
        ok "Restored backup to $BINARY_PATH"
      else
        # Backup exists but so does binary - clean up backup
        log "Removing stale backup at $BACKUP_PATH"
        rm -f "$BACKUP_PATH"
      fi
    fi
  done
fi

# 3. Start service (always runs regardless of upgrade outcome)
# The entrypoint's background watcher waits for this script to update /etc/default/catnip
# before starting, so catnip will start with the correct environment.
log "Starting catnip service..."
service catnip start
