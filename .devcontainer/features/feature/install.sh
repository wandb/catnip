#!/usr/bin/env bash
set -euo pipefail

SSHD_PORT="${SSHD_PORT:-2222}"

# --- user ------------------------------------------------------------------
USERNAME="${USERNAME:-"${_REMOTE_USER:-"automatic"}"}"
# Determine the appropriate non-root user
if [ "${USERNAME}" = "auto" ] || [ "${USERNAME}" = "automatic" ]; then
    USERNAME=""
    POSSIBLE_USERS=("vscode" "node" "codespace" "$(awk -v val=1000 -F ":" '$3==val{print $1}' /etc/passwd)")
    for CURRENT_USER in "${POSSIBLE_USERS[@]}"; do
        if id -u ${CURRENT_USER} > /dev/null 2>&1; then
            USERNAME=${CURRENT_USER}
            break
        fi
    done
    if [ "${USERNAME}" = "" ]; then
        USERNAME=root
    fi
elif [ "${USERNAME}" = "none" ] || ! id -u ${USERNAME} > /dev/null 2>&1; then
    USERNAME=root
fi

# --- helpers ---------------------------------------------------------------
is_apt() { command -v apt-get >/dev/null 2>&1; }
have() { command -v "$1" >/dev/null 2>&1; }

APT_UPDATED=""
apt_update_once() {
  if is_apt && [[ -z "${APT_UPDATED}" ]]; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -y
    APT_UPDATED=1
  fi
}

log() { printf "[catnip] %s\n" "$*"; }
ok()  { printf "[catnip] ✅ %s\n" "$*"; }
warn(){ printf "[catnip] ⚠️  %s\n" "$*" >&2; }

# Resolve group + home for USERNAME
USERGROUP="$(id -gn "$USERNAME" 2>/dev/null || echo "$USERNAME")"
USERHOME="$(getent passwd "$USERNAME" | cut -d: -f6 || true)"
USERHOME="${USERHOME:-/home/$USERNAME}"
[[ "$USERNAME" == "root" ]] && USERHOME="/root"

CATNIP_ROOT="${USERHOME}/.catnip"
OPT_DIR="/opt/catnip"
VOLUME_DIR="${OPT_DIR}/state"

# Root helper
run_as_root() {
  if [[ $EUID -eq 0 ]]; then "$@"
  elif command -v sudo >/dev/null 2>&1; then sudo "$@"
  else warn "Need root for: $*"; return 1
  fi
}

# Run a command as the user
run_as_user() {
  local user="${USERNAME:-root}"

  if [ "$user" = "root" ]; then
    # already root → just run directly
    "$@"
  else
    # use login shell to get correct PATH and env
    sudo -u "$user" -i bash -c "$*"
  fi
}

ensure_dir_mode() {
  local dir="$1" mode="$2"
  if [[ ! -d "$dir" ]]; then
    install -d -m "$mode" "$dir"
  else
    chmod "$mode" "$dir" || true
  fi
}

ensure_owner() {
  local path="$1" user="$2" group="$3"
  # only chown if needed
  local curu curg
  curu="$(stat -c '%U' "$path" 2>/dev/null || echo '')"
  curg="$(stat -c '%G' "$path" 2>/dev/null || echo '')"
  if [[ "$curu" != "$user" || "$curg" != "$group" ]]; then
    run_as_root chown -R "${user}:${group}" "$path"
  fi
}

# signal to our entrypoint if we should start sshd
DISABLE_SSHD=""

install_pkg_if_missing() {
  local pkg="$1"
  if ! dpkg -s "$pkg" >/dev/null 2>&1; then
    apt_update_once
    apt-get install -y --no-install-recommends "$pkg"
  fi
}

ensure_base_tools() {
  if is_apt; then
    install_pkg_if_missing ca-certificates
    install_pkg_if_missing curl
    install_pkg_if_missing gnupg
    install_pkg_if_missing ncurses-term
    install_pkg_if_missing dirmngr || true
  fi
}

