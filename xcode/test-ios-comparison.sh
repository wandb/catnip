#!/bin/bash
set -e

# Test on both iOS 18 and iOS 26 to ensure compatibility
# Using iPad Pro 13-inch on both versions for true OS comparison
DEVICE_IOS18="73B540BB-FE5C-4D99-A07D-7518DAFC4A90"  # iPad Pro 13-inch (M4), iOS 18.5
DEVICE_IOS26="FD3F1EFD-2EFD-4889-A484-008CB60E27B2"  # iPad Pro 13-inch (M4), iOS 26.0
APP_BUNDLE="com.wandb.catnip"
SCREENSHOT_DIR="$HOME/Desktop/catnip-screenshots"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

echo "🚀 Testing iPad on iOS 18 and iOS 26 for compatibility..."

# Create screenshot directory
mkdir -p "$SCREENSHOT_DIR"

# Build app once for both devices
echo "🔨 Building app..."
xcodebuild -project catnip.xcodeproj \
  -scheme catnip \
  -configuration Debug \
  -sdk iphonesimulator \
  -quiet \
  build

# Function to test a device
test_device() {
    local DEVICE_ID=$1
    local IOS_VERSION=$2
    local DEVICE_NAME=$3

    echo ""
    echo "📱 Testing on iOS ${IOS_VERSION} (${DEVICE_NAME})..."

    # Boot simulator if needed
    xcrun simctl boot "$DEVICE_ID" 2>/dev/null || true
    sleep 2

    # Install app
    echo "📦 Installing app on iOS ${IOS_VERSION}..."
    xcrun simctl install "$DEVICE_ID" \
      ~/Library/Developer/Xcode/DerivedData/Catnip-*/Build/Products/Debug-iphonesimulator/catnip.app

    # Launch app
    echo "🚀 Launching app on iOS ${IOS_VERSION}..."
    xcrun simctl launch "$DEVICE_ID" "$APP_BUNDLE" \
      -UITesting \
      -SkipAuthentication \
      -UseMockData \
      -ShowWorkspacesList

    # Wait for app to settle
    sleep 5

    # Take sidebar screenshot
    SCREENSHOT_SIDEBAR="$SCREENSHOT_DIR/ios${IOS_VERSION}-sidebar-${TIMESTAMP}.png"
    echo "📸 Taking sidebar screenshot..."
    xcrun simctl io "$DEVICE_ID" screenshot "$SCREENSHOT_SIDEBAR"
    echo "   Saved: $SCREENSHOT_SIDEBAR"

    # Terminate app
    xcrun simctl terminate "$DEVICE_ID" "$APP_BUNDLE" 2>/dev/null || true
}

# Test both versions (same device model, different iOS versions)
test_device "$DEVICE_IOS18" "18" "iPad Pro 13-inch (M4)"
test_device "$DEVICE_IOS26" "26" "iPad Pro 13-inch (M4)"

echo ""
echo "✅ Compatibility testing complete!"
echo ""
echo "📸 Screenshots saved to: $SCREENSHOT_DIR"
echo "   iOS 18: ios18-sidebar-${TIMESTAMP}.png"
echo "   iOS 26: ios26-sidebar-${TIMESTAMP}.png"
echo ""
echo "Compare these screenshots to verify layout consistency across iOS versions."
