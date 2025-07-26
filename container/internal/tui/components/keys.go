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

	// UI/Port shortcuts
	KeyOpenUI = "ctrl+0"

	// Common keys
	KeyEscape    = "esc"
	KeyEnter     = "enter"
	KeyTab       = "tab"
	KeyBackspace = "backspace"
)

// Port keys (require Ctrl modifier for consistency)
const (
	KeyPort1 = "ctrl+1"
	KeyPort2 = "ctrl+2"
	KeyPort3 = "ctrl+3"
	KeyPort4 = "ctrl+4"
	KeyPort5 = "ctrl+5"
	KeyPort6 = "ctrl+6"
	KeyPort7 = "ctrl+7"
	KeyPort8 = "ctrl+8"
	KeyPort9 = "ctrl+9"
)

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

// GetPortKeys returns all port selection keys
func GetPortKeys() []string {
	return []string{
		KeyPort1, KeyPort2, KeyPort3, KeyPort4, KeyPort5,
		KeyPort6, KeyPort7, KeyPort8, KeyPort9,
	}
}

// IsPortKey checks if the given key is a port selection key
func IsPortKey(key string) bool {
	portKeys := GetPortKeys()
	for _, portKey := range portKeys {
		if key == portKey {
			return true
		}
	}
	return false
}

// GetPortIndex returns the port index (0-8) for a given port key
func GetPortIndex(key string) int {
	if !IsPortKey(key) {
		return -1
	}
	// Extract number from "ctrl+N" format
	if len(key) >= 6 && key[:5] == "ctrl+" {
		return int(key[5] - '1')
	}
	return -1
}

// IsGlobalNavigationKey checks if a key is a global navigation command
func IsGlobalNavigationKey(key string) bool {
	switch key {
	case KeyQuit, KeyQuitAlt, KeyOverview, KeyLogs, KeyShell, KeyOpenUI:
		return true
	}
	return IsPortKey(key)
}
