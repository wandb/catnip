#!/bin/bash
set -e

# Use iPad Pro 13-inch for testing
DEVICE_ID="73B540BB-FE5C-4D99-A07D-7518DAFC4A90"
APP_BUNDLE="com.wandb.catnip"
SCREENSHOT_DIR="$HOME/Desktop/catnip-screenshots"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

echo "🚀 Testing iPad sidebar visibility..."

# Create screenshot directory
mkdir -p "$SCREENSHOT_DIR"

# Ensure simulator is booted
xcrun simctl boot "$DEVICE_ID" 2>/dev/null || true
sleep 2

# Build if needed (quick check)
if [ ! -d ~/Library/Developer/Xcode/DerivedData/Catnip-*/Build/Products/Debug-iphonesimulator/catnip.app ]; then
    echo "🔨 Building app..."
    xcodebuild -project catnip.xcodeproj \
      -scheme catnip \
      -configuration Debug \
      -sdk iphonesimulator \
      -destination "id=$DEVICE_ID" \
      -quiet \
      build
fi

# Install app
echo "📦 Installing app..."
xcrun simctl install "$DEVICE_ID" \
  ~/Library/Developer/Xcode/DerivedData/Catnip-*/Build/Products/Debug-iphonesimulator/catnip.app

# Launch app with mock data
echo "🚀 Launching app..."
xcrun simctl launch "$DEVICE_ID" "$APP_BUNDLE" \
  -UITesting \
  -SkipAuthentication \
  -UseMockData \
  -ShowWorkspacesList

# Wait for app to settle
sleep 5

# Take initial screenshot
SCREENSHOT_INITIAL="$SCREENSHOT_DIR/sidebar-initial-${TIMESTAMP}.png"
echo "📸 Taking initial screenshot..."
xcrun simctl io "$DEVICE_ID" screenshot "$SCREENSHOT_INITIAL"
echo "Initial: $SCREENSHOT_INITIAL"

# Tap the sidebar toggle button (upper left, approximate coordinates)
# For iPad Pro 13-inch (2048x2732), sidebar button is around x=40, y=90
echo "👆 Tapping sidebar toggle button..."
xcrun simctl io "$DEVICE_ID" tap 40 90

# Wait for animation
sleep 2

# Take screenshot with sidebar visible
SCREENSHOT_SIDEBAR="$SCREENSHOT_DIR/sidebar-visible-${TIMESTAMP}.png"
echo "📸 Taking sidebar screenshot..."
xcrun simctl io "$DEVICE_ID" screenshot "$SCREENSHOT_SIDEBAR"
echo "Sidebar visible: $SCREENSHOT_SIDEBAR"

echo ""
echo "✅ Screenshots saved:"
echo "   Initial: $SCREENSHOT_INITIAL"
echo "   Sidebar: $SCREENSHOT_SIDEBAR"
