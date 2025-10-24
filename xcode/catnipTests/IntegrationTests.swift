//
//  IntegrationTests.swift
//  catnipTests
//
//  Integration and end-to-end scenario tests
//

import Testing
import Foundation
@testable import catnip

struct IntegrationTests {

    // MARK: - Workspace Workflow Tests

    @Test func testWorkspaceListingAndFiltering() throws {
        let workspaces = [
            MockDataFactory.createActiveWorkspace(),
            MockDataFactory.createInactiveWorkspace(),
            MockDataFactory.createWorkspace(
                id: "ws-3",
                name: "another-workspace",
                claudeActivityState: .running
            )
        ]

        // Filter active workspaces
        let activeWorkspaces = workspaces.filter { $0.claudeActivityState == .active }
        #expect(activeWorkspaces.count == 1)
        #expect(activeWorkspaces.first?.id == "active-ws")

        // Filter by dirty state
        let dirtyWorkspaces = workspaces.filter { $0.isDirty == true }
        #expect(dirtyWorkspaces.count == 1)

        // Sort by name
        let sorted = workspaces.sorted { $0.name < $1.name }
        #expect(sorted.first?.name == "another-workspace")
    }

    @Test func testWorkspaceWithTodoList() {
        let todos = MockDataFactory.createTodoList()
        let workspace = MockDataFactory.createWorkspace(todos: todos)

        #expect(workspace.todos?.count == 3)

        // Check todo statuses
        let completed = workspace.todos?.filter { $0.status == .completed }
        let inProgress = workspace.todos?.filter { $0.status == .inProgress }
        let pending = workspace.todos?.filter { $0.status == .pending }

        #expect(completed?.count == 1)
        #expect(inProgress?.count == 1)
        #expect(pending?.count == 1)
    }

    // MARK: - Diff Workflow Tests

    @Test func testFullDiffWorkflow() {
        let diffResponse = MockDataFactory.createWorktreeDiffResponse(fileCount: 5)

        #expect(diffResponse.totalFiles == 5)
        #expect(diffResponse.fileDiffs.count == 5)

        // Parse each diff
        for fileDiff in diffResponse.fileDiffs {
            if let diffText = fileDiff.diffText {
                let lines = DiffParser.parse(diffText)
                #expect(lines.count > 0)

                let stats = DiffParser.calculateStats(diffText)
                #expect(stats.additions >= 0)
                #expect(stats.deletions >= 0)
            }
        }
    }

    @Test func testDiffStatsAggregation() {
        let diff1 = DiffTestHelper.createAdditionDiff(lines: ["line1", "line2"])
        let diff2 = DiffTestHelper.createDeletionDiff(lines: ["old1"])
        let diff3 = MockDataFactory.createSampleDiff()

        let stats1 = DiffParser.calculateStats(diff1)
        let stats2 = DiffParser.calculateStats(diff2)
        let stats3 = DiffParser.calculateStats(diff3)

        let totalAdditions = stats1.additions + stats2.additions + stats3.additions
        let totalDeletions = stats1.deletions + stats2.deletions + stats3.deletions

        #expect(totalAdditions >= 2) // At least from diff1
        #expect(totalDeletions >= 1) // At least from diff2
    }

    // MARK: - JSON Round-Trip Tests

    @Test func testWorkspaceJSONRoundTrip() throws {
        let original = MockDataFactory.createActiveWorkspace()

        // Encode to JSON
        let jsonData = try JSONTestHelper.encode(original)

        // Decode back
        let decoded = try JSONDecoder().decode(WorkspaceInfo.self, from: jsonData)

        // Verify key fields
        #expect(decoded.id == original.id)
        #expect(decoded.name == original.name)
        #expect(decoded.branch == original.branch)
        #expect(decoded.claudeActivityState == original.claudeActivityState)
        #expect(decoded.latestSessionTitle == original.latestSessionTitle)
    }

    @Test func testFileDiffJSONRoundTrip() throws {
        let original = MockDataFactory.createFileDiff()

        let jsonData = try JSONTestHelper.encode(original)
        let decoded = try JSONDecoder().decode(FileDiff.self, from: jsonData)

        #expect(decoded.filePath == original.filePath)
        #expect(decoded.changeType == original.changeType)
        #expect(decoded.isExpanded == original.isExpanded)
    }

    @Test func testTodoJSONRoundTrip() throws {
        let original = MockDataFactory.createTodo(
            content: "Test task",
            status: .inProgress,
            activeForm: "Testing"
        )

        let jsonData = try JSONTestHelper.encode(original)
        let decoded = try JSONDecoder().decode(Todo.self, from: jsonData)

        #expect(decoded.content == original.content)
        #expect(decoded.status == original.status)
        #expect(decoded.activeForm == original.activeForm)
    }

    // MARK: - Date and Time Tests

    @Test func testWorkspaceTimeDisplayWithVariousDates() {
        // Today
        let todayWorkspace = MockDataFactory.createWorkspace(
            lastAccessed: MockDataFactory.createISO8601Date(daysAgo: 0, hoursAgo: 2)
        )
        #expect(!todayWorkspace.timeDisplay.isEmpty)

        // Yesterday
        let yesterdayWorkspace = MockDataFactory.createWorkspace(
            lastAccessed: MockDataFactory.createISO8601Date(daysAgo: 1)
        )
        #expect(yesterdayWorkspace.timeDisplay == "Yesterday")

        // This week
        let thisWeekWorkspace = MockDataFactory.createWorkspace(
            lastAccessed: MockDataFactory.createISO8601Date(daysAgo: 3)
        )
        #expect(!thisWeekWorkspace.timeDisplay.isEmpty)
        #expect(thisWeekWorkspace.timeDisplay != "Yesterday")
    }

