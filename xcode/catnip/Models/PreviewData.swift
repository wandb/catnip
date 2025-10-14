//
//  PreviewData.swift
//  catnip
//
//  Mock data for SwiftUI previews
//

import Foundation
import Combine

extension WorkspaceInfo {
    static let preview1 = WorkspaceInfo(
        id: "workspace-1",
        name: "feature-api-docs",
        branch: "feature/api-docs",
        repoId: "wandb/catnip",
        claudeActivityState: .running,
        commitCount: 3,
        isDirty: true,
        lastAccessed: ISO8601DateFormatter().string(from: Date()),
        createdAt: ISO8601DateFormatter().string(from: Date().addingTimeInterval(-86400)),
        todos: [
            Todo(content: "Review API documentation", status: .completed, activeForm: "Reviewing API documentation"),
            Todo(content: "Update endpoint descriptions", status: .inProgress, activeForm: "Updating endpoint descriptions"),
            Todo(content: "Add code examples", status: .pending, activeForm: "Adding code examples")
        ],
        latestSessionTitle: "I'll help you update the API documentation for v2.0. I've reviewed the endpoint descriptions and added code examples for better clarity.",
        latestUserPrompt: "Can you help me update our API documentation for the v2.0 endpoints?",
        pullRequestUrl: nil,
        path: "/workspaces/catnip/feature-api-docs",
        cacheStatus: nil
    )

    static let preview2 = WorkspaceInfo(
        id: "workspace-2",
        name: "fix-authentication-bug",
        branch: "bugfix/auth-token",
        repoId: "wandb/catnip",
        claudeActivityState: .running,
        commitCount: 1,
        isDirty: false,
        lastAccessed: ISO8601DateFormatter().string(from: Date().addingTimeInterval(-3600)),
        createdAt: ISO8601DateFormatter().string(from: Date().addingTimeInterval(-172800)),
        todos: nil,
        latestSessionTitle: nil,
        latestUserPrompt: "Fix the token refresh logic in the authentication module",
        pullRequestUrl: "https://github.com/wandb/catnip/pull/123",
        path: "/workspaces/catnip/fix-auth",
        cacheStatus: nil
    )

    static let preview3 = WorkspaceInfo(
        id: "workspace-3",
        name: "main",
        branch: "main",
        repoId: "acme/project",
        claudeActivityState: .inactive,
        commitCount: 0,
        isDirty: false,
        lastAccessed: ISO8601DateFormatter().string(from: Date().addingTimeInterval(-604800)),
        createdAt: ISO8601DateFormatter().string(from: Date().addingTimeInterval(-2592000)),
        todos: nil,
        latestSessionTitle: nil,
        latestUserPrompt: nil,
        pullRequestUrl: nil,
        path: "/workspaces/project/main",
        cacheStatus: nil
    )

    static let previewList = [preview1, preview2, preview3]
}

extension CodespaceInfo {
    static let preview1 = CodespaceInfo(
        name: "legendary-space-giggle",
        lastUsed: Date().timeIntervalSince1970 * 1000,
        repository: "wandb/catnip"
    )

    static let preview2 = CodespaceInfo(
        name: "solid-carnival-4269",
        lastUsed: Date().addingTimeInterval(-86400).timeIntervalSince1970 * 1000,
        repository: "acme/project"
    )

    static let previewList = [preview1, preview2]
}

extension Todo {
    static let preview1 = Todo(
        content: "Implement user authentication flow",
        status: .completed,
        activeForm: "Implementing user authentication flow"
    )

    static let preview2 = Todo(
        content: "Add input validation for forms",
        status: .inProgress,
        activeForm: "Adding input validation for forms"
    )

    static let preview3 = Todo(
        content: "Write unit tests for API endpoints",
        status: .pending,
        activeForm: "Writing unit tests for API endpoints"
    )

    static let previewList = [preview1, preview2, preview3]
}

class MockAuthManager: AuthManager {
    override init() {
        super.init()
        self.isAuthenticated = true
        self.isLoading = false
        self.sessionToken = "mock-token"
        self.username = "johndoe"
    }

    @MainActor
    override func login() async -> Bool {
        return true
    }

    @MainActor
    override func logout() async {
        // No-op for mock
    }
}