# --- SSH -------------------------------------------------------------------
ensure_ssh() {
  if have sshd || [[ -f /usr/sbin/sshd ]] || [[ -f /usr/local/sbin/sshd ]]; then
    log "sshd already present"
    DISABLE_SSHD=1
    return
  fi
  if is_apt; then
    install_pkg_if_missing openssh-server
    install_pkg_if_missing openssh-client
  else
    log "Non-apt distro; please install openssh-server via your package manager." >&2
    return
  fi
  # Add ssh group if it doesn't exist
  if [ $(getent group ssh) ]; then
    log "'ssh' group already exists."
  else
    log "adding 'ssh' group, as it does not already exist."
    groupadd ssh
  fi

  # Add user to ssh group
  if [ "${USERNAME}" != "root" ]; then
    usermod -aG ssh ${USERNAME}
  fi
  # Setup sshd
  mkdir -p /var/run/sshd
  sed -i 's/session\s*required\s*pam_loginuid\.so/session optional pam_loginuid.so/g' /etc/pam.d/sshd
  sed -i 's/#*PermitRootLogin prohibit-password/PermitRootLogin yes/g' /etc/ssh/sshd_config
  sed -i -E "s/#*\s*Port\s+.+/Port ${SSHD_PORT}/g" /etc/ssh/sshd_config
  # Need to UsePAM so /etc/environment is processed
  sed -i -E "s/#?\s*UsePAM\s+.+/UsePAM yes/g" /etc/ssh/sshd_config
}

# --- catnip ----------------------------------------------------------------
install_catnip() {
  ensure_base_tools
  run_as_user "curl -sSfL install.catnip.sh | sh -s -- --version v0.9.2-dev.8"

  log "🏠 Setting up codespace directories for ${USERNAME}..."
  for d in "$CATNIP_ROOT" "$VOLUME_DIR" "$OPT_DIR" "$OPT_DIR/bin" "/worktrees"; do
    ensure_dir_mode "$d" 0755
    ensure_owner "$d" "$USERNAME" "$USERGROUP"
    ok "Prepared: $d"
  done

  # 1) User-owned runners that handle daemonization and logging
  cp $(dirname $0)/catnip-run.sh "$OPT_DIR/bin/catnip-run.sh"
  cp $(dirname $0)/catnip-stop.sh "$OPT_DIR/bin/catnip-stop.sh"
  cp $(dirname $0)/catnip-vsix.sh "$OPT_DIR/bin/catnip-vsix.sh"
  cp $(dirname $0)/catnip-ensure.sh "$OPT_DIR/bin/catnip-ensure.sh"
  cp $(dirname $0)/catnip-systemctl.sh "$OPT_DIR/bin/catnip-systemctl.sh"
  ensure_owner "$OPT_DIR/bin/catnip-run.sh" "$USERNAME" "$USERGROUP"
  ensure_owner "$OPT_DIR/bin/catnip-stop.sh" "$USERNAME" "$USERGROUP"
  ensure_owner "$OPT_DIR/bin/catnip-vsix.sh" "$USERNAME" "$USERGROUP"
  ensure_owner "$OPT_DIR/bin/catnip-ensure.sh" "$USERNAME" "$USERGROUP"
  ensure_owner "$OPT_DIR/bin/catnip-systemctl.sh" "$USERNAME" "$USERGROUP"

  # 2) Root-owned init that just handles sshd startup
  tee /usr/local/share/catnip-init.sh >/dev/null <<EOF
#!/usr/bin/env bash
set -Eeuo pipefail

# Start sshd if enabled at install time
if [[ -z "${DISABLE_SSHD}" ]]; then
  echo "[catnip] starting sshd, find logs at $OPT_DIR/sshd.log"
  : > $OPT_DIR/sshd.log
  if [[ -x /etc/init.d/ssh ]]; then
    sudo /etc/init.d/ssh start >> $OPT_DIR/sshd.log 2>&1 || true
  else
    sudo service ssh start >> $OPT_DIR/sshd.log 2>&1 || true
  fi
fi

set +e
exec "\$@"
EOF
  run_as_root chmod +x /usr/local/share/catnip-init.sh
  ok "Prepared: /usr/local/share/catnip-init.sh"
  
  # Install systemd user service for catnip auto-start
  install_systemd_service

  # Install shell profile integration for backup catnip auto-start
  install_shell_integration
}

