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

    // Preview mode flag - set by AuthManager
    static var isInPreviewMode: Bool = false

    static var shouldUseMockData: Bool {
        ProcessInfo.processInfo.arguments.contains("-UseMockData") || isInPreviewMode
    }

    static var shouldShowWorkspacesList: Bool {
        ProcessInfo.processInfo.arguments.contains("-ShowWorkspacesList")
    }

    static var shouldShowEmptyWorkspaces: Bool {
        ProcessInfo.processInfo.arguments.contains("-EmptyWorkspaces")
    }

    // MARK: - Codespace Scenario Flags

    /// Scenario 1: User has no codespaces and no repos with Catnip → "Install Catnip"
    static var hasNoCodespacesNoReposWithCatnip: Bool {
        ProcessInfo.processInfo.arguments.contains("-NoCodespacesNoRepos")
    }

    /// Scenario 2: User has no codespaces but has repos with Catnip → "Launch New Codespace"
    static var hasNoCodespacesButHasReposWithCatnip: Bool {
        ProcessInfo.processInfo.arguments.contains("-NoCodespacesHasRepos")
    }

    /// Scenario 3: User has codespaces → "Access My Codespace"
    static var hasCodespaces: Bool {
        ProcessInfo.processInfo.arguments.contains("-HasCodespaces")
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
            // Use the preview with code blocks as the first workspace for screenshot tests
            WorkspaceInfo.previewWithCodeBlocks,
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
                latestClaudeMessage: "I've implemented the OAuth flow with GitHub. The token validation is in progress.",
                pullRequestUrl: "https://github.com/wandb/catnip/pull/123",
                pullRequestState: "OPEN",
                hasCommitsAheadOfRemote: nil,
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
                latestClaudeMessage: "The API error handling has been fixed.",
                pullRequestUrl: nil,
                pullRequestState: nil,
                hasCommitsAheadOfRemote: nil,
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
                latestClaudeMessage: nil,
                pullRequestUrl: nil,
                pullRequestState: nil,
                hasCommitsAheadOfRemote: nil,
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

    static func getMockLatestMessage(worktreePath: String) -> LatestMessageResponse {
        return LatestMessageResponse(
            content: "Mock message for \(worktreePath)",
            isError: false
        )
    }

    static func getMockWorkspaceDiff(id: String) -> WorktreeDiffResponse {
        // Return simple mock diff directly without JSON parsing
        return WorktreeDiffResponse(
            summary: "Mock diff for workspace",
            fileDiffs: [
                FileDiff(
                    filePath: "src/auth.swift",
                    changeType: "modified",
                    oldContent: nil,
                    newContent: nil,
                    diffText: """
@@ -10,6 +10,7 @@
 func authenticate() {
+    // Added mock authentication
     return true
 }
""",
                    isExpanded: true
                )
            ],
            totalFiles: 1,
            worktreeId: id,
            worktreeName: "mock-workspace",
            sourceBranch: "main",
            forkCommit: "abc123"
        )
    }

    static func getMockBranches(repoId: String) -> [String] {
        return ["main", "develop", "feature/test-branch"]
    }

    static func getMockClaudeSettings() -> ClaudeSettings {
        let jsonData = """
        {
            "theme": "dark",
            "notificationsEnabled": true,
            "isAuthenticated": true,
            "hasCompletedOnboarding": true,
            "numStartups": 5,
            "version": "1.0.0"
        }
        """.data(using: .utf8)!

        let decoder = JSONDecoder()
        return try! decoder.decode(ClaudeSettings.self, from: jsonData)
    }

    static func getMockPRSummary(branch: String) -> PRSummary {
        return PRSummary(
            title: "Mock PR: \(branch)",
            description: "This is a mock pull request description for testing purposes."
        )
    }

    static func getMockSessionData(workspacePath: String) -> SessionData? {
        guard shouldUseMockData else { return nil }

        // Return mock session data for active workspaces
        if workspacePath.contains("feature-auth") {
            return SessionData(
                sessionInfo: SessionSummary(
                    worktreePath: workspacePath,
                    sessionStartTime: ISO8601DateFormatter().string(from: Date().addingTimeInterval(-3600)),
                    sessionEndTime: nil,
                    turnCount: 5,
                    isActive: true,
                    lastSessionId: "mock-session-1",
                    currentSessionId: "mock-session-1",
                    header: "Implementing GitHub OAuth",
                    lastCost: 0.15,
                    lastDuration: 1800,
                    lastTotalInputTokens: 25000,
                    lastTotalOutputTokens: 12000
                ),
                allSessions: nil,
                latestUserPrompt: "Add GitHub authentication",
                latestMessage: "I've started implementing the OAuth flow...",
                latestThought: "Let me analyze the current authentication structure...",
                stats: SessionStats(
                    totalMessages: 12,
                    userMessages: 5,
                    assistantMessages: 7,
                    humanPromptCount: 5,
                    toolCallCount: 15,
                    totalInputTokens: 45000,
                    totalOutputTokens: 18000,
                    cacheReadTokens: 35000,
                    cacheCreationTokens: 10000,
                    lastContextSizeTokens: 125000,
                    apiCallCount: 8,
                    sessionDurationSeconds: 1800.5,
                    activeDurationSeconds: 900.25,
                    thinkingBlockCount: 5,
                    subAgentCount: 2,
                    compactionCount: 0,
                    imageCount: 0,
                    activeToolNames: ["Read": 8, "Edit": 5, "Bash": 2]
                ),
                todos: [
                    Todo(content: "Implement OAuth callback handler", status: .completed, activeForm: "Implementing OAuth callback handler"),
                    Todo(content: "Add token refresh logic", status: .inProgress, activeForm: "Adding token refresh logic"),
                    Todo(content: "Write unit tests for auth flow", status: .pending, activeForm: "Writing unit tests for auth flow")
                ],
                latestSessionTitle: "Implementing GitHub OAuth"
            )
        }

        return nil
    }

    static func shouldAutoNavigateToWorkspaces() -> Bool {
        isUITesting && shouldShowWorkspacesList
    }

    // MARK: - Codespace Mock Data

    static func getMockRepositories() -> [Repository] {
        // Scenario 1: No repos with Catnip
        if hasNoCodespacesNoReposWithCatnip {
            return [
                Repository(
                    id: 1,
                    name: "test-repo",
                    fullName: "testuser/test-repo",
                    defaultBranch: "main",
                    isPrivate: false,
                    isFork: false,
                    hasDevcontainer: false,
                    hasCatnipFeature: false
                ),
                Repository(
                    id: 2,
                    name: "another-repo",
                    fullName: "testuser/another-repo",
                    defaultBranch: "main",
                    isPrivate: true,
                    isFork: false,
                    hasDevcontainer: true,
                    hasCatnipFeature: false
                )
            ]
        }

        // Scenario 2: Has repos with Catnip
        if hasNoCodespacesButHasReposWithCatnip {
            return [
                Repository(
                    id: 1,
                    name: "catnip-ready-repo",
                    fullName: "testuser/catnip-ready-repo",
                    defaultBranch: "main",
                    isPrivate: false,
                    isFork: false,
                    hasDevcontainer: true,
                    hasCatnipFeature: true
                ),
                Repository(
                    id: 2,
                    name: "another-catnip-repo",
                    fullName: "testuser/another-catnip-repo",
                    defaultBranch: "main",
                    isPrivate: false,
                    isFork: false,
                    hasDevcontainer: true,
                    hasCatnipFeature: true
                )
            ]
        }

        // Scenario 3: Has codespaces (repos don't matter for button text)
        if hasCodespaces {
            return [
                Repository(
                    id: 1,
                    name: "existing-repo",
                    fullName: "testuser/existing-repo",
                    defaultBranch: "main",
                    isPrivate: false,
                    isFork: false,
                    hasDevcontainer: true,
                    hasCatnipFeature: true
                )
            ]
        }

        // Preview mode: return repository matching mock workspaces
        if isInPreviewMode {
            return [
                Repository(
                    id: 1,
                    name: "mobile-app",
                    fullName: "acme/mobile-app",
                    defaultBranch: "main",
                    isPrivate: false,
                    isFork: false,
                    hasDevcontainer: true,
                    hasCatnipFeature: true
                )
            ]
        }

        // Default: empty
        return []
    }

    static func getMockUserStatus() -> UserStatus {
        // Scenarios 1 & 2: No codespaces
        if hasNoCodespacesNoReposWithCatnip || hasNoCodespacesButHasReposWithCatnip {
            return UserStatus(hasAnyCodespaces: false)
        }

        // Scenario 3: Has codespaces
        if hasCodespaces {
            return UserStatus(hasAnyCodespaces: true)
        }

        // Preview mode: show that user has codespaces
        if isInPreviewMode {
            return UserStatus(hasAnyCodespaces: true)
        }

        // Default: no codespaces
        return UserStatus(hasAnyCodespaces: false)
    }
}
