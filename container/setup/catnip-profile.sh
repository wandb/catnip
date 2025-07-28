# Catnip development environment
export CATNIP_ROOT="/opt/catnip"
export WORKSPACE="/workspace"
export PATH="${CATNIP_ROOT}/bin:${PATH}"
export NVM_DIR="${CATNIP_ROOT}/nvm"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"
[ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion"
export CARGO_HOME="${CATNIP_ROOT}/cargo"
export RUSTUP_HOME="${CATNIP_ROOT}/rustup"
export PATH="${CATNIP_ROOT}/cargo/bin:${PATH}"
export GOROOT="${CATNIP_ROOT}/go"
export GOPATH="${CATNIP_ROOT}/go-workspace"
export PATH="${CATNIP_ROOT}/go/bin:${GOPATH}/bin:${PATH}"
export PATH="${HOME}/.local/bin:${PATH}"
# pipx configuration
export PIPX_BIN_DIR="${CATNIP_ROOT}/bin"
export PIPX_HOME="${CATNIP_ROOT}/pipx"

# Enable Node.js PTY title interceptor for Claude detection
# Set CATNIP_DISABLE_PTY_INTERCEPTOR=1 to disable if it causes issues
if [ "$CATNIP_DISABLE_PTY_INTERCEPTOR" != "1" ] && [ "$CATNIP_DISABLE_PTY_INTERCEPTOR" != "true" ]; then
    export NODE_OPTIONS="--require ${CATNIP_ROOT}/lib/pty-title-interceptor.js"
fi

# Use custom username if provided
if [ -n "$CATNIP_USERNAME" ]; then
    export USER="$CATNIP_USERNAME"
    export USERNAME="$CATNIP_USERNAME"
fi