install_systemd_service() {
  log "Installing systemd user service for catnip auto-start..."

  # Create systemd user directory
  local systemd_user_dir="$USERHOME/.config/systemd/user"
  run_as_user "mkdir -p '$systemd_user_dir'"
  ensure_owner "$systemd_user_dir" "$USERNAME" "$USERGROUP"

  # Copy service file
  local service_file="$systemd_user_dir/catnip.service"
  cp "$(dirname $0)/catnip.service" "$service_file"
  ensure_owner "$service_file" "$USERNAME" "$USERGROUP"

  # Enable lingering for the user so systemd user services start without login
  if command -v loginctl >/dev/null 2>&1; then
    run_as_root loginctl enable-linger "$USERNAME" || warn "Could not enable lingering for $USERNAME"
  fi

  # Enable and start the service as the user
  run_as_user "systemctl --user daemon-reload"
  run_as_user "systemctl --user enable catnip.service"

  ok "Systemd user service installed and enabled"
  log "Service will start automatically when user session begins"
  log "Manual control: systemctl --user {start|stop|restart|status} catnip"
}

install_shell_integration() {
  log "Installing shell profile integration for catnip auto-start..."
  
  local source_line="source /opt/catnip/bin/catnip-ensure.sh"
  local marker="# catnip: auto-start integration"
  
  # Add to ~/.bashrc
  local bashrc="$USERHOME/.bashrc"
  if [[ -f "$bashrc" ]]; then
    if ! grep -q "$marker" "$bashrc" 2>/dev/null; then
      log "Adding catnip auto-start to $bashrc"
      run_as_user "echo '' >> '$bashrc'"
      run_as_user "echo '$marker' >> '$bashrc'"
      run_as_user "echo '$source_line' >> '$bashrc'"
      ok "Added catnip auto-start to $bashrc"
    else
      log "Catnip auto-start already configured in $bashrc"
    fi
  fi
  
  # Add to ~/.zshrc
  local zshrc="$USERHOME/.zshrc"
  if [[ -f "$zshrc" ]] || run_as_user "touch '$zshrc'"; then
    ensure_owner "$zshrc" "$USERNAME" "$USERGROUP"
    
    if ! grep -q "$marker" "$zshrc" 2>/dev/null; then
      log "Adding catnip auto-start to $zshrc"
      run_as_user "echo '' >> '$zshrc'"
      run_as_user "echo '$marker' >> '$zshrc'"
      run_as_user "echo '$source_line' >> '$zshrc'"
      ok "Added catnip auto-start to $zshrc"
    else
      log "Catnip auto-start already configured in $zshrc"
    fi
  fi
}

# --- Claude CLI ------------------------------------------------------------
install_claude() {
  if have claude; then
    log "claude already installed"
    # Wrap existing claude with purr interceptor
    wrap_claude_with_purr
    # Install catnip Claude hooks for activity tracking
    install_claude_hooks
    return
  fi
  ensure_base_tools
  # Per vendor docs: curl -fsSL https://claude.ai/install.sh | bash
  run_as_user "curl -fsSL https://claude.ai/install.sh | bash"
  # After installation, wrap with purr interceptor
  wrap_claude_with_purr
  # Install catnip Claude hooks for activity tracking
  install_claude_hooks
}


create_claude_alias() {
  log "Creating claude alias to use catnip wrapper"
  
  local alias_line="alias claude='/opt/catnip/bin/claude'"
  local alias_marker="# catnip: claude alias"
  
  # Add to ~/.bash_aliases if it exists or is sourced by bashrc
  local bash_aliases="$USERHOME/.bash_aliases"
  if [[ -f "$bash_aliases" ]] || run_as_user "touch '$bash_aliases'"; then
    ensure_owner "$bash_aliases" "$USERNAME" "$USERGROUP"
    
    if ! grep -q "$alias_marker" "$bash_aliases" 2>/dev/null; then
      log "Adding claude alias to $bash_aliases"
      run_as_user "echo '' >> '$bash_aliases'"
      run_as_user "echo '$alias_marker' >> '$bash_aliases'"
      run_as_user "echo '$alias_line' >> '$bash_aliases'"
      ok "Added claude alias to $bash_aliases"
    else
      log "Claude alias already set in $bash_aliases"
    fi
  fi
  
  # Also add to ~/.bashrc for systems that don't use .bash_aliases
  local bashrc="$USERHOME/.bashrc"
  if [[ -f "$bashrc" ]]; then
    if ! grep -q "$alias_marker" "$bashrc" 2>/dev/null; then
      log "Adding claude alias to $bashrc"
      run_as_user "echo '' >> '$bashrc'"
      run_as_user "echo '$alias_marker' >> '$bashrc'"
      run_as_user "echo '$alias_line' >> '$bashrc'"
      ok "Added claude alias to $bashrc"
    else
      log "Claude alias already set in $bashrc"
    fi
  fi
  
  # Add to ~/.zshrc for zsh users
  local zshrc="$USERHOME/.zshrc"
  if [[ -f "$zshrc" ]] || run_as_user "touch '$zshrc'"; then
    ensure_owner "$zshrc" "$USERNAME" "$USERGROUP"
    
    if ! grep -q "$alias_marker" "$zshrc" 2>/dev/null; then
      log "Adding claude alias to $zshrc"
      run_as_user "echo '' >> '$zshrc'"
      run_as_user "echo '$alias_marker' >> '$zshrc'"
      run_as_user "echo '$alias_line' >> '$zshrc'"
      ok "Added claude alias to $zshrc"
    else
      log "Claude alias already set in $zshrc"
    fi
  fi
}

