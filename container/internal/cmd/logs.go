package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs [command...]",
	Short: "📋 Stream command output to console and log file",
	Long: `# 📋 Stream Command Logs

**Run any command while streaming output to both console and a timestamped log file.**

## ✨ Features

- 🖥️  **Dual output**: Display in console and save to file
- 📝 **Timestamped logs**: Auto-generated filenames with timestamps
- 🔗 **Latest symlink**: Always points to the most recent log
- ⚡ **Real-time streaming**: No buffering delays
- 🛑 **Signal handling**: Proper cleanup on interruption

## 📁 Log Files

Logs are saved to **/tmp/** directory:
- **/tmp/command-YYYYMMDD-HHMMSS.log** - Timestamped log file
- **/tmp/command-latest.log** - Symlink to latest log

## 💡 Examples

Stream wrangler dev logs:
` + "```bash\ncatnip logs wrangler dev\n```" + `

Stream build process:
` + "```bash\ncatnip logs pnpm run build\n```" + `

Tail the latest log (in another terminal):
` + "```bash\ntail -f /tmp/command-latest.log\n```",
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runWithLogging(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func runWithLogging(args []string) error {
	// Use /tmp for logs
	logsDir := "/tmp"

	// Generate timestamped log filename
	timestamp := time.Now().Format("20060102-150405")
	commandName := strings.ReplaceAll(args[0], "/", "-")
	logFile := filepath.Join(logsDir, fmt.Sprintf("%s-%s.log", commandName, timestamp))
	latestSymlink := filepath.Join(logsDir, fmt.Sprintf("%s-latest.log", commandName))

	// Create the log file
	file, err := os.Create(logFile)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	defer file.Close()

	// Create or update the latest symlink
	_ = os.Remove(latestSymlink) // Remove existing symlink (ignore errors)
	if err := os.Symlink(filepath.Base(logFile), latestSymlink); err != nil {
		// Not fatal, just warn
		fmt.Fprintf(os.Stderr, "Warning: failed to create symlink: %v\n", err)
	}

	fmt.Printf("📋 Logging to: %s\n", logFile)
	fmt.Printf("🔗 Latest link: %s\n", latestSymlink)
	fmt.Printf("🖥️  Command: %s\n", strings.Join(args, " "))
	fmt.Println("---")

	// Set up the command
	command := exec.Command(args[0], args[1:]...)

	// Create pipes for stdout and stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := command.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := command.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Channel to signal when scanning is done
	done := make(chan error, 1)

	// Handle stdout and stderr in separate goroutines
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Println(line) // Print to console
			if _, err := fmt.Fprintln(file, line); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing to log file: %v\n", err)
			}
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Fprintln(os.Stderr, line) // Print to console stderr
			if _, err := fmt.Fprintln(file, line); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing to log file: %v\n", err)
			}
		}
	}()

	// Wait for command completion or signal
	go func() {
		done <- command.Wait()
	}()

	select {
	case sig := <-sigChan:
		fmt.Printf("\n🛑 Received signal %v, terminating command...\n", sig)
		if command.Process != nil {
			_ = command.Process.Signal(sig)
		}
		return <-done
	case err := <-done:
		if err != nil {
			fmt.Printf("\n❌ Command exited with error: %v\n", err)
		} else {
			fmt.Printf("\n✅ Command completed successfully\n")
		}
		return err
	}
}

func init() {
	rootCmd.AddCommand(logsCmd)
}
