# Catnip iOS App Test Suite

This directory contains a comprehensive test suite for the Catnip iOS app using Swift Testing framework.

## Test Structure

### Core Test Files

1. **catnipTests.swift** - Smoke tests and basic sanity checks
   - Quick verification tests for core functionality
   - Useful for catching obvious breaks

2. **WorkspaceInfoTests.swift** - Model tests for WorkspaceInfo
   - Display name logic
   - Branch name cleaning (refs/catnip/\* handling)
   - Status text computation
   - Time display formatting (today, yesterday, week, etc.)
   - Activity description priority
   - JSON encoding/decoding
   - Edge cases and null handling

3. **DiffParserTests.swift** - Diff parsing and statistics
   - Basic diff parsing (additions, deletions, modifications)
   - Line number tracking
   - Multiple hunks
   - Complex real-world diffs
   - Statistics calculation (additions/deletions count)
   - Edge cases (empty diffs, special characters)

4. **APIModelsTests.swift** - API models and error handling
   - ClaudeSessionData decoding
   - LatestMessageResponse decoding
   - CheckoutResponse decoding
   - WorktreeDiffResponse decoding
   - FileDiff decoding
   - APIError descriptions
   - Todo model tests
   - ClaudeActivityState enum tests

5. **KeychainHelperTests.swift** - Keychain operations
   - Save and load operations
   - Delete operations
   - Overwrite behavior
   - Multiple independent keys
   - UTF-8 encoding (emoji, international characters)
   - Realistic OAuth workflows
   - Session management patterns

6. **IntegrationTests.swift** - Integration scenarios
   - Workspace listing and filtering
   - Workspace with todos workflow
   - Full diff workflow
   - JSON round-trip tests
   - Date/time handling
   - Branch cleaning edge cases
   - Activity description priority
   - Complex multi-hunk diffs
   - URL encoding
   - Hashable/Identifiable compliance

7. **TestHelpers.swift** - Shared utilities
   - `MockDataFactory` - Factory for creating test data
   - `JSONTestHelper` - JSON encoding/decoding utilities
   - `AsyncTestHelper` - Async testing utilities with timeout
   - `URLTestHelper` - URL encoding/parsing utilities
   - `DiffTestHelper` - Diff generation utilities

## Running Tests

### From Xcode

1. Open `catnip.xcodeproj`
2. Select the test target
3. Press Cmd+U to run all tests
4. Or use Cmd+6 to open Test Navigator and run individual tests

### From Command Line

```bash
cd xcode
xcodebuild test -scheme catnip -destination 'platform=iOS Simulator,name=iPhone 16,OS=latest'
```

### Build Tests Only

```bash
cd xcode
xcodebuild build-for-testing -scheme catnip -destination 'platform=iOS Simulator,name=iPhone 16,OS=latest'
```

## Test Coverage

The test suite covers:

- ✅ **Models**: All computed properties, business logic, and edge cases
- ✅ **Utilities**: DiffParser, date formatting, URL encoding
- ✅ **Services**: KeychainHelper with real keychain operations
- ✅ **API**: Error handling, model decoding, JSON serialization
- ✅ **Integration**: End-to-end workflows and realistic scenarios

## Testing Framework

Uses **Swift Testing** (not XCTest), Apple's modern testing framework:

- `@Test` attribute for test functions
- `#expect()` for assertions
- Better async/await support
- Cleaner syntax and better error messages

## Best Practices

1. **Isolation**: Each test is independent and doesn't rely on others
2. **Cleanup**: Keychain tests clean up after themselves using unique keys
3. **Mock Data**: Use `MockDataFactory` for consistent test data
4. **Edge Cases**: Tests cover empty strings, nil values, special characters
5. **Real Scenarios**: Integration tests reflect actual user workflows

## Adding New Tests

1. Create a new test file or add to existing
2. Import Testing framework and @testable import catnip
3. Create a test struct
4. Add test functions with `@Test` attribute
5. Use `#expect()` for assertions
6. Use MockDataFactory for test data

Example:

```swift
import Testing
@testable import catnip

struct MyNewTests {
    @Test func testSomething() {
        let workspace = MockDataFactory.createWorkspace()
        #expect(workspace.id != nil)
    }
}
```

## Future Enhancements

Potential areas for additional tests:

- [ ] SSEService integration tests (requires mocking)
- [ ] CatnipAPI network tests (requires URL mocking/stubs)
- [ ] AuthManager OAuth flow (requires ASWebAuthenticationSession mocking)
- [ ] View model tests when added
- [ ] UI snapshot tests
- [ ] Performance tests for large diffs
- [ ] Error recovery scenarios
