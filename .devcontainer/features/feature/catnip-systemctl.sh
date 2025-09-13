#!/bin/bash
# Catnip systemd service management script

set -euo pipefail

COMMAND="${1:-status}"

log() { printf "[catnip-systemctl] %s\n" "$*"; }
ok()  { printf "[catnip-systemctl] ✅ %s\n" "$*"; }
warn(){ printf "[catnip-systemctl] ⚠️  %s\n" "$*" >&2; }

case "$COMMAND" in
  "start")
    log "Starting catnip service..."
    systemctl --user start catnip.service
    ok "Catnip service started"
    ;;
  "stop")
    log "Stopping catnip service..."
    systemctl --user stop catnip.service
    ok "Catnip service stopped"
    ;;
  "restart")
    log "Restarting catnip service..."
    systemctl --user restart catnip.service
    ok "Catnip service restarted"
    ;;
  "status")
    systemctl --user status catnip.service
    ;;
  "enable")
    log "Enabling catnip service..."
    systemctl --user enable catnip.service
    ok "Catnip service enabled"
    ;;
  "disable")
    log "Disabling catnip service..."
    systemctl --user disable catnip.service
    ok "Catnip service disabled"
    ;;
  "logs")
    journalctl --user -u catnip.service "${@:2}"
    ;;
  "follow"|"tail")
    journalctl --user -u catnip.service -f
    ;;
  *)
    cat << EOF
Catnip systemd service management

USAGE:
    catnip-systemctl [COMMAND]

COMMANDS:
    start       Start catnip service
    stop        Stop catnip service
    restart     Restart catnip service
    status      Show service status (default)
    enable      Enable service to start automatically
    disable     Disable automatic startup
    logs        Show service logs
    follow      Follow live service logs
    tail        Same as follow

EXAMPLES:
    catnip-systemctl status
    catnip-systemctl restart
    catnip-systemctl logs --since "1 hour ago"
EOF
    ;;
esac