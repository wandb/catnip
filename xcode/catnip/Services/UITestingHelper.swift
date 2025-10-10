//
//  UITestingHelper.swift
//  catnip
//
//  Helper for UI testing support
//

import Foundation

struct UITestingHelper {

    // MARK: - Testing Flags

    static var isUITesting: Bool {
        ProcessInfo.processInfo.arguments.contains("-UITesting")
    }

    static var isRunningTests: Bool {
        ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] != nil ||
        ProcessInfo.processInfo.arguments.contains("-UITesting")
    }

    static var shouldDisableAnimations: Bool {
        ProcessInfo.processInfo.arguments.contains("-DisableAnimations")
    }

    static var shouldSkipAuthentication: Bool {
        ProcessInfo.processInfo.arguments.contains("-SkipAuthentication")
    }

    static var shouldUseMockData: Bool {
        ProcessInfo.processInfo.arguments.contains("-UseMockData")
    }

    static var shouldShowWorkspacesList: Bool {
        ProcessInfo.processInfo.arguments.contains("-ShowWorkspacesList")
    }

    static var shouldShowEmptyWorkspaces: Bool {
        ProcessInfo.processInfo.arguments.contains("-EmptyWorkspaces")
    }

    // MARK: - Mock Data

    static func setupMockAuthenticationIfNeeded(authManager: AuthManager) async {
        guard isUITesting && shouldSkipAuthentication else { return }

        // Set up mock authentication
        await MainActor.run {
            authManager.sessionToken = "mock-session-token"
            authManager.username = "testuser"
            authManager.isAuthenticated = true
            authManager.isLoading = false
        }

        // Save mock credentials to keychain
        try? await KeychainHelper.save(key: "session_token", value: "mock-session-token")
        try? await KeychainHelper.save(key: "username", value: "testuser")
    }

    static func getMockWorkspaces() -> [WorkspaceInfo] {
        guard shouldUseMockData else { return [] }

        if shouldShowEmptyWorkspaces {
            return []
        }

        return [
            WorkspaceInfo(
                id: "mock-ws-1",
                name: "feature-authentication",
                branch: "feature/auth",
                repoId: "wandb/catnip",
                claudeActivityState: .active,
                commitCount: 3,
                isDirty: true,
                lastAccessed: ISO8601DateFormatter().string(from: Date()),
                createdAt: ISO8601DateFormatter().string(from: Date().addingTimeInterval(-86400)),
                todos: [
                    Todo(content: "Implement OAuth flow", status: .completed, activeForm: nil),
                    Todo(content: "Add token validation", status: .inProgress, activeForm: "Adding token validation"),
                    Todo(content: "Write tests", status: .pending, activeForm: nil)
                ],
                latestSessionTitle: "Implementing GitHub OAuth",
                latestUserPrompt: "Add GitHub authentication",
                pullRequestUrl: "https://github.com/wandb/catnip/pull/123",
                path: "/workspaces/feature-auth",
                cacheStatus: nil
            ),
            WorkspaceInfo(
                id: "mock-ws-2",
                name: "bugfix-api",
                branch: "bugfix/api-error",
                repoId: "wandb/catnip",
                claudeActivityState: .inactive,
                commitCount: 1,
                isDirty: false,
                lastAccessed: ISO8601DateFormatter().string(from: Date().addingTimeInterval(-172800)),
                createdAt: ISO8601DateFormatter().string(from: Date().addingTimeInterval(-259200)),
                todos: nil,
                latestSessionTitle: "Fixed API error handling",
                latestUserPrompt: "Fix API errors",
                pullRequestUrl: nil,
                path: "/workspaces/bugfix-api",
                cacheStatus: nil
            ),
            WorkspaceInfo(
                id: "mock-ws-3",
                name: "refactor-ui",
                branch: "main",
                repoId: "acme/project",
                claudeActivityState: .inactive,
                commitCount: 0,
                isDirty: false,
                lastAccessed: ISO8601DateFormatter().string(from: Date().addingTimeInterval(-604800)),
                createdAt: ISO8601DateFormatter().string(from: Date().addingTimeInterval(-1209600)),
                todos: nil,
                latestSessionTitle: nil,
                latestUserPrompt: nil,
                pullRequestUrl: nil,
                path: "/workspaces/refactor",
                cacheStatus: nil
            )
        ]
    }

    static func getMockClaudeSessions() -> [String: ClaudeSessionData] {
        guard shouldUseMockData else { return [:] }

        return [
            "/workspaces/feature-auth": ClaudeSessionData(turnCount: 5, isActive: true),
            "/workspaces/bugfix-api": ClaudeSessionData(turnCount: 3, isActive: false),
        ]
    }

    static func shouldAutoNavigateToWorkspaces() -> Bool {
        isUITesting && shouldShowWorkspacesList
    }
}
