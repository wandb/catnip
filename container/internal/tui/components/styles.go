package components

import "github.com/charmbracelet/lipgloss"

// Color scheme
const (
	ColorPrimary   = "6"  // Cyan
	ColorSecondary = "8"  // Gray
	ColorSuccess   = "2"  // Green
	ColorWarning   = "3"  // Yellow
	ColorError     = "1"  // Red
	ColorInfo      = "4"  // Blue
	ColorHighlight = "5"  // Magenta
	ColorText      = "15" // White
	ColorMuted     = "8"  // Dark gray
	ColorAccent    = "11" // Bright yellow
	ColorBorder    = "8"  // Border color
)

// Header styles
var (
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(ColorPrimary)).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true)

	SectionHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(ColorSuccess))

	SubHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(ColorInfo))
)

// Text styles
var (
	KeyHighlightStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(ColorAccent)).
				Bold(true)

	ErrorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(ColorError))

	MutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(ColorMuted))

	SearchHighlightStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(ColorAccent)).
				Foreground(lipgloss.Color("0"))

	StatusConnectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(ColorSuccess))

	StatusDisconnectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(ColorError))
)

// Container styles
var (
	MainContentStyle = lipgloss.NewStyle().
				Padding(1)

	FooterStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(ColorMuted)).
			BorderStyle(lipgloss.NormalBorder()).
			BorderTop(true).
			Padding(0, 1)

	ShellHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(ColorText)).
				Background(lipgloss.Color(ColorMuted)).
				Padding(0, 1)

	CenteredStyle = lipgloss.NewStyle().
			Align(lipgloss.Center)
)

// ApplyWidth applies width to a style and returns a new style
func ApplyWidth(style lipgloss.Style, width int) lipgloss.Style {
	return style.Width(width - 2)
}

// ApplyHeight applies height to a style and returns a new style
func ApplyHeight(style lipgloss.Style, height int) lipgloss.Style {
	return style.Height(height)
}

// ApplySize applies both width and height to a style
func ApplySize(style lipgloss.Style, width, height int) lipgloss.Style {
	return style.Width(width - 2).Height(height)
}
