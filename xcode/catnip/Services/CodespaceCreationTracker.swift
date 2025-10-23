//
//  CodespaceCreationTracker.swift
//  catnip
//
//  Tracks active codespace creation and manages Live Activity
//

import Foundation
import Combine
import ActivityKit
import SwiftUI

/// Tracks the state of an active codespace creation
class CodespaceCreationTracker: ObservableObject {
    static let shared = CodespaceCreationTracker()

    // MARK: - Published State

    @Published var isCreating: Bool = false
    @Published var repositoryName: String?
    @Published var codespaceName: String?
    @Published var progress: Double = 0.0
    @Published var elapsedTime: TimeInterval = 0

    // MARK: - Private State

    private var startTime: Date?
    private var progressTimer: Timer?
    private var activity: Activity<CodespaceActivityAttributes>?
    private let estimatedDuration: TimeInterval = 5 * 60 // 5 minutes

    // App Group for sharing data with widget extension
    private let sharedDefaults = UserDefaults(suiteName: "group.com.wandb.catnip")

    private init() {
        NSLog("🎯 CodespaceCreationTracker initialized")

        // Verify app group is accessible
        if sharedDefaults != nil {
            NSLog("🎯 ✅ App group 'group.com.wandb.catnip' is accessible")
        } else {
            NSLog("🎯 ⚠️ App group 'group.com.wandb.catnip' not accessible - using standard UserDefaults")
        }
    }

    // MARK: - Public API

    /// Start tracking a new codespace creation
    /// - Parameters:
    ///   - repositoryName: The repository being launched
    ///   - codespaceName: The codespace name (if available)
    func startCreation(repositoryName: String, codespaceName: String? = nil) {
        NSLog("🎯 Starting codespace creation tracking for: \(repositoryName)")

        // Clean up any existing tracking
        stopCreation()

        // Set state
        DispatchQueue.main.async {
            self.isCreating = true
            self.repositoryName = repositoryName
            self.codespaceName = codespaceName
            self.progress = 0.0
            self.elapsedTime = 0
            self.startTime = Date()
        }

        // Start Live Activity
        startLiveActivity(repositoryName: repositoryName)

        // Start progress timer
        startProgressTimer()
    }

    /// Update with the codespace name once available
    /// - Parameter codespaceName: The codespace name
    func updateCodespaceName(_ codespaceName: String) {
        NSLog("🎯 Updating codespace name to: \(codespaceName)")
        DispatchQueue.main.async {
            self.codespaceName = codespaceName
        }
    }

    /// Mark creation as complete and send success notification
    /// - Parameter codespaceName: The completed codespace name
    func completeCreation(codespaceName: String) {
        NSLog("🎯 ✅ Codespace creation completed: \(codespaceName)")

        guard isCreating else {
            NSLog("🎯 ⚠️ No active creation to complete")
            return
        }

        // Update to 100% and end activity
        DispatchQueue.main.async {
            self.progress = 1.0
            self.updateLiveActivity()

            // Send success notification
            if let repoName = self.repositoryName {
                NotificationManager.shared.scheduleCodespaceReadyNotification(
                    codespaceName: codespaceName,
                    repositoryName: repoName
                )
            }

            // End live activity after a brief delay
            DispatchQueue.main.asyncAfter(deadline: .now() + 2.0) {
                self.endLiveActivity()
                self.cleanup()
            }
        }
    }

    /// Mark creation as failed and send error notification
    /// - Parameter error: The error message
    func failCreation(error: String) {
        NSLog("🎯 ❌ Codespace creation failed: \(error)")

        guard isCreating else {
            NSLog("🎯 ⚠️ No active creation to fail")
            return
        }

        DispatchQueue.main.async {
            // Send failure notification
            if let repoName = self.repositoryName {
                NotificationManager.shared.scheduleCodespaceFailedNotification(
                    repositoryName: repoName,
                    errorMessage: error
                )
            }

            // End live activity
            self.endLiveActivity()
            self.cleanup()
        }
    }

    /// Stop tracking (e.g., when user cancels or app terminates)
    func stopCreation() {
        NSLog("🎯 Stopping codespace creation tracking")

        DispatchQueue.main.async {
            self.progressTimer?.invalidate()
            self.progressTimer = nil
            self.endLiveActivity()
            self.cleanup()
        }
    }

