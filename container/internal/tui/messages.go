package tui

import "time"

// Core message types
type tickMsg time.Time
type animationTickMsg time.Time
type logsTickMsg time.Time
type quitMsg struct{}

// Data fetch messages
type containerInfoMsg map[string]interface{}
type repositoryInfoMsg map[string]interface{}
type containerReposMsg map[string]interface{}
type logsMsg []string
type portsMsg []string
type errMsg error
type healthStatusMsg bool
type workspacesMsg []WorkspaceInfo

// Shell-related messages
type shellOutputMsg struct {
	sessionID string
	data      []byte
}
type shellErrorMsg struct {
	sessionID string
	err       error
}
type shellConnectedMsg struct {
	sessionID string
}

// SSE event messages
type sseConnectedMsg struct{}
type sseDisconnectedMsg struct{}
type sseErrorMsg struct {
	err error
}
type ssePortOpenedMsg struct {
	port     int
	service  string
	title    string
	protocol string
}
type ssePortClosedMsg struct {
	port int
}
type sseContainerStatusMsg struct {
	status  string
	message string
}
type sseWorktreeUpdatedMsg struct{}
