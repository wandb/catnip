#!/bin/bash

# Test entrypoint for Catnip integration tests
# This script properly handles user switching and environment setup for tests

set -e

# Source the catnip profile to get all environment variables
source /etc/profile.d/catnip.sh

# Set up git configuration for the catnip user
if [[ "$EUID" -eq 0 ]]; then
    # Running as root, need to switch to catnip user properly
    
    # Ensure catnip user owns necessary directories
    chown -R catnip:catnip /opt/catnip/test /workspace 2>/dev/null || true
    chown -R catnip:catnip /tmp 2>/dev/null || true
    
    # Create catnip user's home directory if it doesn't exist
    if [[ ! -d /home/catnip ]]; then
        mkdir -p /home/catnip
        chown catnip:catnip /home/catnip
    fi
    
    # Switch to catnip user using gosu and execute the command
    exec gosu catnip "$@"
else
    # Already running as catnip user or another non-root user
    exec "$@"
fi