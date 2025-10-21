#!/usr/bin/env bash
#
# reset-claude-auth.sh
#
# Resets Claude authentication by moving credential files to .off extensions
# for testing the onboarding flow.
#
# Usage: ./scripts/reset-claude-auth.sh

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}🔄 Resetting Claude authentication...${NC}\n"

# Function to safely move a file if it exists
move_if_exists() {
    local src="$1"
    local dst="$2"

    if [ -f "$src" ]; then
        echo -e "${YELLOW}Moving:${NC} $src -> $dst"
        mv "$src" "$dst"
    else
        echo -e "${YELLOW}Not found:${NC} $src (skipping)"
    fi
}

# Check for CATNIP_VOLUME_DIR
if [ -z "${CATNIP_VOLUME_DIR:-}" ]; then
    echo -e "${YELLOW}⚠️  CATNIP_VOLUME_DIR not set, using default: ~/.catnip${NC}"
    CATNIP_VOLUME_DIR="$HOME/.catnip"
fi

echo -e "${GREEN}Using CATNIP_VOLUME_DIR:${NC} $CATNIP_VOLUME_DIR\n"

# Reset CATNIP_VOLUME_DIR credentials
echo -e "${GREEN}📁 Resetting CATNIP_VOLUME_DIR credentials...${NC}"
move_if_exists "$CATNIP_VOLUME_DIR/.claude/claude.json" "$CATNIP_VOLUME_DIR/.claude/claude.json.off"
move_if_exists "$CATNIP_VOLUME_DIR/.claude/.credentials.json" "$CATNIP_VOLUME_DIR/.claude/.credentials.json.off"

echo ""

# Reset home directory credentials
echo -e "${GREEN}🏠 Resetting home directory credentials...${NC}"
move_if_exists "$HOME/.claude.json" "$HOME/.claude.json.off"
move_if_exists "$HOME/.claude/.credentials.json" "$HOME/.claude/.credentials.json.off"

echo ""
echo -e "${GREEN}✅ Claude authentication reset complete!${NC}"
echo -e "${YELLOW}💡 Tip: Refresh your browser to trigger the auth flow${NC}"
