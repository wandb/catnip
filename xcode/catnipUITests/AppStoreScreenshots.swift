//
//  AppStoreScreenshots.swift
//  catnipUITests
//
//  Screenshots for App Store submission using Fastlane Snapshot
//

import XCTest

final class AppStoreScreenshots: XCTestCase {

    @MainActor var app: XCUIApplication!

    @MainActor
    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()

        // Force portrait orientation for App Store screenshots
        XCUIDevice.shared.orientation = .portrait
    }

    @MainActor
    override func tearDownWithError() throws {
        app = nil
    }

    // Helper to launch app with workspaces list visible
    @MainActor
    private func launchWithWorkspacesList() {
        app.launchArguments = [
            "-UITesting",
            "-SkipAuthentication",
            "-UseMockData",
            "-ShowWorkspacesList"
        ]
        setupSnapshot(app)
        app.launch()
    }

    // Helper to launch app showing the codespace connect screen
    @MainActor
    private func launchWithConnectScreen() {
        app.launchArguments = [
            "-UITesting",
            "-SkipAuthentication",
            "-UseMockData"
            // Note: No -ShowWorkspacesList, so it shows the connect screen
        ]
        setupSnapshot(app)
        app.launch()
    }

    // MARK: - App Store Screenshots
    // Run with: fastlane screenshots
    //
    // Screenshots are numbered to control the order they appear in App Store Connect.

    @MainActor
    func test01_WorkspacesList() throws {
        // Screenshot 1: Main workspaces list showing active sessions
        launchWithWorkspacesList()
        sleep(2)
        snapshot("01_WorkspacesList")
    }

    @MainActor
    func test02_WorkspaceDetail() throws {
        // Screenshot 2: Workspace detail view showing Claude's work
        launchWithWorkspacesList()
        sleep(2)

        // Tap the first workspace to see detail view
        let firstCell = app.cells.firstMatch
        if firstCell.waitForExistence(timeout: 5) {
            firstCell.tap()
            sleep(2)
            snapshot("02_WorkspaceDetail")
        } else {
            snapshot("02_WorkspaceDetail_Empty")
        }
    }

    @MainActor
    func test03_CodespaceConnect() throws {
        // Screenshot 3: Codespace connection/launch screen
        launchWithConnectScreen()
        sleep(2)
        snapshot("03_CodespaceConnect")
    }

    @MainActor
    func test04_TerminalView() throws {
        // Screenshot 4: Terminal view showing Claude interface
        launchWithWorkspacesList()
        sleep(2)

        // Navigate to workspace detail
        let firstCell = app.cells.firstMatch
        if firstCell.waitForExistence(timeout: 5) {
            firstCell.tap()

            // On iPad, terminal is already visible in split view (no button needed)
            // On iPhone, we need to tap the terminal button to show it
            let terminalButton = app.buttons["terminal"]
            if terminalButton.waitForExistence(timeout: 3) {
                // iPhone: tap the terminal button
                terminalButton.tap()
            }
            // else: iPad - terminal already visible in split view

            // Wait for mock PTY data to replay (10s capture at 5x speed = 2s + buffer)
            sleep(4)
            snapshot("04_TerminalView")
        } else {
            snapshot("04_TerminalView_Empty")
        }
    }
}
