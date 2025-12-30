package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vanpelt/catnip/internal/config"
	"github.com/vanpelt/catnip/internal/logger"
)

// ClaudeSettings represents Claude Code settings structure
type ClaudeSettings struct {
	Hooks map[string][]HookMatcher `json:"hooks,omitempty"`
}

type HookMatcher struct {
	Matcher string     `json:"matcher"`
	Hooks   []HookSpec `json:"hooks"`
}

type HookSpec struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// ClaudeHookEvent represents Claude hook event payload
type ClaudeHookEvent struct {
	HookEventName string `json:"hook_event_name"`
	CWD           string `json:"cwd"`
}

// CatnipHookPayload represents Catnip hook API payload
type CatnipHookPayload struct {
	EventType        string `json:"event_type"`
	WorkingDirectory string `json:"working_directory"`
}

var installHooksCmd = &cobra.Command{
	Use:   "install-hooks",
	Short: "Install Claude Code hooks for activity tracking",
	Long: `# 🔧 Install Claude Code Hooks

Install and configure Claude Code hooks to enable enhanced activity tracking in catnip.

This command will:
- Create the Claude settings directory if it doesn't exist
- Configure Claude Code to send activity events to catnip
- Set up hooks for UserPromptSubmit, PostToolUse, and Stop events

## ✨ Features
- **Automatic port detection** - No manual configuration needed
- **Safe installation** - Backs up existing settings  
- **Cross-platform** - Works on any system with catnip installed`,
	Example: `  # Install hooks with default settings
  catnip install-hooks

  # Install hooks with custom catnip host
  CATNIP_HOST=myserver:6369 catnip install-hooks

  # Install hooks and show verbose output
  catnip install-hooks --verbose`,
	RunE: runInstallHooks,
}

var hookCmd = &cobra.Command{
	Use:    "hook",
	Short:  "Process Claude Code hook events (internal use)",
	Hidden: true,
	Long: `# 🪝 Process Claude Code Hook Events

This command processes hook events from Claude Code and forwards them to the catnip server.

**Note:** This command is primarily intended for internal use by Claude Code hooks.
It reads JSON event data from stdin and sends it to the catnip server for activity tracking.

## 📋 Supported Events
- **UserPromptSubmit** - User submitted a prompt to Claude
- **PostToolUse** - Claude finished using a tool
- **Stop** - Claude finished generating a response`,
	Example: `  # Process a hook event (typically called by Claude Code)
  echo '{"hook_event_name":"UserPromptSubmit","cwd":"/path/to/project"}' | catnip hook

  # Test hook processing
  catnip hook < event.json`,
	RunE: runHook,
}

var verboseHooks bool

func init() {
	installHooksCmd.Flags().BoolVarP(&verboseHooks, "verbose", "v", false, "Show verbose output")

	rootCmd.AddCommand(installHooksCmd)
	rootCmd.AddCommand(hookCmd)
}

