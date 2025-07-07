#!/bin/bash

# Generate dynamic catnip banner with actual versions

# Check if banner is disabled
if [ "$CATNIP_BANNER" = "0" ] || [ "$CATNIP_BANNER" = "false" ]; then
    exit 0
fi

# Get the welcome message with username
if [ -n "$CATNIP_USERNAME" ]; then
    WELCOME_MSG="🐱 Welcome $CATNIP_USERNAME to your purrfect development environment! 🐱"
else
    WELCOME_MSG="🐱 Welcome to your purrfect development environment! 🐱"
fi

# Get actual versions (suppress errors for missing tools)
NODE_VERSION=""
PYTHON_VERSION=""
RUST_VERSION=""
GO_VERSION=""

# Source environment to get access to tools
source /etc/profile.d/catnip.sh >/dev/null 2>&1 || true
source "$NVM_DIR/nvm.sh" >/dev/null 2>&1 || true

# Get versions safely
if command -v node >/dev/null 2>&1; then
    NODE_VERSION=$(node --version 2>/dev/null | sed 's/v//')
fi

if command -v python3 >/dev/null 2>&1; then
    PYTHON_VERSION=$(python3 --version 2>/dev/null | cut -d' ' -f2)
fi

if command -v rustc >/dev/null 2>&1; then
    RUST_VERSION=$(rustc --version 2>/dev/null | cut -d' ' -f2)
fi

if command -v go >/dev/null 2>&1; then
    GO_VERSION=$(go version 2>/dev/null | cut -d' ' -f3 | sed 's/go//')
fi

# Colors for gradient (purple to magenta)
PURPLE='\033[38;5;93m'      # Dark purple
PURPLE2='\033[38;5;129m'    # Purple
PURPLE3='\033[38;5;135m'    # Light purple  
MAGENTA='\033[38;5;165m'    # Dark magenta
MAGENTA2='\033[38;5;171m'   # Medium magenta
MAGENTA3='\033[38;5;177m'   # Light magenta
BRIGHT_MAGENTA='\033[38;5;201m' # Bright magenta
RESET='\033[0m'

# Generate the banner with gradient
echo
echo -e "${PURPLE} \$\$\\      \$\$\\ \$\$\$\$\$\$\$\$\\  \$\$\$\$\$\$\\  \$\$\\      \$\$\\ ${RESET}"
echo -e "${PURPLE2} \$\$\$\\    \$\$\$ |\$\$  _____|\$\$  __\$\$\\ \$\$ | \$\\  \$\$ |${RESET}"
echo -e "${PURPLE3} \$\$\$\$\\  \$\$\$\$ |\$\$ |      \$\$ /  \$\$ |\$\$ |\$\$\$\\ \$\$ |${RESET}"
echo -e "${MAGENTA} \$\$\\\$\$\\\$\$ \$\$ |\$\$\$\$\$\\    \$\$ |  \$\$ |\$\$ \$\$ \$\$\\\$\$ |${RESET}"
echo -e "${MAGENTA2} \$\$ \\\$\$\$  \$\$ |\$\$  __|   \$\$ |  \$\$ |\$\$\$\$  _\$\$\$\$ |${RESET}"
echo -e "${MAGENTA3} \$\$ |\\$  /\$\$ |\$\$ |      \$\$ |  \$\$ |\$\$\$  / \\\$\$\$ |${RESET}"
echo -e "${BRIGHT_MAGENTA} \$\$ | \\_/ \$\$ |\$\$\$\$\$\$\$\$\\  \$\$\$\$\$\$  |\$\$  /   \\\$\$ |${RESET}"
echo -e "${BRIGHT_MAGENTA} \\__|     \\__|\\_________|\\______/ \\__/     \\__|${RESET}"
echo
echo "   $WELCOME_MSG"
echo "   "
echo "   Languages installed:"

# Add language versions dynamically
if [ -n "$NODE_VERSION" ]; then
    echo "   • Node.js $NODE_VERSION + pnpm/yarn/npm via corepack"
else
    echo "   • Node.js + pnpm/yarn/npm via corepack"
fi

if [ -n "$PYTHON_VERSION" ]; then
    echo "   • Python $PYTHON_VERSION + uv + pipx for package management"
else
    echo "   • Python + uv + pipx for package management"
fi

if [ -n "$RUST_VERSION" ]; then
    echo "   • Rust $RUST_VERSION + cargo"
else
    echo "   • Rust + cargo"
fi

if [ -n "$GO_VERSION" ]; then
    echo "   • Go $GO_VERSION"
else
    echo "   • Go"
fi

echo ""
echo "   Tools available:"
echo "   • Claude Code CLI for AI-powered development"
echo "   • Docker for building images, GitHub CLI"
echo "   • All your favorite dev utilities"
echo ""
echo "   Happy coding! 🚀"
echo ""