//
//  CatnipAPI.swift
//  catnip
//
//  Network service for Catnip API
//

import Foundation
import Combine
import Security

enum APIError: LocalizedError {
    case invalidURL
    case noSessionToken
    case networkError(Error)
    case decodingError(Error)
    case serverError(Int, String)
    case sseConnectionFailed(String)

    var errorDescription: String? {
        switch self {
        case .invalidURL:
            return "Invalid URL"
        case .noSessionToken:
            return "No session token available"
        case .networkError(let error):
            return "Network error: \(error.localizedDescription)"
        case .decodingError(let error):
            return "Decoding error: \(error.localizedDescription)"
        case .serverError(let code, let message):
            return "Server error \(code): \(message)"
        case .sseConnectionFailed(let message):
            return "SSE connection failed: \(message)"
        }
    }
}

class CatnipAPI: ObservableObject {
    static let shared = CatnipAPI()

    private let baseURL = "https://catnip.run"
    private let decoder = JSONDecoder()

    private init() {
        // Don't use automatic snake_case conversion - we have manual CodingKeys
    }

    // MARK: - Authentication Helpers

    private func getSessionToken() async throws -> String {
        guard let token = try? await KeychainHelper.load(key: "session_token") else {
            throw APIError.noSessionToken
        }
        return token
    }

    private func getCodespaceName() -> String? {
        UserDefaults.standard.string(forKey: "codespace_name")
    }

    private func getHeaders(includeCodespace: Bool = false) async throws -> [String: String] {
        let token = try await getSessionToken()

        var headers = [
            "Content-Type": "application/json",
            "Authorization": "Bearer \(token)"
        ]

        if includeCodespace {
            if let codespaceName = getCodespaceName(), !codespaceName.isEmpty {
                headers["X-Codespace-Name"] = codespaceName
            }
        }

        return headers
    }

    // MARK: - Workspace API

    /// Fetch workspaces with optional ETag support for efficient polling
    /// Returns nil if server returns 304 Not Modified (content unchanged)
    func getWorkspaces(ifNoneMatch: String? = nil) async throws -> (workspaces: [WorkspaceInfo], etag: String?)? {
        NSLog("🐱 [getWorkspaces] Fetching workspaces...")
        var headers = try await getHeaders(includeCodespace: true)

        // Add If-None-Match header for conditional request
        if let etag = ifNoneMatch {
            headers["If-None-Match"] = etag
        }

        // Log the codespace name being used
        if let codespaceName = getCodespaceName() {
            NSLog("🐱 [getWorkspaces] Using codespace: \(codespaceName)")
        } else {
            NSLog("🐱 [getWorkspaces] No codespace name set")
        }

        guard let url = URL(string: "\(baseURL)/v1/git/worktrees") else {
            NSLog("🐱 [getWorkspaces] ❌ Invalid URL")
            throw APIError.invalidURL
        }

        var request = URLRequest(url: url)
        request.allHTTPHeaderFields = headers

        do {
            NSLog("🐱 [getWorkspaces] Making request to \(url)")
            let (data, response) = try await URLSession.shared.data(for: request)

            guard let httpResponse = response as? HTTPURLResponse else {
                NSLog("🐱 [getWorkspaces] ❌ Invalid response type")
                throw APIError.networkError(NSError(domain: "Invalid response", code: -1))
            }

            NSLog("🐱 [getWorkspaces] Got response with status: \(httpResponse.statusCode)")

            // Handle 304 Not Modified - content unchanged
            if httpResponse.statusCode == 304 {
                NSLog("🐱 [getWorkspaces] Workspaces not modified (304)")
                return nil
            }

            if httpResponse.statusCode != 200 {
                let errorMessage = String(data: data, encoding: .utf8) ?? "Unknown error"
                NSLog("🐱 [getWorkspaces] ❌ Server error \(httpResponse.statusCode): \(errorMessage)")
                throw APIError.serverError(httpResponse.statusCode, errorMessage)
            }

            if data.isEmpty {
                NSLog("🐱 [getWorkspaces] Empty response data")
                let etag = httpResponse.value(forHTTPHeaderField: "ETag")
                return (workspaces: [], etag: etag)
            }

            NSLog("🐱 [getWorkspaces] Received \(data.count) bytes of data")

            // Log first 200 bytes of response for debugging
            if let preview = String(data: data.prefix(200), encoding: .utf8) {
                NSLog("🐱 [getWorkspaces] Response preview: \(preview)")
            }

            let workspaces = try decoder.decode([WorkspaceInfo].self, from: data)
            NSLog("🐱 [getWorkspaces] ✅ Successfully decoded \(workspaces.count) workspaces")

            // Extract ETag from response headers
            let etag = httpResponse.value(forHTTPHeaderField: "ETag")

            return (workspaces: workspaces, etag: etag)
        } catch let error as DecodingError {
            NSLog("🐱 [getWorkspaces] ❌ Decoding error: \(error)")
            throw APIError.decodingError(error)
        } catch let error as APIError {
            NSLog("🐱 [getWorkspaces] ❌ API error: \(error)")
            throw error
        } catch {
            NSLog("🐱 [getWorkspaces] ❌ Network error: \(error)")
            throw APIError.networkError(error)
        }
    }

