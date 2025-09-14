#!/bin/bash
set -Eeuo pipefail

# This script is run whenever the container is started ensuring the latest catnip code
# is installed and running.

sudo service catnip stop
rm /home/vscode/.local/bin/catnip

echo "Installing latest catnip binary"
cd /workspaces/catnip/container && just install

echo "Restarting catnip service"
sudo service catnip start
