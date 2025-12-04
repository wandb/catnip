#!/bin/bash
set -e

# Test on both iOS 18 and iOS 26 to ensure compatibility
DEVICE_IOS18="73B540BB-FE5C-4D99-A07D-7518DAFC4A90"  # iPad Pro 13-inch (M4), iOS 18.5
DEVICE_IOS26="8059BBCF-6AB4-4C5F-AF9E-64A0B63BCE97"  # iPad (A16), iOS 26.0
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

    echo ""
    echo "📱 Testing on iOS ${IOS_VERSION}..."

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
    sleep 4

    # Take sidebar screenshot
    SCREENSHOT_SIDEBAR="$SCREENSHOT_DIR/ios${IOS_VERSION}-sidebar-${TIMESTAMP}.png"
    echo "📸 Taking sidebar screenshot..."
    xcrun simctl io "$DEVICE_ID" screenshot "$SCREENSHOT_SIDEBAR"
    echo "   Saved: $SCREENSHOT_SIDEBAR"

    # Tap first workspace (coordinates vary by device)
    echo "👆 Tapping first workspace..."
    if [ "$IOS_VERSION" = "26" ]; then
        # iPad (A16) has different resolution
        xcrun simctl ui "$DEVICE_ID" tap 180 280
    else
        # iPad Pro 13-inch
        xcrun simctl ui "$DEVICE_ID" tap 180 300
    fi

    # Wait for detail view to load
    sleep 4

    # Take detail screenshot
    SCREENSHOT_DETAIL="$SCREENSHOT_DIR/ios${IOS_VERSION}-detail-${TIMESTAMP}.png"
    echo "📸 Taking detail view screenshot..."
    xcrun simctl io "$DEVICE_ID" screenshot "$SCREENSHOT_DETAIL"
    echo "   Saved: $SCREENSHOT_DETAIL"

    # Terminate app
    xcrun simctl terminate "$DEVICE_ID" "$APP_BUNDLE" 2>/dev/null || true
}

# Test both versions
test_device "$DEVICE_IOS18" "18"
test_device "$DEVICE_IOS26" "26"

echo ""
echo "✅ Compatibility testing complete!"
echo ""
echo "📸 Screenshots saved to: $SCREENSHOT_DIR"
echo "   iOS 18: ios18-sidebar-${TIMESTAMP}.png, ios18-detail-${TIMESTAMP}.png"
echo "   iOS 26: ios26-sidebar-${TIMESTAMP}.png, ios26-detail-${TIMESTAMP}.png"
