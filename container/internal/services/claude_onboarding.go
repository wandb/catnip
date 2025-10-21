package services

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/vanpelt/catnip/internal/logger"
)

// OnboardingState represents the current state in the Claude onboarding flow
type OnboardingState string

// Onboarding state constants representing each step in the Claude Code onboarding flow
const (
	StateIdle          OnboardingState = "idle"
	StateThemeSelect   OnboardingState = "theme_select"
	StateAuthMethod    OnboardingState = "auth_method"
	StateAuthURL       OnboardingState = "auth_url"
	StateAuthWaiting   OnboardingState = "auth_waiting"
	StateAuthConfirm   OnboardingState = "auth_confirm"
	StateSecurityNotes OnboardingState = "security_notes"
	StateTerminalSetup OnboardingState = "terminal_setup"
	StateComplete      OnboardingState = "complete"
	StateError         OnboardingState = "error"
)

// OnboardingStatus represents the current status of the onboarding process
type OnboardingStatus struct {
	State        OnboardingState `json:"state"`
	OAuthURL     string          `json:"oauth_url,omitempty"`
	Message      string          `json:"message,omitempty"`
	ErrorMessage string          `json:"error_message,omitempty"`
	Output       string          `json:"output,omitempty"`
}

// ClaudeOnboardingService manages the automated Claude Code onboarding flow
type ClaudeOnboardingService struct {
	mu               sync.RWMutex
	currentState     OnboardingState
	previousState    OnboardingState
	sessionID        string
	ptyCmd           *exec.Cmd
	ptyFile          *os.File
	outputBuffer     *bytes.Buffer
	oauthURL         string
	errorMessage     string
	stopChan         chan struct{}
	stateEnteredAt   time.Time
	isRunning        bool
	codeSubmitted    bool
	pendingCodeInput string
}

// NewClaudeOnboardingService creates a new onboarding service instance
func NewClaudeOnboardingService() *ClaudeOnboardingService {
	return &ClaudeOnboardingService{
		currentState: StateIdle,
		outputBuffer: &bytes.Buffer{},
		stopChan:     make(chan struct{}),
	}
}

// Start begins the onboarding process by spawning claude in a PTY or using an existing one
func (s *ClaudeOnboardingService) Start() error {
	return s.StartWithPTY(nil, nil, "")
}

// StartWithPTY begins onboarding with an optional existing PTY session
// If ptyFile and cmd are provided, uses them instead of creating new ones
func (s *ClaudeOnboardingService) StartWithPTY(ptyFile *os.File, cmd *exec.Cmd, workDir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		return fmt.Errorf("onboarding already in progress")
	}

	logger.Infof("🚀 Starting Claude Code onboarding process")

	var tmpDir string
	var err error

	// If no existing PTY provided, create a new one
	if ptyFile == nil || cmd == nil {
		// Use provided workDir or create temp directory
		if workDir == "" {
			tmpDir, err = os.MkdirTemp("", "claude-onboarding-*")
			if err != nil {
				return fmt.Errorf("failed to create temp directory: %w", err)
			}
			workDir = tmpDir
		}

		// Start claude command in PTY with --dangerously-skip-permissions flag
		cmd = exec.Command("claude", "--dangerously-skip-permissions")
		cmd.Dir = workDir

		// Start the command with a PTY
		ptyFile, err = pty.Start(cmd)
		if err != nil {
			if tmpDir != "" {
				os.RemoveAll(tmpDir)
			}
			return fmt.Errorf("failed to start PTY: %w", err)
		}

		logger.Infof("✅ Created new Claude PTY session with --dangerously-skip-permissions in %s", workDir)
	} else {
		logger.Infof("✅ Using existing Claude PTY session for onboarding")
	}

	s.ptyCmd = cmd
	s.ptyFile = ptyFile
	s.sessionID = fmt.Sprintf("onboarding-%d", time.Now().Unix())
	s.isRunning = true
	s.currentState = StateThemeSelect
	s.stateEnteredAt = time.Now()
	s.outputBuffer.Reset()
	s.codeSubmitted = false
	s.pendingCodeInput = ""

	// Start monitoring goroutine
	go s.monitorOutput()
	go s.checkTimeouts()

	logger.Infof("✅ Onboarding session started: %s", s.sessionID)
	return nil
}

// Stop cancels the onboarding process and cleans up
func (s *ClaudeOnboardingService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return fmt.Errorf("onboarding not running")
	}

	logger.Infof("🛑 Stopping onboarding session: %s", s.sessionID)

	close(s.stopChan)

	if s.ptyFile != nil {
		s.ptyFile.Close()
	}

	if s.ptyCmd != nil && s.ptyCmd.Process != nil {
		_ = s.ptyCmd.Process.Kill()
	}

	s.isRunning = false
	s.currentState = StateIdle

	return nil
}

