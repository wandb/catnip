package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var purrCmd = &cobra.Command{
	Use:   "purr [command...]",
	Short: "Execute command while intercepting terminal title changes",
	Long: `Execute a command while monitoring its output for terminal title escape sequences.
	
This command wraps another command and logs any terminal title changes to a file
for integration with development environments like Claude Code.

The title log format is: timestamp|pid|cwd|title

Environment variables:
- CATNIP_TITLE_LOG: Path to title log file (default: ~/.catnip/title_events.log)
- CATNIP_DISABLE_PTY_INTERCEPTOR: Set to "1" or "true" to disable interception`,
	Example: `  catnip purr claude --version
  catnip purr /home/vscode/.local/bin/claude-real chat
  CATNIP_TITLE_LOG=/tmp/titles.log catnip purr some-command`,
	Args: cobra.MinimumNArgs(1),
	RunE: runPurr,
}

func init() {
	rootCmd.AddCommand(purrCmd)
}

func runPurr(cmd *cobra.Command, args []string) error {
	// Check if interceptor is disabled
	if disabled := os.Getenv("CATNIP_DISABLE_PTY_INTERCEPTOR"); disabled == "1" || disabled == "true" {
		// Just execute the command directly
		return execDirect(args)
	}

	// Get title log path
	titleLogPath := os.Getenv("CATNIP_TITLE_LOG")
	if titleLogPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		titleLogPath = filepath.Join(homeDir, ".catnip", "title_events.log")
	}

	// Ensure log directory exists
	if err := os.MkdirAll(filepath.Dir(titleLogPath), 0755); err != nil {
		// Silent failure - continue without logging
	}

	// Execute command with title interception
	return execWithTitleInterception(args, titleLogPath)
}

func execDirect(args []string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func execWithTitleInterception(args []string, titleLogPath string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin

	// Create pipes for stdout and stderr
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	// Process stdout and stderr concurrently
	done := make(chan error, 2)

	go func() {
		done <- processOutput(stdoutPipe, os.Stdout, titleLogPath)
	}()

	go func() {
		done <- processOutput(stderrPipe, os.Stderr, titleLogPath)
	}()

	// Wait for both goroutines to complete
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			// Log error but don't fail the command
			fmt.Fprintf(os.Stderr, "purr: output processing error: %v\n", err)
		}
	}

	// Wait for the command to complete
	return cmd.Wait()
}

func processOutput(reader io.Reader, writer io.Writer, titleLogPath string) error {
	scanner := bufio.NewScanner(reader)

	// Compile regex for title sequences
	titleRegex := regexp.MustCompile(`\x1b\]0;([^\x07]*)\x07`)

	for scanner.Scan() {
		line := scanner.Text()

		// Write line to output
		fmt.Fprintln(writer, line)

		// Check for title sequences
		matches := titleRegex.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) > 1 {
				title := strings.TrimSpace(match[1])
				if title != "" {
					logTitleChange(titleLogPath, title)
				}
			}
		}
	}

	return scanner.Err()
}

func logTitleChange(titleLogPath, title string) {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "unknown"
	}

	// Create log entry
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	pid := os.Getpid()
	logEntry := fmt.Sprintf("%s|%d|%s|%s\n", timestamp, pid, cwd, title)

	// Append to log file (create if it doesn't exist)
	file, err := os.OpenFile(titleLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		// Silent failure
		return
	}
	defer file.Close()

	_, _ = file.WriteString(logEntry)
}
