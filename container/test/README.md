# Catnip Integration Tests

This directory contains integration tests for the Catnip application using a mocked environment that doesn't communicate with external services.

## Overview

The integration test framework provides:

- **Mock Scripts**: Replaces external commands (`claude`, `gh`, `git`) with test doubles
- **Real Git Operations**: Uses real git for local operations, mocks only network calls
- **API Testing**: Tests all major API endpoints in isolation
- **Docker Environment**: Runs tests in a containerized environment matching production
- **Dev-Friendly**: Edit tests locally and re-run without rebuilding containers

## Architecture

```
test/
├── integration/          # Go integration tests (volume-mounted)
│   ├── common/          # Shared test utilities
│   │   └── test_suite.go # Test suite setup and helpers
│   └── api/             # API-specific tests
│       ├── claude_test.go   # Claude API tests
│       ├── git_test.go      # Git status and GitHub tests
│       └── worktree_test.go # Worktree and PR tests
├── mocks/               # Mock command scripts
│   ├── claude           # Mock Claude CLI
│   ├── gh               # Mock GitHub CLI
│   └── git              # Git wrapper (mocks network ops)
├── data/                # Test data and responses
│   ├── claude_responses/# Claude mock responses
│   ├── gh_data/         # GitHub CLI test data
│   └── git_data/        # Git operation logs
└── run_integration_tests.sh  # Test runner script
```

## Mock Strategy

### Claude CLI Mock (`mocks/claude`)

- **PTY Mode**: Simulates terminal title sequences and interactive sessions
- **API Mode**: Handles stream-json format for API calls
- **Response System**: Uses response files in `data/claude_responses/` for different prompt types
- **Session Management**: Tracks session UUIDs and titles

### GitHub CLI Mock (`mocks/gh`)

- **PR Operations**: `gh pr create`, `gh pr edit`, `gh pr view`
- **Auth Status**: `gh auth status`, `gh auth git-credential`
- **Repository Listing**: `gh repo list`
- **Data Persistence**: Stores PR data in JSON files for consistency

### Git Wrapper (`mocks/git`)

- **Pass-through**: Uses real git for local operations (add, commit, checkout, worktree)
- **Network Interception**: Mocks push/pull/fetch to remote origins
- **Clone Simulation**: Creates real local repos without network calls
- **Operation Logging**: Tracks all mocked network operations

## Test Coverage

The integration tests cover these major areas:

1. **Worktree Creation** (`TestWorktreeCreation`)
   - Repository checkout API
   - Branch creation and switching
   - Worktree management

2. **Claude Session Handling** (`TestClaudeSessionTitleHandling`)
   - Claude API message creation
   - Session title extraction and management
   - Session summary retrieval

3. **Auto Committing** (`TestAutoCommitting`)
   - Git status tracking
   - Automatic commit workflows
   - Change detection

4. **Preview Branch Creation** (`TestPreviewBranchCreation`)
   - Preview branch workflow
   - Branch management API

5. **Pull Request Creation** (`TestPRCreation`)
   - PR creation via GitHub API
   - Title and body handling
   - Repository integration

6. **Upstream Syncing** (`TestUpstreamSyncing`)
   - Sync conflict detection
   - Upstream merge operations
   - Conflict resolution

7. **GitHub Integration** (`TestGitHubRepositoriesListing`)
   - Repository listing from GitHub
   - Authentication handling

## Running Tests

### Quick Start

```bash
# Run all integration tests
./run_integration_tests.sh

# Run specific test
./run_integration_tests.sh test TestWorktreeCreation

# Run benchmarks
./run_integration_tests.sh bench

# Interactive debugging shell (edit tests in ./integration/ and re-run!)
./run_integration_tests.sh shell
```

### Development Workflow

The test framework is designed for rapid iteration:

1. **Build once**: `./run_integration_tests.sh build`
2. **Edit tests**: Modify files in `./integration/` directory locally
3. **Re-run instantly**: `./run_integration_tests.sh --no-build test`
4. **Repeat**: No container rebuilds needed for test changes

