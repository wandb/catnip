//
//  SSEService.swift
//  catnip
//
//  Server-Sent Events service for codespace connections
//

import Foundation

enum SSEEvent {
    case status(String)
    case success(String, String?) // message, codespaceUrl
    case error(String)
    case setup(String, nextAction: String)
    case multiple([CodespaceInfo])
}

class SSEService {
    private var task: URLSessionDataTask?
    private var eventCallback: ((SSEEvent) -> Void)?

    func connect(codespaceName: String? = nil, org: String? = nil, headers: [String: String], onEvent: @escaping (SSEEvent) -> Void) {
        self.eventCallback = onEvent

        // Skip real connection in UI testing mode - simulate successful connection
        if UITestingHelper.shouldUseMockData {
            print("🐱 Mock SSE connection (no real network call)")
            // Simulate a quick successful connection
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) {
                onEvent(.status("Connecting to codespace..."))
            }
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.2) {
                onEvent(.success("Connected successfully", nil))
            }
            return
        }

        let baseURL = org != nil ? "https://\(org!).catnip.run" : "https://catnip.run"
        var urlString = "\(baseURL)/v1/codespace"
        if let codespace = codespaceName, let encoded = codespace.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) {
            urlString += "?codespace=\(encoded)"
        }

        guard let url = URL(string: urlString) else {
            onEvent(.error("Invalid URL"))
            return
        }

        var request = URLRequest(url: url)
        request.allHTTPHeaderFields = headers
        request.timeoutInterval = 120 // 2 minute timeout

        print("🐱 Creating SSE connection to: \(urlString)")

        let session = URLSession.shared
        task = session.dataTask(with: request) { [weak self] data, response, error in
            guard let self = self else { return }

            if let error = error {
                print("🐱 SSE connection error: \(error)")
                self.eventCallback?(.error(error.localizedDescription))
                return
            }

            guard let httpResponse = response as? HTTPURLResponse else {
                self.eventCallback?(.error("Invalid response"))
                return
            }

            if httpResponse.statusCode != 200 {
                let errorMessage = data.flatMap { String(data: $0, encoding: .utf8) } ?? "Unknown error"
                print("🐱 SSE error response: \(httpResponse.statusCode) - \(errorMessage)")
                self.eventCallback?(.error(errorMessage))
                return
            }

            guard let data = data else {
                self.eventCallback?(.error("No data received"))
                return
            }

            self.parseSSEData(data)
        }

        // Use a delegate-based approach for streaming
        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = 120
        config.timeoutIntervalForResource = 120

        let delegateSession = URLSession(configuration: config, delegate: SSEDelegate(onEvent: onEvent), delegateQueue: nil)
        task = delegateSession.dataTask(with: request)
        task?.resume()

        print("🐱 SSE connection started")
    }

    func disconnect() {
        task?.cancel()
        task = nil
        eventCallback = nil
    }

    private func parseSSEData(_ data: Data) {
        guard let text = String(data: data, encoding: .utf8) else { return }
        let lines = text.components(separatedBy: "\n")

        var eventType: String?
        var eventData: String?

        for line in lines {
            if line.hasPrefix("event:") {
                eventType = String(line.dropFirst(6).trimmingCharacters(in: .whitespaces))
            } else if line.hasPrefix("data:") {
                eventData = String(line.dropFirst(5).trimmingCharacters(in: .whitespaces))
            } else if line.isEmpty, let type = eventType, let data = eventData {
                handleEvent(type: type, data: data)
                eventType = nil
                eventData = nil
            }
        }
    }

    private func handleEvent(type: String, data: String) {
        print("🐱 SSE event: \(type)")

        guard let jsonData = data.data(using: .utf8),
              let json = try? JSONSerialization.jsonObject(with: jsonData) as? [String: Any] else {
            print("🐱 Failed to parse SSE data")
            return
        }

        switch type {
        case "status":
            if let message = json["message"] as? String {
                eventCallback?(.status(message))
            }
        case "success":
            if let message = json["message"] as? String {
                let codespaceUrl = json["codespaceUrl"] as? String
                eventCallback?(.success(message, codespaceUrl))
            }
        case "error":
            if let message = json["message"] as? String {
                eventCallback?(.error(message))
            }
        case "setup":
            if let message = json["message"] as? String {
                let nextAction = json["next_action"] as? String ?? "install"
                eventCallback?(.setup(message, nextAction: nextAction))
            }
        case "multiple":
            if let codespacesData = json["codespaces"] as? [[String: Any]] {
                let codespaces: [CodespaceInfo] = codespacesData.compactMap { dict in
                    guard let name = dict["name"] as? String,
                          let lastUsed = dict["lastUsed"] as? TimeInterval else {
                        return nil
                    }
                    let repository = dict["repository"] as? String
                    return CodespaceInfo(name: name, lastUsed: lastUsed, repository: repository)
                }
                eventCallback?(.multiple(codespaces))
            }
        default:
            print("🐱 Unknown SSE event type: \(type)")
        }
    }
}

