package tui

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/hinshun/vt10x"
)

// Attribute mode constants from vt10x internal
// These match the mode bits used in vt10x
const (
	attrBold      = 1 << 0
	attrUnderline = 1 << 1
	attrBlink     = 1 << 2
	attrReverse   = 1 << 3
	attrItalic    = 1 << 4
)

// TerminalEmulator wraps vt10x to provide terminal emulation for PTY output
type TerminalEmulator struct {
	terminal vt10x.Terminal
	cols     int
	rows     int
}

// NewTerminalEmulator creates a new terminal emulator
func NewTerminalEmulator(cols, rows int) *TerminalEmulator {
	debugLog("Creating terminal emulator with size: %dx%d", cols, rows)
	vt := vt10x.New(vt10x.WithSize(cols, rows))
	return &TerminalEmulator{
		terminal: vt,
		cols:     cols,
		rows:     rows,
	}
}

// Write processes PTY output through the terminal emulator
func (te *TerminalEmulator) Write(data []byte) {
	// Feed to terminal emulator for parsing
	_, _ = te.terminal.Write(data)
}

// Resize updates the terminal dimensions
func (te *TerminalEmulator) Resize(cols, rows int) {
	te.cols = cols
	te.rows = rows
	te.terminal.Resize(cols, rows)
}

// RenderForReconnection returns a clean terminal view for reconnection without cursor positioning
func (te *TerminalEmulator) RenderForReconnection() string {
	return te.renderInternal(false)
}

// Render returns the current terminal view as a string with ANSI color codes
func (te *TerminalEmulator) Render() string {
	return te.renderInternal(true)
}

// renderInternal does the actual rendering with optional cursor positioning
func (te *TerminalEmulator) renderInternal(includeCursor bool) string {
	var buf bytes.Buffer

	// Get cursor information
	cursor := te.terminal.Cursor()
	cursorVisible := te.terminal.CursorVisible()

	// Track current attributes to minimize ANSI codes
	var lastFg, lastBg vt10x.Color
	var lastMode int16
	resetNeeded := false

	for row := 0; row < te.rows; row++ {
		if row > 0 {
			buf.WriteString("\n")
		}

		for col := 0; col < te.cols; col++ {
			cell := te.terminal.Cell(col, row)

			// Handle colors and attributes
			if cell.FG != lastFg || cell.BG != lastBg || cell.Mode != lastMode {
				// Reset if needed
				if resetNeeded {
					buf.WriteString("\033[0m")
				}

				// Apply new attributes based on Mode
				if cell.Mode&attrBold != 0 {
					buf.WriteString("\033[1m")
				}
				if cell.Mode&attrUnderline != 0 {
					buf.WriteString("\033[4m")
				}
				if cell.Mode&attrReverse != 0 {
					buf.WriteString("\033[7m")
				}

				// Apply foreground color
				if cell.FG != vt10x.DefaultFG {
					if cell.FG < 8 {
						// Standard colors (30-37)
						buf.WriteString(fmt.Sprintf("\033[%dm", 30+cell.FG))
					} else if cell.FG < 16 {
						// Bright colors (90-97)
						buf.WriteString(fmt.Sprintf("\033[%dm", 90+(cell.FG-8)))
					} else if cell.FG < 256 {
						// 256 colors
						buf.WriteString(fmt.Sprintf("\033[38;5;%dm", cell.FG))
					} else {
						// True color (24-bit RGB)
						r := (cell.FG >> 16) & 0xFF
						g := (cell.FG >> 8) & 0xFF
						b := cell.FG & 0xFF
						buf.WriteString(fmt.Sprintf("\033[38;2;%d;%d;%dm", r, g, b))
					}
				}

				// Apply background color
				if cell.BG != vt10x.DefaultBG {
					if cell.BG < 8 {
						// Standard colors (40-47)
						buf.WriteString(fmt.Sprintf("\033[%dm", 40+cell.BG))
					} else if cell.BG < 16 {
						// Bright colors (100-107)
						buf.WriteString(fmt.Sprintf("\033[%dm", 100+(cell.BG-8)))
					} else if cell.BG < 256 {
						// 256 colors
						buf.WriteString(fmt.Sprintf("\033[48;5;%dm", cell.BG))
					} else {
						// True color (24-bit RGB)
						r := (cell.BG >> 16) & 0xFF
						g := (cell.BG >> 8) & 0xFF
						b := cell.BG & 0xFF
						buf.WriteString(fmt.Sprintf("\033[48;2;%d;%d;%dm", r, g, b))
					}
				}

				lastFg = cell.FG
				lastBg = cell.BG
				lastMode = cell.Mode
				resetNeeded = true
			}

			// Handle cursor position
			if includeCursor && cursorVisible && row == cursor.Y && col == cursor.X {
				// Use reverse video for cursor
				buf.WriteString("\033[7m")
				if cell.Char == 0 || cell.Char == ' ' {
					buf.WriteRune(' ')
				} else {
					buf.WriteRune(cell.Char)
				}
				buf.WriteString("\033[27m") // Reset reverse
			} else if cell.Char == 0 {
				buf.WriteRune(' ')
			} else {
				// Handle special characters properly
				if cell.Char == '�' || cell.Char == '\uFFFD' {
					// Skip replacement characters
					buf.WriteRune(' ')
				} else {
					buf.WriteRune(cell.Char)
				}
			}
		}
	}

	// Final reset if needed
	if resetNeeded {
		buf.WriteString("\033[0m")
	}

	// Trim trailing empty lines
	output := buf.String()
	lines := strings.Split(output, "\n")

	// Find last non-empty line
	lastNonEmpty := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			lastNonEmpty = i
			break
		}
	}

	if lastNonEmpty >= 0 {
		lines = lines[:lastNonEmpty+1]
		output = strings.Join(lines, "\n")
	}

	// Only add cursor positioning if requested and cursor is not at the end of content
	// This prevents issues with reconnection where cursor positioning might conflict
	if includeCursor {
		cursorRow, cursorCol := cursor.Y+1, cursor.X+1 // Convert 0-based to 1-based
		if cursorVisible && (cursor.Y < lastNonEmpty || (cursor.Y == lastNonEmpty && cursor.X > 0)) {
			output += fmt.Sprintf("\033[%d;%dH", cursorRow, cursorCol)
		}
	}

	return output
}

// GetCursorPosition returns the current cursor position
func (te *TerminalEmulator) GetCursorPosition() (row, col int) {
	cursor := te.terminal.Cursor()
	return cursor.Y, cursor.X
}

// Clear clears the terminal
func (te *TerminalEmulator) Clear() {
	_, _ = te.terminal.Write([]byte("\033[2J\033[H"))
}
