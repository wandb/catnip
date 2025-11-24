//
//  PTYWebSocketManager.swift
//  catnip
//
//  WebSocket manager for PTY connections matching backend protocol
//

import Foundation
import Combine

// Control message types matching backend protocol
struct PTYControlMessage: Sendable {
    let type: String
    var data: String?
    var submit: Bool?
    var cols: UInt16?
    var rows: UInt16?
    var focused: Bool?
}

// Separate Codable conformance to avoid MainActor isolation warnings
extension PTYControlMessage: Codable {}

// Manager for WebSocket PTY connections
class PTYWebSocketManager: NSObject, ObservableObject {
    @Published var isConnected = false
    @Published var error: String?

    private var webSocketTask: URLSessionWebSocketTask?
    private var session: URLSession?
    private var reconnectAttempts = 0
    private let maxReconnectAttempts = 5
    private var isManuallyDisconnected = false  // Prevents auto-reconnect after manual disconnect

    // Callbacks for handling data
    var onData: ((Data) -> Void)?
    var onJSONMessage: ((PTYControlMessage) -> Void)?

    private let workspaceId: String
    private let agent: String
    private let baseURL: String
    private let codespaceName: String?
    private let authToken: String?

    init(workspaceId: String, agent: String = "claude", baseURL: String, codespaceName: String? = nil, authToken: String? = nil) {
        self.workspaceId = workspaceId
        self.agent = agent
        self.baseURL = baseURL
        self.codespaceName = codespaceName
        self.authToken = authToken
        super.init()

        // Create URLSession with delegate
        let configuration = URLSessionConfiguration.default
        configuration.timeoutIntervalForRequest = 30
        configuration.timeoutIntervalForResource = 300
        self.session = URLSession(configuration: configuration, delegate: self, delegateQueue: nil)
    }

    func connect() {
        guard webSocketTask == nil else {
            NSLog("🔌 PTY WebSocket already connected")
            return
        }

        // Clear previous error and reset manual disconnect flag
        DispatchQueue.main.async {
            self.error = nil
        }
        isManuallyDisconnected = false

        // Skip real connection in UI testing mode - just mark as connected
        if UITestingHelper.shouldUseMockData {
            NSLog("🔌 Mock PTY WebSocket connection (no real network call)")
            DispatchQueue.main.async {
                self.isConnected = true
                self.reconnectAttempts = 0
            }
            return
        }

        // Build WebSocket URL with query parameters
        var components = URLComponents(string: baseURL)
        components?.path = "/v1/pty"
        components?.queryItems = [
            URLQueryItem(name: "session", value: workspaceId),
            URLQueryItem(name: "agent", value: agent)
        ]

        guard let url = components?.url else {
            error = "Invalid WebSocket URL"
            return
        }

        NSLog("🔌 Connecting PTY WebSocket to: %@", url.absoluteString)

        // Create request with custom headers for mobile authentication
        var request = URLRequest(url: url)

        // Add authentication header if available
        if let authToken = authToken {
            request.setValue("Bearer \(authToken)", forHTTPHeaderField: "Authorization")
            NSLog("🔐 Added Authorization header")
        }

        // Add codespace name header for mobile routing
        if let codespaceName = codespaceName {
            request.setValue(codespaceName, forHTTPHeaderField: "X-Codespace-Name")
            NSLog("📦 Added X-Codespace-Name: %@", codespaceName)
        }

        // Create WebSocket task with custom headers
        webSocketTask = session?.webSocketTask(with: request)
        webSocketTask?.resume()

        // Start receiving messages
        // Note: isConnected will be set to true by the delegate when connection opens
        receiveMessage()
    }

    func disconnect() {
        NSLog("🔌 Disconnecting PTY WebSocket")
        isManuallyDisconnected = true  // Prevent auto-reconnect
        webSocketTask?.cancel(with: .goingAway, reason: nil)
        webSocketTask = nil

        DispatchQueue.main.async {
            self.isConnected = false
        }
    }

    // Send PTY input
    func sendInput(_ data: String) {
        let message = PTYControlMessage(type: "input", data: data)
        sendControlMessage(message)
    }

    // Send terminal resize
    func sendResize(cols: UInt16, rows: UInt16) {
        let message = PTYControlMessage(type: "resize", cols: cols, rows: rows)
        sendControlMessage(message)
        NSLog("📐 Sent resize: %dx%d", cols, rows)
    }

    // Send ready signal (triggers buffer replay)
    func sendReady() {
        let message = PTYControlMessage(type: "ready")
        sendControlMessage(message)
        NSLog("🔧 Sent ready signal")
    }

    // Send focus state
    func sendFocus(focused: Bool) {
        let message = PTYControlMessage(type: "focus", focused: focused)
        sendControlMessage(message)
    }

