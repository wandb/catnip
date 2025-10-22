package services

import (
	"regexp"
	"strings"
	"time"

	"github.com/vanpelt/catnip/internal/logger"
)

// StateDetector handles state detection logic
type StateDetector struct {
	patterns      []StatePattern
	errorPatterns []ErrorPattern
	config        OnboardingConfig
}

// NewStateDetector creates a new state detector
func NewStateDetector(config OnboardingConfig) *StateDetector {
	patterns := GetStatePatterns()

	// Sort by priority (highest first)
	// Using simple bubble sort since list is small
	for i := 0; i < len(patterns)-1; i++ {
		for j := i + 1; j < len(patterns); j++ {
			if patterns[j].Priority > patterns[i].Priority {
				patterns[i], patterns[j] = patterns[j], patterns[i]
			}
		}
	}

	return &StateDetector{
		patterns:      patterns,
		errorPatterns: GetErrorPatterns(),
		config:        config,
	}
}

// DetectStateResult contains the result of state detection
type DetectStateResult struct {
	State        OnboardingState
	StateChanged bool
	OAuthURL     string
	Error        *DetectedError
}

// DetectedError represents an error detected in the output
type DetectedError struct {
	Message     string
	ShouldRetry bool
	RetryAction string
}

// DetectState analyzes output and returns the detected state
func (d *StateDetector) DetectState(output string, currentState OnboardingState, existingOAuthURL string) DetectStateResult {
	// Strip ANSI codes for reliable pattern matching
	cleanOutput := stripANSI(output)

	logger.Debugf("🔍 Detecting state from %d bytes of output (current: %s)", len(output), currentState)

	// Log sample for debugging
	sampleLen := 500
	if len(cleanOutput) < sampleLen {
		sampleLen = len(cleanOutput)
	}
	if sampleLen > 0 {
		sample := cleanOutput[len(cleanOutput)-sampleLen:]
		logger.Debugf("📋 Output sample: ...%s", sample)
	}

	result := DetectStateResult{
		State:    currentState,
		OAuthURL: existingOAuthURL,
	}

	// Check for completion first (shell prompt)
	if d.isComplete(cleanOutput) {
		logger.Debugf("✅ Detected COMPLETE state (shell prompt)")
		result.State = StateComplete
		result.StateChanged = result.State != currentState
		return result
	}

	// Extract OAuth URL if present and not already extracted
	if result.OAuthURL == "" {
		if url := d.extractOAuthURL(output); url != "" {
			result.OAuthURL = url
			logger.Infof("🔗 Extracted OAuth URL: %s", url)
		}
	}

	// Check for errors
	if err := d.detectError(cleanOutput); err != nil {
		logger.Warnf("⚠️ Detected error: %s", err.Message)
		result.Error = err

		// If error is retryable, stay in AUTH_WAITING state
		if err.ShouldRetry {
			result.State = StateAuthWaiting
			result.StateChanged = result.State != currentState
			return result
		}
	}

	// Check state patterns in priority order
	for _, pattern := range d.patterns {
		if containsPattern(cleanOutput, pattern.Patterns) {
			logger.Debugf("✅ Detected %s state (priority %d)", pattern.State, pattern.Priority)
			result.State = pattern.State
			result.StateChanged = result.State != currentState
			return result
		}
	}

	// No pattern matched, keep current state
	logger.Debugf("⚠️ No pattern matched, keeping current state: %s", currentState)
	return result
}

// isComplete checks if onboarding is complete (shell prompt visible)
func (d *StateDetector) isComplete(cleanOutput string) bool {
	// Look for common shell prompt patterns
	return containsPattern(cleanOutput, []string{"/worktrees/", "claude-onboarding"}) &&
		strings.Contains(cleanOutput, ">")
}

// extractOAuthURL extracts the OAuth URL from output
func (d *StateDetector) extractOAuthURL(output string) string {
	// Look for pattern: https://claude.ai/oauth/authorize?...
	re := regexp.MustCompile(`(https://claude\.ai/oauth/authorize\?[^\s]+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// detectError checks for error patterns in output
func (d *StateDetector) detectError(cleanOutput string) *DetectedError {
	for _, errPattern := range d.errorPatterns {
		if containsPattern(cleanOutput, errPattern.Patterns) {
			return &DetectedError{
				Message:     errPattern.ErrorMessage,
				ShouldRetry: errPattern.ShouldRetry,
				RetryAction: errPattern.RetryAction,
			}
		}
	}
	return nil
}

// GetAdvanceInput returns the input needed to advance from a given state
func (d *StateDetector) GetAdvanceInput(state OnboardingState) string {
	switch state {
	case StateThemeSelect:
		return "\r" // Enter - accept default (dark mode)
	case StateAuthMethod:
		return "\r" // Enter - accept default (Claude account)
	case StateAuthConfirm:
		return "\r" // Enter - confirm auth
	case StateBypassPermissions:
		return "2\r" // "2" + Enter - select "Yes, I accept"
	case StateSecurityNotes:
		return "\r" // Enter - acknowledge
	case StateTerminalSetup:
		return "\r" // Enter - accept default
	default:
		// Unknown state - Enter as fallback
		logger.Warnf("⚠️ Unknown state for input: %s - using Enter", state)
		return "\r"
	}
}

// ShouldAutoAdvance returns whether a state should automatically advance
func (d *StateDetector) ShouldAutoAdvance(state OnboardingState) bool {
	// Don't auto-advance for states requiring user input or terminal states
	switch state {
	case StateAuthWaiting, StateComplete, StateError:
		return false
	default:
		return true
	}
}

// GetStateTimeout returns the timeout for a given state
func (d *StateDetector) GetStateTimeout(state OnboardingState) time.Duration {
	if state == StateAuthWaiting {
		return d.config.AuthWaitingTimeout
	}
	return d.config.DefaultStateTimeout
}
