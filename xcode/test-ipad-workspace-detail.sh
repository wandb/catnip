#!/bin/bash
set -e

# Use iPad Pro 13-inch for testing
DEVICE_ID="73B540BB-FE5C-4D99-A07D-7518DAFC4A90"
APP_BUNDLE="com.wandb.catnip"
SCREENSHOT_DIR="$HOME/Desktop/catnip-screenshots"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

echo "🚀 Testing iPad workspace detail split view..."

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
sleep 3

# Take initial screenshot (sidebar view)
SCREENSHOT_SIDEBAR="$SCREENSHOT_DIR/workspace-sidebar-${TIMESTAMP}.png"
echo "📸 Taking sidebar screenshot..."
xcrun simctl io "$DEVICE_ID" screenshot "$SCREENSHOT_SIDEBAR"
echo "Sidebar: $SCREENSHOT_SIDEBAR"

# Tap on first workspace in the list
# For iPad Pro 13-inch landscape (2732x2048), first workspace should be around x=180, y=300
echo "👆 Tapping first workspace..."
xcrun simctl io "$DEVICE_ID" tap 180 300

# Wait for workspace detail to load
sleep 4

# Take screenshot with workspace detail visible
SCREENSHOT_DETAIL="$SCREENSHOT_DIR/workspace-detail-${TIMESTAMP}.png"
echo "📸 Taking workspace detail screenshot..."
xcrun simctl io "$DEVICE_ID" screenshot "$SCREENSHOT_DETAIL"
echo "Detail view: $SCREENSHOT_DETAIL"

echo ""
echo "✅ Screenshots saved:"
echo "   Sidebar: $SCREENSHOT_SIDEBAR"
echo "   Detail:  $SCREENSHOT_DETAIL"
