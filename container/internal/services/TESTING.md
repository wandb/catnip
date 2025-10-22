# Claude Onboarding State Machine Testing

## Overview

This document describes the testing infrastructure for the Claude onboarding state machine (`claude_onboarding.go`).

**Status:** ✅ Comprehensive test suite implemented and passing

## Test Architecture

### Components

1. **TestProxy**: HTTP/HTTPS proxy for testing
   - Tunnels HTTPS connections to real Claude API
   - Allows test isolation with custom environment
   - Note: Cannot intercept encrypted OAuth exchanges (by design)

2. **PTYTestHelper**: Manages Claude PTY sessions for testing
   - Creates isolated HOME directories for each test
   - Prevents browser opening (`BROWSER=true`, `DISPLAY=`)
   - Does NOT monitor PTY when handing to service (avoids byte stealing)
   - Provides debugging utilities
   - Automatic cleanup of resources

3. **Integration Tests**: End-to-end state machine testing against real Claude CLI
   - State transition verification (theme → auth method → auth waiting)
   - OAuth URL extraction
   - Error detection and retry logic
   - Code submission flow

## Running Tests

### Prerequisites

1. `claude` command must be in PATH
2. Go 1.21+ for running tests

### Running All Tests

```bash
cd container
go test -v -timeout 2m ./internal/services -run TestOnboarding
```

### Running Specific Tests

```bash
# Unit tests only (no claude command required)
go test -v ./internal/services -run "TestStripANSI|TestContainsPattern"

# Integration tests (requires claude command)
go test -v -timeout 2m ./internal/services -run TestOnboardingStateMachine
```

### Running with More Verbose Output

```bash
# Enable detailed logging
go test -v -timeout 2m ./internal/services -run TestOnboardingStateMachine 2>&1 | tee test-output.log
```

## Test Coverage

### Unit Tests

- `TestStripANSI`: Validates ANSI escape code removal
- `TestContainsPattern`: Validates pattern matching logic

### Integration Tests

- `testSuccessfulOAuthFlow`: Full authentication flow end-to-end
- `testInvalidOAuthCode`: Error handling for invalid OAuth codes
- `testStateTransitions`: Verifies all expected state transitions occur

## How the Tests Work

### 1. OAuth Interception

The TestProxy intercepts OAuth token exchange requests:

```
Claude CLI → TestProxy → Claude API
              ↓
         (intercepts /oauth/token)
              ↓
         Returns success/failure
```

### 2. PTY Isolation

Each test creates an isolated environment:

```
Temp HOME dir → Fresh Claude config → Onboarding flow
```

Setting `HOME` to a temp directory ensures Claude runs through the onboarding flow.

### 3. State Detection

Tests wait for specific states by polling `GetStatus()`:

```go
waitForState(service, StateAuthWaiting, 30*time.Second)
```

### 4. Output Debugging

When tests fail, they dump the PTY output with ANSI codes stripped:

```
📋 PTY Output (1234 bytes):
Choose the text style...
Select login method...
Paste code here:
```

## Test Scenarios

### Successful OAuth Flow

1. Start onboarding service
2. Wait for AUTH_WAITING state
3. Extract OAuth URL
4. Submit test OAuth code
5. Verify progression past AUTH_WAITING
6. Check for completion or error state

### Invalid OAuth Code

1. Configure proxy to fail token exchange
2. Start onboarding service
3. Wait for AUTH_WAITING state
4. Submit invalid OAuth code
5. Verify error handling
6. Check error message is set

### State Transitions

1. Start onboarding service
2. Monitor all state transitions
3. Verify expected states are visited:
   - StateThemeSelect
   - StateAuthMethod
   - StateAuthURL
   - StateAuthWaiting
4. Log all state changes for debugging

## Common Issues

### Test Skipped: "claude command not found"

The `claude` command must be in your PATH. Install it or skip integration tests:

```bash
# Run only unit tests
go test -v ./internal/services -run "TestStripANSI|TestContainsPattern"
```

### Test Timeout

Integration tests have a 2-minute timeout. If tests timeout:

1. Check if Claude CLI is responsive: `claude --version`
2. Review test output for stuck states
3. Check network connectivity (OAuth flows require internet)

### State Detection Failures

If state detection fails, the test will dump the PTY output. Look for:

1. Unexpected prompts or error messages
2. ANSI escape codes that aren't being stripped
3. Pattern matching issues in `detectState()`

## Debugging Tests

### Enable Detailed Logging

The onboarding service logs extensively. To see all logs:

```bash
go test -v -timeout 2m ./internal/services -run TestOnboardingStateMachine 2>&1 | grep -E "(INFO|WARN|ERROR|DEBUG)"
```

### Dump PTY Output

Tests automatically dump PTY output on failure. You can also manually inspect:

```go
pty.DumpOutput()  // In test code
```

### Monitor State Transitions

Add logging to track state changes:

```go
for {
    status := service.GetStatus()
    t.Logf("Current state: %s - %s", status.State, status.Message)
    time.Sleep(1 * time.Second)
}
```

## Extending Tests

### Adding New Test Scenarios

1. Create a new test function in `claude_onboarding_test.go`:

```go
func testMyScenario(t *testing.T, proxyAddr string) {
    pty, err := NewPTYTestHelper(t, proxyAddr)
    if err != nil {
        t.Fatalf("Failed to create PTY helper: %v", err)
    }
    defer pty.Close()

    // Your test logic here
}
```

2. Add it to the main test:

```go
func TestOnboardingStateMachine(t *testing.T) {
    // ... existing setup ...

    t.Run("MyScenario", func(t *testing.T) {
        testMyScenario(t, proxy.Addr())
    })
}
```

### Simulating Network Conditions

Configure the test proxy:

```go
proxy.SetExchangeDelay(5 * time.Second)  // Slow network
proxy.SetShouldFailExchange(true)        // OAuth failure
```

## Known Limitations

1. **Integration tests require internet**: OAuth flows need to reach Claude API
2. **Timing-dependent**: State transitions have built-in delays (300ms)
3. **PTY output buffering**: May miss rapid state changes
4. **ANSI escape code complexity**: Some rare codes might not be stripped correctly

## Future Improvements

1. **Mock OAuth server**: Remove internet dependency
2. **Configurable timing**: Make delays configurable for faster tests
3. **State transition assertions**: More rigorous state machine validation
4. **Performance tests**: Measure state transition timing
5. **Chaos testing**: Random delays, network failures, etc.
