//
//  UserJourneyTests.swift
//  catnipUITests
//
//  Clean UI tests using Page Object pattern
//

import XCTest

final class UserJourneyTests: XCTestCase {

    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()

        // Speed up tests by disabling animations
        app.launchArguments.append("-DisableAnimations")
    }

    override func tearDownWithError() throws {
        app = nil
    }

    // MARK: - Complete User Journey

    @MainActor
    func testCompleteUserJourney() throws {
        // Configure app for testing with mock data
        app.launchArguments = ["-UITesting", "-SkipAuthentication", "-UseMockData"]
        app.launch()

        // Step 1: Verify we're on codespace screen (since we skipped auth)
        let codespacePage = CodespacePage(app: app)
        XCTAssertTrue(codespacePage.isDisplayed(), "Should start on codespace screen")

        // Step 2: Connect to codespace (mock)
        codespacePage.tapAccessMyCodespace()

        // Step 3: Should navigate to workspaces list
        let workspacesPage = WorkspacesListPage(app: app)
        XCTAssertTrue(workspacesPage.waitForConnection(timeout: 5), "Should navigate to workspaces")
        XCTAssertTrue(workspacesPage.isDisplayed(), "Workspaces list should be displayed")

        // Step 4: Verify we have workspaces
        let workspaceCount = workspacesPage.numberOfWorkspaces()
        XCTAssertGreaterThan(workspaceCount, 0, "Should have mock workspaces")

        // Step 5: Open create workspace sheet
        workspacesPage.tapCreate()

        let createSheet = CreateWorkspaceSheet(app: app)
        XCTAssertTrue(createSheet.isDisplayed(), "Create sheet should appear")

        // Step 6: Enter a prompt and cancel
        createSheet.enterPrompt("Fix authentication bug")
        createSheet.tapCancel()

        // Should be back on workspaces list
        XCTAssertTrue(workspacesPage.isDisplayed(), "Should return to workspaces list")

        // Step 7: Select first workspace
        workspacesPage.tapWorkspace(at: 0)

        let detailPage = WorkspaceDetailPage(app: app)
        XCTAssertTrue(detailPage.isDisplayed(), "Detail page should appear")

        // Step 8: Navigate back to workspaces
        detailPage.tapBack()
        XCTAssertTrue(workspacesPage.isDisplayed(), "Should navigate back to workspaces")

        // Step 9: Navigate back to codespace screen
        let backButton = app.navigationBars.buttons.firstMatch
        if backButton.exists {
            backButton.tap()
        }

        XCTAssertTrue(codespacePage.isDisplayed(), "Should navigate back to codespace screen")

        // Step 10: Logout
        codespacePage.tapLogout()

        let authPage = AuthPage(app: app)
        XCTAssertTrue(authPage.isDisplayed(), "Should navigate to auth screen after logout")
    }

    // MARK: - Individual Journeys

    @MainActor
    func testAuthenticationFlow() throws {
        // Launch without skip authentication
        app.launchArguments = ["-UITesting"]
        app.launch()

        let authPage = AuthPage(app: app)
        XCTAssertTrue(authPage.isDisplayed(), "Auth page should appear")

        // Verify all elements
        XCTAssertTrue(authPage.logoImage.exists, "Logo should exist")
        XCTAssertTrue(authPage.catnipTitle.exists, "Title should exist")
        XCTAssertTrue(authPage.signInButton.exists, "Sign in button should exist")
        XCTAssertTrue(authPage.signInButton.isEnabled, "Sign in button should be enabled")
    }

    @MainActor
    func testCodespaceConnectionFlow() throws {
        app.launchArguments = ["-UITesting", "-SkipAuthentication"]
        app.launch()

        let codespacePage = CodespacePage(app: app)
        XCTAssertTrue(codespacePage.isDisplayed(), "Codespace page should appear")

        // Verify main access button exists
        XCTAssertTrue(codespacePage.accessButton.exists, "Access button should exist")
        XCTAssertTrue(codespacePage.accessButton.isEnabled, "Access button should be enabled")

        // Verify more options menu exists (contains logout)
        XCTAssertTrue(codespacePage.moreOptionsButton.exists, "More options button should exist")
    }

    @MainActor
    func testWorkspacesListFlow() throws {
        app.launchArguments = ["-UITesting", "-SkipAuthentication", "-UseMockData", "-ShowWorkspacesList"]
        app.launch()

        let workspacesPage = WorkspacesListPage(app: app)
        XCTAssertTrue(workspacesPage.isDisplayed(), "Workspaces page should appear")

        // Wait for the list to appear
        let list = app.descendants(matching: .any)["workspacesList"]
        XCTAssertTrue(list.waitForExistence(timeout: 5), "Workspaces list should appear")

        // Wait for the first workspace button to appear (NavigationLinks are exposed as buttons)
        let firstButton = list.buttons.firstMatch
        XCTAssertTrue(firstButton.waitForExistence(timeout: 3), "First workspace should appear")

        // Verify we have workspaces
        XCTAssertGreaterThan(workspacesPage.numberOfWorkspaces(), 0, "Should have workspaces")

        // Test pull to refresh
        workspacesPage.pullToRefresh()
        XCTAssertTrue(workspacesPage.isDisplayed(), "List should still be displayed after refresh")
    }

    @MainActor
    func testEmptyWorkspacesFlow() throws {
        app.launchArguments = ["-UITesting", "-SkipAuthentication", "-UseMockData", "-EmptyWorkspaces", "-ShowWorkspacesList"]
        app.launch()

        let workspacesPage = WorkspacesListPage(app: app)
        XCTAssertTrue(workspacesPage.isDisplayed(), "Workspaces page should appear")

        // Should show empty state
        XCTAssertTrue(workspacesPage.isEmptyStateDisplayed(), "Empty state should be displayed")

        // Tap create from empty state
        workspacesPage.tapEmptyStateCreate()

        let createSheet = CreateWorkspaceSheet(app: app)
        XCTAssertTrue(createSheet.isDisplayed(), "Create sheet should appear")
    }

    @MainActor
    func testWorkspaceDetailFlow() throws {
        app.launchArguments = ["-UITesting", "-SkipAuthentication", "-UseMockData", "-ShowWorkspacesList"]
        app.launch()

        let workspacesPage = WorkspacesListPage(app: app)
        XCTAssertTrue(workspacesPage.isDisplayed(), "Workspaces page should appear")

        // Select first workspace
        workspacesPage.tapWorkspace(at: 0)

        let detailPage = WorkspaceDetailPage(app: app)
        XCTAssertTrue(detailPage.isDisplayed(), "Detail page should appear")

        // Navigate back
        detailPage.tapBack()
        XCTAssertTrue(workspacesPage.isDisplayed(), "Should navigate back")
    }

    @MainActor
    func testCreateWorkspaceFlow() throws {
        app.launchArguments = ["-UITesting", "-SkipAuthentication", "-UseMockData", "-ShowWorkspacesList"]
        app.launch()

        let workspacesPage = WorkspacesListPage(app: app)
        XCTAssertTrue(workspacesPage.isDisplayed(), "Workspaces page should appear")

        // Open create sheet
        workspacesPage.tapCreate()

        let createSheet = CreateWorkspaceSheet(app: app)
        XCTAssertTrue(createSheet.isDisplayed(), "Create sheet should appear")

        // Enter prompt
        createSheet.enterPrompt("Implement new feature")

        // Cancel
        createSheet.tapCancel()

        // Verify we're back on list
        XCTAssertTrue(workspacesPage.isDisplayed(), "Should be back on workspaces list")
    }

    @MainActor
    func testLogoutFlow() throws {
        app.launchArguments = ["-UITesting", "-SkipAuthentication"]
        app.launch()

        let codespacePage = CodespacePage(app: app)
        XCTAssertTrue(codespacePage.isDisplayed(), "Codespace page should appear")

        // Logout
        codespacePage.tapLogout()

        // Should navigate to auth screen
        let authPage = AuthPage(app: app)
        XCTAssertTrue(authPage.waitForElement(authPage.signInButton, timeout: 3), "Should navigate to auth screen")
    }

    @MainActor
    func testNavigationHierarchy() throws {
        app.launchArguments = ["-UITesting", "-SkipAuthentication", "-UseMockData"]
        app.launch()

        // Start at codespace
        let codespacePage = CodespacePage(app: app)
        XCTAssertTrue(codespacePage.isDisplayed(), "Should start at codespace")

        // Connect and navigate to workspaces
        codespacePage.tapAccessMyCodespace()

        let workspacesPage = WorkspacesListPage(app: app)
        XCTAssertTrue(workspacesPage.waitForConnection(timeout: 5), "Should navigate to workspaces")

        // Open workspace detail
        workspacesPage.tapWorkspace(at: 0)

        let detailPage = WorkspaceDetailPage(app: app)
        XCTAssertTrue(detailPage.isDisplayed(), "Should navigate to detail")

        // Back to workspaces
        detailPage.tapBack()
        XCTAssertTrue(workspacesPage.isDisplayed(), "Should be back at workspaces")

        // Back to codespace
        let backToCodespace = app.navigationBars.buttons.firstMatch
        if backToCodespace.exists {
            backToCodespace.tap()
            XCTAssertTrue(codespacePage.isDisplayed(), "Should be back at codespace")
        }
    }
}

// MARK: - Helper Extension for Connection Tests

extension WorkspacesListPage {
    func waitForConnection(timeout: TimeInterval) -> Bool {
        navigationBar.waitForExistence(timeout: timeout)
    }
}
