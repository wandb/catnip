//go:build darwin

package tui

/*
#cgo CFLAGS: -x objective-c -mmacosx-version-min=10.14
#cgo LDFLAGS: -framework Foundation -framework UserNotifications

#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>
#import <dispatch/dispatch.h>

static int notificationPermissionGranted = 0;

void requestNotificationPermission() {
    @autoreleasepool {
        dispatch_semaphore_t semaphore = dispatch_semaphore_create(0);

        UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
        [center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound | UNAuthorizationOptionBadge)
            completionHandler:^(BOOL granted, NSError * _Nullable error) {
                notificationPermissionGranted = granted ? 1 : 0;
                dispatch_semaphore_signal(semaphore);
            }];

        dispatch_semaphore_wait(semaphore, DISPATCH_TIME_FOREVER);
    }
}

void sendNotification(const char* title, const char* body, const char* subtitle) {
    @autoreleasepool {
        if (!notificationPermissionGranted) {
            requestNotificationPermission();
        }

        UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
        content.title = [NSString stringWithUTF8String:title];
        content.body = [NSString stringWithUTF8String:body];
        if (subtitle && strlen(subtitle) > 0) {
            content.subtitle = [NSString stringWithUTF8String:subtitle];
        }
        content.sound = [UNNotificationSound defaultSound];

        // Create trigger (immediate delivery)
        UNTimeIntervalNotificationTrigger *trigger =
            [UNTimeIntervalNotificationTrigger triggerWithTimeInterval:0.1 repeats:NO];

        // Create unique identifier
        NSString *identifier = [[NSUUID UUID] UUIDString];

        // Create request
        UNNotificationRequest *request =
            [UNNotificationRequest requestWithIdentifier:identifier content:content trigger:trigger];

        // Add notification request
        [[UNUserNotificationCenter currentNotificationCenter]
            addNotificationRequest:request withCompletionHandler:^(NSError * _Nullable error) {
                if (error) {
                    NSLog(@"Error sending notification: %@", error.localizedDescription);
                }
            }];
    }
}

int isNotificationPermissionGranted() {
    return notificationPermissionGranted;
}
*/
import "C"
import (
	"unsafe"
)

func init() {
	// Request permission on startup
	C.requestNotificationPermission()
}

// SendNativeNotification sends a native macOS notification
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

// HasNotificationPermission checks if notification permission is granted
func HasNotificationPermission() bool {
	return C.isNotificationPermissionGranted() == 1
}