// MARK: - SSE Delegate

class SSEDelegate: NSObject, URLSessionDataDelegate {
    private let onEvent: (SSEEvent) -> Void
    private var buffer = Data()

    init(onEvent: @escaping (SSEEvent) -> Void) {
        self.onEvent = onEvent
    }

    func urlSession(_ session: URLSession, dataTask: URLSessionDataTask, didReceive data: Data) {
        buffer.append(data)

        // Try to parse complete events from buffer
        guard let text = String(data: buffer, encoding: .utf8) else { return }
        let lines = text.components(separatedBy: "\n\n")

        // Process all complete events (all but the last, which might be incomplete)
        for i in 0..<(lines.count - 1) {
            parseEvent(lines[i])
        }

        // Keep the last (possibly incomplete) event in the buffer
        if let lastLine = lines.last {
            buffer = lastLine.data(using: .utf8) ?? Data()
        }
    }

    private func parseEvent(_ eventText: String) {
        let lines = eventText.components(separatedBy: "\n")
        var eventType: String?
        var eventData: String?

        for line in lines {
            if line.hasPrefix("event:") {
                eventType = String(line.dropFirst(6).trimmingCharacters(in: .whitespaces))
            } else if line.hasPrefix("data:") {
                eventData = String(line.dropFirst(5).trimmingCharacters(in: .whitespaces))
            }
        }

        guard let type = eventType, let data = eventData else { return }
        handleEvent(type: type, data: data)
    }

    private func handleEvent(type: String, data: String) {
        print("🐱 SSE event: \(type)")

        guard let jsonData = data.data(using: .utf8),
              let json = try? JSONSerialization.jsonObject(with: jsonData) as? [String: Any] else {
            print("🐱 Failed to parse SSE data")
            return
        }

        switch type {
        case "status":
            if let message = json["message"] as? String {
                onEvent(.status(message))
            }
        case "success":
            if let message = json["message"] as? String {
                let codespaceUrl = json["codespaceUrl"] as? String
                onEvent(.success(message, codespaceUrl))
            }
        case "error":
            if let message = json["message"] as? String {
                onEvent(.error(message))
            }
        case "setup":
            if let message = json["message"] as? String {
                let nextAction = json["next_action"] as? String ?? "install"
                onEvent(.setup(message, nextAction: nextAction))
            }
        case "multiple":
            if let codespacesData = json["codespaces"] as? [[String: Any]] {
                let codespaces: [CodespaceInfo] = codespacesData.compactMap { dict in
                    guard let name = dict["name"] as? String,
                          let lastUsed = dict["lastUsed"] as? TimeInterval else {
                        return nil
                    }
                    let repository = dict["repository"] as? String
                    return CodespaceInfo(name: name, lastUsed: lastUsed, repository: repository)
                }
                onEvent(.multiple(codespaces))
            }
        default:
            print("🐱 Unknown SSE event type: \(type)")
        }
    }
}