    /// Fetch a specific workspace with optional ETag support for efficient polling
    /// Returns nil if server returns 304 Not Modified (content unchanged)
    func getWorkspace(id: String, ifNoneMatch: String? = nil) async throws -> (workspace: WorkspaceInfo, etag: String?)? {
        // Get all workspaces with ETag support
        guard let result = try await getWorkspaces(ifNoneMatch: ifNoneMatch) else {
            // 304 Not Modified - content unchanged
            return nil
        }

        guard let workspace = result.workspaces.first(where: { $0.id == id }) else {
            throw APIError.serverError(404, "Workspace with ID \(id) not found")
        }

        return (workspace: workspace, etag: result.etag)
    }

    func getClaudeSessions() async throws -> [String: ClaudeSessionData] {
        let headers = try await getHeaders(includeCodespace: true)
        guard let url = URL(string: "\(baseURL)/v1/claude/sessions") else {
            throw APIError.invalidURL
        }

        var request = URLRequest(url: url)
        request.allHTTPHeaderFields = headers

        do {
            let (data, response) = try await URLSession.shared.data(for: request)

            guard let httpResponse = response as? HTTPURLResponse else {
                return [:]
            }

            if httpResponse.statusCode != 200 {
                print("🐱 Failed to fetch Claude sessions: \(httpResponse.statusCode)")
                return [:]
            }

            let sessions = try decoder.decode([String: ClaudeSessionData].self, from: data)
            return sessions
        } catch {
            print("🐱 Error fetching Claude sessions: \(error)")
            return [:]
        }
    }

    func getLatestMessage(worktreePath: String) async throws -> LatestMessageResponse {
        let headers = try await getHeaders(includeCodespace: true)
        guard let encodedPath = worktreePath.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed),
              let url = URL(string: "\(baseURL)/v1/claude/latest-message?worktree_path=\(encodedPath)") else {
            throw APIError.invalidURL
        }

        var request = URLRequest(url: url)
        request.allHTTPHeaderFields = headers

