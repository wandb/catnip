//
//  WorkspaceInfoTests.swift
//  catnipTests
//
//  Tests for WorkspaceInfo model logic
//

import Testing
import Foundation
@testable import Catnip

struct WorkspaceInfoTests {

    // MARK: - Display Name Tests

    @Test func testDisplayNameWithName() {
        let workspace = WorkspaceInfo(
            id: "test-id",
            name: "feature-branch",
            branch: "main",
            repoId: "repo123",
            claudeActivityState: nil,
            commitCount: nil,
            isDirty: nil,
            lastAccessed: nil,
            createdAt: nil,
            todos: nil,
            latestSessionTitle: nil,
            latestUserPrompt: nil,
            pullRequestUrl: nil,
            path: "/path/to/workspace",
            cacheStatus: nil
        )

        #expect(workspace.displayName == "feature-branch")
    }

    @Test func testDisplayNameWithEmptyName() {
        let workspace = WorkspaceInfo(
            id: "test-id",
            name: "",
            branch: "main",
            repoId: "repo123",
            claudeActivityState: nil,
            commitCount: nil,
            isDirty: nil,
            lastAccessed: nil,
            createdAt: nil,
            todos: nil,
            latestSessionTitle: nil,
            latestUserPrompt: nil,
            pullRequestUrl: nil,
            path: "/path/to/workspace",
            cacheStatus: nil
        )

        #expect(workspace.displayName == "Unnamed workspace")
    }

    // MARK: - Clean Branch Tests

    @Test func testCleanBranchWithCatnipPrefix() {
        let workspace = WorkspaceInfo(
            id: "test-id",
            name: "test",
            branch: "refs/catnip/feature-123",
            repoId: "repo123",
            claudeActivityState: nil,
            commitCount: nil,
            isDirty: nil,
            lastAccessed: nil,
            createdAt: nil,
            todos: nil,
            latestSessionTitle: nil,
            latestUserPrompt: nil,
            pullRequestUrl: nil,
            path: "/path/to/workspace",
            cacheStatus: nil
        )

        #expect(workspace.cleanBranch == "feature-123")
    }

    @Test func testCleanBranchWithLeadingSlash() {
        let workspace = WorkspaceInfo(
            id: "test-id",
            name: "test",
            branch: "/feature-abc",
            repoId: "repo123",
            claudeActivityState: nil,
            commitCount: nil,
            isDirty: nil,
            lastAccessed: nil,
            createdAt: nil,
            todos: nil,
            latestSessionTitle: nil,
            latestUserPrompt: nil,
            pullRequestUrl: nil,
            path: "/path/to/workspace",
            cacheStatus: nil
        )

        #expect(workspace.cleanBranch == "feature-abc")
    }

    @Test func testCleanBranchWithRegularBranch() {
        let workspace = WorkspaceInfo(
            id: "test-id",
            name: "test",
            branch: "feature/new-feature",
            repoId: "repo123",
            claudeActivityState: nil,
            commitCount: nil,
            isDirty: nil,
            lastAccessed: nil,
            createdAt: nil,
            todos: nil,
            latestSessionTitle: nil,
            latestUserPrompt: nil,
            pullRequestUrl: nil,
            path: "/path/to/workspace",
            cacheStatus: nil
        )

        #expect(workspace.cleanBranch == "feature/new-feature")
    }

    @Test func testCleanBranchWithEmptyBranch() {
        let workspace = WorkspaceInfo(
            id: "test-id",
            name: "test",
            branch: "",
            repoId: "repo123",
            claudeActivityState: nil,
            commitCount: nil,
            isDirty: nil,
            lastAccessed: nil,
            createdAt: nil,
            todos: nil,
            latestSessionTitle: nil,
            latestUserPrompt: nil,
            pullRequestUrl: nil,
            path: "/path/to/workspace",
            cacheStatus: nil
        )

        #expect(workspace.cleanBranch == "main")
    }

    @Test func testCleanBranchWithCatnipPrefixAndSlash() {
        let workspace = WorkspaceInfo(
            id: "test-id",
            name: "test",
            branch: "refs/catnip//test",
            repoId: "repo123",
            claudeActivityState: nil,
            commitCount: nil,
            isDirty: nil,
            lastAccessed: nil,
            createdAt: nil,
            todos: nil,
            latestSessionTitle: nil,
            latestUserPrompt: nil,
            pullRequestUrl: nil,
            path: "/path/to/workspace",
            cacheStatus: nil
        )

        #expect(workspace.cleanBranch == "test")
    }

    // MARK: - Status Text Tests

