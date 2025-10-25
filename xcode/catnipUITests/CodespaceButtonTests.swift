//
//  CodespaceButtonTests.swift
//  catnipUITests
//
//  Tests for conditional CodespaceView button text based on user status
//

import XCTest

final class CodespaceButtonTests: XCTestCase {

    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
    }

    override func tearDownWithError() throws {
        app = nil
    }

    // MARK: - Button Text Tests

    /// Test Scenario 1: User has no codespaces and no repos with Catnip
    /// Expected: Button text should be "Install Catnip"
    @MainActor
    func testButtonText_NoCodespaces_NoReposWithCatnip_ShowsInstallCatnip() throws {
        // Configure app for Scenario 1
        app.launchArguments = [
            "-UITesting",
            "-SkipAuthentication",
            "-UseMockData",
            "-NoCodespacesNoRepos"  // User has no codespaces and no repos with Catnip
        ]

        app.launch()

        // Wait for CodespaceView to load
        let accessButton = app.buttons["primaryActionButton"]
        XCTAssertTrue(accessButton.waitForExistence(timeout: 5), "Primary action button should appear")

        // Verify button text
        let buttonLabel = accessButton.label
        XCTAssertEqual(
            buttonLabel,
            "Install Catnip",
            "Button should show 'Install Catnip' when user has no codespaces and no repos with Catnip"
        )

        // Verify button is enabled
        XCTAssertTrue(accessButton.isEnabled, "Button should be enabled")
    }

    /// Test Scenario 2: User has no codespaces but has repos with Catnip installed
    /// Expected: Button text should be "Launch New Codespace"
    @MainActor
    func testButtonText_NoCodespaces_HasReposWithCatnip_ShowsLaunchNewCodespace() throws {
        // Configure app for Scenario 2
        app.launchArguments = [
            "-UITesting",
            "-SkipAuthentication",
            "-UseMockData",
            "-NoCodespacesHasRepos"  // User has no codespaces but has repos with Catnip
        ]

        app.launch()

        // Wait for CodespaceView to load
        let accessButton = app.buttons["primaryActionButton"]
        XCTAssertTrue(accessButton.waitForExistence(timeout: 5), "Primary action button should appear")

        // Verify button text
        let buttonLabel = accessButton.label
        XCTAssertEqual(
            buttonLabel,
            "Launch New Codespace",
            "Button should show 'Launch New Codespace' when user has no codespaces but has repos with Catnip"
        )

        // Verify button is enabled
        XCTAssertTrue(accessButton.isEnabled, "Button should be enabled")
    }

    /// Test Scenario 3: User has codespaces
    /// Expected: Button text should be "Access My Codespace"
    @MainActor
    func testButtonText_HasCodespaces_ShowsAccessMyCodespace() throws {
        // Configure app for Scenario 3
        app.launchArguments = [
            "-UITesting",
            "-SkipAuthentication",
            "-UseMockData",
            "-HasCodespaces"  // User has codespaces
        ]

        app.launch()

        // Wait for CodespaceView to load
        let accessButton = app.buttons["primaryActionButton"]
        XCTAssertTrue(accessButton.waitForExistence(timeout: 5), "Primary action button should appear")

        // Verify button text
        let buttonLabel = accessButton.label
        XCTAssertEqual(
            buttonLabel,
            "Access My Codespace",
            "Button should show 'Access My Codespace' when user has codespaces"
        )

        // Verify button is enabled
        XCTAssertTrue(accessButton.isEnabled, "Button should be enabled")
    }

    // MARK: - Button Interaction Tests

    /// Test that tapping "Install Catnip" button navigates to repository selection
    @MainActor
    func testInstallCatnipButton_NavigatesToRepositorySelection() throws {
        app.launchArguments = [
            "-UITesting",
            "-SkipAuthentication",
            "-UseMockData",
            "-NoCodespacesNoRepos"
        ]

        app.launch()

        let accessButton = app.buttons["primaryActionButton"]
        XCTAssertTrue(accessButton.waitForExistence(timeout: 5), "Button should appear")

        // Tap the button
        accessButton.tap()

        // Should navigate to repository selection view with "Select a Repository" header
        let repoSelectionHeader = app.staticTexts["Select a Repository"]
        XCTAssertTrue(
            repoSelectionHeader.waitForExistence(timeout: 5),
            "Should navigate to repository selection view with installation mode header"
        )

        // Verify we see the mock repositories
        let firstRepo = app.staticTexts["testuser/test-repo"]
        XCTAssertTrue(firstRepo.waitForExistence(timeout: 3), "Should display mock repositories")
    }

    /// Test that tapping "Launch New Codespace" button navigates to repository selection
    @MainActor
    func testLaunchNewCodespaceButton_NavigatesToRepositorySelection() throws {
        app.launchArguments = [
            "-UITesting",
            "-SkipAuthentication",
            "-UseMockData",
            "-NoCodespacesHasRepos"
        ]

        app.launch()

        let accessButton = app.buttons["primaryActionButton"]
        XCTAssertTrue(accessButton.waitForExistence(timeout: 5), "Button should appear")

        // Tap the button
        accessButton.tap()

        // Should navigate to repository selection view with "Select Repository to Launch" header
        let repoSelectionHeader = app.staticTexts["Select Repository to Launch"]
        XCTAssertTrue(
            repoSelectionHeader.waitForExistence(timeout: 5),
            "Should navigate to repository selection view with launch mode header"
        )

        // Verify we see the mock Catnip-ready repositories
        let firstRepo = app.staticTexts["testuser/catnip-ready-repo"]
        XCTAssertTrue(firstRepo.waitForExistence(timeout: 3), "Should display mock Catnip-ready repositories")
    }

    /// Test that tapping "Access My Codespace" button triggers connection flow
    @MainActor
    func testAccessMyCodespaceButton_TriggersConnection() throws {
        app.launchArguments = [
            "-UITesting",
            "-SkipAuthentication",
            "-UseMockData",
            "-HasCodespaces"
        ]

        app.launch()

        let accessButton = app.buttons["primaryActionButton"]
        XCTAssertTrue(accessButton.waitForExistence(timeout: 5), "Button should appear")

        // Tap the button
        accessButton.tap()

        // In mock mode with -HasCodespaces, should trigger mock connection flow
        // which navigates to workspaces view
        let workspacesTitle = app.navigationBars["Workspaces"]
        XCTAssertTrue(
            workspacesTitle.waitForExistence(timeout: 5),
            "Should navigate to workspaces after connecting"
        )
    }

    // MARK: - Screen Elements Tests

    /// Verify that other screen elements are present along with the button
    @MainActor
    func testCodespaceView_HasRequiredElements() throws {
        app.launchArguments = [
            "-UITesting",
            "-SkipAuthentication",
            "-UseMockData",
            "-NoCodespacesNoRepos"
        ]

        app.launch()

        // Verify logo
        let logo = app.images["logo"]
        XCTAssertTrue(logo.waitForExistence(timeout: 5), "Logo should appear")

        // Verify title text
        let titleText = app.staticTexts["Access your GitHub Codespaces"]
        XCTAssertTrue(titleText.exists, "Title text should appear")

        // Verify primary button
        let accessButton = app.buttons["primaryActionButton"]
        XCTAssertTrue(accessButton.exists, "Primary action button should exist")

        // Verify more options menu button
        let moreOptionsButton = app.buttons["moreOptionsButton"]
        XCTAssertTrue(moreOptionsButton.exists, "More options button should exist")
    }

    // MARK: - Accessibility Tests

    /// Verify button is accessible in all scenarios
    @MainActor
    func testButtonAccessibility_AllScenarios() throws {
        let scenarios: [(args: String, expected: String)] = [
            ("-NoCodespacesNoRepos", "Install Catnip"),
            ("-NoCodespacesHasRepos", "Launch New Codespace"),
            ("-HasCodespaces", "Access My Codespace")
        ]

        for (scenarioArg, expectedText) in scenarios {
            // Launch with scenario
            app.launchArguments = [
                "-UITesting",
                "-SkipAuthentication",
                "-UseMockData",
                scenarioArg
            ]
            app.launch()

            let accessButton = app.buttons["primaryActionButton"]
            XCTAssertTrue(
                accessButton.waitForExistence(timeout: 5),
                "Button should appear for scenario \(scenarioArg)"
            )

            // Verify accessibility
            XCTAssertTrue(
                accessButton.isHittable,
                "Button should be hittable (accessible) for scenario \(scenarioArg)"
            )

            XCTAssertEqual(
                accessButton.label,
                expectedText,
                "Button label should match expected text for scenario \(scenarioArg)"
            )

            // Terminate for next iteration
            app.terminate()
        }
    }
}
