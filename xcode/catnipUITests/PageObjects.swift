//
//  PageObjects.swift
//  catnipUITests
//
//  Page object pattern for UI tests
//

import XCTest

// MARK: - Base Page Object

class BasePage {
    let app: XCUIApplication

    init(app: XCUIApplication) {
        self.app = app
    }

    func waitForElement(_ element: XCUIElement, timeout: TimeInterval = 5) -> Bool {
        element.waitForExistence(timeout: timeout)
    }
}

// MARK: - Auth Page

class AuthPage: BasePage {

    var logoImage: XCUIElement {
        app.images["logo"]
    }

    var catnipTitle: XCUIElement {
        app.staticTexts["Catnip"]
    }

    var subtitle: XCUIElement {
        app.staticTexts["Access your GitHub Codespaces"]
    }

    var signInButton: XCUIElement {
        app.buttons["Sign in with GitHub"]
    }

    func isDisplayed() -> Bool {
        waitForElement(signInButton)
    }

    func tapSignIn() {
        signInButton.tap()
    }
}

// MARK: - Codespace Page

class CodespacePage: BasePage {

    var titleText: XCUIElement {
        app.staticTexts["Access your GitHub Codespaces"]
    }

    var accessButton: XCUIElement {
        app.buttons["primaryActionButton"]
    }

    var orgTextField: XCUIElement {
        app.textFields["organizationTextField"]
    }

    var goButton: XCUIElement {
        app.buttons["goButton"]
    }

    var logoutButton: XCUIElement {
        app.buttons["logoutButton"]
    }

    var statusMessage: XCUIElement {
        app.staticTexts.matching(NSPredicate(format: "label CONTAINS '🔄'")).firstMatch
    }

    func isDisplayed() -> Bool {
        waitForElement(accessButton)
    }

    func tapAccessMyCodespace() {
        accessButton.tap()
    }

    func enterOrganization(_ org: String) {
        orgTextField.tap()
        orgTextField.typeText(org)
    }

    func tapGo() {
        goButton.tap()
    }

    func tapLogout() {
        logoutButton.tap()
    }

    func waitForConnection(timeout: TimeInterval = 10) -> Bool {
        // Wait for navigation to workspaces
        app.navigationBars["Workspaces"].waitForExistence(timeout: timeout)
    }
}

// MARK: - Workspaces List Page

class WorkspacesListPage: BasePage {

    var navigationBar: XCUIElement {
        app.navigationBars["Workspaces"]
    }

    var createButton: XCUIElement {
        app.buttons["plus"]
    }

    var scrollView: XCUIElement {
        app.scrollViews.firstMatch
    }

    var emptyStateMessage: XCUIElement {
        app.staticTexts["No workspaces"]
    }

    var emptyStateCreateButton: XCUIElement {
        app.buttons["Create Workspace"]
    }

    func isDisplayed() -> Bool {
        waitForElement(navigationBar)
    }

    func isEmptyStateDisplayed() -> Bool {
        waitForElement(emptyStateMessage, timeout: 2)
    }

    func tapCreate() {
        createButton.tap()
    }

    func tapEmptyStateCreate() {
        emptyStateCreateButton.tap()
    }

    func workspaceCard(at index: Int) -> XCUIElement {
        // NavigationLinks in List are exposed as buttons
        let list = app.descendants(matching: .any)["workspacesList"]
        return list.buttons.element(boundBy: index)
    }

    func tapWorkspace(at index: Int) {
        workspaceCard(at: index).tap()
    }

    func pullToRefresh() {
        let list = app.descendants(matching: .any)["workspacesList"]
        list.swipeDown()
    }

    func numberOfWorkspaces() -> Int {
        // NavigationLinks in List are exposed as buttons
        let list = app.descendants(matching: .any)["workspacesList"]
        return list.buttons.count
    }
}

// MARK: - Create Workspace Sheet

class CreateWorkspaceSheet: BasePage {

    var navigationBar: XCUIElement {
        app.navigationBars["New Workspace"]
    }

    var cancelButton: XCUIElement {
        app.buttons["Cancel"]
    }

    var promptEditor: XCUIElement {
        app.textViews.firstMatch
    }

    var submitButton: XCUIElement {
        app.buttons.matching(NSPredicate(format: "identifier CONTAINS 'submit'")).firstMatch
    }

    func isDisplayed() -> Bool {
        waitForElement(navigationBar, timeout: 2)
    }

    func enterPrompt(_ text: String) {
        promptEditor.tap()
        promptEditor.typeText(text)
    }

    func selectRepository(_ repo: String) {
        let repoButton = app.buttons[repo]
        if repoButton.exists {
            repoButton.tap()
        }
    }

    func selectBranch(_ branch: String) {
        let branchButton = app.buttons[branch]
        if branchButton.exists {
            branchButton.tap()
        }
    }

    func tapCancel() {
        cancelButton.tap()
    }

    func tapSubmit() {
        submitButton.tap()
    }
}

// MARK: - Workspace Detail Page

class WorkspaceDetailPage: BasePage {

    var navigationBar: XCUIElement {
        app.navigationBars.element(boundBy: 0)
    }

    var backButton: XCUIElement {
        navigationBar.buttons.element(boundBy: 0)
    }

    var emptyStateTitle: XCUIElement {
        app.staticTexts["Start Working"]
    }

    var startWorkingButton: XCUIElement {
        app.buttons["Start Working"]
    }

    var workingIndicator: XCUIElement {
        app.staticTexts["Claude is working..."]
    }

    var askForChangesButton: XCUIElement {
        app.buttons["Ask for changes"]
    }

    func isDisplayed() -> Bool {
        waitForElement(navigationBar, timeout: 3)
    }

    func isEmptyState() -> Bool {
        waitForElement(emptyStateTitle, timeout: 2)
    }

    func isWorkingState() -> Bool {
        waitForElement(workingIndicator, timeout: 2)
    }

    func tapStartWorking() {
        startWorkingButton.tap()
    }

    func tapAskForChanges() {
        askForChangesButton.tap()
    }

    func tapBack() {
        backButton.tap()
    }
}

// MARK: - Prompt Sheet

class PromptSheet: BasePage {

    var promptEditor: XCUIElement {
        app.textViews.firstMatch
    }

    var submitButton: XCUIElement {
        app.buttons.matching(NSPredicate(format: "identifier CONTAINS 'submit' OR identifier CONTAINS 'arrow.up'")).firstMatch
    }

    var closeButton: XCUIElement {
        app.buttons["Close"]
    }

    func isDisplayed() -> Bool {
        waitForElement(promptEditor, timeout: 2)
    }

    func enterPrompt(_ text: String) {
        promptEditor.tap()
        promptEditor.typeText(text)
    }

    func tapSubmit() {
        submitButton.tap()
    }

    func tapClose() {
        closeButton.tap()
    }
}

// MARK: - Test Helpers

extension XCUIElement {
    /// Wait for element to disappear
    func waitForDisappearance(timeout: TimeInterval = 5) -> Bool {
        let predicate = NSPredicate(format: "exists == false")
        let expectation = XCTNSPredicateExpectation(predicate: predicate, object: self)
        let result = XCTWaiter.wait(for: [expectation], timeout: timeout)
        return result == .completed
    }

    /// Check if element is visible and hittable
    var isVisible: Bool {
        exists && isHittable
    }
}
