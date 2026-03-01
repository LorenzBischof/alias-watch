package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap holds all key bindings for the TUI.
type KeyMap struct {
	Up         key.Binding
	Down       key.Binding
	Tab        key.Binding
	Quit       key.Binding
	Add        key.Binding
	Delete     key.Binding
	Flag       key.Binding
	Edit       key.Binding
	Confirm    key.Binding
	Cancel     key.Binding
	FormNext   key.Binding
	FormPrev   key.Binding
	FormSubmit key.Binding
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch pane"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Add: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "add sender"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete"),
		),
		Flag: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "toggle flagged"),
		),
		Edit: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "toggle domain rule"),
		),
		Confirm: key.NewBinding(
			key.WithKeys("enter", "y"),
			key.WithHelp("enter/y", "confirm"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc", "n"),
			key.WithHelp("esc/n", "cancel"),
		),
		FormNext: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next field"),
		),
		FormPrev: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev field"),
		),
		FormSubmit: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "submit"),
		),
	}
}
