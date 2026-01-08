#!/usr/bin/env bash
# More defensive version - we want to reach `service catnip start` no matter what
set -u  # Treat unset variables as errors, but don't exit on command failures

OPT_DIR="/opt/catnip"
UPGRADE_LOG="$OPT_DIR/upgrade.log"

# Ensure log file exists and is writable
touch "$UPGRADE_LOG" 2>/dev/null || UPGRADE_LOG="/tmp/catnip-upgrade.log"

# Log to both stdout and file with timestamp
log() {
  local msg="[$(date '+%Y-%m-%d %H:%M:%S')] [catnip] $*"
  printf "%s\n" "$msg"
  printf "%s\n" "$msg" >> "$UPGRADE_LOG"
}
warn() {
  local msg="[$(date '+%Y-%m-%d %H:%M:%S')] [catnip] ⚠️  $*"
  printf "%s\n" "$msg" >&2
  printf "%s\n" "$msg" >> "$UPGRADE_LOG"
}
ok() {
  local msg="[$(date '+%Y-%m-%d %H:%M:%S')] [catnip] ✅ $*"
  printf "%s\n" "$msg"
  printf "%s\n" "$msg" >> "$UPGRADE_LOG"
}

# Mark start of this run
log "=========================================="
log "postStartCommand starting (PID $$)"
log "Running as user: $(id -un), uid: $(id -u)"
log "Working directory: $(pwd)"
log "=========================================="

# Ensure catnip is in PATH for upgrade command
export PATH="$OPT_DIR/bin:$HOME/.local/bin:$PATH"
log "PATH set to: $PATH"

# 1. Capture current environment to /etc/default/catnip
log "Step 1: Updating catnip service environment..."

if TEMP_ENV=$(mktemp 2>&1); then
  log "Created temp file: $TEMP_ENV"
  trap "rm -f $TEMP_ENV" EXIT

  # Use bash's declare -px to get properly quoted environment exports
  {
    echo ""
    echo "# ==================== RUNTIME ENVIRONMENT ===================="
    echo "# Updated from codespace environment at $(date)"
    echo "# This section is regenerated on each startup"
    echo ""
    declare -px
  } > "$TEMP_ENV"

  # Remove any previous runtime environment section and append the new one
  if sudo sed -i '/^# ==================== RUNTIME ENVIRONMENT ====================/,$d' /etc/default/catnip 2>&1; then
    if sudo cat "$TEMP_ENV" | sudo tee -a /etc/default/catnip >/dev/null 2>&1; then
      sudo chmod 644 /etc/default/catnip 2>/dev/null || true
      ok "Environment captured to /etc/default/catnip"
    else
      warn "Failed to append environment to /etc/default/catnip"
    fi
  else
    warn "Failed to update /etc/default/catnip (sed failed)"
  fi
else
  warn "Failed to create temp file: $TEMP_ENV"
fi

# 2. Attempt upgrade with timeout (non-blocking, failures are OK)
log "Step 2: Checking for catnip updates..."
if command -v catnip >/dev/null 2>&1; then
  if timeout 10s catnip upgrade --yes 2>&1 | while read -r line; do log "  $line"; done; then
    ok "Upgrade check completed"
  else
    EXIT_CODE=$?
    if [ $EXIT_CODE -eq 124 ]; then
      warn "Upgrade check timed out after 10s, proceeding with existing version"
    else
      warn "Upgrade check failed (exit $EXIT_CODE), proceeding with existing version"
    fi

    # If upgrade failed/timed-out, check if it left us with a backup but no binary
    log "Checking for orphaned backup files..."
    for BACKUP_PATH in "$HOME/.local/bin/catnip.backup" "$OPT_DIR/bin/catnip.backup"; do
      if [ -f "$BACKUP_PATH" ]; then
        BINARY_PATH="${BACKUP_PATH%.backup}"
        if [ ! -f "$BINARY_PATH" ]; then
          warn "Found backup at $BACKUP_PATH but no binary at $BINARY_PATH - restoring backup"
          if mv "$BACKUP_PATH" "$BINARY_PATH" && chmod +x "$BINARY_PATH"; then
            ok "Restored backup to $BINARY_PATH"
          else
            warn "Failed to restore backup"
          fi
        else
          log "Removing stale backup at $BACKUP_PATH"
          rm -f "$BACKUP_PATH" || true
        fi
      fi
    done
  fi
else
  warn "catnip command not found in PATH, skipping upgrade"
fi

# 3. Restart service (ALWAYS runs - this is the critical step)
# Use restart instead of start to ensure catnip picks up fresh environment from /etc/default/catnip
# This handles the case where the container was paused/resumed with catnip still running but stale env
log "Step 3: Restarting catnip service..."
if service catnip restart 2>&1 | while read -r line; do log "  $line"; done; then
  ok "service catnip restart completed"
else
  EXIT_CODE=$?
  warn "service catnip restart returned exit code $EXIT_CODE"
fi

# Final status check
log "Step 4: Verifying service status..."
if service catnip status 2>&1 | while read -r line; do log "  $line"; done; then
  ok "catnip service is running"
else
  warn "catnip service may not be running - check /opt/catnip/catnip.log for details"
fi

log "postStartCommand completed"
log "=========================================="
