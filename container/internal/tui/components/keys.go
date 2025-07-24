package components

// Global key bindings
const (
	KeyQuit      = "q"
	KeyQuitAlt   = "ctrl+c"
	KeyOverview  = "o"
	KeyLogs      = "l"
	KeyShell     = "t"
	KeyOpenUI    = "0"
	KeyEscape    = "esc"
	KeyEnter     = "enter"
	KeyTab       = "tab"
	KeyBackspace = "backspace"
)

// Port keys (1-9)
const (
	KeyPort1 = "1"
	KeyPort2 = "2"
	KeyPort3 = "3"
	KeyPort4 = "4"
	KeyPort5 = "5"
	KeyPort6 = "6"
	KeyPort7 = "7"
	KeyPort8 = "8"
	KeyPort9 = "9"
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
	KeyShellOverview   = "ctrl+o"
	KeyShellQuit       = "ctrl+q"
	KeyShellScrollUp   = "alt+up"
	KeyShellScrollDown = "alt+down"
	KeyShellPageUp     = "ctrl+b"
	KeyShellPageDown   = "ctrl+f"
	KeyShellHome       = "ctrl+home"
	KeyShellEnd        = "ctrl+end"
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
	return int(key[0] - '1')
}
