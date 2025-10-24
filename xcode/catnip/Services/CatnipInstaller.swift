//
//  CatnipInstaller.swift
//  catnip
//
//  Service for installing Catnip feature in repositories
//

import Foundation
import SwiftUI
import Combine

// MARK: - Models

struct Repository: Identifiable, Codable {
    let id: Int
    let name: String
    let fullName: String
    let defaultBranch: String
    let isPrivate: Bool
    let isFork: Bool
    let hasDevcontainer: Bool
    let hasCatnipFeature: Bool

    var displayName: String {
        fullName
    }

    var statusIcon: String {
        if hasCatnipFeature {
            return "checkmark.circle.fill"
        } else if hasDevcontainer {
            return "doc.text.fill"
        } else {
            return "doc.badge.plus"
        }
    }

    var statusColor: Color {
        if hasCatnipFeature {
            return .green
        } else if hasDevcontainer {
            return .blue
        } else {
            return .secondary
        }
    }

    var statusText: String {
        if hasCatnipFeature {
            return "Catnip installed"
        } else if hasDevcontainer {
            return "Has devcontainer"
        } else {
            return "No devcontainer"
        }
    }

    enum CodingKeys: String, CodingKey {
        case id
        case name
        case fullName = "full_name"
        case defaultBranch = "default_branch"
        case isPrivate = "private"
        case isFork = "fork"
        case hasDevcontainer = "has_devcontainer"
        case hasCatnipFeature = "has_catnip_feature"
    }
}

struct RepositoriesResponse: Codable {
    let repositories: [Repository]
    let page: Int
    let perPage: Int

    enum CodingKeys: String, CodingKey {
        case repositories
        case page
        case perPage = "per_page"
    }
}

struct InstallationResult: Codable {
    let success: Bool
    let prUrl: String?
    let prNumber: Int?
    let branch: String?
    let repository: String?
    let codespace: CodespaceCreationInfo?
    let error: String?
    let alreadyInstalled: Bool?

    enum CodingKeys: String, CodingKey {
        case success
        case prUrl = "pr_url"
        case prNumber = "pr_number"
        case branch
        case repository
        case codespace
        case error
        case alreadyInstalled = "already_installed"
    }
}

struct CodespaceCreationInfo: Codable {
    let name: String
    let url: String
}

struct CodespaceCreationResult: Codable {
    let success: Bool
    let codespace: CodespaceInfo?
    let error: String?

    struct CodespaceInfo: Codable {
        let id: Int
        let name: String
        let state: String
        let url: String
        let createdAt: String

        enum CodingKeys: String, CodingKey {
            case id
            case name
            case state
            case url
            case createdAt = "created_at"
        }
    }
}

struct UserStatus: Codable {
    let hasAnyCodespaces: Bool

    enum CodingKeys: String, CodingKey {
        case hasAnyCodespaces = "has_any_codespaces"
    }
}

enum InstallationStep {
    case idle
    case fetchingRepoInfo
    case creatingBranch
    case updatingDevcontainer
    case committingChanges
    case creatingPR
    case startingCodespace
    case creatingCodespace
    case waitingForCodespace
    case complete

    var description: String {
        switch self {
        case .idle: return ""
        case .fetchingRepoInfo: return "Fetching repository information..."
        case .creatingBranch: return "Creating install-catnip branch..."
        case .updatingDevcontainer: return "Updating devcontainer.json..."
        case .committingChanges: return "Committing changes..."
        case .creatingPR: return "Creating pull request..."
        case .startingCodespace: return "Starting codespace..."
        case .creatingCodespace: return "Creating codespace..."
        case .waitingForCodespace: return "Waiting for codespace to be ready..."
        case .complete: return "Complete!"
        }
    }

    var icon: String {
        switch self {
        case .idle: return ""
        case .fetchingRepoInfo: return "info.circle"
        case .creatingBranch: return "arrow.branch"
        case .updatingDevcontainer: return "doc.text"
        case .committingChanges: return "checkmark.circle"
        case .creatingPR: return "arrow.up.doc"
        case .startingCodespace: return "terminal"
        case .creatingCodespace: return "terminal.fill"
        case .waitingForCodespace: return "hourglass"
        case .complete: return "checkmark.circle.fill"
        }
    }
}

// MARK: - Service