    @Test func testStatusTextActive() {
        let workspace = WorkspaceInfo(
            id: "test-id",
            name: "test",
            branch: "main",
            repoId: "repo123",
            claudeActivityState: .active,
            commitCount: nil,
            isDirty: nil,
            lastAccessed: nil,
            createdAt: nil,
            todos: nil,
            latestSessionTitle: nil,
            latestUserPrompt: nil,
            pullRequestUrl: nil,
            path: "/path/to/workspace",
            cacheStatus: nil
        )

        #expect(workspace.statusText == "Active now")
    }

    @Test func testStatusTextRunning() {
        let workspace = WorkspaceInfo(
            id: "test-id",
            name: "test",
            branch: "main",
            repoId: "repo123",
            claudeActivityState: .running,
            commitCount: nil,
            isDirty: nil,
            lastAccessed: nil,
            createdAt: nil,
            todos: nil,
            latestSessionTitle: nil,
            latestUserPrompt: nil,
            pullRequestUrl: nil,
            path: "/path/to/workspace",
            cacheStatus: nil
        )

        #expect(workspace.statusText == "Running")
    }

    @Test func testStatusTextInactive() {
        let workspace = WorkspaceInfo(
            id: "test-id",
            name: "test",
            branch: "main",
            repoId: "repo123",
            claudeActivityState: .inactive,
            commitCount: nil,
            isDirty: nil,
            lastAccessed: nil,
            createdAt: nil,
            todos: nil,
            latestSessionTitle: nil,
            latestUserPrompt: nil,
            pullRequestUrl: nil,
            path: "/path/to/workspace",
            cacheStatus: nil
        )

        #expect(workspace.statusText == "Inactive")
    }

    @Test func testStatusTextNil() {
        let workspace = WorkspaceInfo(
            id: "test-id",
            name: "test",
            branch: "main",
            repoId: "repo123",
            claudeActivityState: nil,
            commitCount: nil,
            isDirty: nil,
            lastAccessed: nil,
            createdAt: nil,
            todos: nil,
            latestSessionTitle: nil,
            latestUserPrompt: nil,
            pullRequestUrl: nil,
            path: "/path/to/workspace",
            cacheStatus: nil
        )

        #expect(workspace.statusText == "Inactive")
    }

    // MARK: - Time Display Tests

    @Test func testTimeDisplayToday() {
        let calendar = Calendar.current
        let now = Date()
        let twoHoursAgo = calendar.date(byAdding: .hour, value: -2, to: now)!

        let formatter = ISO8601DateFormatter()
        let isoString = formatter.string(from: twoHoursAgo)

        let workspace = WorkspaceInfo(
            id: "test-id",
            name: "test",
            branch: "main",
            repoId: "repo123",
            claudeActivityState: nil,
            commitCount: nil,
            isDirty: nil,
            lastAccessed: isoString,
            createdAt: nil,
            todos: nil,
            latestSessionTitle: nil,
            latestUserPrompt: nil,
            pullRequestUrl: nil,
            path: "/path/to/workspace",
            cacheStatus: nil
        )

        // Should display time of day
        #expect(!workspace.timeDisplay.isEmpty)
        #expect(!workspace.timeDisplay.contains("Yesterday"))
    }

    @Test func testTimeDisplayYesterday() {
        let calendar = Calendar.current
        let yesterday = calendar.date(byAdding: .day, value: -1, to: Date())!

        let formatter = ISO8601DateFormatter()
        let isoString = formatter.string(from: yesterday)

        let workspace = WorkspaceInfo(
            id: "test-id",
            name: "test",
            branch: "main",
            repoId: "repo123",
            claudeActivityState: nil,
            commitCount: nil,
            isDirty: nil,
            lastAccessed: isoString,
            createdAt: nil,
            todos: nil,
            latestSessionTitle: nil,
            latestUserPrompt: nil,
            pullRequestUrl: nil,
            path: "/path/to/workspace",
            cacheStatus: nil
        )

        #expect(workspace.timeDisplay == "Yesterday")
    }

    @Test func testTimeDisplayNoDate() {
        let workspace = WorkspaceInfo(
            id: "test-id",
            name: "test",
            branch: "main",
            repoId: "repo123",
            claudeActivityState: nil,
            commitCount: nil,
            isDirty: nil,
            lastAccessed: nil,
            createdAt: nil,
            todos: nil,
            latestSessionTitle: nil,
            latestUserPrompt: nil,
            pullRequestUrl: nil,
            path: "/path/to/workspace",
            cacheStatus: nil
        )

        #expect(workspace.timeDisplay.isEmpty)
    }

    // MARK: - Activity Description Tests

