//go:build darwin

package tui

/*
#cgo CFLAGS: -x objective-c -mmacosx-version-min=10.14
#cgo LDFLAGS: -framework Foundation -framework Cocoa

#import <Foundation/Foundation.h>
#import <Cocoa/Cocoa.h>

@interface NotificationDelegate : NSObject <NSUserNotificationCenterDelegate>
@end

@implementation NotificationDelegate
- (BOOL)userNotificationCenter:(NSUserNotificationCenter *)center shouldPresentNotification:(NSUserNotification *)notification {
    return YES;
}

- (void)userNotificationCenter:(NSUserNotificationCenter *)center didActivateNotification:(NSUserNotification *)notification {
    if (notification.activationType == NSUserNotificationActivationTypeActionButtonClicked) {
        NSString *url = notification.userInfo[@"url"];
        if (url) {
            [[NSWorkspace sharedWorkspace] openURL:[NSURL URLWithString:url]];
        }
    }
}
@end

static NotificationDelegate *notificationDelegate = nil;

#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"

// Clean, simple notification implementation based on terminal-notifier pattern
void sendNotification(const char* title, const char* body, const char* subtitle, const char* url) {
    @autoreleasepool {
        // Debug: NSLog(@"[Catnip] Sending notification: %s", title);

        // Initialize NSApplication - required for notifications
        NSApplication *app = [NSApplication sharedApplication];
        [app setActivationPolicy:NSApplicationActivationPolicyAccessory];

        // Check bundle identifier to ensure we're running from app bundle
        NSBundle *bundle = [NSBundle mainBundle];
        NSString *bundleId = [bundle bundleIdentifier];
        // Debug: NSLog(@"[Catnip] Bundle ID: %@", bundleId);
        // Debug: NSLog(@"[Catnip] Bundle path: %@", [bundle bundlePath]);

        if (!bundleId || [bundleId isEqualToString:@""]) {
            NSLog(@"[Catnip] ERROR: No bundle identifier - notifications may not work properly");
        }

        // Create notification
        NSUserNotificationCenter *center = [NSUserNotificationCenter defaultUserNotificationCenter];
        if (!center) {
            NSLog(@"[Catnip] ERROR: Could not get NSUserNotificationCenter");
            return;
        }

        // Set up delegate for handling clicks (only once)
        if (!notificationDelegate) {
            notificationDelegate = [[NotificationDelegate alloc] init];
            center.delegate = notificationDelegate;
        }

        NSUserNotification *notification = [[NSUserNotification alloc] init];
        notification.title = [NSString stringWithUTF8String:title];
        notification.informativeText = [NSString stringWithUTF8String:body];

        if (subtitle && strlen(subtitle) > 0) {
            notification.subtitle = [NSString stringWithUTF8String:subtitle];
        }

        // Add URL to userInfo if provided
        if (url && strlen(url) > 0) {
            NSString *urlString = [NSString stringWithUTF8String:url];
            notification.userInfo = @{@"url": urlString};
            notification.hasActionButton = YES;
            notification.actionButtonTitle = @"Show";
        }

        notification.soundName = NSUserNotificationDefaultSoundName;

        // Deliver notification
        [center deliverNotification:notification];

        // CRITICAL: Run event loop briefly to let notification system process
        // This is the key missing piece that makes notifications work reliably
        NSDate *timeout = [NSDate dateWithTimeIntervalSinceNow:0.1];
        [[NSRunLoop currentRunLoop] runUntilDate:timeout];

        // Debug: NSLog(@"[Catnip] Notification delivered successfully");
    }
}

// No-op for permission requests - NSUserNotification doesn't need explicit permissions
void requestNotificationPermission() {
    NSLog(@"[Catnip] NSUserNotification doesn't require permission requests");
}

int isNotificationPermissionGranted() {
    return 1; // NSUserNotification works without explicit permission for app bundles
}

#pragma clang diagnostic pop
*/
import "C"
import (
	"unsafe"
)

// SendNativeNotification sends a native macOS notification using the clean, simple approach
func SendNativeNotification(title, body, subtitle string) error {
	cTitle := C.CString(title)
	cBody := C.CString(body)
	cSubtitle := C.CString(subtitle)

	defer C.free(unsafe.Pointer(cTitle))
	defer C.free(unsafe.Pointer(cBody))
	defer C.free(unsafe.Pointer(cSubtitle))

	C.sendNotification(cTitle, cBody, cSubtitle)
	return nil
}

// IsNotificationSupported returns true on macOS
func IsNotificationSupported() bool {
	return true
}

// HasNotificationPermission always returns true for NSUserNotification
func HasNotificationPermission() bool {
	return C.isNotificationPermissionGranted() == 1
}
