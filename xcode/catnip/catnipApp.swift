//
//  catnipApp.swift
//  catnip
//
//  Created by CVP on 10/5/25.
//

import SwiftUI
import UserNotifications

// App Delegate for handling push notification registration
class AppDelegate: NSObject, UIApplicationDelegate {
    func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil
    ) -> Bool {
        // Note: Notification permission is requested when user installs Catnip,
        // not on app launch. This prevents the permission dialog from appearing
        // immediately and interrupting UI tests.
        return true
    }

    func application(
        _ application: UIApplication,
        didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data
    ) {
        NotificationManager.shared.setDeviceToken(deviceToken)
    }

    func application(
        _ application: UIApplication,
        didFailToRegisterForRemoteNotificationsWithError error: Error
    ) {
        NSLog("📱 Failed to register for remote notifications: \(error)")
    }
}

@main
struct catnipApp: App {
    @UIApplicationDelegateAdaptor(AppDelegate.self) var appDelegate
    @StateObject private var authManager = AuthManager()
    @StateObject private var notificationManager = NotificationManager.shared
    @State private var navigationPath = NavigationPath()
    @State private var pendingCodespaceName: String?

    init() {
        // Disable animations during UI testing for faster tests
        if UITestingHelper.shouldDisableAnimations {
            UIView.setAnimationsEnabled(false)
        }
    }

    var body: some Scene {
        WindowGroup {
            ContentView()
                .withAdaptiveTheme()
                .environmentObject(authManager)
                .environmentObject(notificationManager)
                .onAppear {
                    setupNotificationHandling()
                }
        }
    }

    private func setupNotificationHandling() {
        // Set up notification tap handler
        notificationManager.onNotificationTap = { identifier, action in
            NSLog("🔔 App handling notification tap - identifier: \(identifier), action: \(action ?? "none")")

            if action == "open_codespace" && !identifier.isEmpty {
                // Store the codespace name that should be opened
                UserDefaults.standard.set(identifier, forKey: "codespace_name")
                NSLog("🔔 Stored codespace name for connection: \(identifier)")

                // Post notification to trigger connection flow
                NotificationCenter.default.post(
                    name: NSNotification.Name("TriggerCodespaceConnection"),
                    object: nil,
                    userInfo: ["codespace_name": identifier]
                )
            } else if action == "open_workspace" && !identifier.isEmpty {
                // Store the workspace ID for navigation
                UserDefaults.standard.set(identifier, forKey: "pending_workspace_id")
                NSLog("🔔 Stored workspace ID for navigation: \(identifier)")

                // Post notification to trigger workspace navigation
                NotificationCenter.default.post(
                    name: NSNotification.Name("OpenWorkspace"),
                    object: nil,
                    userInfo: ["workspaceId": identifier]
                )
            }
        }
    }
}