    // MARK: - Private Methods

    private func cleanup() {
        isCreating = false
        repositoryName = nil
        codespaceName = nil
        progress = 0.0
        elapsedTime = 0
        startTime = nil
    }

    private func startProgressTimer() {
        // Update progress every 10 seconds
        progressTimer = Timer.scheduledTimer(withTimeInterval: 10.0, repeats: true) { [weak self] _ in
            self?.updateProgress()
        }
    }

    private func updateProgress() {
        guard let startTime = startTime else { return }

        let elapsed = Date().timeIntervalSince(startTime)
        let rawProgress = elapsed / estimatedDuration

        // Use easing function to slow down near completion
        // Never reach 100% until actually complete
        let easedProgress = easeOutCubic(rawProgress)
        let cappedProgress = min(easedProgress, 0.98) // Cap at 98%

        DispatchQueue.main.async {
            self.elapsedTime = elapsed
            self.progress = cappedProgress
            self.updateLiveActivity()

            NSLog("🎯 Progress update: \(Int(cappedProgress * 100))% (\(Int(elapsed))s elapsed)")
        }
    }

    /// Ease-out cubic easing function for smoother progress
    private func easeOutCubic(_ t: Double) -> Double {
        let t = min(max(t, 0), 1) // Clamp to [0, 1]
        let x = 1 - t
        return 1 - (x * x * x)
    }

    // MARK: - Live Activity Management

    private func startLiveActivity(repositoryName: String) {
        // Only available on iOS 16.1+
        guard #available(iOS 16.1, *) else {
            NSLog("🎯 Live Activities not available on this iOS version")
            return
        }

        // Check if Live Activities are enabled
        guard ActivityAuthorizationInfo().areActivitiesEnabled else {
            NSLog("🎯 Live Activities are disabled by user")
            return
        }

        do {
            let attributes = CodespaceActivityAttributes(repositoryName: repositoryName)
            let initialState = CodespaceActivityAttributes.ContentState(
                status: "Creating codespace in \(repositoryName)...",
                progress: 0.0,
                elapsedSeconds: 0
            )

            if #available(iOS 16.2, *) {
                activity = try Activity<CodespaceActivityAttributes>.request(
                    attributes: attributes,
                    content: .init(state: initialState, staleDate: nil),
                    pushType: nil
                )
            } else {
                activity = try Activity<CodespaceActivityAttributes>.request(
                    attributes: attributes,
                    contentState: initialState,
                    pushType: nil
                )
            }

            NSLog("🎯 ✅ Started Live Activity: \(activity?.id ?? "unknown")")
        } catch {
            NSLog("🎯 ❌ Failed to start Live Activity: \(error)")
        }
    }

    private func updateLiveActivity() {
        guard #available(iOS 16.1, *),
              let activity = activity,
              let repositoryName = repositoryName else {
            return
        }

        let state = CodespaceActivityAttributes.ContentState(
            status: "Creating codespace in \(repositoryName)...",
            progress: progress,
            elapsedSeconds: Int(elapsedTime)
        )

        Task {
            if #available(iOS 16.2, *) {
                await activity.update(.init(state: state, staleDate: nil))
            } else {
                await activity.update(using: state)
            }
        }
    }

    private func endLiveActivity() {
        guard #available(iOS 16.1, *),
              let activity = activity else {
            return
        }

        Task {
            let finalState = CodespaceActivityAttributes.ContentState(
                status: progress >= 1.0 ? "Codespace ready!" : "Creation stopped",
                progress: progress,
                elapsedSeconds: Int(elapsedTime)
            )

            if #available(iOS 16.2, *) {
                await activity.end(.init(state: finalState, staleDate: nil), dismissalPolicy: .after(.now + 3))
            } else {
                await activity.end(using: finalState, dismissalPolicy: .after(.now + 3))
            }
            NSLog("🎯 Ended Live Activity")
            self.activity = nil
        }
    }
}

// MARK: - Activity Attributes (for Live Activity)

@available(iOS 16.1, *)
struct CodespaceActivityAttributes: ActivityAttributes {
    public struct ContentState: Codable, Hashable {
        var status: String
        var progress: Double
        var elapsedSeconds: Int
    }

    var repositoryName: String
}
