//
//  catnipApp.swift
//  catnip
//
//  Created by CVP on 10/5/25.
//

import SwiftUI

@main
struct catnipApp: App {
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
                .environmentObject(authManager)
                .environmentObject(notificationManager)
                .onAppear {
                    setupNotificationHandling()
                }
        }
    }

    private func setupNotificationHandling() {
        // Set up notification tap handler
        notificationManager.onNotificationTap = { codespaceName, action in
            NSLog("🔔 App handling notification tap - codespace: \(codespaceName), action: \(action ?? "none")")

            if action == "open_codespace" && !codespaceName.isEmpty {
                // Store the codespace name that should be opened
                UserDefaults.standard.set(codespaceName, forKey: "codespace_name")

                // The navigation will be handled by ContentView/CodespaceView
                // when it appears or becomes active. The existing handleConnect()
                // flow will pick up the stored codespace name and trigger SSE connection.
                NSLog("🔔 Stored codespace name for connection: \(codespaceName)")

                // Post notification to trigger connection flow
                NotificationCenter.default.post(
                    name: NSNotification.Name("TriggerCodespaceConnection"),
                    object: nil,
                    userInfo: ["codespace_name": codespaceName]
                )
            }
        }
    }
}
