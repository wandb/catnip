//
//  FastUserJourneyTests.swift
//  catnipUITests
//
//  Optimized UI tests with faster execution
//

import XCTest

final class FastUserJourneyTests: XCTestCase {

    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()

        // Performance optimizations
        app.launchArguments = [
            "-UITesting",
            "-DisableAnimations",     // Disable animations for faster tests
            "-SkipAuthentication",    // Skip OAuth flow
            "-UseMockData"            // Use mock data, no network calls
        ]
    }

    override func tearDownWithError() throws {
        app = nil
    }

    // MARK: - Fast Smoke Tests

    @MainActor
    func testQuickAuthScreen() throws {
        // Launch without skip auth to test login screen
        app.launchArguments = ["-UITesting", "-DisableAnimations"]
        app.launch()

        let signInButton = app.buttons["Sign in with GitHub"]
        XCTAssertTrue(signInButton.waitForExistence(timeout: 3), "Sign in should appear")
    }

    @MainActor
    func testQuickCodespaceAccess() throws {
        app.launch()

        let accessButton = app.buttons["Access My Codespace"]
        XCTAssertTrue(accessButton.waitForExistence(timeout: 3), "Access button should appear")

        accessButton.tap()

        // Should navigate to workspaces
        let workspacesNav = app.navigationBars["Workspaces"]
        XCTAssertTrue(workspacesNav.waitForExistence(timeout: 3), "Should navigate to workspaces")
    }

    @MainActor
    func testQuickWorkspaceNavigation() throws {
        app.launchArguments.append("-ShowWorkspacesList")
        app.launch()

        let workspacesNav = app.navigationBars["Workspaces"]
        XCTAssertTrue(workspacesNav.waitForExistence(timeout: 3), "Workspaces should appear")

        // Verify at least one workspace exists
        let firstCell = app.cells.firstMatch
        XCTAssertTrue(firstCell.waitForExistence(timeout: 2), "Should have workspaces")

        // Tap first workspace
        firstCell.tap()

        // Verify detail appears
        let detailNav = app.navigationBars.element(boundBy: 0)
        XCTAssertTrue(detailNav.waitForExistence(timeout: 2), "Detail should appear")
    }

    @MainActor
    func testQuickCreateWorkspace() throws {
        app.launchArguments.append("-ShowWorkspacesList")
        app.launch()

        let workspacesNav = app.navigationBars["Workspaces"]
        XCTAssertTrue(workspacesNav.waitForExistence(timeout: 3))

        // Tap create
        let createButton = app.buttons["plus"]
        createButton.tap()

        // Verify sheet appears
        let sheetNav = app.navigationBars["New Workspace"]
        XCTAssertTrue(sheetNav.waitForExistence(timeout: 1), "Create sheet should appear")

        // Cancel
        app.buttons["Cancel"].tap()

        // Verify back on list
        XCTAssertTrue(workspacesNav.waitForExistence(timeout: 1))
    }

    @MainActor
    func testQuickLogout() throws {
        app.launch()

        let logoutButton = app.buttons["Logout"]
        XCTAssertTrue(logoutButton.waitForExistence(timeout: 2))

        logoutButton.tap()

        // Should return to auth
        let signInButton = app.buttons["Sign in with GitHub"]
        XCTAssertTrue(signInButton.waitForExistence(timeout: 2), "Should return to auth")
    }

    // MARK: - Minimal E2E Test

    @MainActor
    func testMinimalEndToEnd() throws {
        // This is the fastest possible end-to-end test
        app.launch()

        // 1. Verify codespace screen
        XCTAssertTrue(app.buttons["Access My Codespace"].exists)

        // 2. Connect
        app.buttons["Access My Codespace"].tap()

        // 3. Verify workspaces
        XCTAssertTrue(app.navigationBars["Workspaces"].waitForExistence(timeout: 3))

        // 4. Navigate back to codespace
        let backButton = app.navigationBars.buttons.firstMatch
        if backButton.exists {
            backButton.tap()
        }

        // 5. Logout
        let logoutButton = app.buttons["Logout"]
        XCTAssertTrue(logoutButton.waitForExistence(timeout: 2), "Logout button should exist")
        logoutButton.tap()

        // 6. Verify back at auth
        XCTAssertTrue(app.buttons["Sign in with GitHub"].waitForExistence(timeout: 2))
    }
}
