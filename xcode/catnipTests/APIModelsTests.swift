//
//  APIModelsTests.swift
//  catnipTests
//
//  Tests for API models and error handling
//

import Testing
import Foundation
@testable import catnip

struct APIModelsTests {

    // MARK: - ClaudeSessionData Tests

    @Test func testDecodeClaudeSessionData() throws {
        let json = """
        {
            "turnCount": 5,
            "isActive": true
        }
        """

        let data = json.data(using: .utf8)!
        let session = try JSONDecoder().decode(ClaudeSessionData.self, from: data)

        #expect(session.turnCount == 5)
        #expect(session.isActive == true)
    }

    @Test func testDecodeClaudeSessionsResponse() throws {
        let json = """
        {
            "workspace-1": {
                "turnCount": 3,
                "isActive": false
            },
            "workspace-2": {
                "turnCount": 10,
                "isActive": true
            }
        }
        """

        let data = json.data(using: .utf8)!
        let sessions = try JSONDecoder().decode([String: ClaudeSessionData].self, from: data)

        #expect(sessions.count == 2)
        #expect(sessions["workspace-1"]?.turnCount == 3)
        #expect(sessions["workspace-1"]?.isActive == false)
        #expect(sessions["workspace-2"]?.turnCount == 10)
        #expect(sessions["workspace-2"]?.isActive == true)
    }

    // MARK: - LatestMessageResponse Tests

    @Test func testDecodeLatestMessageResponse() throws {
        let json = """
        {
            "content": "Hello, this is a message",
            "isError": false
        }
        """

        let data = json.data(using: .utf8)!
        let message = try JSONDecoder().decode(LatestMessageResponse.self, from: data)

        #expect(message.content == "Hello, this is a message")
        #expect(message.isError == false)
    }

    @Test func testDecodeLatestMessageResponseWithError() throws {
        let json = """
        {
            "content": "An error occurred",
            "isError": true
        }
        """

        let data = json.data(using: .utf8)!
        let message = try JSONDecoder().decode(LatestMessageResponse.self, from: data)

        #expect(message.content == "An error occurred")
        #expect(message.isError == true)
    }

    // MARK: - CheckoutResponse Tests

    @Test func testDecodeCheckoutResponse() throws {
        let json = """
        {
            "worktree": {
                "id": "wt-123",
                "name": "feature-branch",
                "branch": "feature/test",
                "repo_id": "repo-456",
                "path": "/path/to/workspace"
            }
        }
        """

        let data = json.data(using: .utf8)!
        let response = try JSONDecoder().decode(CheckoutResponse.self, from: data)

        #expect(response.worktree.id == "wt-123")
        #expect(response.worktree.name == "feature-branch")
        #expect(response.worktree.branch == "feature/test")
    }

    // MARK: - WorktreeDiffResponse Tests

    @Test func testDecodeWorktreeDiffResponse() throws {
        let json = """
        {
            "summary": "3 files changed, 25 insertions(+), 8 deletions(-)",
            "file_diffs": [
                {
                    "file_path": "src/file.ts",
                    "change_type": "modified",
                    "old_content": null,
                    "new_content": null,
                    "diff_text": "@@ -1,2 +1,2 @@",
                    "is_expanded": true
                }
            ],
            "total_files": 1,
            "worktree_id": "wt-123",
            "worktree_name": "feature/test",
            "source_branch": "main",
            "fork_commit": "abc123"
        }
        """

        let data = json.data(using: .utf8)!
        let response = try JSONDecoder().decode(WorktreeDiffResponse.self, from: data)

        #expect(response.summary == "3 files changed, 25 insertions(+), 8 deletions(-)")
        #expect(response.fileDiffs.count == 1)
        #expect(response.totalFiles == 1)
        #expect(response.worktreeId == "wt-123")
        #expect(response.worktreeName == "feature/test")
        #expect(response.sourceBranch == "main")
        #expect(response.forkCommit == "abc123")
    }

    @Test func testDecodeFileDiff() throws {
        let json = """
        {
            "file_path": "README.md",
            "change_type": "added",
            "old_content": null,
            "new_content": "# New File",
            "diff_text": null,
            "is_expanded": false
        }
        """

        let data = json.data(using: .utf8)!
        let fileDiff = try JSONDecoder().decode(FileDiff.self, from: data)

        #expect(fileDiff.filePath == "README.md")
        #expect(fileDiff.changeType == "added")
        #expect(fileDiff.oldContent == nil)
        #expect(fileDiff.newContent == "# New File")
        #expect(fileDiff.diffText == nil)
        #expect(fileDiff.isExpanded == false)
    }