class CatnipInstaller: ObservableObject {
    static let shared = CatnipInstaller()

    @Published var repositories: [Repository] = []
    @Published var isLoading = false
    @Published var currentStep: InstallationStep = .idle
    @Published var error: String?
    @Published var userStatus: UserStatus?

    private let baseURL = "https://catnip.run"
    private let decoder = JSONDecoder()
    private let encoder = JSONEncoder()

    // Cache settings
    private let cacheValidityDuration: TimeInterval = 30 * 60 // 30 minutes
    private let repositoriesCacheKey = "cached_repositories"
    private let lastFetchTimestampKey = "repositories_last_fetch"

    private init() {
        // Load cached repositories on initialization
        loadCachedRepositories()
    }

    // MARK: - Computed Properties

    /// Check if user has any repositories with Catnip installed (from cached data)
    var hasRepositoriesWithCatnip: Bool {
        repositories.contains { $0.hasCatnipFeature }
    }

    // MARK: - Cache Management

    private func loadCachedRepositories() {
        guard let data = UserDefaults.standard.data(forKey: repositoriesCacheKey),
              let cached = try? decoder.decode([Repository].self, from: data) else {
            return
        }
        repositories = cached
        NSLog("🐱 [CatnipInstaller] Loaded \(cached.count) repositories from cache")
    }

    private func saveRepositoriesToCache(_ repos: [Repository]) {
        guard let data = try? encoder.encode(repos) else {
            NSLog("🐱 [CatnipInstaller] Failed to encode repositories for cache")
            return
        }
        UserDefaults.standard.set(data, forKey: repositoriesCacheKey)
        UserDefaults.standard.set(Date().timeIntervalSince1970, forKey: lastFetchTimestampKey)
        NSLog("🐱 [CatnipInstaller] Saved \(repos.count) repositories to cache")
    }

    private func isCacheValid() -> Bool {
        // Cache is only valid if we have data AND the timestamp is recent
        guard !repositories.isEmpty else {
            NSLog("🐱 [CatnipInstaller] Cache invalid: no repositories loaded")
            return false
        }

        let lastFetch = UserDefaults.standard.double(forKey: lastFetchTimestampKey)
        guard lastFetch > 0 else {
            NSLog("🐱 [CatnipInstaller] Cache invalid: no timestamp")
            return false
        }

        let elapsed = Date().timeIntervalSince1970 - lastFetch
        let isValid = elapsed < cacheValidityDuration
        NSLog("🐱 [CatnipInstaller] Cache age: \(Int(elapsed))s, has \(repositories.count) repos, valid: \(isValid)")
        return isValid
    }

    func clearCache() {
        UserDefaults.standard.removeObject(forKey: repositoriesCacheKey)
        UserDefaults.standard.removeObject(forKey: lastFetchTimestampKey)
        repositories = []
        NSLog("🐱 [CatnipInstaller] Cache cleared")
    }

    /// Mark a repository as having Catnip installed (optimistic update after installation)
    func markRepositoryAsHavingCatnip(_ repositoryFullName: String) {
        guard let index = repositories.firstIndex(where: { $0.fullName == repositoryFullName }) else {
            NSLog("🐱 [CatnipInstaller] Repository \(repositoryFullName) not found in cache for update")
            return
        }

        let repo = repositories[index]

        // Create updated repository with Catnip feature enabled
        let updatedRepo = Repository(
            id: repo.id,
            name: repo.name,
            fullName: repo.fullName,
            defaultBranch: repo.defaultBranch,
            isPrivate: repo.isPrivate,
            isFork: repo.isFork,
            hasDevcontainer: true,  // Installing Catnip means we have a devcontainer now
            hasCatnipFeature: true
        )

        // Update in array
        repositories[index] = updatedRepo

        // Save updated cache
        saveRepositoriesToCache(repositories)

        NSLog("🐱 [CatnipInstaller] Optimistically marked \(repositoryFullName) as having Catnip")
    }

    // MARK: - Helper Methods

    private func getSessionToken() async throws -> String {
        guard let token = try? await KeychainHelper.load(key: "session_token") else {
            throw APIError.noSessionToken
        }
        return token
    }

    private func getHeaders() async throws -> [String: String] {
        let token = try await getSessionToken()
        return [
            "Content-Type": "application/json",
            "Authorization": "Bearer \(token)"
        ]
    }

