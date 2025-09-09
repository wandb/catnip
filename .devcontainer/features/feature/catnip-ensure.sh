#!/usr/bin/env bash
# Catnip auto-start script for shell profiles
# This gets sourced by ~/.bashrc or ~/.zshrc to ensure catnip runs

# Only run in interactive shells to avoid breaking scripts
[[ $- != *i* ]] && return

# Only run once per shell session
[[ -n "${CATNIP_ENSURE_RAN:-}" ]] && return
export CATNIP_ENSURE_RAN=1

# Only run if we're in a codespace/devcontainer environment
[[ -z "${CODESPACE_NAME:-}${DEVCONTAINER:-}" ]] && return

# Check if catnip command exists
if ! command -v catnip >/dev/null 2>&1; then
    return
fi

# Function to check if catnip is running
catnip_is_running() {
    local pidfile="/opt/catnip/catnip.pid"
    
    # Check if pid file exists
    [[ ! -f "$pidfile" ]] && return 1
    
    # Read PID
    local pid
    pid=$(cat "$pidfile" 2>/dev/null) || return 1
    
    # Check if PID is valid and process is running
    [[ "$pid" =~ ^[0-9]+$ ]] && kill -0 "$pid" 2>/dev/null
}

# Function to start catnip quietly
catnip_start_quiet() {
    if [[ -f "/opt/catnip/bin/catnip-run.sh" ]]; then
        bash "/opt/catnip/bin/catnip-run.sh" >/dev/null 2>&1 &
    fi
}

# Check and start catnip if needed (run in background to not slow shell startup)
(
    if ! catnip_is_running; then
        # Small delay to avoid racing with other startup scripts
        sleep 1
        catnip_is_running || catnip_start_quiet
    fi
) &