    // MARK: - APIError Tests

    @Test func testAPIErrorInvalidURL() {
        let error = APIError.invalidURL
        #expect(error.errorDescription == "Invalid URL")
    }

    @Test func testAPIErrorNoSessionToken() {
        let error = APIError.noSessionToken
        #expect(error.errorDescription == "No session token available")
    }

    @Test func testAPIErrorNetworkError() {
        let underlyingError = NSError(domain: "TestError", code: 500, userInfo: [NSLocalizedDescriptionKey: "Connection failed"])
        let error = APIError.networkError(underlyingError)

        #expect(error.errorDescription?.contains("Network error") == true)
        #expect(error.errorDescription?.contains("Connection failed") == true)
    }

    @Test func testAPIErrorServerError() {
        let error = APIError.serverError(404, "Not Found")
        #expect(error.errorDescription == "Server error 404: Not Found")
    }

    @Test func testAPIErrorDecodingError() {
        let underlyingError = NSError(domain: "DecodingError", code: 1, userInfo: [NSLocalizedDescriptionKey: "Invalid JSON"])
        let error = APIError.decodingError(underlyingError)

        #expect(error.errorDescription?.contains("Decoding error") == true)
        #expect(error.errorDescription?.contains("Invalid JSON") == true)
    }

    @Test func testAPIErrorSSEConnectionFailed() {
        let error = APIError.sseConnectionFailed("Timeout")
        #expect(error.errorDescription == "SSE connection failed: Timeout")
    }

    // MARK: - Todo Model Tests

    @Test func testDecodeTodo() throws {
        let json = """
        {
            "content": "Fix the bug",
            "status": "pending",
            "activeForm": "Fixing the bug"
        }
        """

        let data = json.data(using: .utf8)!
        let todo = try JSONDecoder().decode(Todo.self, from: data)

        #expect(todo.content == "Fix the bug")
        #expect(todo.status == .pending)
        #expect(todo.activeForm == "Fixing the bug")
    }

    @Test func testDecodeTodoWithInProgressStatus() throws {
        let json = """
        {
            "content": "Build feature",
            "status": "in_progress",
            "activeForm": null
        }
        """

        let data = json.data(using: .utf8)!
        let todo = try JSONDecoder().decode(Todo.self, from: data)

        #expect(todo.content == "Build feature")
        #expect(todo.status == .inProgress)
        #expect(todo.activeForm == nil)
    }

    @Test func testDecodeTodoWithCompletedStatus() throws {
        let json = """
        {
            "content": "Write tests",
            "status": "completed",
            "activeForm": null
        }
        """

        let data = json.data(using: .utf8)!
        let todo = try JSONDecoder().decode(Todo.self, from: data)

        #expect(todo.content == "Write tests")
        #expect(todo.status == .completed)
    }

    // MARK: - FileDiffStats Tests

    @Test func testFileDiffStatsTotalChanges() {
        let stats = FileDiffStats(additions: 10, deletions: 5)
        #expect(stats.totalChanges == 15)
    }

    @Test func testFileDiffStatsNoChanges() {
        let stats = FileDiffStats(additions: 0, deletions: 0)
        #expect(stats.totalChanges == 0)
    }

    @Test func testFileDiffStatsOnlyAdditions() {
        let stats = FileDiffStats(additions: 20, deletions: 0)
        #expect(stats.totalChanges == 20)
    }

    @Test func testFileDiffStatsOnlyDeletions() {
        let stats = FileDiffStats(additions: 0, deletions: 15)
        #expect(stats.totalChanges == 15)
    }

    // MARK: - ClaudeActivityState Tests

    @Test func testDecodeClaudeActivityState() throws {
        let activeJson = "\"active\"".data(using: .utf8)!
        let active = try JSONDecoder().decode(ClaudeActivityState.self, from: activeJson)
        #expect(active == .active)

        let runningJson = "\"running\"".data(using: .utf8)!
        let running = try JSONDecoder().decode(ClaudeActivityState.self, from: runningJson)
        #expect(running == .running)

        let inactiveJson = "\"inactive\"".data(using: .utf8)!
        let inactive = try JSONDecoder().decode(ClaudeActivityState.self, from: inactiveJson)
        #expect(inactive == .inactive)
    }
}
