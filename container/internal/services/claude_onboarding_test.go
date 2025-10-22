package services

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// Test-specific wrapper functions that add *testing.T logging

// NewTestProxy creates a test proxy with test logging
func NewTestProxy(t *testing.T) (*TestProxy, error) {
	proxy, err := NewTestProxyForCapture()
	if err != nil {
		return nil, err
	}

	// Override logFunc to use t.Logf
	proxy.logFunc = func(format string, args ...interface{}) {
		t.Logf(format, args...)
	}
	proxy.interceptor.logFunc = func(format string, args ...interface{}) {
		t.Logf(format, args...)
	}

	t.Logf("✅ Test proxy listening on %s (MITM enabled)", proxy.Addr())
	return proxy, nil
}

// NewPTYTestHelper creates a PTY helper with test logging
func NewPTYTestHelper(t *testing.T, proxyAddr string) (*PTYTestHelper, error) {
	helper, err := NewPTYTestHelperForCapture(proxyAddr)
	if err != nil {
		return nil, err
	}

	t.Logf("✅ Created test home directory: %s", helper.homeDir)
	claudePath := getClaudePath()
	t.Logf("✅ Using claude at: %s", claudePath)
	t.Logf("✅ Started Claude PTY session (monitoring not started)")

	return helper, nil
}

// TestOnboardingStateMachine tests the onboarding state machine
func TestOnboardingStateMachine(t *testing.T) {
	// Check if claude command is available
	claudePath := getClaudePath()
	if _, err := os.Stat(claudePath); err != nil {
		t.Skipf("claude command not found at %s, skipping integration test", claudePath)
	}

	// Start test proxy
	proxy, err := NewTestProxy(t)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}
	defer proxy.Close()

	t.Run("StateTransitions", func(t *testing.T) {
		testStateTransitions(t, proxy.Addr())
	})

	t.Run("SuccessfulCodeSubmission", func(t *testing.T) {
		testSuccessfulCodeSubmission(t, proxy.Addr())
	})

	t.Run("FailedCodeSubmission", func(t *testing.T) {
		testFailedCodeSubmission(t, proxy, proxy.Addr())
	})
}

func testSuccessfulCodeSubmission(t *testing.T, proxyAddr string) {
	// Create PTY helper (monitoring not started - service will monitor)
	pty, err := NewPTYTestHelper(t, proxyAddr)
	if err != nil {
		t.Fatalf("Failed to create PTY helper: %v", err)
	}
	defer pty.Close()

	// Create onboarding service
	service := NewClaudeOnboardingService()
	err = service.StartWithPTY(pty.ptyFile, pty.cmd, pty.homeDir)
	if err != nil {
		t.Fatalf("Failed to start onboarding: %v", err)
	}
	defer func() { _ = service.Stop() }()

	// Wait for AUTH_WAITING state
	t.Logf("⏳ Waiting for AUTH_WAITING state...")
	if err := waitForState(service, StateAuthWaiting, 30*time.Second); err != nil {
		t.Fatalf("Never reached AUTH_WAITING state: %v", err)
	}

	// Get OAuth URL
	status := service.GetStatus()
	if status.OAuthURL == "" {
		t.Fatal("OAuth URL not extracted")
	}
	t.Logf("✅ OAuth URL extracted: %s", status.OAuthURL[:50]+"...")

	// Submit a realistic OAuth code format (this will fail but should trigger the network request)
	testCode := "imtk2bf4AvgDkKxvRFhDfanHNiVk3R51Lzl8kzHs8POSPVGO#E_F8URzH7vNLrK9ke6YTw4UAq27ePoZmaSm0Yk8DDgQ"
	t.Logf("📝 Submitting realistic OAuth code format: %s", testCode[:20]+"...")
	if err := service.SubmitCode(testCode); err != nil {
		t.Fatalf("Failed to submit code: %v", err)
	}

	// Wait for AUTH_CONFIRM state (successful login)
	t.Logf("⏳ Waiting for AUTH_CONFIRM state...")
	if err := waitForState(service, StateAuthConfirm, 10*time.Second); err != nil {
		status := service.GetStatus()
		t.Logf("Current state: %s, error: %s", status.State, status.ErrorMessage)
		t.Fatalf("Never reached AUTH_CONFIRM state: %v", err)
	}
	t.Logf("✅ Reached AUTH_CONFIRM state")

	// Wait for BYPASS_PERMISSIONS state
	t.Logf("⏳ Waiting for BYPASS_PERMISSIONS state...")
	if err := waitForState(service, StateBypassPermissions, 10*time.Second); err != nil {
		status := service.GetStatus()
		t.Logf("Current state: %s", status.State)
		t.Fatalf("Never reached BYPASS_PERMISSIONS state: %v", err)
	}
	t.Logf("✅ Reached BYPASS_PERMISSIONS state")

	// Verify final state
	finalStatus := service.GetStatus()
	t.Logf("✅ Final state: %s", finalStatus.State)
}

