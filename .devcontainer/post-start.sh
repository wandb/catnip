#!/bin/bash

# This script is run whenever the container is started ensuring the latest catnip code
# is installed and running.

echo "Stopping main catnip server"
bash "/opt/catnip/bin/catnip-stop.sh"

echo "Installing latest catnip binary"
cd /workspaces/catnip/container && just install

LOG=/opt/catnip/catnip.log
PIDFILE=/opt/catnip/catnip.pid
: > "$LOG"

echo "Starting catnip dev server"
# start catnip dev server
cd /workspaces/catnip

# Start the dev server in background
nohup just dev >>"$LOG" 2>&1 &
DEV_PID=$!
echo $DEV_PID > "$PIDFILE"

# Wait a moment for the process to start
sleep 2

# Check if the process is still running
if kill -0 $DEV_PID 2>/dev/null; then
    echo "✅ catnip dev server started successfully, pid: $DEV_PID"
    echo "catnip dev server started successfully, pid: $DEV_PID" >> "$LOG"
    
    # Wait a bit longer and check again to catch early failures
    sleep 3
    if kill -0 $DEV_PID 2>/dev/null; then
        echo "✅ Dev server confirmed running after 5 seconds"
    else
        echo "❌ Dev server died shortly after starting - check logs:"
        tail -20 "$LOG"
        exit 1
    fi
else
    echo "❌ Failed to start catnip dev server - check logs:"
    tail -20 "$LOG"
    exit 1
fi

# Show recent log output for debugging
echo "Recent dev server output:"
tail -10 "$LOG"