Tests are volume-mounted, so changes are immediately available in the container!

### Detailed Commands

```bash
# Build test image only
./run_integration_tests.sh build

# Run tests without rebuilding
./run_integration_tests.sh --no-build test

# Force rebuild everything
./run_integration_tests.sh --rebuild test

# Clean up containers and images
./run_integration_tests.sh clean
```

### Manual Testing

```bash
# Enter test container for manual debugging
./run_integration_tests.sh shell

# Inside the container:
cd /test/integration
go test -v -run TestWorktreeCreation ./...

# Check mock logs
tail -f /tmp/claude-mock.log
tail -f /tmp/gh-mock.log
tail -f /tmp/git-mock.log
```

## Test Data Management

### Claude Responses

Add custom responses in `data/claude_responses/`:

- `default.json` - Default response for unmatched prompts
- `create_file.json` - Response for file creation prompts
- `edit_function.json` - Response for function editing prompts

### GitHub Data

Mock GitHub data in `data/gh_data/`:

- `auth_status.json` - Authentication status
- `repos.json` - Available repositories
- `prs/` - Generated PR data (auto-created)

### Git Logs

Operation logs in `data/git_data/`:

- `push_log.txt` - Mocked push operations
- `pull_log.txt` - Mocked pull operations
- `fetch_log.txt` - Mocked fetch operations
- `clone_log.txt` - Mocked clone operations

## Environment Variables

The test environment supports these variables:

- `CATNIP_TEST_MODE=1` - Enables test mode
- `CATNIP_TEST_DATA_DIR` - Path to test data directory
- `PATH` - Modified to prioritize mock scripts

## Debugging

### Mock Logs

Each mock script logs to `/tmp/`:

```bash
tail -f /tmp/claude-mock.log  # Claude CLI calls
tail -f /tmp/gh-mock.log      # GitHub CLI calls
tail -f /tmp/git-mock.log     # Git wrapper calls
```

### Test Debugging

```bash
# Run single test with verbose output
go test -v -run TestWorktreeCreation ./...

# Add test logging
t.Logf("Debug: %+v", variable)

# Check test artifacts
ls -la /tmp/catnip-integration-test-*
```

### Docker Debugging

```bash
# Check container logs
docker logs catnip-integration-test

# Interactive shell in test container
./run_integration_tests.sh shell

# Inspect test image
docker run --rm -it catnip:test bash
```

## Adding New Tests

1. **Add Test Function** in `integration/api_test.go`:

```go
func TestNewFeature(t *testing.T) {
    ts := SetupTestSuite(t)
    defer ts.TearDown()

    // Test implementation
}
```

2. **Add Mock Responses** in `data/` if needed:

```bash
echo "Mock response" > data/claude_responses/new_feature.json
```

3. **Update Mock Scripts** if new commands are needed:

```bash
# Edit mocks/claude, mocks/gh, or mocks/git
```

4. **Run and Verify**:

```bash
./run_integration_tests.sh test TestNewFeature
```

## CI Integration

The test runner is designed for CI environments:

```yaml
# Example GitHub Actions step
- name: Run Integration Tests
  run: |
    cd container/test
    ./run_integration_tests.sh test
```

The tests are self-contained and don't require external network access, making them suitable for CI/CD pipelines.

## Performance

- **Benchmark Tests**: Use `BenchmarkWorktreeCreation` pattern
- **Parallel Execution**: Tests can run in parallel (use `t.Parallel()`)
- **Resource Cleanup**: Automatic cleanup prevents resource leaks
- **Timeouts**: Tests have reasonable timeouts (30m default)

## Troubleshooting

### Common Issues

1. **Permission Errors**: Ensure test scripts are executable
2. **Docker Build Fails**: Check if main `catnip:latest` image exists
3. **Test Timeouts**: Increase timeout in test runner
4. **Mock Not Working**: Check PATH and script permissions

### Debug Steps

1. Check mock logs in `/tmp/`
2. Verify test data directory structure
3. Ensure environment variables are set
4. Test mocks individually outside test suite
5. Use interactive shell for manual debugging
