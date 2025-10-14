# Catnip iOS App UI Tests

This directory contains comprehensive UI tests for the Catnip iOS app using XCUITest framework.

## Test Structure

### Test Files

1. **WorkflowUITests.swift** - Full workflow tests covering the complete user journey
   - Complete user workflow from auth to logout
   - Individual screen tests (auth, codespace, workspaces)
   - Create workspace flow
   - Empty state handling
   - Accessibility tests
   - Performance/launch tests

2. **UserJourneyTests.swift** - Clean tests using Page Object pattern
   - Complete user journey with page objects
   - Individual feature flows
   - Navigation hierarchy tests
   - More maintainable and readable

3. **PageObjects.swift** - Page Object pattern implementation
   - `AuthPage` - Login screen interactions
   - `CodespacePage` - Codespace connection screen
   - `WorkspacesListPage` - Workspaces list interactions
   - `CreateWorkspaceSheet` - Create workspace sheet
   - `WorkspaceDetailPage` - Workspace detail screen
   - `PromptSheet` - Prompt input sheet
   - Helper extensions and utilities

4. **catnipUITests.swift** - Basic smoke tests
5. **catnipUITestsLaunchTests.swift** - Launch performance tests

### Support Files

**UITestingHelper.swift** (in main app target)

- Handles launch arguments for test configuration
- Provides mock data for UI tests
- Skips authentication in test mode
- Auto-configures app state for testing

## Launch Arguments

The app supports several launch arguments for UI testing:

### Core Testing Flags

- **`-UITesting`** - Enables UI testing mode
- **`-SkipAuthentication`** - Bypasses OAuth flow, uses mock credentials
- **`-UseMockData`** - Returns mock data instead of real API calls
- **`-ShowWorkspacesList`** - Auto-navigates to workspaces list
- **`-EmptyWorkspaces`** - Shows empty state (no workspaces)

### Common Combinations

```swift
// Full authenticated flow with mock data
app.launchArguments = ["-UITesting", "-SkipAuthentication", "-UseMockData"]

// Fast tests (disable animations)
app.launchArguments = ["-UITesting", "-DisableAnimations", "-SkipAuthentication", "-UseMockData"]

// Empty workspaces state
app.launchArguments = ["-UITesting", "-SkipAuthentication", "-UseMockData", "-EmptyWorkspaces", "-ShowWorkspacesList"]

// Direct to workspaces list
app.launchArguments = ["-UITesting", "-SkipAuthentication", "-UseMockData", "-ShowWorkspacesList"]
```

### Performance Flags

- **`-DisableAnimations`** - Disables UI animations for 2-3x faster test execution

## Running Tests

### From Xcode

1. Open `catnip.xcodeproj`
2. Select `catnipUITests` target
3. Press Cmd+U to run all UI tests
4. Or use Cmd+6 to open Test Navigator and run individual tests

### From Command Line

```bash
cd xcode

# Run all UI tests
xcodebuild test \
  -scheme catnip \
  -destination 'platform=iOS Simulator,name=iPhone 16,OS=latest' \
  -only-testing:catnipUITests

# Run specific test class
xcodebuild test \
  -scheme catnip \
  -destination 'platform=iOS Simulator,name=iPhone 16,OS=latest' \
  -only-testing:catnipUITests/UserJourneyTests

# Run specific test
xcodebuild test \
  -scheme catnip \
  -destination 'platform=iOS Simulator,name=iPhone 16,OS=latest' \
  -only-testing:catnipUITests/UserJourneyTests/testCompleteUserJourney
```

### Build for Testing

```bash
xcodebuild build-for-testing \
  -scheme catnip \
  -destination 'platform=iOS Simulator,name=iPhone 16,OS=latest'
```

## Test Coverage

The UI test suite covers:

- ✅ **Authentication Flow**: Login screen, OAuth trigger (mocked)
- ✅ **Codespace Connection**: Connection UI, organization input, navigation
- ✅ **Workspaces List**: List display, pull-to-refresh, empty state
- ✅ **Create Workspace**: Sheet presentation, input, repository/branch selection
- ✅ **Workspace Detail**: Detail view, navigation, back button
- ✅ **Navigation**: Full navigation hierarchy, back navigation
- ✅ **Logout**: Logout flow, session cleanup
- ✅ **Accessibility**: Basic accessibility checks

## Page Object Pattern

Tests use the Page Object pattern for better maintainability:

