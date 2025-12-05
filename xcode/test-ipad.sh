#!/bin/bash
set -e

# Use iPad Pro 13-inch for testing
DEVICE_ID="73B540BB-FE5C-4D99-A07D-7518DAFC4A90"
APP_BUNDLE="com.wandb.catnip"
SCREENSHOT_DIR="$HOME/Desktop/catnip-screenshots"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

echo "🚀 Starting iPad simulator test..."

# Create screenshot directory
mkdir -p "$SCREENSHOT_DIR"

# Boot simulator if not already booted
echo "📱 Booting iPad simulator..."
xcrun simctl boot "$DEVICE_ID" 2>/dev/null || true

# Wait for boot
sleep 3

# Build and install app
echo "🔨 Building app..."
xcodebuild -project catnip.xcodeproj \
  -scheme catnip \
  -configuration Debug \
  -sdk iphonesimulator \
  -destination "id=$DEVICE_ID" \
  -quiet \
  build

echo "📦 Installing app..."
xcrun simctl install "$DEVICE_ID" \
  ~/Library/Developer/Xcode/DerivedData/Catnip-*/Build/Products/Debug-iphonesimulator/catnip.app

# Launch app in UI testing mode to bypass authentication
echo "🚀 Launching app in UI testing mode..."
xcrun simctl launch --console "$DEVICE_ID" "$APP_BUNDLE" \
  -UITesting \
  -SkipAuthentication \
  -UseMockData \
  -ShowWorkspacesList &

# Wait for app to load
echo "⏳ Waiting 8 seconds for app to load..."
sleep 8

# Take screenshot
SCREENSHOT_PATH="$SCREENSHOT_DIR/ipad-${TIMESTAMP}.png"
echo "📸 Taking screenshot..."
xcrun simctl io "$DEVICE_ID" screenshot "$SCREENSHOT_PATH"

echo "✅ Screenshot saved to: $SCREENSHOT_PATH"
echo ""
echo "To view: open '$SCREENSHOT_PATH'"
