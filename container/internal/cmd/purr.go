package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/spf13/cobra"
	"golang.org/x/term"
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
	Args:                  cobra.MinimumNArgs(1),
	DisableFlagParsing:    true,
	DisableFlagsInUseLine: true,
	RunE:                  runPurr,
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
		return fmt.Errorf("failed to create log directory: %w", err)
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

	// Start the command with a PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("failed to start command with PTY: %w", err)
	}
	defer ptmx.Close()

	// Put stdin in raw mode to preserve terminal behavior
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to make stdin raw: %w", err)
	}
	defer func() {
		if err := term.Restore(int(os.Stdin.Fd()), oldState); err != nil {
			fmt.Fprintf(os.Stderr, "purr: failed to restore terminal: %v\n", err)
		}
	}()

	// Handle window size changes
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			_ = pty.InheritSize(os.Stdin, ptmx) // Ignore resize errors
		}
	}()
	ch <- syscall.SIGWINCH // Initial resize

	// Handle input/output concurrently
	done := make(chan error, 2)

	// Copy input from stdin to PTY in raw mode
	go func() {
		_, err := io.Copy(ptmx, os.Stdin)
		done <- err
	}()

	// Process output from PTY - transparent passthrough with title scanning
	go func() {
		done <- scanAndPassthrough(ptmx, os.Stdout, titleLogPath)
	}()

	// Wait for either goroutine to complete
	err = <-done
	if err != nil && err != io.EOF {
		fmt.Fprintf(os.Stderr, "purr: I/O error: %v\n", err)
	}

	// Wait for the command to complete
	return cmd.Wait()
}

func scanAndPassthrough(reader io.Reader, writer io.Writer, titleLogPath string) error {
	buf := make([]byte, 4096)
	titleRegex := regexp.MustCompile(`\x1b\]0;([^\x07]*)\x07`)

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			data := buf[:n]

			// Pass data through unchanged - preserve all formatting, colors, cursor positioning
			if _, writeErr := writer.Write(data); writeErr != nil {
				return fmt.Errorf("failed to write output: %w", writeErr)
			}

			// Scan for title sequences in the raw data
			matches := titleRegex.FindAllSubmatch(data, -1)
			for _, match := range matches {
				if len(match) > 1 {
					title := strings.TrimSpace(string(match[1]))
					if title != "" {
						logTitleChange(titleLogPath, title)
					}
				}
			}
		}

		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("failed to read PTY output: %w", err)
		}
	}
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
	file, err := os.OpenFile(titleLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		// Silent failure
		return
	}
	defer file.Close()

	_, _ = file.WriteString(logEntry)
}
