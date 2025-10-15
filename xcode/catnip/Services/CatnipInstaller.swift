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

enum InstallationStep {
    case idle
    case fetchingRepoInfo
    case creatingBranch
    case updatingDevcontainer
    case committingChanges
    case creatingPR
    case startingCodespace
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
        case .complete: return "Installation complete!"
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

    private let baseURL = "https://catnip.run"
    private let decoder = JSONDecoder()

    private init() {}

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
    func fetchRepositories(page: Int = 1, perPage: Int = 30) async throws {
        await MainActor.run { isLoading = true }
        defer { Task { await MainActor.run { isLoading = false } } }

        let headers = try await getHeaders()
        guard let url = URL(string: "\(baseURL)/v1/repositories?page=\(page)&per_page=\(perPage)") else {
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
                await MainActor.run { currentStep = step }
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
}
