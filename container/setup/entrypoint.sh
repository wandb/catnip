#!/bin/bash
set -e

# Source the catnip environment
source /etc/profile.d/catnip.sh

# Handle custom username if provided
if [ -n "$CATNIP_USERNAME" ] && [ "$CATNIP_USERNAME" != "catnip" ]; then
    # Change the username but keep the same UID and home directory
    usermod -l "$CATNIP_USERNAME" catnip 2>/dev/null || true
    # Update the group name too
    groupmod -n "$CATNIP_USERNAME" catnip 2>/dev/null || true
    # Unlock the account for SSH (set password to * which allows key auth only)
    usermod -p '*' "$CATNIP_USERNAME" 2>/dev/null || true
    # Export for starship and other tools
    export USER="$CATNIP_USERNAME"
    export USERNAME="$CATNIP_USERNAME"
fi

# Configure git for the user
if [ -n "$CATNIP_USERNAME" ]; then
    GIT_USERNAME="$CATNIP_USERNAME"
    GIT_EMAIL="${CATNIP_USERNAME}@catnip.run"
else
    GIT_USERNAME="whiskers"
    GIT_EMAIL="whiskers@catnip.run"
fi

# Override email if CATNIP_EMAIL is provided
if [ -n "$CATNIP_EMAIL" ]; then
    GIT_EMAIL="$CATNIP_EMAIL"
fi

# Copy any existing root configuration to catnip user home
# This handles cases where setup might have accidentally created files in /root
if [ -f "/root/.gitconfig" ] && [ ! -f "/home/catnip/.gitconfig" ]; then
    echo "📋 Migrating git config from /root to /home/catnip"
    cp /root/.gitconfig /home/catnip/.gitconfig 2>/dev/null || true
    chown 1000:1000 /home/catnip/.gitconfig 2>/dev/null || true
fi

# Copy other common configuration files that might be in /root
for config_dir in .ssh .aws .config .local .cargo .rustup; do
    if [ -d "/root/$config_dir" ] && [ ! -d "/home/catnip/$config_dir" ]; then
        echo "📋 Migrating $config_dir from /root to /home/catnip"
        cp -r "/root/$config_dir" "/home/catnip/" 2>/dev/null || true
        chown -R 1000:1000 "/home/catnip/$config_dir" 2>/dev/null || true
    fi
done

# Set git config for the catnip user (not root) and mark as safe repo
gosu 1000:1000 git config --global user.name "$GIT_USERNAME"
gosu 1000:1000 git config --global user.email "$GIT_EMAIL"
gosu 1000:1000 git config --global init.defaultBranch main
# TODO: This should be tightly scoped to mounted volume. Bad!
gosu 1000:1000 git config --global --add safe.directory "*"

# Install specific versions if requested via environment variables (run as catnip user)
if [ -n "$CATNIP_NODE_VERSION" ]; then
    echo "Installing Node.js version: $CATNIP_NODE_VERSION"
    gosu 1000:1000 bash -c 'source /etc/profile.d/catnip.sh && source "$NVM_DIR/nvm.sh" && nvm install "$CATNIP_NODE_VERSION" && nvm use "$CATNIP_NODE_VERSION"'
fi

if [ -n "$CATNIP_PYTHON_VERSION" ]; then
    if [ "$CATNIP_PYTHON_VERSION" != "system" ]; then
        echo "Installing Python version: $CATNIP_PYTHON_VERSION"
        # Use uv to install and manage Python versions as catnip user
        gosu 1000:1000 bash -c 'source /etc/profile.d/catnip.sh && uv python install "$CATNIP_PYTHON_VERSION" && uv python pin "$CATNIP_PYTHON_VERSION"'
    else
        echo "Using system Python: $(python3 --version)"
    fi
fi

