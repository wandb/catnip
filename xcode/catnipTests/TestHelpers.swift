//
//  TestHelpers.swift
//  catnipTests
//
//  Shared test helpers and mock data
//

import Foundation
@testable import Catnip

// MARK: - Mock Data Factory

struct MockDataFactory {

    // MARK: - WorkspaceInfo Mocks

    static func createWorkspace(
        id: String = "test-ws-1",
        name: String = "test-workspace",
        branch: String = "main",
        repoId: String = "repo-123",
        claudeActivityState: ClaudeActivityState? = nil,
        commitCount: Int? = nil,
        isDirty: Bool? = nil,
        lastAccessed: String? = nil,
        createdAt: String? = nil,
        todos: [Todo]? = nil,
        latestSessionTitle: String? = nil,
        latestUserPrompt: String? = nil,
        latestClaudeMessage: String? = nil,
        pullRequestUrl: String? = nil,
        pullRequestState: String? = nil,
        hasCommitsAheadOfRemote: Bool? = nil,
        path: String = "/workspaces/test",
        cacheStatus: CacheStatus? = nil
    ) -> WorkspaceInfo {
        WorkspaceInfo(
            id: id,
            name: name,
            branch: branch,
            repoId: repoId,
            claudeActivityState: claudeActivityState,
            commitCount: commitCount,
            isDirty: isDirty,
            lastAccessed: lastAccessed,
            createdAt: createdAt,
            todos: todos,
            latestSessionTitle: latestSessionTitle,
            latestUserPrompt: latestUserPrompt,
            latestClaudeMessage: latestClaudeMessage,
            pullRequestUrl: pullRequestUrl,
            pullRequestState: pullRequestState,
            hasCommitsAheadOfRemote: hasCommitsAheadOfRemote,
            path: path,
            cacheStatus: cacheStatus
        )
    }

    static func createActiveWorkspace() -> WorkspaceInfo {
        createWorkspace(
            id: "active-ws",
            name: "feature-branch",
            branch: "feature/new-feature",
            claudeActivityState: .active,
            commitCount: 5,
            isDirty: true,
            latestSessionTitle: "Implementing authentication",
            latestUserPrompt: "Add login flow"
        )
    }

    static func createInactiveWorkspace() -> WorkspaceInfo {
        createWorkspace(
            id: "inactive-ws",
            name: "old-branch",
            branch: "refs/catnip/old-feature",
            claudeActivityState: .inactive,
            commitCount: 0,
            isDirty: false
        )
    }

    // MARK: - Todo Mocks

    static func createTodo(
        content: String = "Test task",
        status: TodoStatus = .pending,
        activeForm: String? = nil
    ) -> Todo {
        Todo(
            content: content,
            status: status,
            activeForm: activeForm
        )
    }

    static func createTodoList() -> [Todo] {
        [
            createTodo(content: "Implement feature", status: .completed),
            createTodo(content: "Write tests", status: .inProgress, activeForm: "Writing tests"),
            createTodo(content: "Review code", status: .pending)
        ]
    }

    // MARK: - ClaudeSession Mocks

    static func createClaudeSession(
        turnCount: Int = 5,
        isActive: Bool = true
    ) -> ClaudeSessionData {
        ClaudeSessionData(turnCount: turnCount, isActive: isActive)
    }

    // MARK: - Diff Mocks

    static func createFileDiff(
        filePath: String = "src/test.swift",
        changeType: String = "modified",
        diffText: String? = nil,
        isExpanded: Bool = true
    ) -> FileDiff {
        FileDiff(
            filePath: filePath,
            changeType: changeType,
            oldContent: nil,
            newContent: nil,
            diffText: diffText ?? createSampleDiff(),
            isExpanded: isExpanded
        )
    }

    static func createWorktreeDiffResponse(
        fileCount: Int = 3
    ) -> WorktreeDiffResponse {
        let fileDiffs = (0..<fileCount).map { i in
            createFileDiff(filePath: "src/file\(i).swift")
        }

        return WorktreeDiffResponse(
            summary: "\(fileCount) files changed",
            fileDiffs: fileDiffs,
            totalFiles: fileCount,
            worktreeId: "wt-test",
            worktreeName: "test-worktree",
            sourceBranch: "main",
            forkCommit: "abc123"
        )
    }

