package components

// Key Command Groups:
// 1. Global Navigation - Always available with Ctrl modifier
// 2. View-Specific - Available only in specific views
// 3. Terminal Pass-through - All other keys in shell view

// Global Navigation Keys (require Ctrl modifier)
const (
	// Quit commands - available everywhere
	KeyQuit    = "ctrl+q"
	KeyQuitAlt = "ctrl+c"

	// View navigation - consistent across all views
	KeyOverview = "ctrl+o"
	KeyLogs     = "ctrl+l"
	KeyShell    = "ctrl+t"

	// Browser shortcuts
	KeyOpenBrowser = "ctrl+b"

	// Workspace shortcuts
	KeyWorkspace = "ctrl+w"

	// Common keys
	KeyEscape    = "esc"
	KeyEnter     = "enter"
	KeyTab       = "tab"
	KeyBackspace = "backspace"
)

// Port selection keys (removed - now using ctrl+b with overlay menu)

// Navigation keys
const (
	KeyUp       = "up"
	KeyDown     = "down"
	KeyLeft     = "left"
	KeyRight    = "right"
	KeyPageUp   = "pgup"
	KeyPageDown = "pgdown"
	KeyHome     = "home"
	KeyEnd      = "end"
)

// Vim-style navigation
const (
	KeyVimUp       = "k"
	KeyVimDown     = "j"
	KeyVimLeft     = "h"
	KeyVimRight    = "l"
	KeyVimPageUp   = "b"
	KeyVimPageDown = "f"
	KeyVimTop      = "g"
	KeyVimBottom   = "G"
)

// Logs view specific keys
const (
	KeyLogsSearch = "/"
	KeyLogsClear  = "c"
)

// Shell view specific keys
const (
	// Scrolling (Alt/Option for Mac compatibility)
	KeyShellScrollUp   = "alt+up"
	KeyShellScrollDown = "alt+down"
	KeyShellPageUp     = "alt+pgup"
	KeyShellPageDown   = "alt+pgdown"
	KeyShellHome       = "alt+home"
	KeyShellEnd        = "alt+end"

	// Session management (only in session list mode)
	KeyShellNewSession = "n"
)

// Control keys
const (
	KeyCtrlC = "ctrl+c"
	KeyCtrlD = "ctrl+d"
	KeyCtrlZ = "ctrl+z"
)

// Port selection functions removed - now using ctrl+b with overlay menu

// IsGlobalNavigationKey checks if a key is a global navigation command
func IsGlobalNavigationKey(key string) bool {
	switch key {
	case KeyQuit, KeyQuitAlt, KeyOverview, KeyLogs, KeyShell, KeyOpenBrowser, KeyWorkspace:
		return true
	}
	return false
}