    // MARK: - Branch Cleaning Edge Cases

    @Test func testBranchCleaningEdgeCases() {
        let testCases: [(input: String, expected: String)] = [
            ("refs/catnip/feature", "feature"),
            ("/feature", "feature"),
            ("refs/catnip//nested", "nested"),
            ("", "main"),
            ("main", "main"),
            ("feature/test/nested", "feature/test/nested"),
            ("refs/heads/main", "refs/heads/main"), // Should not modify refs/heads
        ]

        for (input, expected) in testCases {
            let workspace = MockDataFactory.createWorkspace(branch: input)
            #expect(workspace.cleanBranch == expected)
        }
    }

    // MARK: - Activity Description Priority Tests

    @Test func testActivityDescriptionPriority() {
        // Session title takes priority
        let ws1 = MockDataFactory.createWorkspace(
            latestSessionTitle: "Session Title",
            latestUserPrompt: "User Prompt"
        )
        #expect(ws1.activityDescription == "Session Title")

        // Falls back to prompt if no title
        let ws2 = MockDataFactory.createWorkspace(
            latestSessionTitle: nil,
            latestUserPrompt: "User Prompt"
        )
        #expect(ws2.activityDescription == "User Prompt")

        // Empty title falls back to prompt
        let ws3 = MockDataFactory.createWorkspace(
            latestSessionTitle: "",
            latestUserPrompt: "User Prompt"
        )
        #expect(ws3.activityDescription == "User Prompt")

        // Empty strings are treated as no description
        let ws4 = MockDataFactory.createWorkspace(
            latestSessionTitle: "",
            latestUserPrompt: ""
        )
        #expect(ws4.activityDescription == nil)
    }

    // MARK: - Complex Diff Scenarios

    @Test func testMultiHunkDiffParsing() {
        let complexDiff = """
        @@ -1,3 +1,3 @@
         import Foundation
        -import OldLib
        +import NewLib

        @@ -50,5 +50,7 @@
         func process() {
        +    // New comment
             let value = 42
        +    print(value)
             return value
         }

        @@ -100,2 +102,2 @@
        -// Old comment
        +// New comment
         }
        """

        let lines = DiffParser.parse(complexDiff)
        let stats = DiffParser.calculateStats(complexDiff)

        // Should have parsed all hunks
        let headers = lines.filter { $0.type == .header }
        #expect(headers.count == 3)

        // Should count all changes
        #expect(stats.additions > 0)
        #expect(stats.deletions > 0)
    }

    @Test func testDiffWithBinaryChanges() {
        let diff = """
        @@ -1,2 +1,2 @@
         # Regular text
        -old binary content
        +new binary content
        """

        let lines = DiffParser.parse(diff)
        let stats = DiffParser.calculateStats(diff)

        #expect(lines.count > 0)
        #expect(stats.additions == 1)
        #expect(stats.deletions == 1)
    }

    // MARK: - URL Encoding Tests

    @Test func testWorkspacePathEncoding() {
        let testPaths = [
            "/simple/path",
            "/path with spaces/file",
            "/path/with/ünïcödé",
            "/path/with/special!@#$%"
        ]

        for path in testPaths {
            let workspace = MockDataFactory.createWorkspace(path: path)
            #expect(workspace.path == path)

            // Test URL encoding
            let encoded = path.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed)
            #expect(encoded != nil)
        }
    }

    // MARK: - Hashable and Identifiable Tests

    @Test func testWorkspaceHashable() {
        let ws1 = MockDataFactory.createWorkspace(id: "ws-1", name: "workspace1")
        let ws2 = MockDataFactory.createWorkspace(id: "ws-1", name: "workspace1")
        let ws3 = MockDataFactory.createWorkspace(id: "ws-2", name: "workspace2")

        #expect(ws1.id == ws2.id)
        #expect(ws1.id != ws3.id)

        // Test in Set
        let set: Set<WorkspaceInfo> = [ws1, ws2, ws3]
        #expect(set.count == 2) // ws1 and ws2 have same id
    }

    @Test func testTodoHashable() {
        let todo1 = MockDataFactory.createTodo(content: "Task 1")
        let todo2 = MockDataFactory.createTodo(content: "Task 2")

        // Each todo has unique UUID
        #expect(todo1.id != todo2.id)

        // Can be used in Set
        let set: Set<Todo> = [todo1, todo2]
        #expect(set.count == 2)
    }

    // MARK: - Error Description Tests

    @Test func testAllAPIErrorDescriptions() {
        let errors: [APIError] = [
            .invalidURL,
            .noSessionToken,
            .networkError(NSError(domain: "test", code: 1)),
            .decodingError(NSError(domain: "test", code: 2)),
            .serverError(404, "Not Found"),
            .timeout
        ]

        for error in errors {
            #expect(error.errorDescription != nil)
            #expect(!error.errorDescription!.isEmpty)
        }
    }
}
