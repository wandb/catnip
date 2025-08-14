#!/bin/bash

# Run the main setup script (it will handle installing pnpm and just if needed)
cd /workspaces/catnip && bash setup.sh

# Add directories needed by catnip
mkdir -p /home/vscode/.catnip/volume
sudo mkdir /catnip
sudo chown vscode:vscode /catnip