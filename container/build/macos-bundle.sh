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

# Copy built binary to app bundle (keep original for signing)
cp "${BINARY_PATH}" "${APP_DIR}/Contents/MacOS/catnip"

# Process Info.plist template to set version
sed "s/__VERSION__/${VERSION}/g" "${STATIC_APP}/Contents/Info.plist" > "${APP_DIR}/Contents/Info.plist"

# Sign the app bundle if signing credentials are available
if [ -n "${APPLE_SIGN_P12:-}" ] && [ -n "${APPLE_SIGN_PASSWORD:-}" ]; then
  echo "🔐 Code signing app bundle..."
  
  # Import certificate to keychain
  TEMP_KEYCHAIN="build-$(date +%s).keychain"
  security create-keychain -p "temppass" "${TEMP_KEYCHAIN}"
  security set-keychain-settings -lut 21600 "${TEMP_KEYCHAIN}"
  security unlock-keychain -p "temppass" "${TEMP_KEYCHAIN}"
  
  # Decode and import certificate
  echo "${APPLE_SIGN_P12}" | base64 --decode > temp-cert.p12
  security import temp-cert.p12 -k "${TEMP_KEYCHAIN}" -P "${APPLE_SIGN_PASSWORD}" -T /usr/bin/codesign
  security list-keychains -d user -s "${TEMP_KEYCHAIN}" $(security list-keychains -d user | sed s/\"//g)
  
  # Sign the app bundle
  /usr/bin/codesign --force --sign "Developer ID Application" --entitlements build/entitlements.plist --options runtime --timestamp "${APP_DIR}"
  
  # Verify signature
  /usr/bin/codesign --verify --verbose "${APP_DIR}"
  
  # Notarize if credentials are available
  if [ -n "${APPLE_ISSUER_ID:-}" ] && [ -n "${APPLE_KEY_ID:-}" ] && [ -n "${APPLE_PRIVATE_KEY:-}" ]; then
    echo "📝 Notarizing app bundle..."
    
    # Create temporary key file
    echo "${APPLE_PRIVATE_KEY}" > temp-key.p8
    
    # Submit for notarization
    xcrun notarytool submit "${APP_DIR}" --key temp-key.p8 --key-id "${APPLE_KEY_ID}" --issuer "${APPLE_ISSUER_ID}" --wait
    
    # Staple the notarization
    xcrun stapler staple "${APP_DIR}"
    
    # Clean up
    rm -f temp-key.p8
  fi
  
  # Clean up keychain and certificate
  security delete-keychain "${TEMP_KEYCHAIN}"
  rm -f temp-cert.p12
  
  echo "✅ App bundle signed and notarized"
else
  echo "ℹ️  Skipping code signing (no credentials provided)"
fi

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