// GetStatus returns the current onboarding status
func (s *ClaudeOnboardingService) GetStatus() OnboardingStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := OnboardingStatus{
		State:        s.currentState,
		OAuthURL:     s.oauthURL,
		ErrorMessage: s.errorMessage,
		Output:       s.outputBuffer.String(),
	}

	// Add helpful messages based on state
	switch s.currentState {
	case StateIdle:
		status.Message = "Onboarding not started"
	case StateThemeSelect:
		status.Message = "Selecting theme..."
	case StateAuthMethod:
		status.Message = "Selecting authentication method..."
	case StateAuthURL:
		status.Message = "Waiting for OAuth URL..."
	case StateAuthWaiting:
		status.Message = "Please visit the OAuth URL and paste the code"
	case StateAuthConfirm:
		status.Message = "Confirming authentication..."
	case StateSecurityNotes:
		status.Message = "Reviewing security notes..."
	case StateTerminalSetup:
		status.Message = "Configuring terminal setup..."
	case StateComplete:
		status.Message = "Onboarding completed successfully!"
	case StateError:
		status.Message = "Onboarding failed"
	}

	return status
}

// SubmitCode submits the OAuth code when in AUTH_WAITING state
func (s *ClaudeOnboardingService) SubmitCode(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return fmt.Errorf("onboarding not running")
	}

	if s.currentState != StateAuthWaiting {
		return fmt.Errorf("not in auth waiting state, current state: %s", s.currentState)
	}

	logger.Infof("📝 Submitting OAuth code")

	// Store the code to be written in the monitoring goroutine
	s.pendingCodeInput = code + "\r"
	s.codeSubmitted = true

	return nil
}

// monitorOutput continuously reads from PTY and updates state
func (s *ClaudeOnboardingService) monitorOutput() {
	buf := make([]byte, 8192)

	for {
		select {
		case <-s.stopChan:
			return
		default:
			// Read from PTY with timeout
			n, err := s.ptyFile.Read(buf)
			if err != nil {
				if err != io.EOF {
					s.mu.Lock()
					s.errorMessage = fmt.Sprintf("PTY read error: %v", err)
					s.currentState = StateError
					s.isRunning = false
					s.mu.Unlock()
				}
				return
			}

			if n > 0 {
				s.mu.Lock()

				// Add to buffer (keep last 4000 chars for pattern matching)
				s.outputBuffer.Write(buf[:n])
				if s.outputBuffer.Len() > 4000 {
					// Trim from the front
					s.outputBuffer = bytes.NewBuffer(s.outputBuffer.Bytes()[s.outputBuffer.Len()-4000:])
				}

				output := s.outputBuffer.String()
				logger.Debugf("📊 PTY output (%d bytes): %s", n, string(buf[:n]))

				// Check if we have pending code to submit
				if s.pendingCodeInput != "" {
					logger.Debugf("✍️ Writing pending code input to PTY")
					if _, err := s.ptyFile.Write([]byte(s.pendingCodeInput)); err != nil {
						logger.Errorf("❌ Failed to write code to PTY: %v", err)
						s.errorMessage = fmt.Sprintf("Failed to submit code: %v", err)
						s.currentState = StateError
					} else {
						s.pendingCodeInput = ""
					}
				}

				// Detect and handle state transitions
				s.detectAndAdvanceState(output)

				s.mu.Unlock()
			}
		}
	}
}

// detectAndAdvanceState detects the current screen and advances the state
func (s *ClaudeOnboardingService) detectAndAdvanceState(output string) {
	previousState := s.currentState

	// Detect current state based on output patterns
	newState := s.detectState(output)

	// If state changed, update and advance
	if newState != s.currentState {
		logger.Infof("🔄 State transition: %s -> %s", s.currentState, newState)
		s.currentState = newState
		s.previousState = previousState
		s.stateEnteredAt = time.Now()

		// Don't auto-advance for AUTH_WAITING (need user input) or COMPLETE/ERROR states
		if newState != StateAuthWaiting && newState != StateComplete && newState != StateError {
			s.advanceState()
		}
	}
}

