package tui

import (
	"fmt"
	"strings"
	"time"
)

// TestClaudeEmulator tests the terminal emulator with Claude-like output
func TestClaudeEmulator() {
	fmt.Println("🧪 Testing Terminal Emulator with Claude-like output...")

	// Create emulator with standard size
	emulator := NewTerminalEmulator(80, 24)

	// Test 1: Basic Claude startup sequence
	fmt.Println("\n📤 Test 1: Claude startup sequence")
	startupSequence := []byte(
		"\x1b]0;Claude Code\x07" + // Terminal title
			"\x1b[2J\x1b[H" + // Clear screen, move cursor to home
			"Welcome to Claude Code!\n" +
			"\x1b[1m> \x1b[0m", // Bold prompt
	)
	emulator.Write(startupSequence)

	rendered := emulator.Render()
	fmt.Printf("Rendered output:\n%s\n", rendered)
	fmt.Printf("Length: %d bytes\n", len(rendered))

	// Test 2: Simulate alternate screen buffer (TUI mode)
	fmt.Println("\n📤 Test 2: Alternate screen buffer simulation")
	tuiSequence := []byte(
		"\x1b[?1049h" + // Enter alternate screen
			"\x1b[2J\x1b[H" + // Clear and home
			"┌─ Claude Code Session ─────────────────────────────────────┐\n" +
			"│                                                           │\n" +
			"│  \x1b[1mWhat would you like me to help you with?\x1b[0m            │\n" +
			"│                                                           │\n" +
			"│  > \x1b[32mType your request here...\x1b[0m                       │\n" +
			"│                                                           │\n" +
			"└───────────────────────────────────────────────────────────┘\n",
	)
	emulator.Write(tuiSequence)

	rendered = emulator.Render()
	fmt.Printf("TUI rendered output:\n%s\n", rendered)
	fmt.Printf("Length: %d bytes\n", len(rendered))

	// Test 3: Simulate user interaction and response
	fmt.Println("\n📤 Test 3: User interaction simulation")
	userInteraction := []byte(
		"\x1b[5;4H" + // Move cursor to input area
			"Fix the bug in main.go" + // User types
			"\r\n" + // Enter pressed
			"\x1b[7;2H" + // Move to response area
			"\x1b[1mAnalyzing main.go...\x1b[0m\n" +
			"I found the issue on line 42.\n" +
			"\x1b[32m✓ Fix applied\x1b[0m\n",
	)
	emulator.Write(userInteraction)

	rendered = emulator.Render()
	fmt.Printf("Interactive session output:\n%s\n", rendered)

	// Test 4: Test state persistence
	fmt.Println("\n📤 Test 4: State persistence test")

	// Get current state
	beforeState := emulator.Render()

	// Simulate some more output
	moreOutput := []byte(
		"\x1b[11;2H" + // Move cursor down
			"Additional context preserved.\n",
	)
	emulator.Write(moreOutput)

	afterState := emulator.Render()

	fmt.Printf("State changed: %t\n", beforeState != afterState)
	fmt.Printf("Final state length: %d bytes\n", len(afterState))

	// Test 5: Memory and performance characteristics
	fmt.Println("\n📤 Test 5: Performance test")
	start := time.Now()

	// Simulate lots of output
	for i := 0; i < 100; i++ {
		testOutput := []byte(fmt.Sprintf("Line %d with some content...\r\n", i))
		emulator.Write(testOutput)
	}

	finalRender := emulator.Render()
	elapsed := time.Since(start)

	fmt.Printf("Processed 100 lines in %v\n", elapsed)
	fmt.Printf("Final output lines: %d\n", strings.Count(finalRender, "\n"))

	fmt.Println("\n✅ Terminal emulator tests completed")
	return
}
