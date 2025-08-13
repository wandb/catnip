package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vanpelt/catnip/internal/tui"
)

var notifyCmd = &cobra.Command{
	Use:    "notify",
	Short:  "📢 Send a native notification",
	Hidden: true, // Hidden from help output - internal testing command
	Long: `# 📢 Notify Command

Send a native macOS notification to test notification functionality.

This command is useful for:
- Testing notification permissions
- Verifying app bundle setup
- Debugging notification issues

## 🎯 Examples

Send a basic notification:
`,
	Example: `  # Basic notification
  catnip notify "Hello" "This is a test notification"
  
  # Notification with subtitle
  catnip notify "Build Complete" "Your project built successfully" "Success"
  
  # Test notification permissions
  catnip notify "Permission Test" "Testing native notifications"`,
	Args: cobra.RangeArgs(2, 3),
	Run: func(cmd *cobra.Command, args []string) {
		if !tui.IsNotificationSupported() {
			fmt.Println("❌ Notifications are not supported on this platform")
			return
		}

		title := args[0]
		body := args[1]
		subtitle := ""

		if len(args) > 2 {
			subtitle = args[2]
		}

		fmt.Printf("📢 Sending notification: %s\n", title)

		fmt.Println("🔄 Waiting 10 seconds for notification display and clicks...")
		err := tui.SendNativeNotification(title, body, subtitle, "")
		if err != nil {
			fmt.Printf("❌ Failed to send notification: %v\n", err)
			return
		}

		fmt.Println("✅ Notification sent successfully!")
		fmt.Println("💡 The \"Show\" button should now work for this and any historical notifications")

		if !tui.HasNotificationPermission() {
			fmt.Println("⚠️  Note: If you don't see the notification, check that you granted permission when prompted.")
		}
	},
}

func init() {
	rootCmd.AddCommand(notifyCmd)
}