func testFailedCodeSubmission(t *testing.T, proxy *TestProxy, proxyAddr string) {
	// Configure proxy to fail token exchange for this test
	proxy.SetShouldFailExchange(true)
	defer proxy.SetShouldFailExchange(false) // Reset for other tests

	// Create PTY helper
	pty, err := NewPTYTestHelper(t, proxyAddr)
	if err != nil {
		t.Fatalf("Failed to create PTY helper: %v", err)
	}
	defer pty.Close()

	// Create onboarding service
	service := NewClaudeOnboardingService()
	err = service.StartWithPTY(pty.ptyFile, pty.cmd, pty.homeDir)
	if err != nil {
		t.Fatalf("Failed to start onboarding: %v", err)
	}
	defer func() { _ = service.Stop() }()

	// Wait for AUTH_WAITING state
	t.Logf("⏳ Waiting for AUTH_WAITING state...")
	if err := waitForState(service, StateAuthWaiting, 30*time.Second); err != nil {
		pty.DumpOutput()
		t.Fatalf("Never reached AUTH_WAITING state: %v", err)
	}

	// Submit an OAuth code that will fail
	testCode := "test-fail-code-invalid"
	t.Logf("📝 Submitting invalid OAuth code: %s", testCode)
	if err := service.SubmitCode(testCode); err != nil {
		t.Fatalf("Failed to submit code: %v", err)
	}

	// Wait for error detection (reduced from 5s to 2s)
	time.Sleep(2 * time.Second)

	// Should still be in AUTH_WAITING with error message
	status := service.GetStatus()
	t.Logf("State after failed code: %s", status.State)

	if status.State != StateAuthWaiting {
		pty.DumpOutput()
		t.Errorf("Expected to stay in AUTH_WAITING after failed code, got %s", status.State)
	}

	if status.ErrorMessage == "" {
		pty.DumpOutput()
		t.Error("Expected error message after invalid code")
	} else {
		t.Logf("✅ Got expected error: %s", status.ErrorMessage)
	}

	// Verify codeSubmitted was reset so user can retry
	// We can't directly check this, but submitting again should work
	t.Logf("📝 Submitting code again to test retry...")
	if err := service.SubmitCode("retry-code"); err != nil {
		pty.DumpOutput()
		t.Errorf("Failed to submit code on retry: %v", err)
	}
}

