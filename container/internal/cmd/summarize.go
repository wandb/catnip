package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vanpelt/catnip/internal/logger"
	"github.com/vanpelt/catnip/internal/services"
)

var summarizeCmd = &cobra.Command{
	Use:    "summarize [prompt]",
	Short:  "🧠 Generate task summary and branch name",
	Hidden: true,
	Long: `Generate a task summary and git branch name using local inference.

This command uses the local Gemma 270M model to generate:
- A concise 2-4 word task summary (Title Case)
- A git branch name (kebab-case with category prefix)

The prompt can be provided as arguments or via the --prompt flag.

Examples:
  catnip summarize "Add user authentication with OAuth2"
  catnip summarize --prompt "Fix login bug on mobile devices"
  catnip summarize Add dark mode toggle to settings`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSummarize(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(summarizeCmd)

	// Add flags
	summarizeCmd.Flags().StringP("prompt", "p", "", "Task description to summarize")
}

func runSummarize(cmd *cobra.Command, args []string) error {
	// Configure logging (quieter for CLI usage)
	logger.Configure(logger.LevelWarn, true)

	// Get prompt from flag or args
	promptFlag, _ := cmd.Flags().GetString("prompt")
	var prompt string

	if promptFlag != "" {
		prompt = promptFlag
	} else if len(args) > 0 {
		prompt = strings.Join(args, " ")
	} else {
		return fmt.Errorf("prompt required: provide via arguments or --prompt flag")
	}

	fmt.Printf("🧠 Generating summary for: %s\n\n", prompt)

	// Initialize inference service
	inferenceConfig := services.InferenceConfig{
		ModelURL: "https://huggingface.co/vanpelt/catnip-summarizer/resolve/main/gemma3-270m-summarizer-Q4_K_M.gguf",
		Checksum: "",
	}

	inferenceService, err := services.NewInferenceService(inferenceConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize inference service: %w\n\nTry running: catnip download", err)
	}

	// Run inference
	result, err := inferenceService.Summarize(prompt)
	if err != nil {
		return fmt.Errorf("inference failed: %w", err)
	}

	// Print results
	fmt.Println("📝 Summary:")
	fmt.Printf("   %s\n\n", result.Summary)
	fmt.Println("🌿 Branch name:")
	fmt.Printf("   %s\n", result.BranchName)

	return nil
}