if [ -n "$CATNIP_RUST_VERSION" ]; then
    echo "Installing Rust version: $CATNIP_RUST_VERSION"
    gosu 1000:1000 bash -c 'source /etc/profile.d/catnip.sh && rustup install "$CATNIP_RUST_VERSION" && rustup default "$CATNIP_RUST_VERSION"'
fi

if [ -n "$CATNIP_GO_VERSION" ]; then
    echo "Installing Go version: $CATNIP_GO_VERSION"
    # Would need to download and install different Go version
    echo "Note: Go version switching not yet implemented"
fi

# Initialize workspace directories as catnip user
gosu 1000:1000 mkdir -p "${GOPATH}/bin" "${GOPATH}/src" "${GOPATH}/pkg"
gosu 1000:1000 mkdir -p "${WORKSPACE}/projects"

# Ensure Go workspace has proper permissions for catnip user (redundant but safe)
chown -R 1000:1000 "${GOPATH}" 2>/dev/null || true

# Initialize volume directory for persistent data
if [ -d "/volume" ]; then
    echo "📁 Setting up persistent volume..."
    # Set permissions so catnip user can write to entire volume
    sudo chown -R 1000:1000 /volume 2>/dev/null || true
    sudo chmod -R 755 /volume 2>/dev/null || true
fi

# Ensure workspace has proper ownership
chown -R 1000:1000 "${WORKSPACE}" 2>/dev/null || true

# Configure and start SSH server if enabled
if [ "$CATNIP_SSH_ENABLED" = "true" ] && [ -f "/home/catnip/.ssh/catnip_remote.pub" ]; then
    echo "🔑 Configuring SSH server..."
    
    # Create SSH host keys if they don't exist
    if [ ! -f "/etc/ssh/ssh_host_rsa_key" ]; then
        ssh-keygen -t rsa -f /etc/ssh/ssh_host_rsa_key -N '' >/dev/null 2>&1
    fi
    if [ ! -f "/etc/ssh/ssh_host_ecdsa_key" ]; then
        ssh-keygen -t ecdsa -f /etc/ssh/ssh_host_ecdsa_key -N '' >/dev/null 2>&1
    fi
    if [ ! -f "/etc/ssh/ssh_host_ed25519_key" ]; then
        ssh-keygen -t ed25519 -f /etc/ssh/ssh_host_ed25519_key -N '' >/dev/null 2>&1
    fi
    
    # Copy the public key to authorized_keys
    gosu 1000:1000 cp /home/catnip/.ssh/catnip_remote.pub /home/catnip/.ssh/authorized_keys
    gosu 1000:1000 chmod 600 /home/catnip/.ssh/authorized_keys
    
    # Determine the actual username
    ACTUAL_USERNAME="${CATNIP_USERNAME:-catnip}"
    
    # Configure SSH daemon with proper home directory
    cat > /etc/ssh/sshd_config.d/99-catnip.conf <<EOF
Port 2222
PubkeyAuthentication yes
PasswordAuthentication no
ChallengeResponseAuthentication no
UsePAM no
AllowUsers ${ACTUAL_USERNAME}
X11Forwarding yes
PermitUserEnvironment yes
AcceptEnv LANG LC_*
Subsystem sftp /usr/lib/openssh/sftp-server
AuthorizedKeysFile /home/catnip/.ssh/authorized_keys
EOF
    
    # Create run directory for sshd
    mkdir -p /run/sshd
    chmod 755 /run/sshd
    
    # Start SSH daemon
    echo "🚀 Starting SSH server on port 2222 for user ${ACTUAL_USERNAME}..."
    /usr/sbin/sshd -D -e &
fi

# Change to catnip user if running as root
if [ "$EUID" -eq 0 ]; then
    # Use gosu for clean user switching without job control issues
    cd "${WORKSPACE}"
    # Use the actual username (which might have been changed)
    ACTUAL_USER=$(id -un 1000 2>/dev/null || echo "catnip")
    exec gosu "$ACTUAL_USER" "$@"
fi

# Execute the command
exec "$@"