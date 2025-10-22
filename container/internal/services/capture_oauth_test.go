package services

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestCaptureRealOAuthResponse is an interactive test to capture a real OAuth response
// This test is skipped by default. To run it manually:
// CAPTURE_OAUTH=1 go test -v -timeout 5m ./internal/services -run TestCaptureRealOAuthResponse
func TestCaptureRealOAuthResponse(t *testing.T) {
	// Skip unless explicitly enabled
	if os.Getenv("CAPTURE_OAUTH") != "1" {
		t.Skip("Skipping interactive OAuth capture test (set CAPTURE_OAUTH=1 to run)")
	}

	// Check if claude command is available
	claudePath := getClaudePath()
	if _, err := os.Stat(claudePath); err != nil {
		t.Skipf("claude command not found at %s, skipping test", claudePath)
	}

	// Start test proxy with MITM
	proxy, err := NewTestProxy(t)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}
	defer proxy.Close()

	// Enable capture mode to forward OAuth to real API and log response
	proxy.SetCaptureMode(true)

	t.Logf("✅ MITM proxy running on %s", proxy.Addr())

	// Create PTY helper
	pty, err := NewPTYTestHelper(t, proxy.Addr())
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

	t.Logf("⏳ Waiting for AUTH_WAITING state...")

	// Wait for AUTH_WAITING state
	if err := waitForState(service, StateAuthWaiting, 30*time.Second); err != nil {
		t.Fatalf("Never reached AUTH_WAITING state: %v", err)
	}

	// Get OAuth URL
	status := service.GetStatus()
	if status.OAuthURL == "" {
		t.Fatal("OAuth URL not extracted")
	}

	// Print instructions for user
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("📋 INTERACTIVE OAUTH CAPTURE")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("\n🔗 Please open this URL in your browser:")
	fmt.Println("\n" + status.OAuthURL)
	fmt.Println("\n📝 After authorizing, you'll see a code. Paste it here and press Enter:")
	fmt.Print("\n> ")

	// Read code from stdin
	reader := bufio.NewReader(os.Stdin)
	code, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read code: %v", err)
	}

	// Trim whitespace
	code = code[:len(code)-1] // Remove newline

	if code == "" {
		t.Fatal("No code entered")
	}

	fmt.Printf("\n✅ Got code (length %d): %s...\n\n", len(code), code[:min(len(code), 20)])
	t.Logf("📤 Submitting OAuth code: %s", code)

	// Submit the code
	if err := service.SubmitCode(code); err != nil {
		t.Fatalf("Failed to submit code: %v", err)
	}

	// Wait a bit for the OAuth exchange to happen
	t.Logf("⏳ Waiting for OAuth exchange...")
	time.Sleep(3 * time.Second)

	// Check state
	finalStatus := service.GetStatus()
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("📊 CAPTURE RESULTS")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("\n✅ Final state: %s\n", finalStatus.State)
	if finalStatus.ErrorMessage != "" {
		fmt.Printf("⚠️  Error: %s\n", finalStatus.ErrorMessage)
	}

	fmt.Println("\n💡 Check the test output above for the captured OAuth request/response!")
	fmt.Println("   Look for lines with 🎯 and 📥 symbols")
	fmt.Println("\n" + strings.Repeat("=", 80) + "\n")

	// Wait a bit more to capture any follow-up requests
	t.Logf("⏳ Waiting to capture any follow-up API calls...")
	time.Sleep(5 * time.Second)

	t.Logf("✅ Capture complete! Check logs above for OAuth details")
}
