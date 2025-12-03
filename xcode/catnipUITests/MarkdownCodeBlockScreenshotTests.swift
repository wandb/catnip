//
//  MarkdownCodeBlockScreenshotTests.swift
//  catnipUITests
//
//  Screenshot tests for enhanced markdown code block styling
//

import XCTest

final class MarkdownCodeBlockScreenshotTests: XCTestCase {

    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false

        app = XCUIApplication()

        // Configure app for UI testing with mock data
        app.launchArguments = [
            "-UITesting",
            "-SkipAuthentication",
            "-UseMockData",
            "-ShowWorkspacesList",
            "-ShowCodeBlockPreview"  // Custom flag for our preview
        ]
    }

    override func tearDownWithError() throws {
        app = nil
    }

    // MARK: - Screenshot Tests

    @MainActor
    func testMarkdownCodeBlocksLightMode() throws {
        // Set up for light mode
        app.launchArguments.append("-UIPreferredColorScheme-Light")
        app.launch()

        // Navigate to workspace with code blocks
        navigateToCodeBlockWorkspace()

        // Wait for content to load
        Thread.sleep(forTimeInterval: 2.0)

        // Take screenshot
        let screenshot = app.screenshot()
        let attachment = XCTAttachment(screenshot: screenshot)
        attachment.name = "markdown-code-blocks-light-mode"
        attachment.lifetime = .keepAlways
        add(attachment)
    }

    @MainActor
    func testMarkdownCodeBlocksDarkMode() throws {
        // Set up for dark mode
        app.launchArguments.append("-UIPreferredColorScheme-Dark")
        app.launch()

        // Navigate to workspace with code blocks
        navigateToCodeBlockWorkspace()

        // Wait for content to load
        Thread.sleep(forTimeInterval: 2.0)

        // Take screenshot
        let screenshot = app.screenshot()
        let attachment = XCTAttachment(screenshot: screenshot)
        attachment.name = "markdown-code-blocks-dark-mode"
        attachment.lifetime = .keepAlways
        add(attachment)
    }

    @MainActor
    func testMarkdownCodeBlocksScrolled() throws {
        app.launch()

        navigateToCodeBlockWorkspace()

        // Wait for content to load
        Thread.sleep(forTimeInterval: 1.0)

        // Scroll to show middle of content
        let scrollView = app.scrollViews.firstMatch
        if scrollView.exists {
            scrollView.swipeUp()
            Thread.sleep(forTimeInterval: 0.5)
        }

        // Take screenshot showing middle code blocks
        let screenshot = app.screenshot()
        let attachment = XCTAttachment(screenshot: screenshot)
        attachment.name = "markdown-code-blocks-scrolled"
        attachment.lifetime = .keepAlways
        add(attachment)
    }

    // MARK: - Helper Methods

    private func navigateToCodeBlockWorkspace() {
        // Wait for workspaces screen
        let workspacesTitle = app.navigationBars["Workspaces"]
        XCTAssertTrue(workspacesTitle.waitForExistence(timeout: 5), "Workspaces screen should appear")

        // Look for the workspace with code blocks
        // The mock data should include our previewWithCodeBlocks workspace
        let workspacesList = app.descendants(matching: .any)["workspacesList"]
        if workspacesList.waitForExistence(timeout: 3) {
            // Tap the first workspace (we'll update mock data to show code block workspace)
            let firstWorkspace = workspacesList.buttons.firstMatch
            if firstWorkspace.waitForExistence(timeout: 2) {
                firstWorkspace.tap()

                // Wait for detail view to load
                Thread.sleep(forTimeInterval: 1.0)
            }
        }
    }
}