        do {
            let (data, response) = try await URLSession.shared.data(for: request)

            guard let httpResponse = response as? HTTPURLResponse else {
                throw APIError.networkError(NSError(domain: "Invalid response", code: -1))
            }

            if httpResponse.statusCode != 200 {
                return LatestMessageResponse(content: "Failed to fetch message", isError: true)
            }

            let result = try decoder.decode(LatestMessageResponse.self, from: data)
            return result
        } catch {
            return LatestMessageResponse(content: "Error fetching message", isError: true)
        }
    }

    func sendPrompt(workspacePath: String, prompt: String) async throws {
        NSLog("🐱 [CatnipAPI] sendPrompt called with workspacePath: \(workspacePath)")
        NSLog("🐱 [CatnipAPI] Prompt length: \(prompt.count) chars")
        NSLog("🐱 [CatnipAPI] Prompt text: '\(prompt)'")
        NSLog("🐱 [CatnipAPI] Prompt UTF-8 bytes: \(prompt.utf8.map { String(format: "%02X", $0) }.joined(separator: " "))")

        let headers = try await getHeaders(includeCodespace: true)
        NSLog("🐱 [CatnipAPI] Got headers, codespace: \(getCodespaceName() ?? "none")")

        // IMPORTANT: Use workspacePath as session ID directly
        // The workspacePath should be the workspace name (e.g., "cyrillic/peanut"), not the full path
        // Use SSE endpoint for PTY session (gives us auto-commits and session tracking)

        // Build URL with proper encoding
        guard var components = URLComponents(string: "\(baseURL)/v1/pty/sse") else {
            NSLog("🐱 [CatnipAPI] ❌ Failed to create URLComponents")
            throw APIError.invalidURL
        }

        // URLQueryItem handles encoding automatically, but let's ensure clean strings
        components.queryItems = [
            URLQueryItem(name: "session", value: workspacePath),
            URLQueryItem(name: "agent", value: "claude"),
            URLQueryItem(name: "prompt", value: prompt)
        ]

        // Verify the query items were set correctly
        if let items = components.queryItems {
            NSLog("🐱 [CatnipAPI] Query items: session=\(items[0].value ?? "nil"), agent=\(items[1].value ?? "nil"), prompt_length=\(items[2].value?.count ?? 0)")
        }

        guard let url = components.url else {
            NSLog("🐱 [CatnipAPI] ❌ Failed to build URL from components")
            throw APIError.invalidURL
        }

        NSLog("🐱 [CatnipAPI] Built SSE URL: \(url.absoluteString)")

        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        request.allHTTPHeaderFields = headers

        // Fire off the SSE request asynchronously - we don't need to process the stream
        // The backend will handle prompt injection into the PTY session
        NSLog("🐱 [CatnipAPI] Launching detached task to send SSE request...")
        Task.detached {
            do {
                NSLog("🐱 [CatnipAPI] Making SSE request...")
                let (_, response) = try await URLSession.shared.data(for: request)

                if let httpResponse = response as? HTTPURLResponse {
                    NSLog("🐱 [CatnipAPI] SSE response status: \(httpResponse.statusCode)")
                    if httpResponse.statusCode == 200 {
                        NSLog("🐱 [CatnipAPI] ✅ Prompt sent successfully via SSE")
                    } else {
                        NSLog("🐱 [CatnipAPI] ❌ Failed to send prompt via SSE: \(httpResponse.statusCode)")
                    }
                }
            } catch {
                NSLog("🐱 [CatnipAPI] ❌ Error sending prompt via SSE: \(error.localizedDescription)")
            }
        }

        NSLog("🐱 [CatnipAPI] sendPrompt returning (detached task launched)")
        // Return immediately - prompt injection happens asynchronously in the PTY session
    }

    func getWorkspaceDiff(id: String) async throws -> WorktreeDiffResponse {
        let headers = try await getHeaders(includeCodespace: true)
        guard let url = URL(string: "\(baseURL)/v1/git/worktrees/\(id)/diff") else {
            throw APIError.invalidURL
        }

        var request = URLRequest(url: url)
        request.allHTTPHeaderFields = headers

        do {
            let (data, response) = try await URLSession.shared.data(for: request)

            guard let httpResponse = response as? HTTPURLResponse else {
                throw APIError.networkError(NSError(domain: "Invalid response", code: -1))
            }

            if httpResponse.statusCode != 200 {
                let errorMessage = String(data: data, encoding: .utf8) ?? "Unknown error"
                throw APIError.serverError(httpResponse.statusCode, errorMessage)
            }

            let diffResponse = try decoder.decode(WorktreeDiffResponse.self, from: data)
            print("🐱 Loaded diff for workspace \(id): \(diffResponse.totalFiles) files")
            return diffResponse
        } catch let error as DecodingError {
            throw APIError.decodingError(error)
        } catch let error as APIError {
            throw error
        } catch {
            throw APIError.networkError(error)
        }
    }

    func fetchBranches(repoId: String) async throws -> [String] {
        // Need to include auth headers and codespace name (web app has these in cookies automatically)
        let headers = try await getHeaders(includeCodespace: true)

        // IMPORTANT: Create custom character set that excludes forward slash
        // Web app uses encodeURIComponent which encodes "/" as %2F
        // Both urlQueryAllowed and urlPathAllowed allow "/" which breaks route matching
        var allowedCharacters = CharacterSet.urlQueryAllowed
        allowedCharacters.remove(charactersIn: "/") // Force encoding of forward slash

        guard let encodedRepoId = repoId.addingPercentEncoding(withAllowedCharacters: allowedCharacters),
              let url = URL(string: "\(baseURL)/v1/git/branches/\(encodedRepoId)") else {
            NSLog("🐱 Invalid URL for branches: \(repoId)")
            throw APIError.invalidURL
        }

        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        request.allHTTPHeaderFields = headers

        do {
            let (data, response) = try await URLSession.shared.data(for: request)

            guard let httpResponse = response as? HTTPURLResponse else {
                throw APIError.networkError(NSError(domain: "Invalid response", code: -1))
            }

            if httpResponse.statusCode != 200 {
                let errorMessage = String(data: data, encoding: .utf8) ?? "Unknown error"
                throw APIError.serverError(httpResponse.statusCode, errorMessage)
            }

            let branches = try decoder.decode([String].self, from: data)
            return branches
        } catch let error as DecodingError {
            throw APIError.decodingError(error)
        } catch let error as APIError {
            throw error
        } catch {
            throw APIError.networkError(error)
        }
    }

    func createWorkspace(orgRepo: String, branch: String?) async throws -> WorkspaceInfo {
        let headers = try await getHeaders(includeCodespace: true)
        let components = orgRepo.split(separator: "/")

        guard components.count == 2 else {
            throw APIError.serverError(400, "Repository must be in format 'org/repo'")
        }

        let org = String(components[0])
        let repo = String(components[1])

        var urlString = "\(baseURL)/v1/git/checkout/\(org)/\(repo)"
        if let branch = branch, let encodedBranch = branch.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) {
            urlString += "?branch=\(encodedBranch)"
        }

        guard let url = URL(string: urlString) else {
            throw APIError.invalidURL
        }

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.allHTTPHeaderFields = headers

        let (data, response) = try await URLSession.shared.data(for: request)

        guard let httpResponse = response as? HTTPURLResponse else {
            throw APIError.networkError(NSError(domain: "Invalid response", code: -1))
        }

        if httpResponse.statusCode != 200 {
            let errorText = String(data: data, encoding: .utf8) ?? "Unknown error"
            throw APIError.serverError(httpResponse.statusCode, errorText)
        }

        let result = try decoder.decode(CheckoutResponse.self, from: data)
        return result.worktree
    }

    func deleteWorkspace(id: String) async throws {
        let headers = try await getHeaders(includeCodespace: true)

        // URL encode the workspace ID in case it contains special characters
        guard let encodedId = id.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed),
              let url = URL(string: "\(baseURL)/v1/git/worktrees/\(encodedId)") else {
            throw APIError.invalidURL
        }

        var request = URLRequest(url: url)
        request.httpMethod = "DELETE"
        request.allHTTPHeaderFields = headers

        let (data, response) = try await URLSession.shared.data(for: request)

        guard let httpResponse = response as? HTTPURLResponse else {
            throw APIError.networkError(NSError(domain: "Invalid response", code: -1))
        }

        if httpResponse.statusCode != 200 {
            let errorText = String(data: data, encoding: .utf8) ?? "Unknown error"
            throw APIError.serverError(httpResponse.statusCode, errorText)
        }
    }

    func checkAuthStatus() async -> (authenticated: Bool, user: String?) {
        do {
            let headers = try await getHeaders()
            guard let url = URL(string: "\(baseURL)/v1/auth/status") else {
                return (false, nil)
            }

            var request = URLRequest(url: url)
            request.allHTTPHeaderFields = headers

            let (data, response) = try await URLSession.shared.data(for: request)

            guard let httpResponse = response as? HTTPURLResponse,
                  httpResponse.statusCode == 200 else {
                return (false, nil)
            }

            if let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
               let authenticated = json["authenticated"] as? Bool {
                let user = json["username"] as? String
                return (authenticated, user)
            }

            return (false, nil)
        } catch {
            return (false, nil)
        }
    }

    // MARK: - Pull Request API

    func generatePRSummary(workspacePath: String, branch: String) async throws -> PRSummary {
        NSLog("🐱 [generatePRSummary] Generating PR summary for workspace: \(workspacePath)")
        let headers = try await getHeaders(includeCodespace: true)

        guard let url = URL(string: "\(baseURL)/v1/claude/messages") else {
            throw APIError.invalidURL
        }

        let prompt = """
I need you to generate a pull request title and description for the branch "\(branch)" based on all the changes we've made in this session.

Please respond with JSON in the following format:
```json
{
  "title": "Brief, descriptive title of the changes",
  "description": "Focused description of what was changed and why, formatted in markdown"
}
```

Make the title concise but descriptive. Keep the description focused but informative - use 1-3 paragraphs explaining:
- What was changed
- Why it was changed
- Any key implementation notes

Avoid overly lengthy explanations or step-by-step implementation details.
"""

        let requestBody: [String: Any] = [
            "prompt": prompt,
            "working_directory": workspacePath,
            "resume": true,
            "max_turns": 1,
            "suppress_events": true
        ]

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.allHTTPHeaderFields = headers
        request.httpBody = try JSONSerialization.data(withJSONObject: requestBody)

        let (data, response): (Data, URLResponse)
        do {
            (data, response) = try await URLSession.shared.data(for: request)
        } catch {
            NSLog("🐱 [generatePRSummary] ❌ Network error: \(error)")
            throw APIError.networkError(error)
        }

        guard let httpResponse = response as? HTTPURLResponse else {
            throw APIError.networkError(NSError(domain: "Invalid response", code: -1))
        }

        if httpResponse.statusCode != 200 {
            let errorMessage = String(data: data, encoding: .utf8) ?? "Unknown error"
            NSLog("🐱 [generatePRSummary] ❌ Server error \(httpResponse.statusCode): \(errorMessage.prefix(500))")
            throw APIError.serverError(httpResponse.statusCode, errorMessage)
        }

        do {
            // Parse Claude's response
            if let json = try JSONSerialization.jsonObject(with: data) as? [String: Any],
               let responseText = json["response"] as? String ?? json["message"] as? String {

                NSLog("🐱 [generatePRSummary] Got response from Claude: \(responseText.prefix(200))")

                // Extract JSON from code fence
                if let jsonMatch = responseText.range(of: "```json\\s*([\\s\\S]*?)\\s*```", options: .regularExpression) {
                    let jsonText = String(responseText[jsonMatch])
                        .replacingOccurrences(of: "```json", with: "")
                        .replacingOccurrences(of: "```", with: "")
                        .trimmingCharacters(in: .whitespacesAndNewlines)

                    if let jsonData = jsonText.data(using: .utf8) {
                        let summary = try decoder.decode(PRSummary.self, from: jsonData)
                        NSLog("🐱 [generatePRSummary] ✅ Successfully generated PR summary")
                        return summary
                    }
                }

                // Try parsing the whole response as JSON
                if let jsonData = responseText.data(using: .utf8),
                   let summary = try? decoder.decode(PRSummary.self, from: jsonData) {
                    NSLog("🐱 [generatePRSummary] ✅ Successfully generated PR summary")
                    return summary
                }

                // Fallback: use the response as description
                NSLog("🐱 [generatePRSummary] Using fallback format")
                return PRSummary(
                    title: "PR: \(branch)",
                    description: responseText
                )
            }

            throw APIError.decodingError(NSError(domain: "Failed to parse Claude response", code: -1))
        } catch let error as DecodingError {
            NSLog("🐱 [generatePRSummary] ❌ Decoding error: \(error)")
            if let rawText = String(data: data, encoding: .utf8) {
                NSLog("🐱 [generatePRSummary] Raw response: \(rawText.prefix(500))")
            }
            throw APIError.decodingError(error)
        }
    }

    func createPullRequest(workspaceId: String, title: String, description: String) async throws -> String {
        NSLog("🐱 [createPullRequest] Creating PR for workspace: \(workspaceId)")
        let headers = try await getHeaders(includeCodespace: true)

        guard let url = URL(string: "\(baseURL)/v1/git/worktrees/\(workspaceId)/pr") else {
            throw APIError.invalidURL
        }

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.allHTTPHeaderFields = headers

        let body = [
            "title": title,
            "body": description
        ]

        request.httpBody = try JSONEncoder().encode(body)

        let (data, response): (Data, URLResponse)
        do {
            (data, response) = try await URLSession.shared.data(for: request)
        } catch {
            NSLog("🐱 [createPullRequest] ❌ Network error: \(error)")
            throw APIError.networkError(error)
        }

        guard let httpResponse = response as? HTTPURLResponse else {
            throw APIError.networkError(NSError(domain: "Invalid response", code: -1))
        }

        if httpResponse.statusCode != 200 {
            let errorMessage = String(data: data, encoding: .utf8) ?? "Unknown error"
            NSLog("🐱 [createPullRequest] ❌ Server error \(httpResponse.statusCode): \(errorMessage)")
            throw APIError.serverError(httpResponse.statusCode, errorMessage)
        }

        // Parse the response to get the PR URL
        if let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
           let prUrl = json["url"] as? String {
            NSLog("🐱 [createPullRequest] ✅ Successfully created PR: \(prUrl)")
            return prUrl
        }

        throw APIError.decodingError(NSError(domain: "Failed to parse PR URL from response", code: -1))
    }
}

// MARK: - Keychain Helper

actor KeychainHelper {
    static func save(key: String, value: String) async throws {
        let data = value.data(using: .utf8)!

        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrAccount as String: key,
            kSecValueData as String: data,
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlock
        ]

        // Delete any existing item
        SecItemDelete(query as CFDictionary)

        // Add new item
        let status = SecItemAdd(query as CFDictionary, nil)
        guard status == errSecSuccess else {
            throw NSError(domain: "Keychain", code: Int(status))
        }
    }

    static func load(key: String) async throws -> String {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrAccount as String: key,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne
        ]

        var result: AnyObject?
        let status = SecItemCopyMatching(query as CFDictionary, &result)

        guard status == errSecSuccess,
              let data = result as? Data,
              let value = String(data: data, encoding: .utf8) else {
            throw NSError(domain: "Keychain", code: Int(status))
        }

        return value
    }

    static func delete(key: String) async throws {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrAccount as String: key
        ]

        let status = SecItemDelete(query as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else {
            throw NSError(domain: "Keychain", code: Int(status))
        }
    }
}
