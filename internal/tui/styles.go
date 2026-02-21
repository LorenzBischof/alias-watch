package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorFocused   = lipgloss.Color("62")
	colorUnfocused = lipgloss.Color("240")
	colorFlagged   = lipgloss.Color("196")
	colorStatus    = lipgloss.Color("214")
	colorBackdrop  = lipgloss.Color("236")

	styleFocusedBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorFocused)

	styleUnfocusedBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorUnfocused)

	styleStatusBar = lipgloss.NewStyle().
			Foreground(colorStatus).
			Padding(0, 1)

	styleFlagged = lipgloss.NewStyle().Foreground(colorFlagged).Bold(true)
	styleHelp    = lipgloss.NewStyle().Foreground(colorUnfocused)
)
