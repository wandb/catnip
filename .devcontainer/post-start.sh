#!/bin/bash

# This script is run whenever the container is started ensuring the latest catnip code
# is installed and running.

echo "Stopping main catnip server"
bash "/opt/catnip/bin/catnip-stop.sh"

echo "Installing latest catnip binary"
cd /workspaces/catnip/container && just install

echo "Starting catnip server"
bash "/opt/catnip/bin/catnip-run.sh"