    static func createSampleDiff() -> String {
        """
        @@ -1,5 +1,6 @@
         import Foundation
        +import SwiftUI

         func example() {
        -    print("old")
        +    print("new")
         }
        """
    }

    // MARK: - Date Helpers

    static func createISO8601Date(daysAgo: Int = 0, hoursAgo: Int = 0) -> String {
        let calendar = Calendar.current
        var date = Date()

        if daysAgo > 0 {
            date = calendar.date(byAdding: .day, value: -daysAgo, to: date)!
        }
        if hoursAgo > 0 {
            date = calendar.date(byAdding: .hour, value: -hoursAgo, to: date)!
        }

        let formatter = ISO8601DateFormatter()
        return formatter.string(from: date)
    }
}

// MARK: - JSON Helpers

struct JSONTestHelper {

    static func encode<T: Encodable>(_ value: T) throws -> Data {
        let encoder = JSONEncoder()
        encoder.outputFormatting = .prettyPrinted
        return try encoder.encode(value)
    }

    static func decode<T: Decodable>(_ type: T.Type, from json: String) throws -> T {
        guard let data = json.data(using: .utf8) else {
            throw NSError(domain: "JSONTestHelper", code: 1, userInfo: [NSLocalizedDescriptionKey: "Invalid JSON string"])
        }
        return try JSONDecoder().decode(type, from: data)
    }

    static func encodeToString<T: Encodable>(_ value: T) throws -> String {
        let data = try encode(value)
        guard let string = String(data: data, encoding: .utf8) else {
            throw NSError(domain: "JSONTestHelper", code: 2, userInfo: [NSLocalizedDescriptionKey: "Failed to convert to string"])
        }
        return string
    }
}

// MARK: - Async Test Helpers

struct AsyncTestHelper {

    static func withTimeout<T>(
        seconds: TimeInterval = 5.0,
        operation: @escaping () async throws -> T
    ) async throws -> T {
        try await withThrowingTaskGroup(of: T.self) { group in
            group.addTask {
                try await operation()
            }

            group.addTask {
                try await Task.sleep(nanoseconds: UInt64(seconds * 1_000_000_000))
                throw TimeoutError()
            }

            guard let result = try await group.next() else {
                throw TimeoutError()
            }

            group.cancelAll()
            return result
        }
    }

    struct TimeoutError: Error, CustomStringConvertible {
        var description: String { "Operation timed out" }
    }
}

// MARK: - URL Encoding Helpers

struct URLTestHelper {

    static func buildQueryURL(base: String, params: [String: String]) -> String? {
        guard var components = URLComponents(string: base) else {
            return nil
        }

        components.queryItems = params.map { key, value in
            URLQueryItem(name: key, value: value)
        }

        return components.url?.absoluteString
    }

    static func parseQueryParameters(from url: String) -> [String: String]? {
        guard let components = URLComponents(string: url),
              let queryItems = components.queryItems else {
            return nil
        }

        var params: [String: String] = [:]
        for item in queryItems {
            params[item.name] = item.value
        }
        return params
    }
}

// MARK: - Diff Test Helpers

struct DiffTestHelper {

    static func createUnifiedDiff(
        filePath: String = "test.txt",
        oldLines: [String],
        newLines: [String]
    ) -> String {
        var diff = ["--- a/\(filePath)", "+++ b/\(filePath)"]

        diff.append("@@ -1,\(oldLines.count) +1,\(newLines.count) @@")

        let maxLines = max(oldLines.count, newLines.count)
        for i in 0..<maxLines {
            if i < oldLines.count && i < newLines.count {
                if oldLines[i] != newLines[i] {
                    diff.append("-\(oldLines[i])")
                    diff.append("+\(newLines[i])")
                } else {
                    diff.append(" \(oldLines[i])")
                }
            } else if i < oldLines.count {
                diff.append("-\(oldLines[i])")
            } else {
                diff.append("+\(newLines[i])")
            }
        }

        return diff.joined(separator: "\n")
    }

    static func createAdditionDiff(lines: [String]) -> String {
        var diff = ["@@ -0,0 +1,\(lines.count) @@"]
        diff.append(contentsOf: lines.map { "+\($0)" })
        return diff.joined(separator: "\n")
    }

    static func createDeletionDiff(lines: [String]) -> String {
        var diff = ["@@ -1,\(lines.count) +0,0 @@"]
        diff.append(contentsOf: lines.map { "-\($0)" })
        return diff.joined(separator: "\n")
    }
}
