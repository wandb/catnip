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
        cp -r "/root/$config_dir" "/home/catnip/" 2>/dev/null || true
        chown -R 1000:1000 "/home/catnip/$config_dir" 2>/dev/null || true
    fi
done

# Set git config for the catnip user (not root) and mark as safe repo
gosu 1000:1000 git config --global user.name "$GIT_USERNAME"
gosu 1000:1000 git config --global user.email "$GIT_EMAIL"
gosu 1000:1000 git config --global init.defaultBranch main
# TODO: This should be tightly scoped to mounted volume. Bad!
gosu 1000:1000 git config --global --add safe.directory "${CATNIP_WORKSPACE_DIR:-/workspace}"

# Install specific versions if requested via environment variables (run as catnip user)
if [ -n "$CATNIP_NODE_VERSION" ]; then
    echo "Installing Node.js version: $CATNIP_NODE_VERSION"
    gosu 1000:1000 bash -c 'source /etc/profile.d/catnip.sh && source "$NVM_DIR/nvm.sh" && nvm install "$CATNIP_NODE_VERSION" && nvm use "$CATNIP_NODE_VERSION"'
fi
# Fix nvm locations in root
sed -i "s|\$HOME/\.nvm|\$NVM_DIR|g" /root/.bashrc

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
# Fix cargo paths again in case rustup modified root files
sed -i "s|/root/\.cargo|${CATNIP_ROOT}/cargo|g" /root/.bashrc /root/.profile 2>/dev/null || true

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
if [ -d "${CATNIP_VOLUME_DIR:-/volume}" ]; then
    echo "📁 Setting up persistent volume..."
    # Set permissions so catnip user can write to entire volume
    sudo chown -R 1000:1000 "${CATNIP_VOLUME_DIR:-/volume}" 2>/dev/null || true
    sudo chmod -R 755 "${CATNIP_VOLUME_DIR:-/volume}" 2>/dev/null || true
fi

# Ensure workspace has proper ownership
chown -R 1000:1000 "${WORKSPACE}" 2>/dev/null || true

# Handle Docker socket permissions if mounted
if [ -S "/var/run/docker-host.sock" ] || [ -S "/var/run/docker.sock" ]; then
    echo "🐳 Docker socket detected, configuring access..."
    
    # Determine which socket exists and strategy
    if [ -S "/var/run/docker-host.sock" ]; then
        # Host socket is mounted at alternative location - create proxy at standard location
        DOCKER_SOCKET="/var/run/docker-host.sock"
        CREATE_PROXY=true
    elif [ -S "/var/run/docker.sock" ]; then
        # Socket is already at standard location - check if we need to proxy
        DOCKER_SOCKET="/var/run/docker.sock"
        # Only create proxy if we can't access it directly
        DOCKER_GID=$(stat -c '%g' $DOCKER_SOCKET)
        if [ "$DOCKER_GID" = "0" ] || [ "$DOCKER_GID" = "1" ]; then
            # Create proxy with different name to avoid conflict
            CREATE_PROXY=true
            PROXY_PATH="/var/run/docker-catnip.sock"
        else
            CREATE_PROXY=false
        fi
    else
        echo "   ❌ No Docker socket found"
        return 0
    fi
    
    # Get the GID of the docker socket
    DOCKER_GID=$(stat -c '%g' $DOCKER_SOCKET)
    echo "   Docker socket at $DOCKER_SOCKET with GID: ${DOCKER_GID}"
    
    if [ "$CREATE_PROXY" = "true" ]; then
        echo "   Creating accessible proxy socket..."
        
        # Install socat if not present (should be in our Dockerfile)
        if ! command -v socat &> /dev/null; then
            apt-get update && apt-get install -y socat
        fi
        
        # Ensure docker group exists with GID 999 (common docker group ID)
        if ! getent group 999 >/dev/null 2>&1; then
            groupadd -g 999 docker 2>/dev/null || true
        fi
        
        # Determine proxy socket path
        if [ -S "/var/run/docker-host.sock" ]; then
            PROXY_PATH="/var/run/docker.sock"
        else
            PROXY_PATH="/var/run/docker-catnip.sock"
        fi
        
        # Remove any existing proxy socket
        rm -f "$PROXY_PATH" 2>/dev/null || true
        
        # Start socat in background to proxy the socket with proper permissions
        socat "UNIX-LISTEN:$PROXY_PATH,fork,mode=660,user=1000,group=999" "UNIX-CONNECT:$DOCKER_SOCKET" &
        SOCAT_PID=$!
        
        # Give socat a moment to create the socket
        sleep 0.5
        
        # Verify the new socket exists
        if [ -S "$PROXY_PATH" ]; then
            echo "   ✅ Docker proxy socket created at $PROXY_PATH"
            echo $SOCAT_PID > /var/run/docker-proxy.pid
            
            # If we created an alternative socket, set up docker command wrapper
            if [ "$PROXY_PATH" = "/var/run/docker-catnip.sock" ]; then
                echo '#!/bin/bash' > /usr/local/bin/docker
                echo 'exec /usr/bin/docker -H unix:///var/run/docker-catnip.sock "$@"' >> /usr/local/bin/docker
                chmod +x /usr/local/bin/docker
                echo "   ✅ Docker command wrapper created"
            fi
        else
            echo "   ⚠️  Failed to create proxy socket, falling back to sudo"
            echo "catnip ALL=(ALL) NOPASSWD: /usr/bin/docker" >> /etc/sudoers
        fi
    else
        # Socket has a non-root GID, configure group access
        echo "   Configuring group access for GID ${DOCKER_GID}..."
        
        # Create or update docker group to match
        if getent group docker >/dev/null 2>&1; then
            groupmod -g ${DOCKER_GID} docker 2>/dev/null || true
        else
            groupadd -g ${DOCKER_GID} docker 2>/dev/null || true
        fi
        
        # Add users to docker group
        usermod -aG docker catnip 2>/dev/null || true
        if [ -n "$CATNIP_USERNAME" ] && [ "$CATNIP_USERNAME" != "catnip" ]; then
            usermod -aG docker "$CATNIP_USERNAME" 2>/dev/null || true
        fi
    fi
    
    echo "✅ Docker access configured for catnip user"
fi

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

    # Set some custom ports for manual ssh sessions
    gosu 1000:1000 cat > /home/catnip/.ssh/environment <<EOF
PORT=3000
PORTZ=[3001,3002,3003,3004,3005]
EOF
    
    # Determine the actual username
    ACTUAL_USERNAME="${CATNIP_USERNAME:-catnip}"
    
    # Configure SSH daemon with proper home directory
    cat > /etc/ssh/sshd_config.d/99-catnip.conf <<EOF
Port 2222
PubkeyAuthentication yes
PasswordAuthentication no
PermitUserEnvironment yes
ChallengeResponseAuthentication no
UsePAM no
AllowUsers ${ACTUAL_USERNAME} catnip
X11Forwarding yes
PermitUserEnvironment yes
AcceptEnv LANG LC_* WORKDIR
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