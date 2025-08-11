package main

import (
	"context"
	"fmt"

	"github.com/vanpelt/catnip/internal/models"
	"github.com/vanpelt/catnip/internal/services"
)

// ClaudeDesktopService wraps the existing Claude service for Wails exposure
type ClaudeDesktopService struct {
	claude *services.ClaudeService
}

// GetWorktreeSessionSummary gets session summary for a specific worktree
func (c *ClaudeDesktopService) GetWorktreeSessionSummary(worktreePath string) (*models.ClaudeSessionSummary, error) {
	return c.claude.GetWorktreeSessionSummary(worktreePath)
}

// GetAllWorktreeSessionSummaries gets all session summaries
func (c *ClaudeDesktopService) GetAllWorktreeSessionSummaries() (map[string]*models.ClaudeSessionSummary, error) {
	return c.claude.GetAllWorktreeSessionSummaries()
}

// GetFullSessionData gets complete session data with messages
func (c *ClaudeDesktopService) GetFullSessionData(worktreePath string, includeFullData bool) (*models.FullSessionData, error) {
	return c.claude.GetFullSessionData(worktreePath, includeFullData)
}

// GetLatestTodos gets the most recent todos from a session
func (c *ClaudeDesktopService) GetLatestTodos(worktreePath string) ([]models.Todo, error) {
	return c.claude.GetLatestTodos(worktreePath)
}

// CreateCompletion creates a completion request to Claude
func (c *ClaudeDesktopService) CreateCompletion(ctx context.Context, req *models.CreateCompletionRequest) (*models.CreateCompletionResponse, error) {
	return c.claude.CreateCompletion(ctx, req)
}

// GetClaudeSettings gets current Claude settings
func (c *ClaudeDesktopService) GetClaudeSettings() (*models.ClaudeSettings, error) {
	return c.claude.GetClaudeSettings()
}

// UpdateClaudeSettings updates Claude settings
func (c *ClaudeDesktopService) UpdateClaudeSettings(req *models.ClaudeSettingsUpdateRequest) (*models.ClaudeSettings, error) {
	return c.claude.UpdateClaudeSettings(req)
}

// GitDesktopService wraps the existing Git service for Wails exposure
type GitDesktopService struct {
	git *services.GitService
}

// GetAllWorktrees gets all git worktrees
func (g *GitDesktopService) GetAllWorktrees() ([]*models.Worktree, error) {
	return g.git.GetAllWorktrees()
}

// GetWorktree gets a specific worktree by ID
func (g *GitDesktopService) GetWorktree(worktreeID string) (*models.Worktree, error) {
	return g.git.GetWorktree(worktreeID)
}

// GetGitStatus gets overall git status
func (g *GitDesktopService) GetGitStatus() (*models.GitStatus, error) {
	return g.git.GetGitStatus()
}

// CreateWorktree creates a new git worktree
func (g *GitDesktopService) CreateWorktree(repoID, branch, directory string) (*models.Worktree, error) {
	return g.git.CreateWorktree(repoID, branch, directory)
}

// DeleteWorktree deletes a git worktree
func (g *GitDesktopService) DeleteWorktree(worktreeID string) error {
	return g.git.DeleteWorktree(worktreeID)
}

// GetRepositories gets all repositories
func (g *GitDesktopService) GetRepositories() ([]*models.Repository, error) {
	return g.git.GetRepositories()
}

// SessionDesktopService wraps the existing Session service for Wails exposure
type SessionDesktopService struct {
	session *services.SessionService
}

// StartActiveSession starts an active session
func (s *SessionDesktopService) StartActiveSession(workspaceDir, claudeSessionUUID string) error {
	return s.session.StartActiveSession(workspaceDir, claudeSessionUUID)
}

// GetActiveSession gets current active session
func (s *SessionDesktopService) GetActiveSession(workspaceDir string) (*models.ActiveSessionInfo, bool) {
	return s.session.GetActiveSession(workspaceDir)
}

// UpdateSessionTitle updates session title
func (s *SessionDesktopService) UpdateSessionTitle(workspaceDir, title, commitHash string) error {
	return s.session.UpdateSessionTitle(workspaceDir, title, commitHash)
}

// GetClaudeActivityState gets Claude activity state for a directory
func (s *SessionDesktopService) GetClaudeActivityState(workDir string) models.ClaudeActivityState {
	return s.session.GetClaudeActivityState(workDir)
}

// SettingsDesktopService manages desktop-specific settings
type SettingsDesktopService struct{}

// AppSettings represents desktop app settings
type AppSettings struct {
	Theme              string `json:"theme"`              // "light", "dark", "system"
	WindowPosition     Point  `json:"windowPosition"`     // Last window position
	WindowSize         Size   `json:"windowSize"`         // Last window size
	AutoStart          bool   `json:"autoStart"`          // Start on system boot
	MinimizeToTray     bool   `json:"minimizeToTray"`     // Minimize to system tray
	CloseToTray        bool   `json:"closeToTray"`        // Close to system tray
	ShowNotifications  bool   `json:"showNotifications"`  // Show desktop notifications
	DefaultProjectPath string `json:"defaultProjectPath"` // Default path for new projects
}

type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type Size struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// GetAppSettings gets current desktop app settings
func (s *SettingsDesktopService) GetAppSettings() (*AppSettings, error) {
	// For now, return default settings
	// In a full implementation, this would load from a config file
	return &AppSettings{
		Theme:              "system",
		WindowPosition:     Point{X: 100, Y: 100},
		WindowSize:         Size{Width: 1400, Height: 900},
		AutoStart:          false,
		MinimizeToTray:     true,
		CloseToTray:        false,
		ShowNotifications:  true,
		DefaultProjectPath: "",
	}, nil
}

// UpdateAppSettings updates desktop app settings
func (s *SettingsDesktopService) UpdateAppSettings(settings *AppSettings) error {
	// In a full implementation, this would save to a config file
	return nil
}

// GetAppInfo gets basic app information
func (s *SettingsDesktopService) GetAppInfo() map[string]interface{} {
	return map[string]interface{}{
		"name":        "Catnip Desktop",
		"version":     "1.0.0",
		"description": "Agentic Coding Environment - Desktop Edition",
	}
}
