//
//  catnipTests.swift
//  catnipTests
//
//  Smoke tests and basic sanity checks
//

import Testing
@testable import Catnip

struct CatnipSmokeTests {

    @Test func testBasicModelCreation() {
        let workspace = MockDataFactory.createWorkspace()
        #expect(workspace.id == "test-ws-1")
        #expect(workspace.name == "test-workspace")
    }

    @Test func testDiffParserExists() {
        let diff = "@@ -1,1 +1,1 @@\n-old\n+new"
        let lines = DiffParser.parse(diff)
        #expect(lines.count > 0)
    }

    @Test func testAPIErrorTypes() {
        let error = APIError.invalidURL
        #expect(error.errorDescription != nil)
    }

    @Test func testClaudeActivityStates() {
        let states: [ClaudeActivityState] = [.active, .running, .inactive]
        #expect(states.count == 3)
    }

    @Test func testTodoStatuses() {
        let statuses: [TodoStatus] = [.pending, .inProgress, .completed]
        #expect(statuses.count == 3)
    }

    @Test func testWorkspaceDisplayName() {
        let workspace = MockDataFactory.createWorkspace(name: "test")
        #expect(workspace.displayName == "test")

        let unnamed = MockDataFactory.createWorkspace(name: "")
        #expect(unnamed.displayName == "Unnamed workspace")
    }

    @Test func testBranchCleaning() {
        let workspace = MockDataFactory.createWorkspace(branch: "refs/catnip/feature")
        #expect(workspace.cleanBranch == "feature")
    }

    @Test func testDiffStatsCalculation() {
        let diff = "+added\n-removed"
        let stats = DiffParser.calculateStats(diff)
        #expect(stats.additions == 1)
        #expect(stats.deletions == 1)
        #expect(stats.totalChanges == 2)
    }
}
