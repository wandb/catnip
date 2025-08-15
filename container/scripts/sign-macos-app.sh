#!/bin/bash
set -euo pipefail

# Sign and notarize macOS app bundle
# This script is called from GoReleaser hooks to properly sign the entire app bundle
# instead of just the binary inside it.
# Arguments:
#   $1 - Path to the app bundle
#   $2 - Version string (optional)

APP_PATH="$1"
VERSION="${2:-0.0.0}"

if [ ! -d "$APP_PATH" ]; then
    echo "Error: App bundle not found at $APP_PATH"
    exit 1
fi

echo "🔐 Signing macOS app bundle: $APP_PATH"

# Debug: Check what's in the app bundle
echo "📂 App bundle structure:"
ls -la "$APP_PATH/Contents/" || true
ls -la "$APP_PATH/Contents/MacOS/" || true

# Find the Developer ID Application certificate
# First extract just the certificate name without the hash
CERT_IDENTITY=$(security find-identity -v -p codesigning | grep "Developer ID Application" | head -1 | awk -F'"' '{print $2}')
if [ -z "$CERT_IDENTITY" ]; then
    echo "Error: No Developer ID Application certificate found"
    exit 1
fi

echo "📋 Using certificate: $CERT_IDENTITY"

# Ensure we have proper Info.plist and Resources
echo "📄 Setting up app bundle structure..."
cp build/Catnip.app/Contents/Info.plist "$APP_PATH/Contents/" 2>/dev/null || true

# Replace template variables in Info.plist
if [ -f "$APP_PATH/Contents/Info.plist" ]; then
    echo "📝 Setting version to $VERSION in Info.plist..."
    sed -i '' "s/{{ \.Version }}/$VERSION/g" "$APP_PATH/Contents/Info.plist"
fi

mkdir -p "$APP_PATH/Contents/Resources"
cp -r build/Catnip.app/Contents/Resources/* "$APP_PATH/Contents/Resources/" 2>/dev/null || true

# Sign the binary directly first (without --deep)
BINARY_PATH="$APP_PATH/Contents/MacOS/catnip"
ENTITLEMENTS_PATH="build/entitlements.plist"

if [ -f "$BINARY_PATH" ]; then
    echo "🔏 Signing binary: $BINARY_PATH"
    if [ -f "$ENTITLEMENTS_PATH" ]; then
        codesign --force --options runtime --timestamp --entitlements "$ENTITLEMENTS_PATH" --sign "$CERT_IDENTITY" "$BINARY_PATH"
    else
        codesign --force --options runtime --timestamp --sign "$CERT_IDENTITY" "$BINARY_PATH"
    fi
fi

# Then sign the app bundle itself (without --deep to avoid re-signing the binary)
echo "🔏 Signing app bundle: $APP_PATH"
if [ -f "$ENTITLEMENTS_PATH" ]; then
    codesign --force --options runtime --timestamp --entitlements "$ENTITLEMENTS_PATH" --sign "$CERT_IDENTITY" "$APP_PATH"
else
    codesign --force --options runtime --timestamp --sign "$CERT_IDENTITY" "$APP_PATH"
fi

# Verify the signature
echo "🔍 Verifying signature..."
codesign --verify --deep --strict --verbose=2 "$APP_PATH"

# Check if we should notarize (skip for local testing)
if [ -z "${MACOS_NOTARY_PROFILE_NAME:-}" ] || [ "${SKIP_NOTARIZATION:-false}" = "true" ]; then
    echo "⚠️  Skipping notarization (MACOS_NOTARY_PROFILE_NAME not set or SKIP_NOTARIZATION=true)"
    echo "✅ Successfully signed (but not notarized): $APP_PATH"
    exit 0
fi

# Step 3: Create zip archive for notarization using ditto (required)
ZIP_PATH="$APP_PATH.zip"
echo "📦 Creating zip archive: $ZIP_PATH"
ditto -c -k --keepParent "$APP_PATH" "$ZIP_PATH"

# Step 4: Submit for notarization
echo "📋 Submitting for notarization..."
echo "📋 Using profile: $MACOS_NOTARY_PROFILE_NAME"

# Check if the profile exists first
if ! xcrun notarytool history --keychain-profile "$MACOS_NOTARY_PROFILE_NAME" 2>/dev/null; then
    echo "⚠️  Profile '$MACOS_NOTARY_PROFILE_NAME' not found in keychain"
    echo "ℹ️  To create it, run:"
    echo "    xcrun notarytool store-credentials '$MACOS_NOTARY_PROFILE_NAME' \\"
    echo "      --apple-id 'YOUR_APPLE_ID' \\"
    echo "      --team-id 'YOUR_TEAM_ID' \\"
    echo "      --password 'APP_SPECIFIC_PASSWORD'"
    echo ""
    echo "✅ Successfully signed (but not notarized): $APP_PATH"
    rm -f "$ZIP_PATH"
    exit 0
fi

SUBMISSION_ID=$(xcrun notarytool submit "$ZIP_PATH" \
    --keychain-profile "$MACOS_NOTARY_PROFILE_NAME" \
    --wait \
    --timeout 30m \
    --output-format json | jq -r '.id')

if [ "$SUBMISSION_ID" = "null" ] || [ -z "$SUBMISSION_ID" ]; then
    echo "Error: Failed to submit for notarization"
    rm -f "$ZIP_PATH"
    exit 1
fi

echo "📋 Submission ID: $SUBMISSION_ID"

# Check notarization status
echo "⏳ Checking notarization status..."
xcrun notarytool info "$SUBMISSION_ID" \
    --keychain-profile "$MACOS_NOTARY_PROFILE_NAME" \
    --output-format json

# Step 5: Staple the notarization to the app bundle
echo "📌 Stapling notarization to app bundle..."
xcrun stapler staple "$APP_PATH"

# Verify stapling
echo "🔍 Verifying stapling..."
xcrun stapler validate "$APP_PATH"

# Clean up zip file
rm -f "$ZIP_PATH"

echo "✅ Successfully signed and notarized: $APP_PATH"