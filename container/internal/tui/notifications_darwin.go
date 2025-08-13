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
    // Debug: NSLog(@"[Catnip] Notification activated with type: %ld", (long)notification.activationType);

    // Handle both action button clicks and notification body clicks
    if (notification.activationType == NSUserNotificationActivationTypeActionButtonClicked ||
        notification.activationType == NSUserNotificationActivationTypeContentsClicked) {
        NSString *url = notification.userInfo[@"url"];
        // Debug: NSLog(@"[Catnip] Opening URL: %@", url);

        if (url && [url length] > 0) {
            NSURL *urlToOpen = [NSURL URLWithString:url];
            if (urlToOpen) {
                BOOL success = [[NSWorkspace sharedWorkspace] openURL:urlToOpen];
                // Debug: NSLog(@"[Catnip] URL open result: %@", success ? @"SUCCESS" : @"FAILED");
            } else {
                // Debug: NSLog(@"[Catnip] ERROR: Invalid URL format: %@", url);
            }
        } else {
            // Debug: NSLog(@"[Catnip] ERROR: No URL found in notification userInfo");
        }
    }
}
@end

static NotificationDelegate *notificationDelegate = nil;

#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"

// Initialize the notification system once per process
void initializeNotificationSystem() {
    static dispatch_once_t onceToken;
    dispatch_once(&onceToken, ^{
        @autoreleasepool {
            // Initialize NSApplication - required for notifications
            NSApplication *app = [NSApplication sharedApplication];
            [app setActivationPolicy:NSApplicationActivationPolicyAccessory];

            // Create notification center and set up persistent delegate
            NSUserNotificationCenter *center = [NSUserNotificationCenter defaultUserNotificationCenter];
            if (!center) {
                // NSLog(@"[Catnip] ERROR: Could not get NSUserNotificationCenter");
                return;
            }

            // Set up persistent delegate for handling all notifications
            if (!notificationDelegate) {
                notificationDelegate = [[NotificationDelegate alloc] init];
                center.delegate = notificationDelegate;

                // Keep the delegate alive for the entire process lifetime
                CFRetain((__bridge CFTypeRef)notificationDelegate);
            }
        }
    });
}

// Clean, simple notification implementation - no event loops needed
void sendNotification(const char* title, const char* body, const char* subtitle, const char* url, int waitSeconds) {
    @autoreleasepool {
        // Initialize notification system (once per process)
        initializeNotificationSystem();

        // Check bundle identifier
        NSBundle *bundle = [NSBundle mainBundle];
        NSString *bundleId = [bundle bundleIdentifier];
        if (!bundleId || [bundleId isEqualToString:@""]) {
            // NSLog(@"[Catnip] ERROR: No bundle identifier - notifications may not work properly");
        }

        // Get notification center
        NSUserNotificationCenter *center = [NSUserNotificationCenter defaultUserNotificationCenter];
        if (!center) {
            // NSLog(@"[Catnip] ERROR: Could not get NSUserNotificationCenter");
            return;
        }

        // Create notification
        NSUserNotification *notification = [[NSUserNotification alloc] init];
        notification.title = [NSString stringWithUTF8String:title];
        notification.informativeText = [NSString stringWithUTF8String:body];

        if (subtitle && strlen(subtitle) > 0) {
            notification.subtitle = [NSString stringWithUTF8String:subtitle];
        }

        // Add URL to userInfo - use default workspace URL if none provided
        NSString *urlString;
        if (url && strlen(url) > 0) {
            urlString = [NSString stringWithUTF8String:url];
        } else {
            urlString = @"http://localhost:8080/workspace";
        }
        notification.userInfo = @{@"url": urlString};
        notification.hasActionButton = YES;
        notification.actionButtonTitle = @"Show";
        notification.soundName = NSUserNotificationDefaultSoundName;

        // Deliver notification
        [center deliverNotification:notification];

        // For CLI usage, wait briefly to ensure notification is displayed
        // For server usage (waitSeconds = 0), delegate handles everything persistently
        if (waitSeconds > 0) {
            NSDate *timeout = [NSDate dateWithTimeIntervalSinceNow:(double)waitSeconds];
            NSRunLoop *runLoop = [NSRunLoop currentRunLoop];

            while ([timeout timeIntervalSinceNow] > 0) {
                [runLoop runMode:NSDefaultRunLoopMode beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.1]];
            }
        }
    }
}

// No-op for permission requests - NSUserNotification doesn't need explicit permissions
void requestNotificationPermission() {
    // Debug: NSLog(@"[Catnip] NSUserNotification doesn't require permission requests");
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

// SendNativeNotification sends a native macOS notification for CLI usage (with wait)
func SendNativeNotification(title, body, subtitle, url string) error {
	cTitle := C.CString(title)
	cBody := C.CString(body)
	cSubtitle := C.CString(subtitle)
	cURL := C.CString(url)

	defer C.free(unsafe.Pointer(cTitle))
	defer C.free(unsafe.Pointer(cBody))
	defer C.free(unsafe.Pointer(cSubtitle))
	defer C.free(unsafe.Pointer(cURL))

	// CLI usage: wait 10 seconds to ensure notification is displayed and clickable
	C.sendNotification(cTitle, cBody, cSubtitle, cURL, C.int(10))
	return nil
}

// SendNativeNotificationAsync sends a native macOS notification for server usage (no wait)
func SendNativeNotificationAsync(title, body, subtitle, url string) error {
	cTitle := C.CString(title)
	cBody := C.CString(body)
	cSubtitle := C.CString(subtitle)
	cURL := C.CString(url)

	defer C.free(unsafe.Pointer(cTitle))
	defer C.free(unsafe.Pointer(cBody))
	defer C.free(unsafe.Pointer(cSubtitle))
	defer C.free(unsafe.Pointer(cURL))

	// Server usage: no wait - persistent delegate handles everything
	C.sendNotification(cTitle, cBody, cSubtitle, cURL, C.int(0))
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