func runInstallHooks(cmd *cobra.Command, args []string) error {
	if verboseHooks {
		logger.Info("🔧 Installing Claude Code hooks for activity tracking...")
	}

	// Get Claude directory (respects XDG_CONFIG_HOME on Linux)
	claudeDir := config.Runtime.ClaudeConfigDir
	settingsFile := filepath.Join(claudeDir, "settings.json")

	// Create Claude config directory if it doesn't exist
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("failed to create Claude config directory: %w", err)
	}

	if verboseHooks {
		logger.Infof("📁 Using Claude directory: %s", claudeDir)
	}

	// Read existing settings or create new ones
	var settings ClaudeSettings
	if _, err := os.Stat(settingsFile); err == nil {
		if verboseHooks {
			logger.Info("📝 Found existing settings.json, backing up...")
		}

		// Backup existing settings
		backupFile := settingsFile + ".backup." + time.Now().Format("20060102-150405")
		if err := copyFile(settingsFile, backupFile); err != nil {
			return fmt.Errorf("failed to backup existing settings: %w", err)
		}

		// Read existing settings
		data, err := os.ReadFile(settingsFile)
		if err != nil {
			return fmt.Errorf("failed to read existing settings: %w", err)
		}

		if err := json.Unmarshal(data, &settings); err != nil {
			if verboseHooks {
				logger.Warn("⚠️  Could not parse existing settings, creating new ones")
			}
			settings = ClaudeSettings{}
		}
	} else {
		if verboseHooks {
			logger.Info("📝 Creating new settings.json...")
		}
		settings = ClaudeSettings{}
	}

	// Get catnip binary path
	catnipPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get catnip binary path: %w", err)
	}

	// Configure hooks
	if settings.Hooks == nil {
		settings.Hooks = make(map[string][]HookMatcher)
	}

	hookCommand := catnipPath + " hook"

	// Define the hook events we want to track
	events := []string{"SessionStart", "UserPromptSubmit", "PostToolUse", "Stop"}

	for _, event := range events {
		settings.Hooks[event] = []HookMatcher{
			{
				Matcher: "*",
				Hooks: []HookSpec{
					{
						Type:    "command",
						Command: hookCommand,
					},
				},
			},
		}
	}

	// Write updated settings
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(settingsFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write settings file: %w", err)
	}

	// Success!
	fmt.Println("✅ Claude hooks installed successfully!")
	fmt.Println("")
	fmt.Printf("📝 Settings configured in: %s\n", settingsFile)
	fmt.Printf("🪝 Hook command: %s\n", hookCommand)
	fmt.Println("")
	fmt.Println("🚀 Claude Code will now send activity events to catnip for improved status tracking")

	if verboseHooks {
		fmt.Println("")
		fmt.Println("💡 To customize the catnip server address, set the CATNIP_HOST environment variable:")
		fmt.Println("   export CATNIP_HOST=your-server:6369")
		fmt.Println("")
		fmt.Println("🔍 To verify the installation:")
		fmt.Printf("   - Settings file: cat '%s'\n", settingsFile)
		fmt.Printf("   - Hook command: %s --help\n", hookCommand)
	}

	return nil
}

func runHook(cmd *cobra.Command, args []string) error {
	// Read JSON input from stdin
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	if len(input) == 0 {
		// No input, exit silently (this is normal for some hook calls)
		return nil
	}

	// Parse the Claude hook event
	var event ClaudeHookEvent
	if err := json.Unmarshal(input, &event); err != nil {
		// Invalid JSON, exit silently to avoid breaking Claude
		return nil
	}

	// Only handle the events we care about for activity tracking
	switch event.HookEventName {
	case "SessionStart", "UserPromptSubmit", "PostToolUse", "Stop":
		// Good, we want to track these events
	default:
		// For other events, exit silently
		return nil
	}

	// Exit if we don't have required fields
	if event.HookEventName == "" || event.CWD == "" {
		return nil
	}

	// Get catnip server address
	catnipHost := os.Getenv("CATNIP_HOST")
	if catnipHost == "" {
		// Try to detect if we're running in a container or codespace
		if isInContainer() {
			catnipHost = "localhost:6369"
		} else {
			catnipHost = "localhost:6369"
		}
	}

	// Build the JSON payload for catnip
	payload := CatnipHookPayload{
		EventType:        event.HookEventName,
		WorkingDirectory: event.CWD,
	}

	payloadData, err := json.Marshal(payload)
	if err != nil {
		// JSON marshal failed, exit silently to avoid breaking Claude
		return nil
	}

	// Send the hook event to catnip server
	url := fmt.Sprintf("http://%s/v1/claude/hooks", catnipHost)
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadData))
	if err != nil {
		// Request creation failed, exit silently
		return nil
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		// Request failed, exit silently to avoid breaking Claude
		return nil
	}
	defer resp.Body.Close()

	// Read response to avoid connection leaks, but don't check status
	// We exit successfully regardless to avoid breaking Claude
	_, _ = io.ReadAll(resp.Body)

	return nil
}

// isInContainer detects if we're running inside a container
func isInContainer() bool {
	// Check for container environment indicators
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	// Check for codespace environment
	if os.Getenv("CODESPACES") == "true" {
		return true
	}

	// Check cgroup for container indicators
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		content := string(data)
		if strings.Contains(content, "docker") ||
			strings.Contains(content, "containerd") ||
			strings.Contains(content, "kubepods") {
			return true
		}
	}

	return false
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
