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

// PTYSessionRestarter defines the interface for restarting Claude PTY sessions
// This interface avoids circular dependencies between services and handlers packages
type PTYSessionRestarter interface {
	RestartClaudeSessions()
}

// OnboardingState represents the current state in the Claude onboarding flow
type OnboardingState string

// Onboarding state constants representing each step in the Claude Code onboarding flow
const (
	StateIdle              OnboardingState = "idle"
	StateThemeSelect       OnboardingState = "theme_select"
	StateAuthMethod        OnboardingState = "auth_method"
	StateAuthWaiting       OnboardingState = "auth_waiting"
	StateAuthConfirm       OnboardingState = "auth_confirm"
	StateBypassPermissions OnboardingState = "bypass_permissions"
	StateSecurityNotes     OnboardingState = "security_notes"
	StateTerminalSetup     OnboardingState = "terminal_setup"
	StateComplete          OnboardingState = "complete"
	StateError             OnboardingState = "error"
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
	ownsPTY          bool                // true if we created the PTY, false if reusing existing
	ptyRestarter     PTYSessionRestarter // optional restarter to refresh existing PTY sessions after auth
}

// NewClaudeOnboardingService creates a new onboarding service instance
// The ptyRestarter parameter is optional and can be nil for tests or standalone usage
func NewClaudeOnboardingService(ptyRestarter PTYSessionRestarter) *ClaudeOnboardingService {
	return &ClaudeOnboardingService{
		currentState: StateIdle,
		outputBuffer: &bytes.Buffer{},
		stopChan:     make(chan struct{}),
		ptyRestarter: ptyRestarter,
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

		s.ownsPTY = true // We created this PTY
		logger.Infof("✅ Created new Claude PTY session with --dangerously-skip-permissions in %s", workDir)
	} else {
		s.ownsPTY = false // Reusing existing PTY
		logger.Infof("✅ Using existing Claude PTY session for onboarding")
	}

	s.ptyCmd = cmd
	s.ptyFile = ptyFile
	s.sessionID = fmt.Sprintf("onboarding-%d", time.Now().Unix())
	s.isRunning = true
	s.currentState = StateIdle // Start idle, let detection trigger first state change
	s.stateEnteredAt = time.Now()
	s.outputBuffer.Reset()
	s.codeSubmitted = false
	s.pendingCodeInput = ""
	// Clear any previous error or OAuth state
	s.errorMessage = ""
	s.oauthURL = ""

	// If reusing an existing PTY, force a screen redraw by resizing
	// This makes the CLI app redraw its current state so we can detect it
	if ptyFile != nil && cmd != nil && workDir != "" {
		// Get current size
		winsize, err := pty.GetsizeFull(ptyFile)
		if err == nil {
			// Resize to same size to trigger redraw
			_ = pty.Setsize(ptyFile, winsize)
			// Small delay to let the redraw happen
			time.Sleep(50 * time.Millisecond)
		}
	}

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
		logger.Warnf("⚠️ Stop called but onboarding not running")
		return nil // Don't error, just return - this is fine
	}

	logger.Infof("🛑 Stopping onboarding session: %s (ownsPTY: %v)", s.sessionID, s.ownsPTY)

	// Signal monitoring goroutines to stop
	close(s.stopChan)

	// Only close/kill PTY if we created it
	// If reusing existing Claude session, leave it running
	if s.ownsPTY {
		if s.ptyFile != nil {
			s.ptyFile.Close()
		}

		if s.ptyCmd != nil && s.ptyCmd.Process != nil {
			_ = s.ptyCmd.Process.Kill()
		}
	}

	// Reset state
	s.isRunning = false
	s.currentState = StateIdle
	s.oauthURL = ""
	s.errorMessage = ""
	s.codeSubmitted = false
	s.pendingCodeInput = ""
	s.outputBuffer.Reset()

	// Recreate stopChan for potential future runs
	s.stopChan = make(chan struct{})

	logger.Infof("✅ Onboarding session stopped")
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
	case StateAuthWaiting:
		status.Message = "Please visit the OAuth URL and paste the code"
	case StateAuthConfirm:
		status.Message = "Confirming authentication..."
	case StateBypassPermissions:
		status.Message = "Accepting bypass permissions..."
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

	logger.Infof("📝 Submitting OAuth code (length: %d)", len(code))

	// Clear any previous error message
	s.errorMessage = ""

	// Store the code to be written in the monitoring goroutine
	// Send code EXACTLY as received, then add carriage return to submit it
	// The code itself doesn't contain submission escape codes - we need to press Enter
	s.pendingCodeInput = code
	s.codeSubmitted = true

	return nil
}

