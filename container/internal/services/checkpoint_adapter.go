package services

import "github.com/vanpelt/catnip/internal/git"

// GitServiceAdapter adapts GitService to implement git.GitService interface
type GitServiceAdapter struct {
	*GitService
}

// NewGitServiceAdapter creates a new adapter
func NewGitServiceAdapter(gs *GitService) *GitServiceAdapter {
	return &GitServiceAdapter{GitService: gs}
}

// GitAddCommitGetHash implements git.Service interface
func (a *GitServiceAdapter) GitAddCommitGetHash(workDir, title string) (string, error) {
	return a.GitService.GitAddCommitGetHash(workDir, title)
}

// SessionServiceAdapter adapts SessionService to implement git.SessionServiceInterface interface
type SessionServiceAdapter struct {
	*SessionService
}

// NewSessionServiceAdapter creates a new adapter
func NewSessionServiceAdapter(ss *SessionService) *SessionServiceAdapter {
	return &SessionServiceAdapter{SessionService: ss}
}

// AddToSessionHistory implements git.SessionServiceInterface interface
func (a *SessionServiceAdapter) AddToSessionHistory(workDir, title, commitHash string) error {
	return a.SessionService.AddToSessionHistory(workDir, title, commitHash)
}

// GetActiveSession implements git.SessionServiceInterface interface
func (a *SessionServiceAdapter) GetActiveSession(workDir string) (interface{}, bool) {
	sessionInfo, exists := a.SessionService.GetActiveSession(workDir)
	if !exists {
		return nil, false
	}

	// Convert to a map structure that the checkpoint manager expects
	result := make(map[string]interface{})
	if sessionInfo.Title != nil {
		result["title"] = map[string]interface{}{
			"title": sessionInfo.Title.Title,
		}
	}

	return result, true
}