// detectState determines the current state based on PTY output
func (s *ClaudeOnboardingService) detectState(output string) OnboardingState {
	// Check for completion first (CWD detection)
	if strings.Contains(output, "/worktrees/") && strings.Contains(output, ">") {
		return StateComplete
	}

	// Check for each state pattern
	if strings.Contains(output, "Choose the text style") {
		return StateThemeSelect
	}

	if strings.Contains(output, "Select login method:") {
		return StateAuthMethod
	}

	if strings.Contains(output, "Browser didn't open?") ||
		strings.Contains(output, "oauth/authorize") {
		// Extract OAuth URL if we haven't already
		if s.oauthURL == "" {
			s.extractOAuthURL(output)
		}

		// If we already submitted code, move to confirm state
		if s.codeSubmitted {
			return StateAuthConfirm
		}

		return StateAuthURL
	}

	if strings.Contains(output, "Paste code here") {
		return StateAuthWaiting
	}

	if strings.Contains(output, "Login successful") || strings.Contains(output, "Logged in as") {
		return StateAuthConfirm
	}

	if strings.Contains(output, "Security notes:") {
		return StateSecurityNotes
	}

	if strings.Contains(output, "Use Claude Code's terminal setup?") {
		return StateTerminalSetup
	}

	// Check for auth errors
	if s.previousState == StateAuthWaiting &&
		(strings.Contains(output, "Invalid") || strings.Contains(output, "Error") || strings.Contains(output, "Failed")) {
		s.errorMessage = "Authentication failed - invalid code or error"
		return StateError
	}

	// Return current state if no pattern matched
	return s.currentState
}

// extractOAuthURL extracts the OAuth URL from the output
func (s *ClaudeOnboardingService) extractOAuthURL(output string) {
	// Look for pattern: https://claude.ai/oauth/authorize?...
	re := regexp.MustCompile(`(https://claude\.ai/oauth/authorize\?[^\s]+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) > 1 {
		s.oauthURL = matches[1]
		logger.Infof("🔗 Extracted OAuth URL: %s", s.oauthURL)
	}
}

// advanceState sends the appropriate input to advance to the next screen
func (s *ClaudeOnboardingService) advanceState() {
	var input string

	switch s.currentState {
	case StateThemeSelect:
		input = "\r" // Enter - accept default (dark mode)
		logger.Debugf("📤 Sending Enter for theme selection")

	case StateAuthMethod:
		input = "\r" // Enter - accept default (Claude account with subscription)
		logger.Debugf("📤 Sending Enter for auth method")

	case StateAuthURL:
		// Wait for user to get the URL - transition to AUTH_WAITING will happen automatically
		logger.Debugf("⏳ Waiting for transition to auth waiting state")
		return

	case StateAuthConfirm:
		input = "\r" // Enter - confirm successful auth
		logger.Debugf("📤 Sending Enter for auth confirmation")

	case StateSecurityNotes:
		input = "\r" // Enter - acknowledge security notes
		logger.Debugf("📤 Sending Enter for security notes")

	case StateTerminalSetup:
		input = "\r" // Enter - accept default (yes, use recommended settings)
		logger.Debugf("📤 Sending Enter for terminal setup")

	default:
		// Unknown state - send Enter as fallback
		logger.Warnf("⚠️ Unknown state detected: %s - sending Enter as fallback", s.currentState)
		input = "\r"
	}

	// Write to PTY
	if input != "" {
		if _, err := s.ptyFile.Write([]byte(input)); err != nil {
			logger.Errorf("❌ Failed to write to PTY: %v", err)
			s.errorMessage = fmt.Sprintf("Failed to send input: %v", err)
			s.currentState = StateError
		}
	}
}

// checkTimeouts monitors for state timeouts
func (s *ClaudeOnboardingService) checkTimeouts() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.mu.Lock()

			if s.isRunning && s.currentState != StateComplete && s.currentState != StateError {
				timeout := s.getStateTimeout(s.currentState)
				elapsed := time.Since(s.stateEnteredAt)

				if elapsed > timeout {
					logger.Errorf("⏱️ Timeout in state %s after %v (timeout: %v)", s.currentState, elapsed, timeout)
					s.errorMessage = fmt.Sprintf("Timeout in state %s after %v", s.currentState, elapsed)
					s.currentState = StateError
					s.isRunning = false
				}
			}

			s.mu.Unlock()
		}
	}
}

// getStateTimeout returns the timeout duration for a given state
func (s *ClaudeOnboardingService) getStateTimeout(state OnboardingState) time.Duration {
	if state == StateAuthWaiting {
		return 3 * time.Minute // User needs time for OAuth flow
	}
	return 10 * time.Second // Automated transitions should be fast
}

// IsRunning returns whether the onboarding is currently running
func (s *ClaudeOnboardingService) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isRunning
}
