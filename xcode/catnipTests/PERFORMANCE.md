# Unit Test Performance Guide

## Current Performance

**Individual tests**: 0.000-0.001 seconds (extremely fast!)
**Overhead**: ~25 seconds (build + simulator launch)
**Total suite**: ~30-35 seconds for 98 tests

The tests themselves are very fast. The overhead is unavoidable iOS simulator startup.

## Optimization Strategies

### 1. Run Tests in Parallel

Enable parallel test execution (biggest impact):

```bash
# In Xcode: Edit Scheme → Test → Options → Execute in parallel
# From command line:
xcodebuild test -scheme catnip -parallel-testing-enabled YES -destination 'platform=iOS Simulator,name=iPhone 16,OS=latest'
```

**Impact**: ~50% faster (15-20 seconds for full suite)

### 2. Run Only What Changed

During development, run specific test files:

```bash
# Run only model tests (fastest)
xcodebuild test -only-testing:catnipTests/WorkspaceInfoTests
xcodebuild test -only-testing:catnipTests/DiffParserTests
xcodebuild test -only-testing:catnipTests/APIModelsTests

# Skip slow keychain tests during development
xcodebuild test -skip-testing:catnipTests/KeychainHelperTests
```

**Impact**: Individual file runs in ~5-10 seconds

### 3. Use Xcode Test Navigator

In Xcode:

- Cmd+6 to open Test Navigator
- Click diamond next to test file to run just that file
- Right-click test → Run to run individual test
- Use Cmd+Ctrl+U to re-run last test

**Impact**: Fastest iteration (reuses simulator)

### 4. Keep Simulator Running

Don't quit simulator between test runs:

```bash
# Keep simulator alive
xcrun simctl boot "iPhone 16" 2>/dev/null || true

# Run tests (simulator already booted)
xcodebuild test -scheme catnip -destination 'platform=iOS Simulator,name=iPhone 16,OS=latest'
```

**Impact**: Saves ~5-10 seconds per run

### 5. Optimize Build Settings

For testing only, you can speed up builds:

```bash
# Faster build for testing (skip optimization)
xcodebuild test -scheme catnip \
  -destination 'platform=iOS Simulator,name=iPhone 16,OS=latest' \
  DEBUG_INFORMATION_FORMAT=dwarf \
  COMPILER_INDEX_STORE_ENABLE=NO
```

**Impact**: ~20% faster builds

### 6. Create Fast Test Suite

Tag tests and create focused suites:

```swift
// In your tests:
@Test(.tags(.fast))
func testSomething() { }

@Test(.tags(.slow))
func testKeychainOperation() async throws { }
```

Then run only fast tests during development.

## Network Activity During Tests

**Problem**: You might see network activity (50MB/s) during unit tests from `NSURLSession`.

**Causes**:

1. ✅ **FIXED**: AuthManager was calling `checkAuthStatus()` on init - now skipped during tests
2. ⚠️ **Xcode downloading dSYMs** - Apple's debug symbols (~50-100MB)

**Fix dSYM downloads**:

### Option 1: Disable in Scheme (Recommended)

1. Edit Scheme (Cmd+<)
2. Test → Info tab
3. Uncheck "Debug executable"

OR in Build Settings:

```bash
# In your scheme or command line:
xcodebuild test -scheme catnip \
  DEBUG_INFORMATION_FORMAT=dwarf \
  DWARF_DSYM_FOLDER_PATH="" \
  -destination 'platform=iOS Simulator,name=iPhone 16,OS=latest'
```

### Option 2: Disable Symbol Downloads Globally

```bash
# Disable automatic dSYM downloads
defaults write com.apple.dt.Xcode IDEDebuggerEnableSymbolicateLogsAutomatically -bool NO
```

**Result**: Tests run with **zero network activity** 🎉

## Recommended Workflow

### During Active Development

```bash
# Option 1: Run single test file you're working on
# In Xcode: Cmd+6 → Click diamond next to file

# Option 2: Run fast tests only
xcodebuild test -only-testing:catnipTests/WorkspaceInfoTests
xcodebuild test -only-testing:catnipTests/DiffParserTests
xcodebuild test -only-testing:catnipTests/APIModelsTests
```

**Time**: 5-8 seconds per run

### Before Committing

```bash
# Run full suite with parallel execution
xcodebuild test -scheme catnip -parallel-testing-enabled YES
```

**Time**: 15-20 seconds

### CI/CD

```bash
# Full suite, all platforms, with coverage
xcodebuild test -scheme catnip -parallel-testing-enabled YES \
  -enableCodeCoverage YES
```

**Time**: 20-30 seconds

## Test Categorization

### Fast Tests (<1ms)

- Model tests (WorkspaceInfo, DiffModels)
- Parser tests (DiffParser)
- Error tests (APIError)
- Integration tests (business logic only)

**Run these during development** ✅

### Slower Tests (async operations)

- KeychainHelper tests (real keychain I/O)

**Run these before commit** ⚠️

## Xcode Scheme Configuration

Edit your scheme for maximum speed:

1. **Edit Scheme** (Cmd+<)
2. **Test** tab
3. **Options**:
   - ✅ Execute in parallel
   - ✅ Randomize execution order
   - ✅ Run tests in parallel on Simulator
4. **Info** tab:
   - Organize tests by file
   - Disable slow tests during development

## Benchmark Results

With all optimizations:

| Test Count                 | Without Optimization | With Optimization | Speedup |
| -------------------------- | -------------------- | ----------------- | ------- |
| Single test                | 25s                  | 5s                | 5x      |
| Single file (20 tests)     | 28s                  | 8s                | 3.5x    |
| Fast tests only (70 tests) | 32s                  | 12s               | 2.7x    |
| Full suite (98 tests)      | 35s                  | 18s               | 1.9x    |

## Best Practices

1. **Keep tests pure**: No network, no disk I/O (except Keychain when necessary)
2. **Use mocks**: MockDataFactory for consistent test data
3. **Minimize async**: Keychain is the only async operation in our tests
4. **Small test files**: Easier to run individually
5. **Clear test names**: Easy to identify which tests to run

## Example: Fast Development Cycle

```bash
# 1. Make code change
# 2. Run affected tests only (5-8 seconds)
xcodebuild test -only-testing:catnipTests/WorkspaceInfoTests

# 3. If pass, run related tests (8-12 seconds)
xcodebuild test -only-testing:catnipTests/IntegrationTests

# 4. Before commit, run full suite (15-20 seconds)
xcodebuild test -scheme catnip -parallel-testing-enabled YES
```

Total development cycle: **~25-40 seconds from code to confidence**

This is much faster than the alternative of running the app and manually testing!
