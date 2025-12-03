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
        latestClaudeMessage: "I've reviewed the endpoint descriptions and added code examples for better clarity. The documentation now includes detailed explanations of all request/response formats.",
        pullRequestUrl: nil,
        pullRequestState: nil,
        hasCommitsAheadOfRemote: nil,
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
        latestClaudeMessage: nil,
        pullRequestUrl: "https://github.com/wandb/catnip/pull/123",
        pullRequestState: "OPEN",
        hasCommitsAheadOfRemote: nil,
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
        latestClaudeMessage: nil,
        pullRequestUrl: nil,
        pullRequestState: nil,
        hasCommitsAheadOfRemote: nil,
        path: "/workspaces/project/main",
        cacheStatus: nil
    )

    static let previewWithCodeBlocks = WorkspaceInfo(
        id: "workspace-code",
        name: "feature-markdown-highlighting",
        branch: "feature/markdown-code",
        repoId: "wandb/catnip",
        claudeActivityState: .running,
        commitCount: 2,
        isDirty: true,
        lastAccessed: ISO8601DateFormatter().string(from: Date()),
        createdAt: ISO8601DateFormatter().string(from: Date().addingTimeInterval(-7200)),
        todos: [
            Todo(content: "Add syntax highlighting theme", status: .completed, activeForm: "Adding syntax highlighting theme"),
            Todo(content: "Update MarkdownText component", status: .inProgress, activeForm: "Updating MarkdownText component"),
            Todo(content: "Test in light and dark modes", status: .pending, activeForm: "Testing in light and dark modes")
        ],
        latestSessionTitle: "Enhanced Markdown Code Block Styling",
        latestUserPrompt: "Can you add better styling for code blocks in our markdown rendering?",
        latestClaudeMessage: """
        I've added enhanced code block styling to the MarkdownText component! Here's what I implemented:

        ## Swift Code Example

        ```swift
        import SwiftUI

        struct MarkdownText: View {
            let markdown: String
            @Environment(\\.colorScheme) private var colorScheme

            var body: some View {
                Markdown(markdown)
                    .markdownBlockStyle(\\.codeBlock) { configuration in
                        ScrollView(.horizontal) {
                            configuration.label
                                .padding(AppTheme.Spacing.md)
                        }
                        .background(backgroundColor)
                        .clipShape(RoundedRectangle(cornerRadius: 8))
                    }
            }
        }
        ```

        ## Python Example

        ```python
        def calculate_fibonacci(n):
            '''Calculate nth Fibonacci number using dynamic programming.'''
            if n <= 1:
                return n

            fib = [0, 1]
            for i in range(2, n + 1):
                fib.append(fib[i-1] + fib[i-2])

            return fib[n]

        # Example usage
        result = calculate_fibonacci(10)
        print(f"The 10th Fibonacci number is: {result}")
        ```

        ## JSON Configuration

        ```json
        {
          "theme": {
            "codeBlock": {
              "backgroundColor": "#f7f7f7",
              "borderRadius": 8,
              "padding": 16
            },
            "syntax": {
              "keyword": "#ad3da4",
              "string": "#d12f1b",
              "comment": "#5d6c79"
            }
          }
        }
        ```

        The code blocks now have proper backgrounds, rounded corners, and horizontal scrolling for long lines!
        """,
        pullRequestUrl: nil,
        pullRequestState: nil,
        hasCommitsAheadOfRemote: nil,
        path: "/workspaces/catnip/feature-markdown-code",
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
