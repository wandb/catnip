//
//  WorkflowUITests.swift
//  catnipUITests
//
//  Full user workflow UI tests
//

import XCTest

final class WorkflowUITests: XCTestCase {

    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false

        app = XCUIApplication()

        // Configure app for UI testing
        app.launchArguments = [
            "-UITesting",
            "-SkipAuthentication",  // Skip OAuth flow
            "-UseMockData"          // Use mock API responses
        ]
    }

    override func tearDownWithError() throws {
        app = nil
    }

    // MARK: - Full Workflow Test

    @MainActor
    func testCompleteUserWorkflow() throws {
        // Launch app
        app.launch()

        // Wait for initial screen to load
        // Since we're using -SkipAuthentication, we should land on CodespaceView
        let accessButton = app.buttons["Access My Codespace"]
        XCTAssertTrue(accessButton.waitForExistence(timeout: 5), "Access My Codespace button should appear")

        // Test codespace connection (mock)
        accessButton.tap()

        // Should navigate to workspaces view after connection
        let workspacesTitle = app.navigationBars["Workspaces"]
        XCTAssertTrue(workspacesTitle.waitForExistence(timeout: 5), "Workspaces screen should appear")

        // Wait for workspaces list to load
        let workspacesList = app.descendants(matching: .any)["workspacesList"]
        XCTAssertTrue(workspacesList.waitForExistence(timeout: 5), "Workspaces list should load")

        // Test creating a new workspace
        let createButton = app.buttons["plus"]
        XCTAssertTrue(createButton.exists, "Create button should exist in toolbar")
        createButton.tap()

        // Should show create workspace sheet
        let createSheetTitle = app.navigationBars["New Workspace"]
        XCTAssertTrue(createSheetTitle.waitForExistence(timeout: 2), "Create workspace sheet should appear")

        // Select a repository
        let repositoryButton = app.buttons.matching(identifier: "repositoryChip").firstMatch
        if repositoryButton.exists {
            repositoryButton.tap()
        }

        // Enter a prompt
        let promptEditor = app.textViews.firstMatch
        if promptEditor.exists {
            promptEditor.tap()
            promptEditor.typeText("Fix the authentication bug")
        }

        // Submit (though we'll cancel for this test)
        let cancelButton = app.buttons["Cancel"]
        XCTAssertTrue(cancelButton.exists, "Cancel button should exist")
        cancelButton.tap()

        // Should be back on workspaces list
        XCTAssertTrue(workspacesTitle.exists, "Should return to workspaces list")

        // Test selecting an existing workspace from the list
        // Find the first workspace button
        let firstWorkspace = workspacesList.buttons.firstMatch
        if firstWorkspace.waitForExistence(timeout: 2) {
            firstWorkspace.tap()

            // Should navigate to workspace detail
            let detailView = app.navigationBars.element(boundBy: 0)
            XCTAssertTrue(detailView.waitForExistence(timeout: 3), "Workspace detail should appear")

            // Go back
            let backButton = app.navigationBars.buttons.firstMatch
            if backButton.exists {
                backButton.tap()
            }

            // Should be back on workspaces list
            XCTAssertTrue(workspacesTitle.waitForExistence(timeout: 2), "Should return to workspaces list")
        }

        // Test navigation back to codespace view
        let codespaceBackButton = app.navigationBars.buttons.firstMatch
        if codespaceBackButton.exists {
            codespaceBackButton.tap()

            // Should be back on codespace screen
            let codespaceScreen = app.staticTexts["Access your GitHub Codespaces"]
            XCTAssertTrue(codespaceScreen.waitForExistence(timeout: 2), "Should return to codespace screen")
        }

        // Test logout
        let logoutButton = app.buttons["logoutButton"]
        XCTAssertTrue(logoutButton.exists, "Logout button should exist in toolbar")
        logoutButton.tap()

        // Should navigate back to login screen
        let signInButton = app.buttons["Sign in with GitHub"]
        XCTAssertTrue(signInButton.waitForExistence(timeout: 3), "Should return to login screen after logout")
    }

    // MARK: - Individual Screen Tests

    @MainActor
    func testAuthenticationScreen() throws {
        // Launch without skip authentication
        app.launchArguments = ["-UITesting"]
        app.launch()

        // Verify auth screen elements
        let logoImage = app.images["logo"]
        XCTAssertTrue(logoImage.waitForExistence(timeout: 5), "Logo should appear")

        let catnipTitle = app.staticTexts["Catnip"]
        XCTAssertTrue(catnipTitle.exists, "App title should appear")

        let signInButton = app.buttons["Sign in with GitHub"]
        XCTAssertTrue(signInButton.exists, "Sign in button should exist")

        // Verify button is tappable (won't actually sign in during test)
        XCTAssertTrue(signInButton.isEnabled, "Sign in button should be enabled")
    }

    @MainActor
    func testCodespaceScreen() throws {
        app.launchArguments = ["-UITesting", "-SkipAuthentication"]
        app.launch()

        // Verify codespace screen elements
        let titleText = app.staticTexts["Access your GitHub Codespaces"]
        XCTAssertTrue(titleText.waitForExistence(timeout: 5), "Title should appear")

        let accessButton = app.buttons["Access My Codespace"]
        XCTAssertTrue(accessButton.exists, "Access button should exist")
        XCTAssertTrue(accessButton.isEnabled, "Access button should be enabled")

        // Verify org input field exists - use accessibility identifier
        let orgTextField = app.textFields["organizationTextField"]
        XCTAssertTrue(orgTextField.exists, "Organization text field should exist")

        // Test entering org name
        orgTextField.tap()
        orgTextField.typeText("wandb")

        // Wait for UI to update
        let goButton = app.buttons["goButton"]
        _ = goButton.waitForExistence(timeout: 2)

        XCTAssertTrue(goButton.exists, "Go button should exist")
        XCTAssertTrue(goButton.isEnabled, "Go button should be enabled when org is entered")

        // Verify logout button - use accessibility identifier
        let logoutButton = app.buttons["logoutButton"]
        XCTAssertTrue(logoutButton.exists, "Logout button should exist in toolbar")
    }

    @MainActor
    func testWorkspacesListNavigation() throws {
        app.launchArguments = ["-UITesting", "-SkipAuthentication", "-UseMockData", "-ShowWorkspacesList"]
        app.launch()

        // Wait for workspaces screen
        let workspacesTitle = app.navigationBars["Workspaces"]
        XCTAssertTrue(workspacesTitle.waitForExistence(timeout: 5), "Workspaces screen should appear")

        // Verify toolbar items
        let createButton = app.buttons["plus"]
        XCTAssertTrue(createButton.exists, "Create button should exist")

        // Test pull to refresh (if there's content)
        let scrollView = app.scrollViews.firstMatch
        if scrollView.exists {
            scrollView.swipeDown()
            // After refresh, list should still exist
            XCTAssertTrue(scrollView.exists, "List should exist after refresh")
        }
    }

    @MainActor
    func testCreateWorkspaceSheet() throws {
        app.launchArguments = ["-UITesting", "-SkipAuthentication", "-UseMockData", "-ShowWorkspacesList"]
        app.launch()

        // Navigate to create sheet
        let workspacesTitle = app.navigationBars["Workspaces"]
        XCTAssertTrue(workspacesTitle.waitForExistence(timeout: 5), "Workspaces screen should appear")

        let createButton = app.buttons["plus"]
        createButton.tap()

        // Verify sheet elements
        let sheetTitle = app.navigationBars["New Workspace"]
        XCTAssertTrue(sheetTitle.waitForExistence(timeout: 2), "Create sheet should appear")

        let promptEditor = app.textViews.firstMatch
        XCTAssertTrue(promptEditor.exists, "Prompt editor should exist")

        let cancelButton = app.buttons["Cancel"]
        XCTAssertTrue(cancelButton.exists, "Cancel button should exist")

        // Test typing in prompt
        promptEditor.tap()
        promptEditor.typeText("Add new feature")

        // Cancel and dismiss
        cancelButton.tap()

        // Should be back on list
        XCTAssertTrue(workspacesTitle.waitForExistence(timeout: 2), "Should return to workspaces list")
    }

    @MainActor
    func testWorkspaceDetailNavigation() throws {
        app.launchArguments = ["-UITesting", "-SkipAuthentication", "-UseMockData", "-ShowWorkspacesList"]
        app.launch()

        // Wait for workspaces list
        let workspacesTitle = app.navigationBars["Workspaces"]
        XCTAssertTrue(workspacesTitle.waitForExistence(timeout: 5), "Workspaces screen should appear")

        // Wait for the list to load and tap first workspace button
        let workspacesList = app.descendants(matching: .any)["workspacesList"]
        if workspacesList.waitForExistence(timeout: 3) {
            let firstWorkspace = workspacesList.buttons.firstMatch
            if firstWorkspace.waitForExistence(timeout: 2) {
                firstWorkspace.tap()

                // Should navigate to detail
                let detailNavigationBar = app.navigationBars.element(boundBy: 0)
                XCTAssertTrue(detailNavigationBar.waitForExistence(timeout: 3), "Detail view should appear")

                // Verify we can navigate back
                let backButton = detailNavigationBar.buttons.firstMatch
                if backButton.exists {
                    backButton.tap()
                    XCTAssertTrue(workspacesTitle.waitForExistence(timeout: 2), "Should navigate back to list")
                }
            }
        }
    }

    @MainActor
    func testEmptyWorkspacesState() throws {
        app.launchArguments = ["-UITesting", "-SkipAuthentication", "-UseMockData", "-EmptyWorkspaces", "-ShowWorkspacesList"]
        app.launch()

        // Wait for workspaces screen to appear
        let workspacesTitle = app.navigationBars["Workspaces"]
        XCTAssertTrue(workspacesTitle.waitForExistence(timeout: 5), "Workspaces screen should appear")

        // Should show empty state
        let emptyMessage = app.staticTexts["No workspaces"]
        XCTAssertTrue(emptyMessage.waitForExistence(timeout: 3), "Empty state message should appear")

        let createButton = app.buttons["Create Workspace"]
        XCTAssertTrue(createButton.exists, "Create workspace button should exist in empty state")

        // Tap to show create sheet
        createButton.tap()

        let sheetTitle = app.navigationBars["New Workspace"]
        XCTAssertTrue(sheetTitle.waitForExistence(timeout: 2), "Create sheet should appear")
    }

    // MARK: - Accessibility Tests

    @MainActor
    func testAccessibility() throws {
        app.launch()

        // Verify key UI elements are accessible
        let signInButton = app.buttons["Sign in with GitHub"]
        if signInButton.waitForExistence(timeout: 5) {
            XCTAssertTrue(signInButton.isHittable, "Sign in button should be accessible")
        }
    }

    // MARK: - Performance Tests

    @MainActor
    func testAppLaunchPerformance() throws {
        measure(metrics: [XCTApplicationLaunchMetric()]) {
            app.launch()
        }
    }
}
