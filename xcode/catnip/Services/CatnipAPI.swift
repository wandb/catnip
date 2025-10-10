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
        var headers = try await getHeaders(includeCodespace: true)

        // Add If-None-Match header for conditional request
        if let etag = ifNoneMatch {
            headers["If-None-Match"] = etag
        }

        guard let url = URL(string: "\(baseURL)/v1/git/worktrees") else {
            throw APIError.invalidURL
        }

        var request = URLRequest(url: url)
        request.allHTTPHeaderFields = headers

        do {
            let (data, response) = try await URLSession.shared.data(for: request)

            guard let httpResponse = response as? HTTPURLResponse else {
                throw APIError.networkError(NSError(domain: "Invalid response", code: -1))
            }

            // Handle 304 Not Modified - content unchanged
            if httpResponse.statusCode == 304 {
                NSLog("🐱 Workspaces not modified (304)")
                return nil
            }

            if httpResponse.statusCode != 200 {
                let errorMessage = String(data: data, encoding: .utf8) ?? "Unknown error"
                throw APIError.serverError(httpResponse.statusCode, errorMessage)
            }

            if data.isEmpty {
                let etag = httpResponse.value(forHTTPHeaderField: "ETag")
                return (workspaces: [], etag: etag)
            }

            let workspaces = try decoder.decode([WorkspaceInfo].self, from: data)

            // Extract ETag from response headers
            let etag = httpResponse.value(forHTTPHeaderField: "ETag")

            return (workspaces: workspaces, etag: etag)
        } catch let error as DecodingError {
            throw APIError.decodingError(error)
        } catch let error as APIError {
            throw error
        } catch {
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
