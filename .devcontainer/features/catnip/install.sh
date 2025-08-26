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
VOLUME_DIR="${CATNIP_ROOT}/volume"
OPT_DIR="/opt/catnip"

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
  run_as_user "curl -sSfL install.catnip.sh | sh"

  log "🏠 Setting up codespace directories for ${USERNAME}..."
  for d in "$CATNIP_ROOT" "$VOLUME_DIR"; do
    ensure_dir_mode "$d" 0755
    ensure_owner "$d" "$USERNAME" "$USERGROUP"
    ok "Prepared: $d"
  done

  # /opt/catnip hierarchy
  run_as_root mkdir -p "$OPT_DIR/bin"
  ensure_owner "$OPT_DIR" "$USERNAME" "$USERGROUP"
  ok "Prepared: $OPT_DIR"

  # 1) User-owned runner that handles nohup/log/pid (runtime expansion)
  tee "$OPT_DIR/bin/catnip-run.sh" >/dev/null <<'USR'
#!/usr/bin/env bash
set -Eeuo pipefail

echo "[$(date -Is)] runner starting as $(id -un) uid=$(id -u) pwd=$PWD"

export PATH="/opt/catnip/bin:$HOME/.local/bin:$PATH"

# Use /opt/catnip for log/pid (not /tmp)
mkdir -p /opt/catnip
LOG=/opt/catnip/catnip.log
: > "$LOG"   # force file creation every start
echo "[$(date -Is)] touching $LOG"
echo "[$(date -Is)] PATH=$PATH" >> "$LOG"

export CATNIP_WORKSPACE_DIR=/opt/catnip
export CATNIP_HOME_DIR="${HOME}"
export CATNIP_VOLUME_DIR="${HOME}/.catnip/volume"
export CATNIP_LIVE_DIR=/workspaces

if command -v catnip >/dev/null 2>&1; then
  echo "[$(date -Is)] launching catnip with nohup"
  nohup catnip serve >>"$LOG" 2>&1 &
  echo $! > /opt/catnip/catnip.pid
  echo "[$(date -Is)] catnip pid $(cat /opt/catnip/catnip.pid)" >> "$LOG"
else
  echo "[$(date -Is)] catnip not on PATH=$PATH" >> "$LOG"
fi
USR
  run_as_root chmod +x "$OPT_DIR/bin/catnip-run.sh"
  ensure_owner "$OPT_DIR/bin/catnip-run.sh" "$USERNAME" "$USERGROUP"

  # 2) Root-owned init that just invokes the runner as $USERNAME
  tee /usr/local/share/catnip-init.sh >/dev/null <<EOF
#!/usr/bin/env bash
set -Eeuo pipefail

echo "[catnip] starting catnip, find logs at $OPT_DIR/catnip.log"

# Start as the container user
sudo -u "${USERNAME}" -i "$OPT_DIR/bin/catnip-run.sh"

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
}

# --- Claude CLI ------------------------------------------------------------
install_claude() {
  if have claude; then
    log "claude already installed"
    return
  fi
  ensure_base_tools
  # Per vendor docs: curl -fsSL https://claude.ai/install.sh | bash
  run_as_user "curl -fsSL https://claude.ai/install.sh | bash"
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

# --- entrypoint ------------------------------------------------------------
main() {
  shopt -s nocasematch
  ${ENSURESSH:-true} && ensure_ssh
  ${INSTALLCATNIP:-true} && install_catnip
  ${INSTALLCLAUDE:-true} && install_claude
  ${INSTALLGH:-true} && install_gh
  shopt -u nocasematch
}
main "$@"