package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

// CaptureMetadata matches the Swift MockPTYDataSource structure
type CaptureMetadata struct {
	CaptureDate     time.Time      `json:"captureDate"`
	TotalBytes      int            `json:"totalBytes"`
	DurationSeconds float64        `json:"durationSeconds"`
	Events          []CaptureEvent `json:"events"`
}

// CaptureEvent matches the Swift MockPTYDataSource structure
type CaptureEvent struct {
	TimestampMs int    `json:"timestampMs"`
	Data        []byte `json:"data"`
}

// Terminal dimensions presets
const (
	// Portrait mode (minimum for Claude TUI from TerminalView.swift)
	portraitCols = 65
	portraitRows = 15

	// Landscape mode (wider for landscape views)
	landscapeCols = 120
	landscapeRows = 30
)

// findClaude looks for the claude executable in common locations
func findClaude() string {
	// Try PATH first
	if path, err := exec.LookPath("claude"); err == nil {
		return path
	}

	// Try common installation locations
	homeDir := os.Getenv("HOME")
	candidates := []string{
		homeDir + "/.claude/local/claude",
		"/usr/local/bin/claude",
		"/opt/homebrew/bin/claude",
	}

	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			// Check if executable
			if info.Mode()&0111 != 0 {
				return candidate
			}
		}
	}

	return ""
}