### Benefits

1. **Encapsulation**: UI elements and actions are in one place
2. **Reusability**: Page objects can be used across multiple tests
3. **Maintainability**: UI changes only require updating page objects
4. **Readability**: Tests read like user actions

### Example

```swift
@MainActor
func testWorkspaceSelection() throws {
    app.launch()

    let workspacesPage = WorkspacesListPage(app: app)
    XCTAssertTrue(workspacesPage.isDisplayed())

    workspacesPage.tapWorkspace(at: 0)

    let detailPage = WorkspaceDetailPage(app: app)
    XCTAssertTrue(detailPage.isDisplayed())
}
```

## Mock Data

UI tests use mock data defined in `UITestingHelper.swift`:

### Mock Workspaces

- **feature-authentication** - Active workspace with todos and commits
- **bugfix-api** - Inactive workspace with completed session
- **refactor-ui** - Older workspace without activity

### Mock Sessions

- Active Claude session for feature-authentication
- Inactive session for bugfix-api

## Best Practices

### Writing UI Tests

1. **Use Page Objects**: Don't access UI elements directly in tests
2. **Wait for Elements**: Use `waitForExistence(timeout:)` for async operations
3. **Descriptive Names**: Test names should describe user action and outcome
4. **One Assertion Per Concept**: Each test should verify one user flow
5. **Clean State**: Each test should be independent

### Debugging Failed Tests

1. **Check Simulator**: Ensure iPhone 16 simulator is available
2. **View Hierarchy**: Use Xcode's UI test failure screenshots
3. **Slow Animation**: Add `app.launchArguments += ["-UITestingSlowAnimations"]` to slow animations
4. **Print Elements**: Use `print(app.debugDescription)` to see UI hierarchy

### Performance

- UI tests are slower than unit tests (~10-30 seconds each normally)
- **With `-DisableAnimations`: ~3-10 seconds each** (2-3x faster!)
- Run frequently during development for quick feedback
- Fast suite (`FastUserJourneyTests`) runs in ~30 seconds
- Full suite runs in ~5 minutes (or ~2 minutes with optimizations)

### Speed Optimization Tips

1. **Use `-DisableAnimations`** - Biggest impact, 2-3x faster
2. **Run fast tests first** - Use `FastUserJourneyTests` for quick validation
3. **Reduce timeouts** - Use shorter `waitForExistence(timeout:)` values
4. **Parallel execution** - Xcode can run tests in parallel:
   ```bash
   xcodebuild test -parallel-testing-enabled YES
   ```
5. **Run specific tests** - Don't run full suite during development:
   ```bash
   -only-testing:catnipUITests/FastUserJourneyTests
   ```
6. **Use simulator from previous run** - Don't boot new simulator each time
7. **Mock everything** - No network calls, no real services

## Limitations

### Cannot Test

- **Actual OAuth**: Real GitHub OAuth requires user interaction
- **Real API Calls**: Tests use mock data to avoid network dependency
- **Push Notifications**: Simulator doesn't support real push notifications
- **Actual Codespace Connection**: SSE connection is mocked

### Workarounds

- Use launch arguments to bypass OAuth
- Mock API responses via UITestingHelper
- Test UI flows without backend dependency

## Future Enhancements

Potential improvements:

- [ ] Add tests for diff viewer interactions
- [ ] Test todo list interactions
- [ ] Add tests for error states
- [ ] Test accessibility with VoiceOver
- [ ] Add screenshot tests for visual regression
- [ ] Test landscape orientation
- [ ] Test iPad layouts
- [ ] Add performance benchmarks
- [ ] Test deep linking
- [ ] Test app state restoration

## Troubleshooting

### Tests Fail to Launch App

- Verify app builds successfully
- Check simulator is running
- Clean build folder (Cmd+Shift+K)

### Elements Not Found

- Check launch arguments are set correctly
- Verify mock data is being used
- Add longer timeout values
- Print UI hierarchy for debugging

### Flaky Tests

- Add explicit waits for async operations
- Use `XCUIElement.waitForExistence()` instead of `exists`
- Avoid hardcoded delays
- Check for race conditions

## Resources

- [XCTest UI Testing Documentation](https://developer.apple.com/documentation/xctest/user_interface_tests)
- [Page Object Pattern](https://martinfowler.com/bliki/PageObject.html)
- [UI Testing Best Practices](https://developer.apple.com/videos/play/wwdc2015/406/)
