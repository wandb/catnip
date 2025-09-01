#!/usr/bin/env bash
set -Eeuo pipefail

log() { printf "[catnip] %s\n" "$*"; }
ok()  { printf "[catnip] ✅ %s\n" "$*"; }
warn(){ printf "[catnip] ⚠️  %s\n" "$*" >&2; }

log "runner starting as $(id -un) uid=$(id -u) pwd=$PWD"

OPT_DIR="/opt/catnip"
export PATH="$OPT_DIR/bin:$HOME/.local/bin:$PATH"

# Use /opt/catnip for log/pid
LOG=$OPT_DIR/catnip.log
: > "$LOG"
log "PATH=$PATH" >> "$LOG"

export CATNIP_WORKSPACE_DIR=/worktrees
export CATNIP_HOME_DIR="$HOME"
export CATNIP_VOLUME_DIR="$OPT_DIR/state"
export CATNIP_LIVE_DIR=/workspaces

# Check for stale PID file before stopping
if [[ -f $OPT_DIR/catnip.pid ]]; then
  PID=$(cat $OPT_DIR/catnip.pid)
  if ! kill -0 $PID 2>/dev/null; then
    warn "PID $PID from $OPT_DIR/catnip.pid no longer exists, removing stale PID file"
    rm -f $OPT_DIR/catnip.pid
  else
    bash "$OPT_DIR/bin/catnip-stop.sh"
  fi
else
  bash "$OPT_DIR/bin/catnip-stop.sh"
fi

if command -v catnip >/dev/null 2>&1; then
  log "launching catnip with nohup"
  nohup catnip serve >>"$LOG" 2>&1 &
  echo $! > $OPT_DIR/catnip.pid
  log "catnip pid $(cat $OPT_DIR/catnip.pid)" >> "$LOG"
else
  warn "catnip not on PATH=$PATH" >> "$LOG"
fi