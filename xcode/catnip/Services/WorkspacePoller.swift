//
//  WorkspacePoller.swift
//  catnip
//
//  Adaptive polling service for workspace updates with network efficiency
//

import Foundation
import UIKit
import Combine

/// Adaptive polling intervals based on workspace activity state
enum PollingInterval {
    case active       // Claude actively working
    case recentWork   // Work finished recently (one-time 5s delay)
    case idle         // No recent activity
    case background   // App backgrounded
    case suspended    // No polling

    var timeInterval: TimeInterval {
        switch self {
        case .active:       return 3.0  // Poll every 3s when Claude is actively working
        case .recentWork:   return 5.0  // One-time 5s delay after work completes
        case .idle:         return .infinity // No polling when idle (.running or .inactive)
        case .background:   return 30.0 // Very slow when backgrounded
        case .suspended:    return .infinity // No polling
        }
    }

    var description: String {
        switch self {
        case .active:       return "active (3s)"
        case .recentWork:   return "recent (5s one-time)"
        case .idle:         return "idle (no polling)"
        case .background:   return "background (30s)"
        case .suspended:    return "suspended"
        }
    }
}

/// Manages adaptive polling for a single workspace
@MainActor
class WorkspacePoller: ObservableObject {
    // MARK: - Published Properties
    @Published private(set) var isPolling = false
    @Published private(set) var currentInterval: PollingInterval = .idle
    @Published private(set) var lastUpdate: Date?
    @Published private(set) var workspace: WorkspaceInfo?
    @Published private(set) var sessionData: SessionData?
    @Published private(set) var error: String?

    // MARK: - Private Properties
    private let workspaceId: String
    private var pollingTask: Task<Void, Never>?
    private var appStateObserver: NSObjectProtocol?
    private var lastETag: String?  // ETag for workspace data polling
    private var lastSessionETag: String?  // ETag for session data polling
    private var lastActivityStateChange: Date = Date()
    private var previousActivityState: ClaudeActivityState?
    private var hasPolledAfterTransition = false  // Track if we've done the one-time poll after .active→.running/.inactive

    // MARK: - Initialization

    init(workspaceId: String, initialWorkspace: WorkspaceInfo? = nil) {
        self.workspaceId = workspaceId

        // Initialize with provided workspace if available
        if let initialWorkspace = initialWorkspace {
            self.workspace = initialWorkspace
            self.lastActivityStateChange = Date()
            self.previousActivityState = initialWorkspace.claudeActivityState
            NSLog("📊 Initialized poller with existing workspace data, id: \(workspaceId)")
        }

        setupAppStateObservers()
    }

    deinit {
        // Cancel polling task
        pollingTask?.cancel()

        // Remove observers
        if let observer = appStateObserver {
            NotificationCenter.default.removeObserver(observer)
        }
    }

    // MARK: - Public API

    /// Start polling with automatic interval adaptation
    func start() {
        guard !isPolling else { return }
        isPolling = true

        NSLog("📊 Starting adaptive polling for workspace: \(workspaceId)")
        scheduleNextPoll()
    }

    /// Stop all polling
    func stop() {
        pollingTask?.cancel()
        pollingTask = nil
        isPolling = false
        NSLog("📊 Stopped polling for workspace: \(workspaceId)")
    }

    /// Force immediate update
    func refresh() {
        NSLog("📊 Force refresh requested for workspace: \(workspaceId)")
        pollingTask?.cancel()
        scheduleNextPoll()
    }

    /// Update session data (for manual hydration)
    func updateSessionData(_ data: SessionData?) {
        self.sessionData = data
    }

    // MARK: - Private Methods

    private func scheduleNextPoll() {
        pollingTask?.cancel()

        let interval = determinePollingInterval()
        let previousInterval = currentInterval
        currentInterval = interval

        if previousInterval != currentInterval {
            NSLog("📊 Polling interval changed: \(previousInterval.description) → \(currentInterval.description)")
        }

        guard isPolling, interval.timeInterval < .infinity else { return }

        pollingTask = Task { [weak self] in
            // Wait for interval
            try? await Task.sleep(nanoseconds: UInt64(interval.timeInterval * 1_000_000_000))

            guard !Task.isCancelled else { return }

            await self?.pollWorkspace()

            // Schedule next poll
            self?.scheduleNextPoll()
        }
    }

