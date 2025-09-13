#!/bin/bash

# This script is run whenever the container is started ensuring the latest catnip code
# is installed and running.

echo "Installing latest catnip binary"
cd /workspaces/catnip/container && just install

echo "Restarting catnip systemd service"
systemctl --user restart catnip.service || bash "/opt/catnip/bin/catnip-run.sh"
