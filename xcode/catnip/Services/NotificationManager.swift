//
//  NotificationManager.swift
//  catnip
//
//  Manages local notifications for codespace creation
//

import Foundation
import UserNotifications
import UIKit
import Combine

class NotificationManager: NSObject, ObservableObject {
    static let shared = NotificationManager()

    // Notification categories and identifiers
    private enum NotificationCategory {
        static let codespaceReady = "CODESPACE_READY"
        static let codespaceFailed = "CODESPACE_FAILED"
    }

    // Published state for permission status
    @Published var authorizationStatus: UNAuthorizationStatus = .notDetermined

    // Callback for handling notification taps
    var onNotificationTap: ((String, String?) -> Void)?

    override private init() {
        super.init()
        UNUserNotificationCenter.current().delegate = self
        checkAuthorizationStatus()
    }

    // MARK: - Permission Management

    /// Check current notification authorization status
    func checkAuthorizationStatus() {
        UNUserNotificationCenter.current().getNotificationSettings { settings in
            DispatchQueue.main.async {
                self.authorizationStatus = settings.authorizationStatus
            }
        }
    }

    /// Request notification permissions from the user
    /// - Returns: True if authorized (or already authorized), false otherwise
    @MainActor
    func requestPermission() async -> Bool {
        let center = UNUserNotificationCenter.current()

        do {
            let granted = try await center.requestAuthorization(options: [.alert, .sound, .badge])
            self.authorizationStatus = granted ? .authorized : .denied
            NSLog("🔔 Notification permission: \(granted ? "granted" : "denied")")
            return granted
        } catch {
            NSLog("🔔 ❌ Failed to request notification permission: \(error)")
            return false
        }
    }

    // MARK: - Notification Scheduling

    /// Schedule a notification for when codespace is ready
    /// - Parameters:
    ///   - codespaceName: The name of the codespace
    ///   - repositoryName: The repository name (for display)
    func scheduleCodespaceReadyNotification(codespaceName: String, repositoryName: String) {
        let content = UNMutableNotificationContent()
        content.title = "Codespace Ready"
        content.body = "Your codespace '\(codespaceName)' is ready!"
        content.sound = .default
        content.categoryIdentifier = NotificationCategory.codespaceReady

        // Add codespace name to userInfo for deep linking
        content.userInfo = [
            "codespace_name": codespaceName,
            "repository_name": repositoryName,
            "action": "open_codespace"
        ]

        // Deliver immediately
        let trigger = UNTimeIntervalNotificationTrigger(timeInterval: 1, repeats: false)
        let request = UNNotificationRequest(
            identifier: "codespace-ready-\(codespaceName)",
            content: content,
            trigger: trigger
        )

        UNUserNotificationCenter.current().add(request) { error in
            if let error = error {
                NSLog("🔔 ❌ Failed to schedule notification: \(error)")
            } else {
                NSLog("🔔 ✅ Scheduled codespace ready notification for: \(codespaceName)")
            }
        }
    }

    /// Schedule a notification for when codespace creation fails
    /// - Parameters:
    ///   - repositoryName: The repository name
    ///   - errorMessage: The error message
    func scheduleCodespaceFailedNotification(repositoryName: String, errorMessage: String) {
        let content = UNMutableNotificationContent()
        content.title = "Codespace Creation Failed"
        content.body = "Failed to create codespace in \(repositoryName): \(errorMessage)"
        content.sound = .default
        content.categoryIdentifier = NotificationCategory.codespaceFailed

        content.userInfo = [
            "repository_name": repositoryName,
            "action": "show_error",
            "error": errorMessage
        ]

        // Deliver immediately
        let trigger = UNTimeIntervalNotificationTrigger(timeInterval: 1, repeats: false)
        let request = UNNotificationRequest(
            identifier: "codespace-failed-\(repositoryName)-\(Date().timeIntervalSince1970)",
            content: content,
            trigger: trigger
        )

        UNUserNotificationCenter.current().add(request) { error in
            if let error = error {
                NSLog("🔔 ❌ Failed to schedule error notification: \(error)")
            } else {
                NSLog("🔔 ✅ Scheduled codespace failed notification for: \(repositoryName)")
            }
        }
    }

    /// Cancel all pending notifications for a specific codespace
    /// - Parameter codespaceName: The codespace name
    func cancelNotifications(forCodespace codespaceName: String) {
        UNUserNotificationCenter.current().removePendingNotificationRequests(
            withIdentifiers: ["codespace-ready-\(codespaceName)"]
        )
        NSLog("🔔 Cancelled notifications for: \(codespaceName)")
    }

    /// Cancel all pending notifications
    func cancelAllNotifications() {
        UNUserNotificationCenter.current().removeAllPendingNotificationRequests()
        NSLog("🔔 Cancelled all notifications")
    }
}

// MARK: - UNUserNotificationCenterDelegate

extension NotificationManager: UNUserNotificationCenterDelegate {
    /// Called when notification is delivered while app is in foreground
    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification,
        withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void
    ) {
        NSLog("🔔 Notification will present: \(notification.request.identifier)")
        // Show notification even when app is in foreground
        completionHandler([.banner, .sound, .badge])
    }

    /// Called when user taps on notification
    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse,
        withCompletionHandler completionHandler: @escaping () -> Void
    ) {
        let userInfo = response.notification.request.content.userInfo
        NSLog("🔔 Notification tapped: \(response.notification.request.identifier)")
        NSLog("🔔 User info: \(userInfo)")

        // Handle the notification action
        if let action = userInfo["action"] as? String {
            switch action {
            case "open_codespace":
                if let codespaceName = userInfo["codespace_name"] as? String {
                    NSLog("🔔 Opening codespace: \(codespaceName)")
                    // Trigger the callback to handle navigation
                    DispatchQueue.main.async {
                        self.onNotificationTap?(codespaceName, "open_codespace")
                    }
                }
            case "show_error":
                NSLog("🔔 Showing error screen")
                DispatchQueue.main.async {
                    self.onNotificationTap?("", "show_error")
                }
            default:
                NSLog("🔔 Unknown action: \(action)")
            }
        }

        completionHandler()
    }
}
