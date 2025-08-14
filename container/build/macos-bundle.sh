#!/bin/bash
# Post-build script to create macOS app bundle
# Called by GoReleaser after building macOS binaries

set -e

# Check if this is a macOS build
if [ "$1" != "darwin" ]; then
  exit 0
fi

BINARY_PATH="$2"
VERSION="$3"
TARGET="$4"

echo "Creating macOS app bundle for ${TARGET} using static structure"

# Use static app bundle structure
STATIC_APP="build/Catnip.app"
APP_DIR="${BINARY_PATH}.app"

# Remove old bundle and copy static structure
rm -rf "${APP_DIR}"
cp -R "${STATIC_APP}" "${APP_DIR}"

# Ensure the MacOS directory exists in the app bundle
mkdir -p "${APP_DIR}/Contents/MacOS"

# Copy built binary to app bundle (keep original for GoReleaser signing)
cp "${BINARY_PATH}" "${APP_DIR}/Contents/MacOS/catnip"

# Process Info.plist template to set version
sed "s/__VERSION__/${VERSION}/g" "${STATIC_APP}/Contents/Info.plist" > "${APP_DIR}/Contents/Info.plist"

# Create CLI wrapper script
DIR_PATH="$(dirname "${BINARY_PATH}")"
cat > "${DIR_PATH}/catnip-cli" << 'EOF'
#!/bin/bash
# Catnip CLI wrapper - calls the binary inside the app bundle
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "${DIR}/Catnip.app/Contents/MacOS/catnip" "$@"
EOF
chmod +x "${DIR_PATH}/catnip-cli"

echo "✅ Static app bundle deployed at ${APP_DIR}"