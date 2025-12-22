//
//  BackgroundProgressManager.swift
//  catnip
//
//  Manages background URLSession polling to keep Live Activity updated
//  when the app is backgrounded or suspended by iOS
//

import Foundation

/// Manages background URLSession polling for codespace creation progress
class BackgroundProgressManager: NSObject {

    // MARK: - Properties

    private var session: URLSession!
    private var currentTask: URLSessionDownloadTask?
    private var isPolling = false
    private let pollingInterval: TimeInterval = 60.0 // 60 seconds between polls (fallback, push is primary)

    // Callback to update progress when response is received
    var onProgressUpdate: (() -> Void)?

    // MARK: - Initialization

    override init() {
        super.init()

        // Skip URLSession creation in UI testing mode to prevent RunLoop blocking in CI
        if UITestingHelper.isUITesting {
            NSLog("🔄 BackgroundProgressManager initialized (UI testing mode - URLSession skipped)")
            return
        }

        // Create background URLSession configuration
        let config = URLSessionConfiguration.background(
            withIdentifier: "com.wandb.catnip.codespace-progress"
        )

        // Allow expensive network access (cellular)
        config.allowsExpensiveNetworkAccess = true
        config.allowsConstrainedNetworkAccess = true

        // Discretionary = false means iOS should run this even if conditions aren't ideal
        config.isDiscretionary = false

        // Session continues even if app is suspended
        config.sessionSendsLaunchEvents = true

        session = URLSession(configuration: config, delegate: self, delegateQueue: nil)

        NSLog("🔄 BackgroundProgressManager initialized")
    }

    // MARK: - Public API

    /// Start background polling for a codespace
    /// - Parameter codespaceName: The codespace to poll status for
    func startPolling(codespaceName: String) {
        // Skip in UI testing mode
        guard session != nil else {
            NSLog("🔄 Skipping polling in UI testing mode")
            return
        }

        guard !isPolling else {
            NSLog("🔄 Already polling, ignoring duplicate start request")
            return
        }

        isPolling = true
        NSLog("🔄 Starting background polling for codespace: \(codespaceName)")
        scheduleNextPoll(codespaceName: codespaceName)
    }

    /// Stop background polling
    func stopPolling() {
        guard isPolling else { return }

        NSLog("🔄 Stopping background polling")
        isPolling = false
        currentTask?.cancel()
        currentTask = nil
    }

    // MARK: - Private Methods

    private func scheduleNextPoll(codespaceName: String) {
        guard isPolling else {
            NSLog("🔄 Not polling, skipping schedule")
            return
        }

        guard let session = session else {
            NSLog("🔄 No session available (UI testing mode)")
            return
        }

        // Cancel any existing task
        currentTask?.cancel()

        // Use the existing status endpoint
        // This will trigger URLSession delegate callbacks even when app is suspended
        let urlString = "https://catnip.run/v1/codespace/status/\(codespaceName)"

        guard let url = URL(string: urlString) else {
            NSLog("🔄 ❌ Invalid URL for codespace: \(codespaceName)")
            return
        }

        NSLog("🔄 Scheduling background download task for: \(urlString)")

        // Use download task instead of data task - iOS handles these better in background
        currentTask = session.downloadTask(with: url)
        currentTask?.earliestBeginDate = Date(timeIntervalSinceNow: pollingInterval)
        currentTask?.resume()

        NSLog("🔄 Task scheduled with earliestBeginDate in \(pollingInterval)s")
    }
}

// MARK: - URLSessionDownloadDelegate

extension BackgroundProgressManager: URLSessionDownloadDelegate {

    func urlSession(_ session: URLSession, downloadTask: URLSessionDownloadTask, didFinishDownloadingTo location: URL) {
        NSLog("🔄 ✅ Background download completed")

        // Read the response (even though we don't necessarily need the data)
        // This ensures the task completes properly
        do {
            let data = try Data(contentsOf: location)
            NSLog("🔄 Downloaded \(data.count) bytes")
        } catch {
            NSLog("🔄 ⚠️ Failed to read download: \(error)")
        }

        // Trigger progress update callback on main thread
        DispatchQueue.main.async { [weak self] in
            self?.onProgressUpdate?()
        }
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
        if let error = error {
            NSLog("🔄 ❌ Background task failed: \(error.localizedDescription)")
        } else {
            NSLog("🔄 ✅ Background task completed successfully")
        }

        // Schedule next poll if still polling
        if isPolling, let codespaceName = extractCodespaceName(from: task.originalRequest?.url) {
            // Add a small delay before scheduling next task
            DispatchQueue.main.asyncAfter(deadline: .now() + 1.0) { [weak self] in
                self?.scheduleNextPoll(codespaceName: codespaceName)
            }
        }
    }

    // MARK: - Helper Methods

    private func extractCodespaceName(from url: URL?) -> String? {
        guard let url = url else { return nil }

        // URL format: https://catnip.run/v1/codespace/status/{codespaceName}
        let components = url.pathComponents
        if components.count >= 4 && components[1] == "v1" && components[2] == "codespace" && components[3] == "status" {
            return components.last
        }

        return nil
    }
}

// MARK: - URLSessionDelegate

extension BackgroundProgressManager: URLSessionDelegate {

    func urlSessionDidFinishEvents(forBackgroundURLSession session: URLSession) {
        NSLog("🔄 URLSession finished background events")

        // This is called when all background tasks complete
        // We'll schedule the next poll cycle here if still polling
    }
}
