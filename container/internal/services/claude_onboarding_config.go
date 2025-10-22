package services

import "time"

// OnboardingConfig holds configuration for the onboarding service
// Extracted to make timing and behavior configurable for testing
type OnboardingConfig struct {
	// StateAdvanceDelay is the delay before automatically advancing to next state
	// This gives the frontend time to poll and display the current state
	StateAdvanceDelay time.Duration

	// CodeSubmitDelay is the delay before sending code to PTY
	// Allows the PTY prompt to fully render before input
	CodeSubmitDelay time.Duration

	// CodeEnterDelay is the delay between code input and pressing Enter
	// Gives Claude time to process pasted text
	CodeEnterDelay time.Duration

	// TickerInterval is how often to check for pending input
	TickerInterval time.Duration

	// TimeoutCheckInterval is how often to check for state timeouts
	TimeoutCheckInterval time.Duration

	// AuthWaitingTimeout is how long to wait in AUTH_WAITING state
	// Users need time to complete OAuth flow
	AuthWaitingTimeout time.Duration

	// DefaultStateTimeout is the default timeout for automated states
	// Automated transitions should be fast
	DefaultStateTimeout time.Duration

	// MaxRecoveryAttempts is how many times to retry before giving up
	MaxRecoveryAttempts int

	// OutputBufferSize is the maximum size of the output buffer
	// Keeps last N bytes for pattern matching
	OutputBufferSize int

	// PTYReadBufferSize is the size of the buffer for reading PTY output
	PTYReadBufferSize int
}

// DefaultOnboardingConfig returns the default configuration
func DefaultOnboardingConfig() OnboardingConfig {
	return OnboardingConfig{
		StateAdvanceDelay:    300 * time.Millisecond,
		CodeSubmitDelay:      100 * time.Millisecond,
		CodeEnterDelay:       100 * time.Millisecond,
		TickerInterval:       100 * time.Millisecond,
		TimeoutCheckInterval: 1 * time.Second,
		AuthWaitingTimeout:   3 * time.Minute,
		DefaultStateTimeout:  10 * time.Second,
		MaxRecoveryAttempts:  2,
		OutputBufferSize:     4000,
		PTYReadBufferSize:    8192,
	}
}

// FastTestingConfig returns a configuration optimized for fast tests
func FastTestingConfig() OnboardingConfig {
	return OnboardingConfig{
		StateAdvanceDelay:    50 * time.Millisecond,
		CodeSubmitDelay:      10 * time.Millisecond,
		CodeEnterDelay:       10 * time.Millisecond,
		TickerInterval:       50 * time.Millisecond,
		TimeoutCheckInterval: 500 * time.Millisecond,
		AuthWaitingTimeout:   30 * time.Second,
		DefaultStateTimeout:  5 * time.Second,
		MaxRecoveryAttempts:  1,
		OutputBufferSize:     4000,
		PTYReadBufferSize:    8192,
	}
}

// StatePattern defines patterns to detect a specific state
type StatePattern struct {
	State    OnboardingState
	Patterns []string
	// Priority determines detection order (higher = check first)
	// Used to resolve ambiguous cases where patterns overlap
	Priority int
}

// GetStatePatterns returns all state detection patterns in priority order
func GetStatePatterns() []StatePattern {
	return []StatePattern{
		// Completion states - highest priority
		{
			State:    StateAuthConfirm,
			Patterns: []string{"Login successful", "Logged in as", "authentication successful"},
			Priority: 100,
		},
		{
			State:    StateBypassPermissions,
			Patterns: []string{"Bypass Permissions mode", "Yes, I accept", "No, exit"},
			Priority: 90,
		},
		{
			State:    StateSecurityNotes,
			Patterns: []string{"Security notes:", "security information", "important security"},
			Priority: 80,
		},
		{
			State:    StateTerminalSetup,
			Patterns: []string{"Use Claude Code's terminal setup?", "terminal setup", "configure terminal"},
			Priority: 70,
		},

		// Active states - medium priority
		{
			State:    StateAuthWaiting,
			Patterns: []string{"Paste code here", "paste the code", "enter code"},
			Priority: 50,
		},
		{
			State:    StateAuthMethod,
			Patterns: []string{"Select login method", "choose login", "authentication method"},
			Priority: 30,
		},

		// Initial states - lowest priority
		{
			State:    StateThemeSelect,
			Patterns: []string{"Choose the text style", "select theme", "text style"},
			Priority: 10,
		},
	}
}

// ErrorPattern defines patterns for detecting errors
type ErrorPattern struct {
	Patterns     []string
	ErrorMessage string
	ShouldRetry  bool
	RetryAction  string // e.g., "press_enter"
}

// GetErrorPatterns returns patterns for error detection
func GetErrorPatterns() []ErrorPattern {
	return []ErrorPattern{
		{
			Patterns:     []string{"Invalid code"},
			ErrorMessage: "Invalid authentication code. Please verify you copied the entire code.",
			ShouldRetry:  true,
			RetryAction:  "press_enter",
		},
		{
			Patterns:     []string{"OAuth error", "authentication failed", "auth error"},
			ErrorMessage: "Authentication error occurred. Please try again.",
			ShouldRetry:  true,
			RetryAction:  "press_enter",
		},
	}
}
