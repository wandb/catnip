package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/spf13/cobra"
)

// Version information
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "unknown"
)

// SetVersionInfo sets the version information from the main package
func SetVersionInfo(v, c, d, b string) {
	version = v
	commit = c
	date = d
	builtBy = b
}

var rootCmd = &cobra.Command{
	Use:   "catctrl",
	Short: "🐱 Catnip - Modern containerized coding environment",
	Long: `# 🐱 Catnip

**A modern CLI tool for managing containerized coding environments.**

## ✨ Features

- 🖥️  **Interactive TUI** for monitoring container status
- 📊 **Real-time logs** with search and filtering
- 🌐 **Port detection** and browser integration  
- 📁 **Git integration** with repository mounting
- ⚡ **Development mode** with optimized workflows

## 🚀 Getting Started

Run **catctrl run** to start a new container with an interactive TUI.

Use **catctrl run --help** for detailed options and examples.`,
	Version: version,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	
	// Add version command
	rootCmd.AddCommand(versionCmd)
	
	// Set custom help function to use Glow for beautiful markdown rendering
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		renderMarkdownHelp(cmd)
	})
}

// Version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Long:  "Display detailed version information including build date and commit.",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("catctrl version %s\n", version)
		if commit != "none" && commit != "unknown" && commit != "" {
			fmt.Printf("Git commit: %s\n", commit)
		}
		if date != "unknown" && date != "" {
			fmt.Printf("Built: %s\n", date)
		}
		if builtBy != "unknown" && builtBy != "" {
			fmt.Printf("Built by: %s\n", builtBy)
		}
	},
}

// renderMarkdownHelp renders command help using glamour for beautiful markdown display
func renderMarkdownHelp(cmd *cobra.Command) {
	// Create the help content
	var helpContent strings.Builder
	
	// Add the long description if available
	if cmd.Long != "" {
		helpContent.WriteString(cmd.Long)
		helpContent.WriteString("\n\n")
	} else if cmd.Short != "" {
		helpContent.WriteString("# " + cmd.Short)
		helpContent.WriteString("\n\n")
	}
	
	// Add usage
	helpContent.WriteString("## 📖 Usage\n\n")
	helpContent.WriteString("```bash\n")
	helpContent.WriteString(cmd.UseLine())
	helpContent.WriteString("\n```\n\n")
	
	// Add available commands
	if cmd.HasAvailableSubCommands() {
		helpContent.WriteString("## 🔧 Available Commands\n\n")
		for _, subCmd := range cmd.Commands() {
			if subCmd.IsAvailableCommand() {
				helpContent.WriteString(fmt.Sprintf("- **%s** - %s\n", subCmd.Name(), subCmd.Short))
			}
		}
		helpContent.WriteString("\n")
	}
	
	// Add flags
	if cmd.HasAvailableFlags() {
		helpContent.WriteString("## ⚙️  Flags\n\n")
		flagUsages := cmd.Flags().FlagUsages()
		if flagUsages != "" {
			helpContent.WriteString("```\n")
			helpContent.WriteString(flagUsages)
			helpContent.WriteString("```\n\n")
		}
	}
	
	// Add global flags if this is a subcommand
	if cmd.HasParent() && cmd.InheritedFlags().HasFlags() {
		helpContent.WriteString("## 🌐 Global Flags\n\n")
		inheritedUsages := cmd.InheritedFlags().FlagUsages()
		if inheritedUsages != "" {
			helpContent.WriteString("```\n")
			helpContent.WriteString(inheritedUsages)
			helpContent.WriteString("```\n\n")
		}
	}
	
	// Render with glamour
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)
	if err != nil {
		// Fallback to default help if glamour fails
		_ = cmd.Help()
		return
	}
	
	rendered, err := renderer.Render(helpContent.String())
	if err != nil {
		// Fallback to default help if rendering fails
		_ = cmd.Help()
		return
	}
	
	fmt.Print(rendered)
}