// monitorOutput continuously reads from PTY and updates state
func (s *ClaudeOnboardingService) monitorOutput() {
	buf := make([]byte, 8192)

	// Use a ticker to periodically check for pending input even when PTY has no output
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// Channel to signal when we have PTY data
	dataChan := make(chan int)

	// Goroutine to read from PTY (blocking)
	go func() {
		for {
			n, err := s.ptyFile.Read(buf)
			if err != nil {
				if err != io.EOF {
					// Log PTY errors but don't expose them to users as they're not actionable
					// The state detection will handle user-facing errors (invalid code, etc)
					logger.Warnf("⚠️ PTY read error (internal): %v", err)

					s.mu.Lock()
					// Only set error state if we don't already have a user-friendly error
					if s.errorMessage == "" {
						// Don't show raw PTY errors to users
						s.errorMessage = "Connection to authentication process lost. Please try again."
						s.currentState = StateError
						s.isRunning = false
					}
					s.mu.Unlock()
				}
				close(dataChan)
				return
			}
			if n > 0 {
				select {
				case dataChan <- n:
				case <-s.stopChan:
					return
				}
			}
		}
	}()

	for {
		select {
		case <-s.stopChan:
			return

		case n, ok := <-dataChan:
			if !ok {
				return // PTY read error
			}

			s.mu.Lock()

			// Add to buffer (keep last 8000 chars for pattern matching)
			s.outputBuffer.Write(buf[:n])
			if s.outputBuffer.Len() > 8000 {
				// Trim from the front
				s.outputBuffer = bytes.NewBuffer(s.outputBuffer.Bytes()[s.outputBuffer.Len()-8000:])
			}

			output := s.outputBuffer.String()

			// Detect and handle state transitions
			s.detectAndAdvanceState(output)

			s.mu.Unlock()

		case <-ticker.C:
			// Periodically check for pending input even when PTY has no output
			s.mu.Lock()
			if s.pendingCodeInput != "" {
				code := s.pendingCodeInput
				s.pendingCodeInput = "" // Clear immediately to prevent double-write

				// Write the code first
				_, err := s.ptyFile.Write([]byte(code))
				if err != nil {
					logger.Errorf("❌ Failed to write code to PTY: %v", err)
					s.errorMessage = fmt.Sprintf("Failed to submit code: %v", err)
					s.currentState = StateError
					s.mu.Unlock()
					continue
				}

				// Unlock before sleeping
				s.mu.Unlock()

				// Small delay to let Claude process the pasted text
				time.Sleep(100 * time.Millisecond)

				// Then send carriage return to submit
				s.mu.Lock()
				if s.ptyFile != nil {
					_, err := s.ptyFile.Write([]byte("\r"))
					if err != nil {
						logger.Errorf("❌ Failed to write CR to PTY: %v", err)
						s.errorMessage = fmt.Sprintf("Failed to submit code: %v", err)
						s.currentState = StateError
					}
				}
			}
			s.mu.Unlock()
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

		// If onboarding completed successfully, restart any existing Claude PTY sessions
		// so they reflect the new authenticated state
		if newState == StateComplete && s.ptyRestarter != nil {
			go func() {
				logger.Infof("🔄 Onboarding complete - triggering Claude session restarts")
				s.ptyRestarter.RestartClaudeSessions()
			}()
		}

		// Don't auto-advance for AUTH_WAITING (need user input) or COMPLETE/ERROR states
		if newState != StateAuthWaiting && newState != StateComplete && newState != StateError {
			// Schedule advancement after a delay in a goroutine to avoid blocking the lock
			// This gives frontend time to poll and display current state
			go func(state OnboardingState) {
				time.Sleep(300 * time.Millisecond)
				s.mu.Lock()
				// Only advance if we're still in the same state (hasn't changed during sleep)
				if s.currentState == state {
					s.advanceState()
				}
				s.mu.Unlock()
			}(newState)
		}
	}
}

// stripANSI removes ANSI escape codes from a string for reliable pattern matching
func stripANSI(s string) string {
	// Remove ANSI escape sequences (CSI sequences, OSC sequences, etc.)
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x07]*\x07|\x1b[><]|\x1b\][^\x1b]*\x1b\\`)
	return ansiRegex.ReplaceAllString(s, "")
}

// containsPattern checks if any of the patterns exist in the text (case-insensitive)
func containsPattern(text string, patterns []string) bool {
	lowerText := strings.ToLower(text)
	for _, pattern := range patterns {
		if strings.Contains(lowerText, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// detectState determines the current state based on PTY output
func (s *ClaudeOnboardingService) detectState(output string) OnboardingState {
	// Strip ANSI codes for more reliable pattern matching
	cleanOutput := stripANSI(output)

	// ALWAYS extract OAuth URL if present (before state detection)
	// This is important because the OAuth screen shows both the URL and "Paste code here"
	if s.oauthURL == "" && (containsPattern(cleanOutput, []string{"Browser didn't open?", "oauth/authorize"})) {
		s.extractOAuthURL(output) // Use original output for URL extraction
	}

	// Check for completion first - Claude is ready for user input
	// After onboarding completes, Claude shows its ready prompt with status indicators

	// Primary indicator: bypass permissions status (appears when Claude is ready)
	if strings.Contains(cleanOutput, "bypass permissions on") {
		return StateComplete
	}

	// Secondary indicator: token counter (for subscription mode without bypass)
	if strings.Contains(cleanOutput, "0 tokens") {
		return StateComplete
	}

	// Check for each state pattern - order matters! Check LATEST/COMPLETION states FIRST
	// since buffer contains historical output, we need to match the most recent state

	// TERMINAL_SETUP: Terminal configuration (check first - latest state)
	if containsPattern(cleanOutput, []string{"Use Claude Code's terminal setup?", "terminal setup", "configure terminal"}) {
		return StateTerminalSetup
	}

	// BYPASS_PERMISSIONS: Bypass Permissions mode dialog (check before SECURITY_NOTES)
	if containsPattern(cleanOutput, []string{"Bypass Permissions mode", "Yes, I accept", "No, exit"}) {
		return StateBypassPermissions
	}

	// SECURITY_NOTES: Security information screen (check after BYPASS_PERMISSIONS)
	if containsPattern(cleanOutput, []string{"Security notes:", "security information", "important security"}) {
		return StateSecurityNotes
	}

	// AUTH_CONFIRM: Login successful (check after more specific states)
	if containsPattern(cleanOutput, []string{"Login successful", "Logged in as", "authentication successful"}) {
		return StateAuthConfirm
	}

	// Check for auth errors FIRST (before AUTH_WAITING)
	// Error screen still shows "Paste code here", so check errors with higher priority
	if containsPattern(cleanOutput, []string{"OAuth error", "Invalid code", "authentication failed", "auth error"}) {
		// Extract the error message
		if containsPattern(cleanOutput, []string{"Invalid code"}) {
			s.errorMessage = "Invalid authentication code. Please verify you copied the entire code."
		} else {
			s.errorMessage = "Authentication error occurred. Please try again."
		}
		logger.Warnf("⚠️ Detected OAuth error, will retry: %s", s.errorMessage)

		// Reset codeSubmitted so user can try again
		s.codeSubmitted = false

		// If we see "Press Enter to retry", send Enter and return to auth waiting
		if containsPattern(cleanOutput, []string{"Press Enter to retry", "press enter", "retry"}) {
			logger.Infof("📤 Sending Enter to retry OAuth flow")
			// Send Enter in a goroutine to avoid blocking
			go func() {
				time.Sleep(100 * time.Millisecond)
				s.mu.Lock()
				if s.ptyFile != nil {
					_, _ = s.ptyFile.Write([]byte("\r"))
				}
				s.mu.Unlock()
			}()
		}

		// Stay in or return to AUTH_WAITING state so user can retry
		return StateAuthWaiting
	}

	// AUTH_WAITING: OAuth URL and paste code prompt appear together
	// We extract the URL but only have one state for this screen
	if containsPattern(cleanOutput, []string{"Paste code here", "paste the code", "enter code"}) {
		return StateAuthWaiting
	}

	// AUTH_METHOD: Select login method
	if containsPattern(cleanOutput, []string{"Select login method", "choose login", "authentication method"}) {
		return StateAuthMethod
	}

	// THEME_SELECT: Theme selection (checked last since it's earliest in the flow)
	// Match on options that appear in the list, not just the header (which may scroll off buffer)
	if containsPattern(cleanOutput, []string{"Dark mode (colorblind-friendly)", "Light mode (colorblind-friendly)"}) ||
		(containsPattern(cleanOutput, []string{"Dark mode", "Light mode"}) && containsPattern(cleanOutput, []string{"function greet"})) {
		return StateThemeSelect
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

	case StateAuthMethod:
		input = "\r" // Enter - accept default (Claude account with subscription)

	case StateAuthConfirm:
		input = "\r" // Enter - confirm successful auth

	case StateBypassPermissions:
		// Special handling: send "2" then "\r" with a delay
		// The CLI needs time to process the menu selection before pressing Enter
		logger.Infof("📤 Sending '2' to select 'Yes, I accept' option")
		if _, err := s.ptyFile.Write([]byte("2")); err != nil {
			logger.Errorf("❌ Failed to write '2' to PTY: %v", err)
			s.errorMessage = fmt.Sprintf("Failed to send input: %v", err)
			s.currentState = StateError
			return
		}

		// Wait for CLI to process the selection
		time.Sleep(200 * time.Millisecond)

		logger.Infof("📤 Sending Enter to confirm selection")
		input = "\r"

	case StateSecurityNotes:
		input = "\r" // Enter - acknowledge security notes

	case StateTerminalSetup:
		input = "\r" // Enter - accept default (yes, use recommended settings)

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

	recoveryAttempts := make(map[OnboardingState]int)

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
					attempts := recoveryAttempts[s.currentState]

					// Try recovery up to 2 times before giving up
					if attempts < 2 {
						recoveryAttempts[s.currentState] = attempts + 1
						logger.Warnf("⏱️ Timeout in state %s after %v (attempt %d/2) - trying recovery", s.currentState, elapsed, attempts+1)

						// Log current PTY output for debugging
						currentOutput := s.outputBuffer.String()
						if len(currentOutput) > 500 {
							currentOutput = currentOutput[len(currentOutput)-500:]
						}
						logger.Infof("📺 Current PTY screen output (last 500 chars):\n%s", currentOutput)

						// Try to recover by sending Enter to advance
						if s.ptyFile != nil {
							logger.Infof("🔄 Sending Enter to try to advance from stuck state")
							_, _ = s.ptyFile.Write([]byte("\r"))

							// Reset the timer to give recovery a chance
							s.stateEnteredAt = time.Now()
						}
					} else {
						// Recovery failed, give up with user-friendly error
						logger.Errorf("❌ Timeout in state %s after %v - recovery failed", s.currentState, elapsed)

						// User-friendly error message (don't expose internal state names)
						s.errorMessage = "Unable to complete authentication automatically. Please run 'claude' directly in your terminal to authenticate."
						s.currentState = StateError
						s.isRunning = false
						recoveryAttempts[s.currentState] = 0
					}
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