    // MARK: - Public API

    /// Fetch user's repositories with devcontainer status
    func fetchRepositories(page: Int = 1, perPage: Int = 50, org: String? = nil, forceRefresh: Bool = false) async throws {
        // Return mock data in UI testing mode
        if UITestingHelper.shouldUseMockData {
            NSLog("🐱 [CatnipInstaller] Using mock repositories")
            await MainActor.run {
                self.repositories = UITestingHelper.getMockRepositories()
                self.isLoading = false
            }
            return
        }

        // Check cache first if not forcing refresh
        if !forceRefresh && isCacheValid() {
            NSLog("🐱 [CatnipInstaller] Using cached repositories (valid for \(Int(cacheValidityDuration - (Date().timeIntervalSince1970 - UserDefaults.standard.double(forKey: lastFetchTimestampKey))))s more)")
            await MainActor.run { isLoading = false }
            return
        }

        await MainActor.run { isLoading = true }
        defer { Task { await MainActor.run { isLoading = false } } }

        NSLog("🐱 [CatnipInstaller] Fetching repositories from API (forceRefresh: \(forceRefresh), org: \(org ?? "none"))")

        let headers = try await getHeaders()
        var urlString = "\(baseURL)/v1/repositories?page=\(page)&per_page=\(perPage)"
        if let org = org {
            urlString += "&org=\(org)"
        }
        guard let url = URL(string: urlString) else {
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

            let repositoriesResponse = try decoder.decode(RepositoriesResponse.self, from: data)

            await MainActor.run {
                self.repositories = repositoriesResponse.repositories
                // Save to cache after successful fetch
                self.saveRepositoriesToCache(repositoriesResponse.repositories)
            }
        } catch {
            await MainActor.run {
                self.error = error.localizedDescription
            }
            throw error
        }
    }

    /// Install Catnip feature in a repository
    func installCatnip(
        repository: String,
        startCodespace: Bool = false
    ) async throws -> InstallationResult {
        await MainActor.run {
            error = nil
            currentStep = .fetchingRepoInfo
        }

        let headers = try await getHeaders()
        guard let url = URL(string: "\(baseURL)/v1/codespace/install") else {
            throw APIError.invalidURL
        }

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.allHTTPHeaderFields = headers

        let body: [String: Any] = [
            "repository": repository,
            "startCodespace": startCodespace
        ]
        request.httpBody = try JSONSerialization.data(withJSONObject: body)

        // Simulate steps for user feedback
        let steps: [InstallationStep] = [
            .fetchingRepoInfo,
            .creatingBranch,
            .updatingDevcontainer,
            .committingChanges,
            .creatingPR
        ]

        // Start a task to cycle through steps while we wait for response
        let stepTask = Task {
            for step in steps {
                if Task.isCancelled { break }
                await MainActor.run { currentStep = step }
                if Task.isCancelled { break }
                try? await Task.sleep(nanoseconds: 800_000_000) // 0.8 seconds per step
            }
        }

        do {
            NSLog("🐱 [CatnipInstaller] Starting installation request for \(repository)")
            let (data, response) = try await URLSession.shared.data(for: request)
            NSLog("🐱 [CatnipInstaller] Got response, data size: \(data.count) bytes")

            // Cancel the step animation
            stepTask.cancel()

            guard let httpResponse = response as? HTTPURLResponse else {
                NSLog("🐱 [CatnipInstaller] ❌ Invalid response type")
                throw APIError.networkError(NSError(domain: "Invalid response", code: -1))
            }

            NSLog("🐱 [CatnipInstaller] HTTP status: \(httpResponse.statusCode)")

            // Log response for debugging
            if let responseString = String(data: data, encoding: .utf8) {
                NSLog("🐱 [CatnipInstaller] Response body: \(responseString)")
            }

            // Parse response even for errors to get details
            let result = try decoder.decode(InstallationResult.self, from: data)
            NSLog("🐱 [CatnipInstaller] Successfully decoded response")

            if httpResponse.statusCode != 200 {
                NSLog("🐱 [CatnipInstaller] ❌ Non-200 status code: \(httpResponse.statusCode)")
                await MainActor.run {
                    self.error = result.error ?? "Installation failed"
                    currentStep = .idle
                }
                throw APIError.serverError(httpResponse.statusCode, result.error ?? "Unknown error")
            }

            NSLog("🐱 [CatnipInstaller] Installation successful, PR URL: \(result.prUrl ?? "none")")

            // If we're starting a codespace, show that step
            if startCodespace && result.codespace != nil {
                NSLog("🐱 [CatnipInstaller] Starting codespace...")
                await MainActor.run { currentStep = .startingCodespace }
                try? await Task.sleep(nanoseconds: 500_000_000) // 0.5 seconds
            }

            NSLog("🐱 [CatnipInstaller] Setting step to complete")
            await MainActor.run {
                currentStep = .complete
            }

            return result
        } catch {
            NSLog("🐱 [CatnipInstaller] ❌ Error during installation: \(error)")
            stepTask.cancel()
            await MainActor.run {
                self.error = error.localizedDescription
                currentStep = .idle
            }
            throw error
        }
    }

