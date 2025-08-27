#!/bin/bash

# This script is run whenever the container is started ensuring the latest catnip code
# is installed and running.

echo "Stopping main catnip server"
bash "/opt/catnip/bin/catnip-stop.sh"

echo "Installing latest catnip binary"
cd /workspaces/catnip/container && just install

LOG=/opt/catnip/catnip.log
: > "$LOG"

echo "Starting catnip dev server"
# start catnip dev server
cd /workspaces/catnip
nohup just dev >>"$LOG" 2>&1 &
echo $! > /opt/catnip/catnip.pid
echo "catnip dev server started, pid: $(cat /opt/catnip/catnip.pid)" >> "$LOG"
