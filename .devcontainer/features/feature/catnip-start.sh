#!/usr/bin/env bash
set -Eeuo pipefail

log() { printf "[catnip] %s\n" "$*"; }
ok()  { printf "[catnip] ✅ %s\n" "$*"; }
warn(){ printf "[catnip] ⚠️  %s\n" "$*" >&2; }

log "runner starting as $(id -un) uid=$(id -u) pwd=$PWD"

OPT_DIR="/opt/catnip"
export PATH="$OPT_DIR/bin:$HOME/.local/bin:$PATH"

# Set CATNIP_VOLUME_DIR if not already set
if [[ -z "${CATNIP_VOLUME_DIR:-}" ]]; then
  export CATNIP_VOLUME_DIR="$OPT_DIR/state"
fi

# Create CATNIP_VOLUME_DIR if it doesn't exist
# Use sudo for /workspaces paths (may have different ownership locally vs Codespaces)
# then fix ownership to ensure current user can write to it
if [[ ! -d "$CATNIP_VOLUME_DIR" ]]; then
  log "creating CATNIP_VOLUME_DIR: $CATNIP_VOLUME_DIR"
  if [[ "$CATNIP_VOLUME_DIR" == /workspaces/* ]]; then
    sudo mkdir -p "$CATNIP_VOLUME_DIR"
    sudo chown -R "$(id -u):$(id -g)" "$CATNIP_VOLUME_DIR"
    # Also fix parent .catnip dir if we created it
    CATNIP_PARENT="$(dirname "$CATNIP_VOLUME_DIR")"
    if [[ -d "$CATNIP_PARENT" ]]; then
      sudo chown "$(id -u):$(id -g)" "$CATNIP_PARENT"
    fi
  else
    mkdir -p "$CATNIP_VOLUME_DIR"
  fi
fi

# Use /opt/catnip for log/pid
LOG="$OPT_DIR/catnip.log"
: > "$LOG"
log "PATH=$PATH" >> "$LOG"
log "CATNIP_VOLUME_DIR=$CATNIP_VOLUME_DIR" >> "$LOG"

export CATNIP_WORKSPACE_DIR=/worktrees
export CATNIP_HOME_DIR="$HOME"
export CATNIP_LIVE_DIR=/workspaces

# Check if catnip is already running
if [[ -f "$OPT_DIR/catnip.pid" ]]; then
  PID=$(cat "$OPT_DIR/catnip.pid")
  if kill -0 $PID 2>/dev/null; then
    ok "catnip already running with PID $PID"
    exit 0
  else
    warn "PID $PID from $OPT_DIR/catnip.pid no longer exists, removing stale PID file"
    sudo rm -f "$OPT_DIR/catnip.pid"
  fi
fi

# Stop any existing catnip processes
bash "$OPT_DIR/bin/catnip-stop.sh"

# Check if GITHUB_TOKEN is set
if [[ -n "${GITHUB_TOKEN:-}" ]]; then
  log "GITHUB_TOKEN is set" >> "$LOG"
else
  log "GITHUB_TOKEN is not set" >> "$LOG"
fi

if command -v catnip >/dev/null 2>&1; then
  log "launching catnip with nohup"
  nohup catnip serve >>"$LOG" 2>&1 &
  PID=$!
  echo $PID | sudo tee "$OPT_DIR/catnip.pid" >/dev/null
  sudo chown root:root "$OPT_DIR/catnip.pid"
  sudo chmod 644 "$OPT_DIR/catnip.pid"
  log "catnip pid $PID" >> "$LOG"
else
  warn "catnip not on PATH=$PATH" >> "$LOG"
fi