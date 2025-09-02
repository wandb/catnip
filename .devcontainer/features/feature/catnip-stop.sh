#!/usr/bin/env bash
set -Eeuo pipefail

log() { printf "[catnip] %s\n" "$*"; }
ok()  { printf "[catnip] ✅ %s\n" "$*"; }
warn(){ printf "[catnip] ⚠️  %s\n" "$*" >&2; }

OPT_DIR="/opt/catnip"

if [[ -f $OPT_DIR/catnip.pid ]]; then
  PID=$(cat $OPT_DIR/catnip.pid)
  
  # Check if the process actually exists
  if ! kill -0 $PID 2>/dev/null; then
    warn "PID $PID from $OPT_DIR/catnip.pid no longer exists, removing stale PID file"
    rm -f $OPT_DIR/catnip.pid
    exit 0
  fi
  
  log "catnip is already running, reinstalling and restarting it"
  
  # First try graceful termination with SIGTERM
  log "sending SIGTERM to catnip process (PID: $PID)..."
  kill $PID
  
  # Wait up to 3 seconds for graceful termination
  log "waiting for graceful termination..."
  for i in {1..3}; do
    if ! kill -0 $PID 2>/dev/null; then
      ok "process terminated gracefully"
      break
    fi
    sleep 1
  done
  
  # If still running, force kill with SIGKILL
  if kill -0 $PID 2>/dev/null; then
    warn "process still running, sending SIGKILL..."
    kill -9 $PID
    
    # Wait up to 7 more seconds for forced termination
    for i in {1..7}; do
      if ! kill -0 $PID 2>/dev/null; then
        ok "process killed with SIGKILL"
        break
      fi
      sleep 1
    done
  fi
  
  # Final check and cleanup
  if kill -0 $PID 2>/dev/null; then
    warn "process $PID still running after 10 seconds, removing PID file"
  else
    ok "process terminated successfully"
  fi
  rm -f $OPT_DIR/catnip.pid
else
  warn "$OPT_DIR/catnip.pid file not found, nothing to do"
fi