    /// Reset state
    func reset() {
        repositories = []
        error = nil
        currentStep = .idle
        isLoading = false
    }

    /// Create a new codespace for a repository
    func createCodespace(
        repository: String,
        branch: String? = nil
    ) async throws -> CodespaceCreationResult.CodespaceInfo {
        await MainActor.run {
            error = nil
            currentStep = .creatingCodespace
        }

        let headers = try await getHeaders()
        guard let url = URL(string: "\(baseURL)/v1/codespace/create") else {
            throw APIError.invalidURL
        }

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.allHTTPHeaderFields = headers

        var body: [String: Any] = ["repository": repository]
        if let branch = branch {
            body["ref"] = branch
        }
        request.httpBody = try JSONSerialization.data(withJSONObject: body)

        do {
            NSLog("🐱 [CatnipInstaller] Creating codespace for \(repository)\(branch.map { " on branch \($0)" } ?? "")")

            await MainActor.run { currentStep = .creatingCodespace }

            let (data, response) = try await URLSession.shared.data(for: request)

            guard let httpResponse = response as? HTTPURLResponse else {
                NSLog("🐱 [CatnipInstaller] ❌ Invalid response type")
                throw APIError.networkError(NSError(domain: "Invalid response", code: -1))
            }

            NSLog("🐱 [CatnipInstaller] HTTP status: \(httpResponse.statusCode)")

            // Log response for debugging
            if let responseString = String(data: data, encoding: .utf8) {
                NSLog("🐱 [CatnipInstaller] Response body: \(responseString)")
            }

            // Parse response
            let result = try decoder.decode(CodespaceCreationResult.self, from: data)

            if httpResponse.statusCode != 200 {
                NSLog("🐱 [CatnipInstaller] ❌ Non-200 status code: \(httpResponse.statusCode)")
                await MainActor.run {
                    self.error = result.error ?? "Failed to create codespace"
                    currentStep = .idle
                }
                throw APIError.serverError(httpResponse.statusCode, result.error ?? "Unknown error")
            }

            guard let codespace = result.codespace else {
                NSLog("🐱 [CatnipInstaller] ❌ No codespace in response")
                await MainActor.run {
                    self.error = "No codespace information in response"
                    currentStep = .idle
                }
                throw APIError.serverError(httpResponse.statusCode, "No codespace in response")
            }

            NSLog("🐱 [CatnipInstaller] Codespace created: \(codespace.name), initial state: \(codespace.state)")

            // Poll for up to 5 minutes waiting for codespace to be built
            // Initial builds can take a long time, so we wait here before triggering SSE flow
            if codespace.state != "Available" {
                NSLog("🐱 [CatnipInstaller] Codespace not yet available, polling status...")
                try await pollCodespaceStatus(codespaceName: codespace.name)
            } else {
                NSLog("🐱 [CatnipInstaller] Codespace already available")
            }

            await MainActor.run { currentStep = .complete }

            return codespace
        } catch {
            NSLog("🐱 [CatnipInstaller] ❌ Error during codespace creation: \(error)")
            await MainActor.run {
                self.error = error.localizedDescription
                currentStep = .idle
            }
            throw error
        }
    }