    @Test func testActivityDescriptionWithSessionTitle() {
        let workspace = WorkspaceInfo(
            id: "test-id",
            name: "test",
            branch: "main",
            repoId: "repo123",
            claudeActivityState: nil,
            commitCount: nil,
            isDirty: nil,
            lastAccessed: nil,
            createdAt: nil,
            todos: nil,
            latestSessionTitle: "Implementing new feature",
            latestUserPrompt: "Write some code",
            pullRequestUrl: nil,
            path: "/path/to/workspace",
            cacheStatus: nil
        )

        #expect(workspace.activityDescription == "Implementing new feature")
    }

    @Test func testActivityDescriptionWithPromptOnly() {
        let workspace = WorkspaceInfo(
            id: "test-id",
            name: "test",
            branch: "main",
            repoId: "repo123",
            claudeActivityState: nil,
            commitCount: nil,
            isDirty: nil,
            lastAccessed: nil,
            createdAt: nil,
            todos: nil,
            latestSessionTitle: nil,
            latestUserPrompt: "Fix the bug",
            pullRequestUrl: nil,
            path: "/path/to/workspace",
            cacheStatus: nil
        )

        #expect(workspace.activityDescription == "Fix the bug")
    }

    @Test func testActivityDescriptionWithEmptyTitle() {
        let workspace = WorkspaceInfo(
            id: "test-id",
            name: "test",
            branch: "main",
            repoId: "repo123",
            claudeActivityState: nil,
            commitCount: nil,
            isDirty: nil,
            lastAccessed: nil,
            createdAt: nil,
            todos: nil,
            latestSessionTitle: "",
            latestUserPrompt: "Do something",
            pullRequestUrl: nil,
            path: "/path/to/workspace",
            cacheStatus: nil
        )

        #expect(workspace.activityDescription == "Do something")
    }

    @Test func testActivityDescriptionWithNone() {
        let workspace = WorkspaceInfo(
            id: "test-id",
            name: "test",
            branch: "main",
            repoId: "repo123",
            claudeActivityState: nil,
            commitCount: nil,
            isDirty: nil,
            lastAccessed: nil,
            createdAt: nil,
            todos: nil,
            latestSessionTitle: nil,
            latestUserPrompt: nil,
            pullRequestUrl: nil,
            path: "/path/to/workspace",
            cacheStatus: nil
        )

        #expect(workspace.activityDescription == nil)
    }

    // MARK: - Codable Tests

    @Test func testDecodingWorkspaceInfo() throws {
        let json = """
        {
            "id": "ws-123",
            "name": "test-workspace",
            "branch": "feature/test",
            "repo_id": "repo-456",
            "claude_activity_state": "active",
            "commit_count": 5,
            "is_dirty": true,
            "last_accessed": "2025-10-06T12:00:00Z",
            "created_at": "2025-10-01T10:00:00Z",
            "path": "/workspaces/test",
            "latest_session_title": "Building feature",
            "latest_user_prompt": "Add new functionality",
            "pull_request_url": "https://github.com/org/repo/pull/123"
        }
        """

        let data = json.data(using: .utf8)!
        let workspace = try JSONDecoder().decode(WorkspaceInfo.self, from: data)

        #expect(workspace.id == "ws-123")
        #expect(workspace.name == "test-workspace")
        #expect(workspace.branch == "feature/test")
        #expect(workspace.repoId == "repo-456")
        #expect(workspace.claudeActivityState == .active)
        #expect(workspace.commitCount == 5)
        #expect(workspace.isDirty == true)
        #expect(workspace.lastAccessed == "2025-10-06T12:00:00Z")
        #expect(workspace.createdAt == "2025-10-01T10:00:00Z")
        #expect(workspace.path == "/workspaces/test")
        #expect(workspace.latestSessionTitle == "Building feature")
        #expect(workspace.latestUserPrompt == "Add new functionality")
        #expect(workspace.pullRequestUrl == "https://github.com/org/repo/pull/123")
    }

    @Test func testDecodingWorkspaceInfoMinimal() throws {
        let json = """
        {
            "id": "ws-123",
            "name": "test-workspace",
            "branch": "main",
            "repo_id": "repo-456",
            "path": "/workspaces/test"
        }
        """

        let data = json.data(using: .utf8)!
        let workspace = try JSONDecoder().decode(WorkspaceInfo.self, from: data)

        #expect(workspace.id == "ws-123")
        #expect(workspace.name == "test-workspace")
        #expect(workspace.claudeActivityState == nil)
        #expect(workspace.commitCount == nil)
        #expect(workspace.isDirty == nil)
        #expect(workspace.todos == nil)
    }
}
