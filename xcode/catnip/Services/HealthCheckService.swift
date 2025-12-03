//
//  HealthCheckService.swift
//  catnip
//
//  Health check service to detect codespace shutdowns
//

import Foundation
import UIKit
import Combine

/// Service that periodically checks codespace health and detects shutdowns
@MainActor
class HealthCheckService: ObservableObject {
    // MARK: - Published Properties
    @Published private(set) var isMonitoring = false
    @Published private(set) var isCodespaceAvailable = true
    @Published private(set) var lastCheckDate: Date?
    @Published private(set) var shutdownDetected = false
    @Published private(set) var shutdownMessage: String?

    // MARK: - Private Properties
    private var pollingTask: Task<Void, Never>?
    private var appStateObserver: NSObjectProtocol?
    private let pollingInterval: TimeInterval = 60.0 // 60 seconds

    // MARK: - Singleton
    static let shared = HealthCheckService()

    private init() {
        setupAppStateObservers()
    }

    deinit {
        // Cancel polling task directly without calling stop() to avoid actor isolation issues
        pollingTask?.cancel()

        if let observer = appStateObserver {
            NotificationCenter.default.removeObserver(observer)
        }
    }

    // MARK: - Public API

    /// Start monitoring codespace health
    func startMonitoring() {
        guard !isMonitoring else { return }
        isMonitoring = true

        NSLog("🏥 Starting health check monitoring (60s interval)")

        // Immediate check on start
        Task {
            await checkHealth()
            scheduleNextCheck()
        }
    }

    /// Stop monitoring
    func stop() {
        pollingTask?.cancel()
        pollingTask = nil
        isMonitoring = false
        NSLog("🏥 Stopped health check monitoring")
    }

    /// Force immediate health check
    @discardableResult
    func checkHealth() async -> Bool {
        NSLog("🏥 Checking codespace health...")

        do {
            // Call the /v1/info endpoint
            _ = try await CatnipAPI.shared.getServerInfo()

            // If we get a successful response, codespace is available
            await MainActor.run {
                if !isCodespaceAvailable {
                    NSLog("🏥 ✅ Codespace is back online")
                }
                isCodespaceAvailable = true
                shutdownDetected = false
                shutdownMessage = nil
                lastCheckDate = Date()
            }

            return true

        } catch let error as APIError {
            await MainActor.run {
                lastCheckDate = Date()

                // Check for CODESPACE_SHUTDOWN error
                if case .httpError(let statusCode, let responseData) = error,
                   statusCode == 502 {
                    // Try to parse the error response
                    if let data = responseData,
                       let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                       let code = json["code"] as? String,
                       code == "CODESPACE_SHUTDOWN" {

                        let message = json["message"] as? String ?? "Your codespace has shut down. Reconnect to restart it."

                        NSLog("🏥 ⚠️ Codespace shutdown detected: \(message)")

                        isCodespaceAvailable = false
                        shutdownDetected = true
                        shutdownMessage = message

                        // Post notification for UI to handle
                        NotificationCenter.default.post(
                            name: .codespaceShutdownDetected,
                            object: nil,
                            userInfo: ["message": message]
                        )

                        return
                    }
                }

                // Other errors - log but don't mark as shutdown
                NSLog("🏥 ⚠️ Health check failed (not a shutdown): \(error.localizedDescription)")
            }

            return false

        } catch {
            await MainActor.run {
                lastCheckDate = Date()
                NSLog("🏥 ⚠️ Health check failed: \(error.localizedDescription)")
            }

            return false
        }
    }

    /// Reset shutdown state (call when user reconnects)
    func resetShutdownState() {
        shutdownDetected = false
        shutdownMessage = nil
        isCodespaceAvailable = true
        NSLog("🏥 Reset shutdown state")
    }

    // MARK: - Private Methods

    private func scheduleNextCheck() {
        pollingTask?.cancel()

        guard isMonitoring else { return }

        pollingTask = Task { [weak self] in
            // Wait for interval
            try? await Task.sleep(nanoseconds: UInt64((self?.pollingInterval ?? 60.0) * 1_000_000_000))

            guard !Task.isCancelled else { return }

            await self?.checkHealth()
            self?.scheduleNextCheck()
        }
    }

    // MARK: - App Lifecycle Observers

    private func setupAppStateObservers() {
        // Observe app entering foreground - immediate health check
        NotificationCenter.default.addObserver(
            forName: UIApplication.willEnterForegroundNotification,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            Task { @MainActor in
                NSLog("🏥 App entered foreground - checking health immediately")
                await self?.checkHealth()
            }
        }

        // Observe app entering background - stop polling to save battery
        appStateObserver = NotificationCenter.default.addObserver(
            forName: UIApplication.didEnterBackgroundNotification,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            Task { @MainActor in
                NSLog("🏥 App entered background - pausing health checks")
                self?.pollingTask?.cancel()
            }
        }
    }
}

// MARK: - Notification Names

extension Notification.Name {
    static let codespaceShutdownDetected = Notification.Name("codespaceShutdownDetected")
    static let shouldReconnectToCodespace = Notification.Name("shouldReconnectToCodespace")
}
