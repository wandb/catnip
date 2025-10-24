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
import UIKit

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

    // Background polling manager (only available in main app, not widget extension)
    #if !WIDGET_EXTENSION
    private let backgroundManager = BackgroundProgressManager()
    #endif
    private var isInBackground = false

    // App Group for sharing data with widget extension
    private let sharedDefaults = UserDefaults(suiteName: "group.com.wandb.catnip")

    private init() {
        NSLog("🎯 CodespaceCreationTracker initialized")

        #if !WIDGET_EXTENSION
        // Skip setup in UI testing mode to prevent RunLoop blocking (main app only)
        if UITestingHelper.isUITesting {
            NSLog("🎯 UI testing mode - skipping background manager and notification observers")
            return
        }
        #endif

        // Verify app group is accessible
        if sharedDefaults != nil {
            NSLog("🎯 ✅ App group 'group.com.wandb.catnip' is accessible")
        } else {
            NSLog("🎯 ⚠️ App group 'group.com.wandb.catnip' not accessible - using standard UserDefaults")
        }

        // Set up background manager callback (main app only)
        #if !WIDGET_EXTENSION
        backgroundManager.onProgressUpdate = { [weak self] in
            self?.updateProgress()
        }
        #endif

        // Observe app state changes (main app only)
        #if !WIDGET_EXTENSION
        NotificationCenter.default.addObserver(
            self,
            selector: #selector(appDidEnterBackground),
            name: UIApplication.didEnterBackgroundNotification,
            object: nil
        )

        NotificationCenter.default.addObserver(
            self,
            selector: #selector(appWillEnterForeground),
            name: UIApplication.willEnterForegroundNotification,
            object: nil
        )
        #endif
    }

    deinit {
        NotificationCenter.default.removeObserver(self)
    }

    // MARK: - App State Handlers

    #if !WIDGET_EXTENSION
    @objc private func appDidEnterBackground() {
        NSLog("🎯 App entered background - switching to background polling")
        isInBackground = true

        guard isCreating, let codespaceName = codespaceName else { return }

        // Stop foreground timer
        progressTimer?.invalidate()
        progressTimer = nil

        // Start background polling
        backgroundManager.startPolling(codespaceName: codespaceName)
    }

    @objc private func appWillEnterForeground() {
        NSLog("🎯 App entering foreground - switching to timer-based updates")
        isInBackground = false

        guard isCreating else { return }

        // Stop background polling
        backgroundManager.stopPolling()

        // Restart foreground timer
        startProgressTimer()

        // Immediately update progress
        updateProgress()
    }
    #endif

    // MARK: - Public API

    /// Start tracking a new codespace creation
    /// - Parameters:
    ///   - repositoryName: The repository being launched
    ///   - codespaceName: The codespace name (if available)
    func startCreation(repositoryName: String, codespaceName: String? = nil) {
        NSLog("🎯 Starting codespace creation tracking for: \(repositoryName)")

        // Prevent re-starting if already creating the same repository
        if isCreating && self.repositoryName == repositoryName {
            NSLog("🎯 ⚠️ Already tracking creation for \(repositoryName), ignoring duplicate call")
            return
        }

        // Clean up any existing tracking for a different repository
        if isCreating {
            NSLog("🎯 Stopping previous creation tracking before starting new one")
            stopCreation()
        }

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

        // Start progress tracking based on app state
        #if !WIDGET_EXTENSION
        if UIApplication.shared.applicationState == .background {
            NSLog("🎯 App is backgrounded, starting background polling")
            isInBackground = true
            if let name = codespaceName {
                backgroundManager.startPolling(codespaceName: name)
            }
        } else {
            NSLog("🎯 App is active, starting timer-based progress")
            isInBackground = false
            startProgressTimer()
        }
        #else
        // Widget extension: always use timer-based progress
        startProgressTimer()
        #endif
    }

    /// Update with the codespace name once available
    /// - Parameter codespaceName: The codespace name
    func updateCodespaceName(_ codespaceName: String) {
        NSLog("🎯 Updating codespace name to: \(codespaceName)")
        DispatchQueue.main.async {
            self.codespaceName = codespaceName

            // If we're in background and didn't have a codespace name before, start polling now
            #if !WIDGET_EXTENSION
            if self.isInBackground && self.isCreating {
                NSLog("🎯 Codespace name now available, starting background polling")
                self.backgroundManager.startPolling(codespaceName: codespaceName)
            }
            #endif
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

        // Prevent duplicate completion calls
        guard progress < 1.0 else {
            NSLog("🎯 ⚠️ Already completed (progress=\(progress)), ignoring duplicate call")
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
            #if !WIDGET_EXTENSION
            self.backgroundManager.stopPolling()
            #endif
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
        guard let startTime = startTime else {
            NSLog("🎯 ⚠️ updateProgress called but no startTime set")
            return
        }

        guard isCreating else {
            NSLog("🎯 ⚠️ updateProgress called but not creating, stopping timer")
            progressTimer?.invalidate()
            progressTimer = nil
            return
        }

        let elapsed = Date().timeIntervalSince(startTime)

        // Progress calculation:
        // - 0-5 minutes: linear from 0% to 95%
        // - After 5 minutes: add 1% per additional minute (96% at 6min, 97% at 7min, etc.)
        let calculatedProgress: Double
        if elapsed <= estimatedDuration {
            // Linear progress to 95% over 5 minutes
            calculatedProgress = (elapsed / estimatedDuration) * 0.95
        } else {
            // After 5 minutes, add 1% per additional minute
            let additionalMinutes = (elapsed - estimatedDuration) / 60.0
            calculatedProgress = 0.95 + (additionalMinutes * 0.01)
        }

        // Cap at 99% - never reach 100% until actually complete
        let cappedProgress = min(calculatedProgress, 0.99)

        DispatchQueue.main.async {
            self.elapsedTime = elapsed
            self.progress = cappedProgress
            self.updateLiveActivity()

            NSLog("🎯 Progress update: \(Int(cappedProgress * 100))% (\(Int(elapsed))s elapsed)")
        }
    }

    // MARK: - Live Activity Management

    private func startLiveActivity(repositoryName: String) {
        NSLog("🎯 🔍 startLiveActivity() called for: \(repositoryName)")

        // Only available on iOS 16.1+
        guard #available(iOS 16.1, *) else {
            NSLog("🎯 ❌ Live Activities not available on this iOS version")
            return
        }
        NSLog("🎯 ✅ iOS version check passed (iOS 16.1+)")

        // Check if Live Activities are enabled
        let authInfo = ActivityAuthorizationInfo()
        let areEnabled = authInfo.areActivitiesEnabled
        NSLog("🎯 🔍 ActivityAuthorizationInfo().areActivitiesEnabled = \(areEnabled)")

        guard areEnabled else {
            NSLog("🎯 ❌ Live Activities are DISABLED by user - check Settings > [Your App] > Live Activities")
            return
        }
        NSLog("🎯 ✅ Live Activities are ENABLED")

        do {
            NSLog("🎯 🔍 Creating activity attributes and initial state...")
            let attributes = CodespaceActivityAttributes(repositoryName: repositoryName)
            let initialState = CodespaceActivityAttributes.ContentState(
                status: "Creating codespace in \(repositoryName)...",
                progress: 0.0,
                elapsedSeconds: 0
            )
            NSLog("🎯 🔍 Attributes: repositoryName=\(repositoryName)")
            NSLog("🎯 🔍 Initial state: status='\(initialState.status)', progress=\(initialState.progress), elapsedSeconds=\(initialState.elapsedSeconds)")

            if #available(iOS 16.2, *) {
                NSLog("🎯 🔍 Requesting activity using iOS 16.2+ API...")
                activity = try Activity<CodespaceActivityAttributes>.request(
                    attributes: attributes,
                    content: .init(state: initialState, staleDate: nil),
                    pushType: nil
                )
            } else {
                NSLog("🎯 🔍 Requesting activity using iOS 16.1 API...")
                activity = try Activity<CodespaceActivityAttributes>.request(
                    attributes: attributes,
                    contentState: initialState,
                    pushType: nil
                )
            }

            if let activity = activity {
                NSLog("🎯 ✅ Successfully started Live Activity!")
                NSLog("🎯    Activity ID: \(activity.id)")
                NSLog("🎯    Activity state: \(activity.activityState)")
                NSLog("🎯    Content: \(activity.content)")
            } else {
                NSLog("🎯 ⚠️ Activity request succeeded but activity is nil")
            }
        } catch {
            NSLog("🎯 ❌ Failed to start Live Activity!")
            NSLog("🎯    Error: \(error)")
            NSLog("🎯    Error localized description: \(error.localizedDescription)")
            NSLog("🎯    Error type: \(type(of: error))")
        }
    }

    private func updateLiveActivity() {
        guard #available(iOS 16.1, *),
              let activity = activity,
              let repositoryName = repositoryName else {
            NSLog("🎯 🔍 updateLiveActivity() skipped - activity=\(activity != nil), repo=\(repositoryName != nil)")
            return
        }

        let state = CodespaceActivityAttributes.ContentState(
            status: "Creating codespace in \(repositoryName)...",
            progress: progress,
            elapsedSeconds: Int(elapsedTime)
        )

        NSLog("🎯 🔍 Updating Live Activity - progress: \(Int(progress * 100))%, elapsed: \(Int(elapsedTime))s")

        Task {
            do {
                if #available(iOS 16.2, *) {
                    await activity.update(.init(state: state, staleDate: nil))
                } else {
                    await activity.update(using: state)
                }
                NSLog("🎯 ✅ Live Activity updated successfully")
            } catch {
                NSLog("🎯 ❌ Failed to update Live Activity: \(error)")
            }
        }
    }

    private func endLiveActivity() {
        guard #available(iOS 16.1, *),
              let activity = activity else {
            NSLog("🎯 🔍 endLiveActivity() skipped - no active activity")
            return
        }

        NSLog("🎯 🔍 Ending Live Activity...")

        Task {
            let finalState = CodespaceActivityAttributes.ContentState(
                status: progress >= 1.0 ? "Codespace ready!" : "Creation stopped",
                progress: progress,
                elapsedSeconds: Int(elapsedTime)
            )

            NSLog("🎯 🔍 Final state: '\(finalState.status)', progress: \(Int(progress * 100))%")

            do {
                if #available(iOS 16.2, *) {
                    await activity.end(.init(state: finalState, staleDate: nil), dismissalPolicy: .after(.now + 3))
                } else {
                    await activity.end(using: finalState, dismissalPolicy: .after(.now + 3))
                }
                NSLog("🎯 ✅ Live Activity ended successfully - will dismiss after 3 seconds")
            } catch {
                NSLog("🎯 ❌ Failed to end Live Activity: \(error)")
            }

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