    private func determinePollingInterval() -> PollingInterval {
        // Check if app is in background
        if UIApplication.shared.applicationState == .background {
            return .background
        }

        guard let workspace = workspace else {
            return .idle  // No workspace yet, use idle rate
        }

        // State meanings:
        // .inactive = no PTY running → no polling needed
        // .running = PTY up, waiting for user input → no polling needed
        // .active = Claude is actively working → poll every 3s

        // .active = Claude is actively working → fast polling
        if workspace.claudeActivityState == .active {
            hasPolledAfterTransition = false  // Reset flag when actively working
            return .active
        }

        // Transitioning from .active to .running or .inactive
        // Do one 5s delayed poll to catch final updates, then stop polling
        if !hasPolledAfterTransition {
            return .recentWork
        }

        // .running or .inactive = no polling needed
        return .idle
    }

    private func pollWorkspace() async {
        // For active sessions, use the lightweight session endpoint
        let activityState = workspace?.claudeActivityState
        let isActiveSession = activityState == .active

        if isActiveSession {
            await pollSessionData()
        } else {
            await pollFullWorkspace()
        }

        // If this was the one-time poll after transitioning from .active, mark it complete
        if currentInterval == .recentWork {
            hasPolledAfterTransition = true
            NSLog("📊 Completed one-time post-transition poll, stopping polling until Claude becomes active again")
        }
    }

    /// Poll the lightweight session endpoint for active sessions with ETag support
    private func pollSessionData() async {
        do {
            let result = try await CatnipAPI.shared.getSessionData(
                workspaceId: workspaceId,
                ifNoneMatch: lastSessionETag
            )

            // Handle 304 Not Modified - data unchanged
            guard let result = result else {
                NSLog("📊 Session data unchanged (304 Not Modified), keeping existing data")
                return
            }

            // Update with new session data and ETag
            self.sessionData = result.sessionData
            lastSessionETag = result.etag
            lastUpdate = Date()
            error = nil

            // Check if session is still active based on session info
            let isActive = result.sessionData.sessionInfo?.isActive ?? false
            let previousState = workspace?.claudeActivityState

            // Track activity state changes for polling interval adaptation
            // .active means Claude is actively working
            // When session stops being active, transition to .running (PTY exists, waiting for input)
            let wasWorking = previousState == .active
            if !isActive && wasWorking {
                lastActivityStateChange = Date()
                NSLog("📊 Claude stopped working, transitioning from .active to .running")

                // Update workspace to .running (PTY still exists, just waiting for input)
                // This ensures determinePollingInterval() sees the correct state
                workspace = workspace?.with(claudeActivityState: .running)
            }
        } catch {
            // Fall back to full workspace polling on error
            NSLog("⚠️ Session polling failed, falling back to full workspace: \(error.localizedDescription)")
            await pollFullWorkspace()
        }
    }

    /// Poll the full workspace endpoint (heavier, used for idle workspaces)
    private func pollFullWorkspace() async {
        do {
            let result = try await CatnipAPI.shared.getWorkspace(
                id: workspaceId,
                ifNoneMatch: lastETag
            )

            // Handle 304 Not Modified - no updates
            if result == nil {
                NSLog("📊 No changes (304 Not Modified)")
                return
            }

            // Update state with new workspace data and ETag
            let previousState = workspace?.claudeActivityState
            workspace = result?.workspace
            lastETag = result?.etag
            lastUpdate = Date()
            error = nil

            // Track activity state changes for interval adaptation
            if previousState != workspace?.claudeActivityState {
                lastActivityStateChange = Date()
                NSLog("📊 Activity state changed: \(previousState?.rawValue ?? "nil") → \(workspace?.claudeActivityState?.rawValue ?? "nil")")
            }

            NSLog("📊 Workspace updated - Activity: \(workspace?.claudeActivityState?.rawValue ?? "unknown"), TODOs: \(workspace?.todos?.count ?? 0)")

        } catch {
            self.error = error.localizedDescription
            NSLog("❌ Polling error: \(error.localizedDescription)")
        }
    }

    // MARK: - App Lifecycle Observers

    private func setupAppStateObservers() {
        // Observe app entering background
        appStateObserver = NotificationCenter.default.addObserver(
            forName: UIApplication.didEnterBackgroundNotification,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            Task { @MainActor in
                NSLog("📊 App entered background - switching to background polling")
                self?.pollingTask?.cancel()
                self?.scheduleNextPoll()
            }
        }

        // Observe app entering foreground
        NotificationCenter.default.addObserver(
            forName: UIApplication.willEnterForegroundNotification,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            Task { @MainActor in
                NSLog("📊 App entered foreground - resuming active polling")
                self?.refresh() // Immediate refresh on foreground
            }
        }
    }

    private func removeAppStateObservers() {
        if let observer = appStateObserver {
            NotificationCenter.default.removeObserver(observer)
        }
    }
}