wrap_claude_with_purr() {
  local claude_path
  claude_path=$(run_as_user "command -v claude" 2>/dev/null || echo "")
  
  if [[ -z "$claude_path" ]]; then
    warn "Could not find claude binary to wrap"
    return
  fi
  
  log "Creating catnip claude wrapper at /opt/catnip/bin/claude"
  
  # Create wrapper script that calls the real claude binary via purr
  run_as_root tee "/opt/catnip/bin/claude" >/dev/null <<EOF
#!/bin/bash
# Catnip Claude wrapper - calls claude via purr for title interception
exec catnip purr "$claude_path" "\$@"
EOF
  
  # Make wrapper executable
  run_as_root chmod +x "/opt/catnip/bin/claude"
  
  # Ensure proper ownership
  ensure_owner "/opt/catnip/bin/claude" "$USERNAME" "$USERGROUP"
  
  ok "Created catnip claude wrapper at /opt/catnip/bin/claude"
  
  # Create bash alias to ensure our wrapper is always used
  create_claude_alias
}

# Install Claude Code hooks using catnip binary
install_claude_hooks() {
  log "Installing Claude Code hooks for improved activity tracking..."
  
  # Use catnip binary to install hooks (auto-detects correct binary path and server configuration)
  if run_as_user "catnip install-hooks"; then
    ok "Claude Code hooks installed successfully"
  else
    warn "Failed to install Claude Code hooks - activity tracking may not work optimally"
    warn "You can manually install hooks later with: catnip install-hooks"
  fi
}

# --- GitHub CLI ------------------------------------------------------------
install_gh() {
  if have gh; then
    log "[catnip] gh already installed"
    return
  fi
  ensure_base_tools
  if is_apt; then
    # Keyring (handle either legacy /usr/share/keyrings or /etc/apt/keyrings)
    local keyring="/usr/share/keyrings/githubcli-archive-keyring.gpg"
    if [[ ! -f "$keyring" ]]; then
      mkdir -p "$(dirname "$keyring")" /etc/apt/sources.list.d
      curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
        | dd of="$keyring"
      chmod go+r "$keyring"
    fi
    if [[ ! -f /etc/apt/sources.list.d/github-cli.list ]]; then
      echo "deb [arch=$(dpkg --print-architecture) signed-by=$keyring] https://cli.github.com/packages stable main" \
        > /etc/apt/sources.list.d/github-cli.list
    fi
    apt_update_once
    apt-get install -y --no-install-recommends gh
  else
    warn "Non-apt distro; install gh by your distro method or binary." >&2
  fi
}

# --- VS Code Extension ----------------------------------------------------
copy_vscode_extension() {
  local vsix_path="$(dirname $0)/catnip-sidebar.vsix"
  
  if [[ ! -f "$vsix_path" ]]; then
    log "Catnip VS Code extension VSIX not found, skipping"
    return
  fi
  
  log "Copying pre-built Catnip VS Code extension..."
  cp "$vsix_path" "/opt/catnip/"
}

# --- entrypoint ------------------------------------------------------------
main() {
  shopt -s nocasematch
  ${ENSURESSH:-true} && ensure_ssh
  ${INSTALLCATNIP:-true} && install_catnip
  ${INSTALLCLAUDE:-true} && install_claude
  ${INSTALLGH:-true} && install_gh
  copy_vscode_extension
  shopt -u nocasematch
}
main "$@"