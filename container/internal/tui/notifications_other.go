//go:build !darwin

package tui

import (
	"fmt"
)

// SendNativeNotification is a no-op on non-macOS platforms
func SendNativeNotification(title, body, subtitle, url string) error {
	debugLog("Notification (unsupported platform): %s - %s (URL: %s)", title, body, url)
	return fmt.Errorf("native notifications not supported on this platform")
}

// IsNotificationSupported returns false on non-macOS platforms
func IsNotificationSupported() bool {
	return false
}

// HasNotificationPermission always returns false on non-macOS platforms
func HasNotificationPermission() bool {
	return false
}
