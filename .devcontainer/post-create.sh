#!/bin/bash

WORKDIR="${1:-$PWD}"   # fallback if not passed
USER="${_REMOTE_USER:-${USERNAME:-vscode}}"
OPT_DIR="/opt/catnip"

# ensure mounted volumes have proper permissions
sudo install -d -o "$USER" -g "$USER" "$WORKDIR/node_modules"
# Ensure /go/pkg exists and fix ownership only where needed
sudo install -d -o "$USER" -g "$USER" "/go/pkg"

# Only fix permissions if they're actually wrong (much faster than chmod -R)
if [ "$(stat -c '%U:%G' /go/pkg 2>/dev/null)" != "$USER:$USER" ]; then
  echo "Fixing /go/pkg permissions..."
  sudo chown "$USER:$USER" /go/pkg
fi

# Fix subdirectories only if they exist and have wrong ownership
if [ -d "/go/pkg/mod" ] && [ "$(stat -c '%U:%G' /go/pkg/mod 2>/dev/null)" != "$USER:$USER" ]; then
  echo "Fixing /go/pkg/mod permissions..."
  sudo chown -R "$USER:$USER" /go/pkg/mod
fi
sudo install -d -o "$USER" -g "$USER" "/home/vscode/.catnip/volume"

# Run the main setup script (it will handle installing pnpm and just if needed)
cd /workspaces/catnip && bash setup.sh


if [[ -f $OPT_DIR/catnip.pid ]]; then
  echo "catnip is already running, reinstalling and restarting it"
  PID=$(cat $OPT_DIR/catnip.pid)
  
  # First try graceful termination with SIGTERM
  echo "Sending SIGTERM to catnip process (PID: $PID)..."
  kill $PID
  
  # Wait up to 3 seconds for graceful termination
  echo "Waiting for graceful termination..."
  for i in {1..3}; do
    if ! kill -0 $PID 2>/dev/null; then
      echo "Process terminated gracefully"
      break
    fi
    sleep 1
  done
  
  # If still running, force kill with SIGKILL
  if kill -0 $PID 2>/dev/null; then
    echo "Process still running, sending SIGKILL..."
    kill -9 $PID
    
    # Wait up to 7 more seconds for forced termination
    for i in {1..7}; do
      if ! kill -0 $PID 2>/dev/null; then
        echo "Process killed with SIGKILL"
        break
      fi
      sleep 1
    done
  fi
  
  # Final check and cleanup
  if kill -0 $PID 2>/dev/null; then
    echo "Warning: Process $PID still running after 10 seconds, removing PID file"
  else
    echo "Process terminated successfully"
  fi
  rm -f $OPT_DIR/catnip.pid
  
  cd /workspaces/catnip/container && just install
else
  echo "catnip not already running!?"
  echo "ls -alh /opt/catnip"
  ls -alh /opt/catnip
fi

bash "$OPT_DIR/bin/catnip-run.sh"

# Add directories needed by catnip
mkdir -p /home/vscode/.catnip/volume
sudo mkdir -p /opt/catnip
sudo chown vscode:vscode /opt/catnip