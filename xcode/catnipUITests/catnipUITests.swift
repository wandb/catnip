//
//  catnipUITests.swift
//  catnipUITests
//
//  Basic smoke tests for UI testing
//

import XCTest

final class catnipUITests: XCTestCase {

    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testAppLaunches() throws {
        let app = XCUIApplication()
        app.launch()

        // Verify app launches successfully
        XCTAssertTrue(app.state == .runningForeground, "App should be running in foreground")
    }

    @MainActor
    func testAuthScreenAppears() throws {
        let app = XCUIApplication()
        app.launchArguments = ["-UITesting"]
        app.launch()

        // Verify key UI elements exist
        let signInButton = app.buttons["Sign in with GitHub"]
        XCTAssertTrue(signInButton.waitForExistence(timeout: 5), "Sign in button should appear")
    }

    @MainActor
    func testLaunchPerformance() throws {
        measure(metrics: [XCTApplicationLaunchMetric()]) {
            XCUIApplication().launch()
        }
    }
}
