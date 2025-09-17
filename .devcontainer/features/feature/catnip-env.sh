#!/usr/bin/env bash
set -Eeuo pipefail

# Update /etc/default/catnip with current environment variables
echo "Updating catnip service environment..."
sudo tee -a /etc/default/catnip >/dev/null <<EOF

# Updated with current codespace environment ($(date))
$(printenv | sed 's/^/export /')
EOF