func main() {
	outputFile := flag.String("output", "pty-capture.json", "Output JSON file for captured PTY data")
	landscape := flag.Bool("landscape", false, "Use landscape dimensions (120x30) instead of portrait (65x15)")
	flag.Parse()

	// Determine terminal size
	cols := portraitCols
	rows := portraitRows
	orientation := "portrait"
	if *landscape {
		cols = landscapeCols
		rows = landscapeRows
		orientation = "landscape"
	}

	fmt.Printf("🎬 Interactive PTY Capture Tool\n")
	fmt.Printf("📝 Output file: %s\n", *outputFile)
	fmt.Printf("📐 Dimensions: %dx%d (%s)\n", cols, rows, orientation)
	fmt.Println()

	// Find claude executable - check common locations
	claudePath := findClaude()
	if claudePath == "" {
		fmt.Fprintf(os.Stderr, "❌ claude command not found\n")
		fmt.Fprintf(os.Stderr, "   Tried:\n")
		fmt.Fprintf(os.Stderr, "     - PATH\n")
		fmt.Fprintf(os.Stderr, "     - ~/.claude/local/claude\n")
		fmt.Fprintf(os.Stderr, "     - /usr/local/bin/claude\n")
		fmt.Fprintf(os.Stderr, "     - /opt/homebrew/bin/claude\n")
		os.Exit(1)
	}

	fmt.Printf("✅ Using claude at: %s\n", claudePath)
	fmt.Printf("✅ Using your real ~/.claude config\n")

	// Start Claude with your real home directory and config
	cmd := exec.Command(claudePath)
	cmd.Env = os.Environ() // Use your real environment
	cmd.Dir = os.Getenv("HOME")

	// Start PTY
	ptyFile, err := pty.Start(cmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to start PTY: %v\n", err)
		os.Exit(1)
	}

	// Set terminal size to match our target dimensions
	winsize := &pty.Winsize{
		Rows: uint16(rows), // #nosec G115 - rows is a constant
		Cols: uint16(cols), // #nosec G115 - cols is a constant
	}
	if err := pty.Setsize(ptyFile, winsize); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Warning: Failed to set terminal size: %v\n", err)
	} else {
		fmt.Printf("✅ Set terminal size to %dx%d\n", cols, rows)
	}

	fmt.Println()
	fmt.Println("🎮 Interactive Mode - Use Claude normally!")
	fmt.Println("   • Type commands, interact with the TUI")
	fmt.Println("   • Everything you see is being recorded")
	fmt.Println("   • Press Ctrl+C when done to save")
	fmt.Println()

	// Put stdin into raw mode for interactive TTY
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to set raw mode: %v\n", err)
		os.Exit(1)
	}

	// Capture metadata
	startTime := time.Now()
	var events []CaptureEvent
	totalBytes := 0

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Channels for coordination
	done := make(chan struct{})
	stdinDone := make(chan struct{})
	ptyDone := make(chan struct{})

	// Track Ctrl+C presses
	ctrlCCount := 0
	var lastCtrlC time.Time

	// Copy stdin to PTY (user input -> Claude)
	go func() {
		defer close(stdinDone)
		buf := make([]byte, 1024)
		for {
			select {
			case <-done:
				return
			default:
				n, err := os.Stdin.Read(buf)
				if err != nil {
					return
				}
				if n > 0 {
					// Check for Ctrl+C (0x03)
					for i := 0; i < n; i++ {
						if buf[i] == 0x03 {
							now := time.Now()
							// Reset count if more than 2 seconds since last Ctrl+C
							if now.Sub(lastCtrlC) > 2*time.Second {
								ctrlCCount = 0
							}
							ctrlCCount++
							lastCtrlC = now

							// On second Ctrl+C within 2 seconds, exit
							if ctrlCCount >= 2 {
								fmt.Fprintf(os.Stderr, "\n\n🛑 Ctrl+C detected twice, stopping capture...\n")
								sigChan <- os.Interrupt
								return
							}

							// First Ctrl+C: show message but let it pass through
							fmt.Fprintf(os.Stderr, "\n⚠️  Press Ctrl+C again to stop recording\n")
						}
					}

					_, err := ptyFile.Write(buf[:n])
					if err != nil {
						return
					}
				}
			}
		}
	}()

	// Copy PTY to stdout AND capture (Claude output -> user + recording)
	go func() {
		defer close(ptyDone)
		buf := make([]byte, 8192)
		for {
			n, err := ptyFile.Read(buf)
			if err != nil {
				return
			}
			if n > 0 {
				// Write to stdout so user sees it
				_, _ = os.Stdout.Write(buf[:n]) // Ignore write errors to stdout

				// Capture for recording
				elapsed := time.Since(startTime)
				timestampMs := int(elapsed.Milliseconds())

				data := make([]byte, n)
				copy(data, buf[:n])

				event := CaptureEvent{
					TimestampMs: timestampMs,
					Data:        data,
				}
				events = append(events, event)
				totalBytes += n
			}
		}
	}()

	// Wait for interrupt signal
	<-sigChan

	// Signal goroutines to stop
	close(done)

	// Restore terminal immediately
	_ = term.Restore(int(os.Stdin.Fd()), oldState) // Best effort restore

	// Kill the Claude process
	if cmd.Process != nil {
		_ = cmd.Process.Kill() // Best effort kill
	}

	// Close PTY file
	ptyFile.Close()

	// Wait briefly for goroutines to finish
	timeout := time.After(500 * time.Millisecond)
	select {
	case <-stdinDone:
	case <-timeout:
	}
	select {
	case <-ptyDone:
	case <-timeout:
	}

	fmt.Println("\n\n🛑 Recording stopped")
	fmt.Println()

	// Create metadata
	duration := time.Since(startTime)
	metadata := CaptureMetadata{
		CaptureDate:     startTime,
		TotalBytes:      totalBytes,
		DurationSeconds: duration.Seconds(),
		Events:          events,
	}

	// Save to file
	fmt.Printf("💾 Saving capture to %s...\n", *outputFile)
	file, err := os.Create(*outputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to create output file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(metadata); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to encode JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ PTY capture saved successfully!")
	fmt.Printf("📊 Summary:\n")
	fmt.Printf("   - Dimensions: %dx%d (%s)\n", cols, rows, orientation)
	fmt.Printf("   - Total bytes: %d\n", totalBytes)
	fmt.Printf("   - Events: %d\n", len(events))
	fmt.Printf("   - Duration: %.2fs\n", duration.Seconds())
	fmt.Println()
	fmt.Printf("🎯 To use in Xcode:\n")
	fmt.Printf("   1. cp %s ../xcode/catnip/PTYCapture/\n", *outputFile)
	fmt.Printf("   2. Add to Xcode project (if not already)\n")
	fmt.Printf("   3. Rebuild and view canvas!\n")
}
