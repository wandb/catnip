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
    case recentWork   // Work finished recently
    case idle         // No recent activity
    case background   // App backgrounded
    case suspended    // No polling

    var timeInterval: TimeInterval {
        switch self {
        case .active:       return 1.5  // Fast updates when Claude is working
        case .recentWork:   return 3.0  // Medium updates for recently completed work
        case .idle:         return 10.0 // Slow updates when idle
        case .background:   return 30.0 // Very slow when backgrounded
        case .suspended:    return .infinity // No polling
        }
    }

    var description: String {
        switch self {
        case .active:       return "active (1.5s)"
        case .recentWork:   return "recent (3s)"
        case .idle:         return "idle (10s)"
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
    @Published private(set) var error: String?

    // MARK: - Private Properties
    private let workspaceId: String
    private var pollingTask: Task<Void, Never>?
    private var appStateObserver: NSObjectProtocol?
    private var lastETag: String?
    private var lastActivityStateChange: Date = Date()
    private var previousActivityState: ClaudeActivityState?

    // MARK: - Initialization

    init(workspaceId: String, initialWorkspace: WorkspaceInfo? = nil) {
        self.workspaceId = workspaceId

        // Initialize with provided workspace if available
        if let initialWorkspace = initialWorkspace {
            self.workspace = initialWorkspace
            self.lastActivityStateChange = Date()
            self.previousActivityState = initialWorkspace.claudeActivityState
            NSLog("📊 Initialized poller with existing workspace data")
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

        let timeSinceLastChange = Date().timeIntervalSince(lastActivityStateChange)

        // Active: Claude is actively working
        if workspace.claudeActivityState == .active {
            return .active
        }

        // Recent work: Work finished less than 2 minutes ago
        // Keep polling at medium rate to catch final TODO updates and messages
        if timeSinceLastChange < 120 { // 2 minutes
            return .recentWork
        }

        // Idle: No recent activity
        return .idle
    }

    private func pollWorkspace() async {
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