    // Send prompt injection
    func sendPrompt(_ prompt: String, submit: Bool = true) {
        let message = PTYControlMessage(type: "prompt", data: prompt, submit: submit)
        sendControlMessage(message)
        NSLog("📝 Sent prompt: %@ (submit: %d)", prompt, submit)
    }

    // Private: Send control message as JSON
    private func sendControlMessage(_ message: PTYControlMessage) {
        // Skip in UI testing mode
        if UITestingHelper.shouldUseMockData {
            NSLog("📝 Mock control message send (no-op): \(message.type)")
            return
        }

        guard let webSocketTask = webSocketTask else {
            NSLog("❌ Cannot send message - WebSocket not connected")
            return
        }

        do {
            let jsonData = try JSONEncoder().encode(message)
            let jsonString = String(data: jsonData, encoding: .utf8) ?? ""
            let message = URLSessionWebSocketTask.Message.string(jsonString)

            webSocketTask.send(message) { error in
                if let error = error {
                    NSLog("❌ WebSocket send error: %@", error.localizedDescription)
                }
            }
        } catch {
            NSLog("❌ Failed to encode control message: %@", error.localizedDescription)
        }
    }

    // Private: Receive messages continuously
    private func receiveMessage() {
        webSocketTask?.receive { [weak self] result in
            guard let self = self else { return }

            switch result {
            case .success(let message):
                switch message {
                case .data(let data):
                    // Binary data from PTY output
                    DispatchQueue.main.async {
                        self.onData?(data)
                    }

                case .string(let text):
                    // JSON control messages
                    if let data = text.data(using: .utf8) {
                        let decoder = JSONDecoder()
                        if let controlMsg = try? decoder.decode(PTYControlMessage.self, from: data) {
                            DispatchQueue.main.async {
                                self.onJSONMessage?(controlMsg)
                            }
                        }
                    }

                @unknown default:
                    NSLog("⚠️ Unknown WebSocket message type")
                }

                // Continue receiving
                self.receiveMessage()

            case .failure(let error):
                NSLog("❌ WebSocket receive error: %@", error.localizedDescription)
                DispatchQueue.main.async {
                    self.isConnected = false
                    self.error = "Reconnecting..."
                }

                // Attempt reconnection
                self.attemptReconnect()
            }
        }
    }

    // Private: Reconnection logic
    private func attemptReconnect() {
        // Don't auto-reconnect if manually disconnected
        guard !isManuallyDisconnected else {
            NSLog("🔌 Skipping auto-reconnect (manually disconnected)")
            return
        }

        guard reconnectAttempts < maxReconnectAttempts else {
            NSLog("❌ Max reconnection attempts reached")
            return
        }

        reconnectAttempts += 1
        let delay = min(pow(2.0, Double(reconnectAttempts)), 30.0) // Exponential backoff, max 30s

        NSLog("🔄 Reconnecting in %.0f seconds (attempt %d/%d)", delay, reconnectAttempts, maxReconnectAttempts)

        DispatchQueue.main.asyncAfter(deadline: .now() + delay) { [weak self] in
            guard let self = self, !self.isManuallyDisconnected else { return }

            // Check health before reconnecting to detect codespace shutdown
            Task {
                let isHealthy = await HealthCheckService.shared.checkHealth()

                await MainActor.run {
                    if isHealthy {
                        // Codespace is available, proceed with reconnection
                        self.webSocketTask = nil
                        self.connect()
                    } else if HealthCheckService.shared.shutdownDetected {
                        // Shutdown was detected - notification already posted by HealthCheckService
                        NSLog("🔌 Skipping reconnect - codespace shutdown detected")
                        self.error = "Codespace unavailable"
                    } else {
                        // Other health check failure - try reconnecting anyway
                        self.webSocketTask = nil
                        self.connect()
                    }
                }
            }
        }
    }

    deinit {
        disconnect()
    }
}

// MARK: - URLSessionWebSocketDelegate

extension PTYWebSocketManager: URLSessionWebSocketDelegate {
    nonisolated func urlSession(_ session: URLSession, webSocketTask: URLSessionWebSocketTask, didOpenWithProtocol protocol: String?) {
        NSLog("✅ PTY WebSocket connected")
        Task { @MainActor in
            self.isConnected = true
            self.reconnectAttempts = 0
        }
    }

    nonisolated func urlSession(_ session: URLSession, webSocketTask: URLSessionWebSocketTask, didCloseWith closeCode: URLSessionWebSocketTask.CloseCode, reason: Data?) {
        NSLog("🔌 PTY WebSocket closed with code: %d", closeCode.rawValue)
        Task { @MainActor in
            self.isConnected = false
        }
    }
}
