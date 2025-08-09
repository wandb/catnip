# Catnip Integration Tests

This directory contains integration tests for the Catnip application using a mocked environment that doesn't communicate with external services.

## Overview

The integration test framework provides:

- **External Test Container**: Runs the Catnip server on port 8181 with hot reloading
- **Mock Scripts**: Replaces external commands (`claude`, `gh`, `git`) with test doubles
- **Real Git Operations**: Uses real git for local operations, mocks only network calls
- **API Testing**: Tests all major API endpoints against the real server
- **Docker Environment**: Test server runs in a containerized environment matching production
- **Dev-Friendly**: Edit server code or tests locally and re-run without rebuilding containers

## Architecture

The new architecture separates the test server from the test runner:

```
test/
├── integration/          # Go integration tests (run from host)
│   ├── common/          # Shared test utilities
│   │   └── test_suite.go # HTTP client setup and helpers
│   └── api/             # API-specific tests
│       ├── claude_test.go   # Claude API tests
│       ├── git_test.go      # Git status and GitHub tests
│       └── worktree_test.go # Worktree and PR tests
├── mocks/               # Mock command scripts (mounted in test container)
│   ├── claude           # Mock Claude CLI
│   ├── gh               # Mock GitHub CLI
│   └── git              # Git wrapper (mocks network ops)
├── data/                # Test data and responses (shared)
│   ├── claude_responses/# Claude mock responses
│   ├── gh_data/         # GitHub CLI test data
│   └── git_data/        # Git operation logs
├── scripts/             # Container scripts
│   └── test-entrypoint.sh  # Test container entry point
├── Dockerfile.test      # Docker configuration for test container
├── docker-compose.test.yml # Docker Compose for test environment
└── run_integration_tests.sh  # Test runner script
```

**Key Changes:**

- **Test Container**: Runs Catnip server on port 8181 with hot reloading
- **External Tests**: Integration tests run from host machine and make HTTP calls to test container
- **Hot Reloading**: Server code changes are reflected immediately without rebuilds
- **Port Isolation**: Test server runs on separate port (8181) to avoid conflicts

## Mock Strategy

### Claude CLI Mock (`mocks/claude`)

The Claude mock simulates the Claude API and CLI behavior:

- **API Mode**: Returns structured JSON responses for programmatic calls
- **Interactive Mode**: Provides a chat-like interface for PTY sessions
- **Title Detection**: Sends terminal title escape sequences to test session tracking
- **File Operations**: Simulates file creation/editing based on prompts
- **Session Management**: Tracks session state and generates UUIDs

Key features:

- Responds to different prompt types with relevant code examples
- Generates terminal title escape sequences for PTY testing
- Logs all operations for debugging
- Supports both JSON API and interactive modes

### GitHub CLI Mock (`mocks/gh`)

The GitHub CLI mock prevents network calls and returns test data:

- **Authentication**: Simulates login/logout and auth status
- **Repository Operations**: Lists test repositories, creates PRs
- **Issue Management**: Returns empty issue lists for testing
- **Data Persistence**: Stores auth state in test data files

Test data files:

- `auth_status.json` - Mock authentication status
- `repos.json` - List of test repositories

### Git Mock (`mocks/git`)

The Git mock intercepts git commands to prevent network operations:

- **Local Operations**: Simulates status, branch, log commands
- **Network Prevention**: Blocks clone, fetch, push, pull operations
- **Operation Logging**: Records all git commands for debugging
- **Conflict Simulation**: Provides mock merge conflicts for testing

## Integration Tests

The integration tests cover these major areas:

1. **Git Operations** (`TestAutoCommitting`)
   - Git status API endpoints
   - Repository state management

2. **GitHub Integration** (`TestGitHubRepositoriesListing`)
   - Repository listing from GitHub
   - Authentication handling

3. **Claude API** (`TestClaudeSessionMessagesEndpoint`)
   - Claude message API
   - Session creation and management

4. **PTY Sessions** (`TestClaudeSessionTitleHandling`)
   - WebSocket PTY connections
   - Terminal title detection
   - Session state tracking

5. **Worktree Management** (planned)
   - Branch creation and management
   - PR workflow testing

## Running Tests

### Quick Start

```bash
# Run all integration tests (starts test container automatically)
./run_integration_tests.sh

# Start test container manually
./run_integration_tests.sh start

# Check if test container is running
./run_integration_tests.sh status

# Run specific test
./run_integration_tests.sh test TestWorktreeCreation

# Run benchmarks
./run_integration_tests.sh bench

# Interactive debugging shell in test container
./run_integration_tests.sh shell

# Stop the test container
./run_integration_tests.sh stop
```

### End-to-End (Playwright)

The runner now also executes Playwright UI tests against the same test container:

```bash
# Ensure container is up, then run UI tests only
./run_integration_tests.sh start
pnpm dlx playwright install --with-deps chromium
CATNIP_TEST_SERVER_URL=http://localhost:8181 pnpm test:e2e
```

In CI, the `Integration & E2E` workflow builds the test container, runs API integration tests, installs Playwright, and runs E2E against `http://localhost:8181`.

### Development Workflow

The new architecture provides excellent development experience:

1. **Start test container**: `./run_integration_tests.sh start` (runs server with hot reloading)
2. **Edit server code**: Modify files in `container/` directory - changes auto-reload in test container
3. **Edit tests**: Modify files in `./integration/` directory locally
4. **Re-run tests**: `./run_integration_tests.sh test` (no rebuilds needed)
5. **Repeat**: Both server and test changes are reflected immediately

**Key Benefits:**

- Server hot reloading via Air - edit Go code and see changes instantly
- Tests run from host - no container rebuilds needed for test changes
- Real HTTP testing - tests interact with actual server endpoints
- Port isolation - test server won't conflict with development server

### Detailed Commands

```bash
# Build test image only
./run_integration_tests.sh build

# Start test container (with build if needed)
./run_integration_tests.sh start

# Stop test container
./run_integration_tests.sh stop

# Check test container status
./run_integration_tests.sh status

# Run tests without rebuilding
./run_integration_tests.sh --no-build test

# Force rebuild everything
./run_integration_tests.sh --rebuild test

# Clean up containers and images
./run_integration_tests.sh clean
```

### Manual Testing

```bash
# Access test server directly
curl http://localhost:8181/health

# Enter test container for manual debugging
./run_integration_tests.sh shell

# Inside the container:
# Check mock logs
tail -f /tmp/claude-mock.log
tail -f /tmp/gh-mock.log
tail -f /tmp/git-mock.log

# Outside container - run individual tests
cd integration
go test -v -run TestWorktreeCreation ./...

# Make manual API calls to test server
curl -X GET http://localhost:8181/v1/git/status
```

## Test Data Management

### Claude Responses

Mock Claude responses are stored in `data/claude_responses/`:

- Session state files with UUIDs
- Response templates for different prompt types
- Terminal title sequences for PTY testing

### GitHub Data

GitHub CLI test data in `data/gh_data/`:

- `auth_status.json` - Authentication state
- `repos.json` - Repository listings
- Operation logs for debugging

### Git Operations

Operation logs in `data/git_data/`:

- `status_log.txt` - Status command calls
- `branch_log.txt` - Branch operations
- `clone_log.txt` - Mocked clone operations
- `pull_log.txt` - Mocked pull operations
- `fetch_log.txt` - Mocked fetch operations
- `clone_log.txt` - Mocked clone operations

## Environment Variables

The test environment supports these variables:

**Test Container:**

- `CATNIP_TEST_MODE=1` - Enables test mode in the server
- `CATNIP_PORT=8181` - Port for the test server
- `PATH` - Modified to prioritize mock scripts

**Test Runner:**

- `CATNIP_TEST_SERVER_URL` - URL of test server (default: http://localhost:8181)
- `CATNIP_TEST_DATA_DIR` - Path to test data directory
- `CATNIP_TEST_MODE=1` - Enables test mode for test runner

## Debugging

### Mock Logs

All mock commands log their operations:

- `/tmp/claude-mock.log` - Claude CLI operations and responses
- `/tmp/gh-mock.log` - GitHub CLI calls and data returned
- `/tmp/git-mock.log` - Git command interception and mocking

### Container Logs

View test container logs:

```bash
docker-compose -f docker-compose.test.yml logs -f catnip-test
```

### Network Debugging

Test server connectivity:

```bash
# Health check
curl http://localhost:8181/health

# API endpoints
curl http://localhost:8181/v1/git/status
curl http://localhost:8181/v1/git/github/repos
```

## Troubleshooting

### Common Issues

1. **Test Server Not Starting**
   - Check if port 8181 is available
   - Verify Docker is running
   - Check container logs for errors

2. **Tests Timing Out**
   - Ensure test server is running: `./run_integration_tests.sh status`
   - Check network connectivity to localhost:8181
   - Verify mock scripts are executable

3. **Mock Commands Not Working**
   - Check PATH includes `/opt/catnip/test/bin`
   - Verify mock scripts have execute permissions
   - Check mock logs for error messages

4. **Hot Reloading Not Working**
   - Ensure Air is installed in test container
   - Check if files are properly mounted
   - Verify file changes trigger rebuild in container logs

### Getting Help

For debugging integration test issues:

1. Check container status: `./run_integration_tests.sh status`
2. View container logs: `docker-compose -f docker-compose.test.yml logs`
3. Access container shell: `./run_integration_tests.sh shell`
4. Check mock operation logs in `/tmp/`
5. Verify test server health: `curl http://localhost:8181/health`