    /// Poll codespace status until it's available AND has credentials (up to 10 minutes)
    private func pollCodespaceStatus(codespaceName: String) async throws {
        await MainActor.run { currentStep = .waitingForCodespace }

        let maxAttempts = 60 // 60 attempts × 10 seconds = 10 minutes
        let pollInterval: UInt64 = 10_000_000_000 // 10 seconds in nanoseconds

        for attempt in 1...maxAttempts {
            NSLog("🐱 [CatnipInstaller] Polling codespace status (attempt \(attempt)/\(maxAttempts))...")

            let headers = try await getHeaders()
            guard let url = URL(string: "\(baseURL)/v1/codespace/status/\(codespaceName)") else {
                throw APIError.invalidURL
            }

            var request = URLRequest(url: url)
            request.allHTTPHeaderFields = headers

            do {
                let (data, response) = try await URLSession.shared.data(for: request)

                guard let httpResponse = response as? HTTPURLResponse else {
                    throw APIError.networkError(NSError(domain: "Invalid response", code: -1))
                }

                if httpResponse.statusCode == 200 {
                    // Parse the response from our Cloudflare worker
                    if let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                       let codespaceData = json["codespace"] as? [String: Any],
                       let state = codespaceData["state"] as? String,
                       let hasCredentials = json["has_credentials"] as? Bool {

                        NSLog("🐱 [CatnipInstaller] Codespace state: \(state), has_credentials: \(hasCredentials)")

                        // Complete when EITHER codespace is Available OR we have credentials
                        if state == "Available" || hasCredentials {
                            NSLog("🐱 [CatnipInstaller] ✅ Codespace ready! (state=\(state), credentials=\(hasCredentials))")

                            // Notify tracker that creation is complete
                            await MainActor.run {
                                CodespaceCreationTracker.shared.completeCreation(codespaceName: codespaceName)
                            }

                            return
                        }
                    }
                } else {
                    NSLog("🐱 [CatnipInstaller] ⚠️ Status check failed with code \(httpResponse.statusCode)")
                }

                // Wait before next attempt (except on last attempt)
                if attempt < maxAttempts {
                    try await Task.sleep(nanoseconds: pollInterval)
                }
            } catch {
                NSLog("🐱 [CatnipInstaller] ⚠️ Error checking codespace status: \(error)")
                // Continue polling despite errors
                if attempt < maxAttempts {
                    try await Task.sleep(nanoseconds: pollInterval)
                }
            }
        }

        // Timeout - codespace didn't become available in time
        NSLog("🐱 [CatnipInstaller] ⏰ Timeout waiting for codespace to be ready")

        // Notify tracker of timeout failure
        await MainActor.run {
            CodespaceCreationTracker.shared.failCreation(
                error: "Codespace did not become ready within 10 minutes"
            )
        }

        throw APIError.serverError(408, "Codespace did not become ready within 10 minutes")
    }

    /// Fetch user status (codespaces and repositories with Catnip)
    func fetchUserStatus() async throws {
        // Skip in UI testing mode
        if UITestingHelper.shouldUseMockData {
            NSLog("🐱 [CatnipInstaller] Using mock user status")
            await MainActor.run {
                self.userStatus = UITestingHelper.getMockUserStatus()
            }
            return
        }

        let headers = try await getHeaders()
        guard let url = URL(string: "\(baseURL)/v1/user/status") else {
            throw APIError.invalidURL
        }

        var request = URLRequest(url: url)
        request.allHTTPHeaderFields = headers

        do {
            NSLog("🐱 [CatnipInstaller] Fetching user status")
            let (data, response) = try await URLSession.shared.data(for: request)

            guard let httpResponse = response as? HTTPURLResponse else {
                throw APIError.networkError(NSError(domain: "Invalid response", code: -1))
            }

            if httpResponse.statusCode != 200 {
                let errorMessage = String(data: data, encoding: .utf8) ?? "Unknown error"
                NSLog("🐱 [CatnipInstaller] ❌ Failed to fetch user status: \(httpResponse.statusCode)")
                throw APIError.serverError(httpResponse.statusCode, errorMessage)
            }

            let status = try decoder.decode(UserStatus.self, from: data)
            NSLog("🐱 [CatnipInstaller] User status: hasCodespaces=\(status.hasAnyCodespaces)")

            await MainActor.run {
                self.userStatus = status
            }
        } catch {
            NSLog("🐱 [CatnipInstaller] ❌ Error fetching user status: \(error)")
            throw error
        }
    }
}
