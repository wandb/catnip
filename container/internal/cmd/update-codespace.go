package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/vanpelt/catnip/internal/logger"
)

var updateCodespaceCmd = &cobra.Command{
	Use:    "update-codespace",
	Short:  "🔑 Update codespace credentials with worker",
	Hidden: true,
	Long: `Update codespace credentials with the Catnip worker.

This command sends the current GitHub token, username, and codespace name
to the Catnip worker so it can authenticate properly for health checks
and other codespace operations.

This is automatically called on codespace startup but can be run manually
if credentials need to be refreshed.`,
	Run: func(cmd *cobra.Command, args []string) {
		updateCodespaceCredentials()
	},
}

func init() {
	rootCmd.AddCommand(updateCodespaceCmd)
}

// updateCodespaceCredentials sends GitHub credentials to the Catnip worker
func updateCodespaceCredentials() {
	githubToken := os.Getenv("GITHUB_TOKEN")
	githubUser := os.Getenv("GITHUB_USER")
	codespaceName := os.Getenv("CODESPACE_NAME")
	githubRepository := os.Getenv("GITHUB_REPOSITORY")

	// Only proceed if we have all required environment variables (indicating we're in a codespace)
	if githubToken == "" || githubUser == "" || codespaceName == "" {
		logger.Debugf("Not in a codespace environment, skipping credential update")
		logger.Debugf("GITHUB_TOKEN: %t, GITHUB_USER: %t, CODESPACE_NAME: %t, GITHUB_REPOSITORY: %t",
			githubToken != "", githubUser != "", codespaceName != "", githubRepository != "")
		return
	}

	logger.Infof("🔑 Updating codespace credentials with worker...")

	// Prepare the payload
	payload := map[string]string{
		"GITHUB_TOKEN":      githubToken,
		"GITHUB_USER":       githubUser,
		"CODESPACE_NAME":    codespaceName,
		"GITHUB_REPOSITORY": githubRepository,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		logger.Errorf("❌ Failed to marshal codespace credentials: %v", err)
		return
	}

	// Determine the worker URL - default to production, or use CATNIP_PROXY if set
	workerURL := "https://catnip.run"
	if proxy := os.Getenv("CATNIP_PROXY"); proxy != "" {
		workerURL = proxy
	}

	endpoint := workerURL + "/v1/auth/github/codespace"

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 15 * time.Second, // Longer timeout for startup calls
	}

	// Make the request
	resp, err := client.Post(endpoint, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		logger.Errorf("❌ Failed to send codespace credentials to worker: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		logger.Infof("✅ Codespace credentials updated successfully")
	} else {
		logger.Errorf("❌ Worker rejected codespace credentials (HTTP %d)", resp.StatusCode)
	}
}
