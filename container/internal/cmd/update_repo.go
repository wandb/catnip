package cmd

import (
	"github.com/spf13/cobra"
	"github.com/vanpelt/catnip/internal/logger"
	"github.com/vanpelt/catnip/internal/services"
)

var updateRepoCmd = &cobra.Command{
	Use:   "update-repo",
	Short: "🔄 Update mounted codespace repository to latest default branch",
	Long: `Update the mounted codespace repository to the latest default branch.

This command updates the repository at /workspaces/$REPO (determined from GITHUB_REPOSITORY)
to the latest commits from the remote default branch.

Behavior:
- If on default branch with changes: stashes → pulls → pops stash
- If on default branch without changes: pulls latest
- If on different branch: switches to default → pulls → switches back

The pull uses --ff-only to prevent merge commits. If a fast-forward is not possible,
the command will fail and you'll need to manually resolve.`,
	Run: func(cmd *cobra.Command, args []string) {
		updateMountedRepository()
	},
}

func init() {
	rootCmd.AddCommand(updateRepoCmd)
}

// updateMountedRepository updates the mounted codespace repository
func updateMountedRepository() {
	// Initialize Git service
	gitService := services.NewGitService()
	defer gitService.Stop()

	// Attempt to update the repository
	if err := gitService.UpdateMountedRepo(); err != nil {
		logger.Errorf("❌ Failed to update repository: %v", err)
		return
	}
}