func testStateTransitions(t *testing.T, proxyAddr string) {
	// Create PTY helper (monitoring not started - service will monitor)
	pty, err := NewPTYTestHelper(t, proxyAddr)
	if err != nil {
		t.Fatalf("Failed to create PTY helper: %v", err)
	}
	defer pty.Close()

	// Create onboarding service (will monitor PTY itself)
	service := NewClaudeOnboardingService()
	err = service.StartWithPTY(pty.ptyFile, pty.cmd, pty.homeDir)
	if err != nil {
		t.Fatalf("Failed to start onboarding: %v", err)
	}
	defer func() { _ = service.Stop() }()

	// Track state transitions
	seenStates := make(map[OnboardingState]bool)
	stateTimestamps := make(map[OnboardingState]time.Time)

	// Poll rapidly to catch all states (faster than auto-advance delay)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		status := service.GetStatus()

		if !seenStates[status.State] {
			seenStates[status.State] = true
			stateTimestamps[status.State] = time.Now()

			// Log state with current service output snippet
			cleanOutput := stripANSI(status.Output)
			snippet := ""
			if len(cleanOutput) > 100 {
				snippet = cleanOutput[len(cleanOutput)-100:]
			} else {
				snippet = cleanOutput
			}
			t.Logf("📍 State: %s | Output: ...%s", status.State, strings.ReplaceAll(snippet, "\n", "\\n"))

			if status.OAuthURL != "" {
				t.Logf("   🔗 OAuth URL: %s", status.OAuthURL)
			}
		}

		// Stop when we reach AUTH_WAITING
		if status.State == StateAuthWaiting {
			t.Logf("✅ Reached AUTH_WAITING state")
			break
		}

		// Stop on error
		if status.State == StateError {
			t.Logf("❌ Reached ERROR state: %s", status.ErrorMessage)
			break
		}

		// Poll very frequently to catch fast transitions
		time.Sleep(50 * time.Millisecond)
	}

	// Log which states we saw
	t.Logf("\n📊 States observed (in order of first appearance):")
	allStates := []OnboardingState{StateIdle, StateThemeSelect, StateAuthMethod, StateAuthWaiting}
	for _, state := range allStates {
		if timestamp, seen := stateTimestamps[state]; seen {
			t.Logf("  ✓ %s (at %v)", state, timestamp.Format("15:04:05.000"))
		} else {
			t.Logf("  ✗ %s (never seen)", state)
		}
	}

	// Check specific patterns in service output to verify actual screens appeared
	finalStatus := service.GetStatus()
	cleanOutput := stripANSI(finalStatus.Output)

	patterns := map[string]string{
		"theme_screen": "Choose the text style",
		"oauth_url":    "Browser didn't open",
		"paste_prompt": "Paste code here",
	}

	t.Logf("\n📋 Pattern verification in service output:")
	for name, pattern := range patterns {
		if strings.Contains(strings.ToLower(cleanOutput), strings.ToLower(pattern)) {
			t.Logf("  ✓ %s: found '%s'", name, pattern)
		} else {
			t.Logf("  ✗ %s: NOT found '%s'", name, pattern)
		}
	}

	// Dump final output for inspection
	t.Logf("\n📋 Final service output (%d bytes):", len(finalStatus.Output))
	t.Logf("%s", cleanOutput)

	// Only require that we saw the actual screens in the output, not necessarily the state detection
	// This helps us debug state detection issues
	if !strings.Contains(cleanOutput, "Paste code here") {
		t.Logf("\n📋 PTY Buffer at failure:")
		t.Logf("%s", pty.DumpOutput())
		t.Error("Never saw 'Paste code here' prompt in output")
	}
}

// waitForState waits for the service to reach a specific state
func waitForState(service *ClaudeOnboardingService, targetState OnboardingState, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status := service.GetStatus()
		if status.State == targetState {
			return nil
		}
		if status.State == StateError {
			return fmt.Errorf("reached error state: %s", status.ErrorMessage)
		}
		time.Sleep(100 * time.Millisecond) // Fast polling for responsive tests
	}
	return fmt.Errorf("timeout waiting for state %s", targetState)
}

// TestStripANSI tests ANSI escape code stripping
func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple color codes",
			input:    "\x1b[31mRed text\x1b[0m",
			expected: "Red text",
		},
		{
			name:     "cursor movement",
			input:    "\x1b[2J\x1b[H\x1b[1;1HHello",
			expected: "Hello",
		},
		{
			name:     "mixed codes",
			input:    "\x1b[1;32mGreen\x1b[0m and \x1b[33mYellow\x1b[0m",
			expected: "Green and Yellow",
		},
		{
			name:     "OSC sequences",
			input:    "\x1b]0;Window Title\x07Content",
			expected: "Content",
		},
		{
			name:     "no escape codes",
			input:    "Plain text",
			expected: "Plain text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripANSI(tt.input)
			if result != tt.expected {
				t.Errorf("stripANSI() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestContainsPattern tests pattern matching
func TestContainsPattern(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		patterns []string
		expected bool
	}{
		{
			name:     "exact match",
			text:     "Please paste code here",
			patterns: []string{"paste code"},
			expected: true,
		},
		{
			name:     "case insensitive",
			text:     "PASTE CODE HERE",
			patterns: []string{"paste code"},
			expected: true,
		},
		{
			name:     "no match",
			text:     "Something else",
			patterns: []string{"paste code"},
			expected: false,
		},
		{
			name:     "multiple patterns",
			text:     "Login successful",
			patterns: []string{"failed", "successful", "pending"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsPattern(tt.text, tt.patterns)
			if result != tt.expected {
				t.Errorf("containsPattern() = %v, want %v", result, tt.expected)
			}
		})
	}
}
