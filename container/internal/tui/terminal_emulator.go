package tui

import (
	"bytes"
	"strings"

	"github.com/hinshun/vt10x"
)

// Attribute mode constants from vt10x internal
const (
	attrReverse = 1 << iota
	attrUnderline
	attrBold
	attrItalic
	attrBlink
)

// TerminalEmulator wraps vt10x to provide terminal emulation for PTY output
type TerminalEmulator struct {
	terminal vt10x.Terminal
	cols     int
	rows     int
}

// NewTerminalEmulator creates a new terminal emulator
func NewTerminalEmulator(cols, rows int) *TerminalEmulator {
	vt := vt10x.New(vt10x.WithSize(cols, rows))
	return &TerminalEmulator{
		terminal: vt,
		cols:     cols,
		rows:     rows,
	}
}

// Write processes PTY output through the terminal emulator
func (te *TerminalEmulator) Write(data []byte) {
	te.terminal.Write(data)
}

// Resize updates the terminal dimensions
func (te *TerminalEmulator) Resize(cols, rows int) {
	te.cols = cols
	te.rows = rows
	te.terminal.Resize(cols, rows)
}

// Render returns the current terminal view as a string
func (te *TerminalEmulator) Render() string {
	var buf bytes.Buffer
	
	// Get cursor information
	cursor := te.terminal.Cursor()
	cursorVisible := te.terminal.CursorVisible()
	
	// Build the output without ANSI codes (viewport doesn't handle them well)
	for row := 0; row < te.rows; row++ {
		if row > 0 {
			buf.WriteString("\n")
		}
		
		for col := 0; col < te.cols; col++ {
			cell := te.terminal.Cell(col, row)
			
			// Handle cursor position with simple highlighting
			if cursorVisible && row == cursor.Y && col == cursor.X {
				if cell.Char == 0 || cell.Char == ' ' {
					buf.WriteRune('█')
				} else {
					// For now, just show the character with cursor
					// TODO: Could use lipgloss styles for proper cursor rendering
					buf.WriteRune(cell.Char)
				}
			} else if cell.Char == 0 {
				buf.WriteRune(' ')
			} else {
				buf.WriteRune(cell.Char)
			}
		}
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
	
	return output
}

// GetCursorPosition returns the current cursor position
func (te *TerminalEmulator) GetCursorPosition() (row, col int) {
	cursor := te.terminal.Cursor()
	return cursor.Y, cursor.X
}

// Clear clears the terminal
func (te *TerminalEmulator) Clear() {
	te.terminal.Write([]byte("\033[2J\033[H"))
}