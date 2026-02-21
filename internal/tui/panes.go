package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"

	"github.com/lorenzbischof/email-monitoring/internal/db"
)

type aliasListItem struct {
	title string
	email string
}

func (i aliasListItem) Title() string {
	if i.title == "" {
		return i.email
	}
	return i.title + " " + styleAliasMeta.Render(i.email)
}

func (i aliasListItem) Description() string {
	return ""
}

func (i aliasListItem) FilterValue() string {
	if i.title == "" {
		return i.email
	}
	return i.title + " " + i.email
}

func buildAliasItems(aliases []db.Alias) []list.Item {
	items := make([]list.Item, 0, len(aliases))
	for _, a := range aliases {
		items = append(items, aliasListItem{
			title: strings.TrimSpace(a.Title),
			email: a.Email,
		})
	}
	return items
}

type senderListItem struct {
	label string
}

func (i senderListItem) Title() string {
	return i.label
}

func (i senderListItem) Description() string {
	return ""
}

func (i senderListItem) FilterValue() string {
	return i.label
}

func buildSenderItems(entries []senderListEntry) []list.Item {
	items := make([]list.Item, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDomain {
			items = append(items, senderListItem{label: "@" + entry.Domain})
			continue
		}
		if entry.Sender != nil {
			items = append(items, senderListItem{label: entry.Sender.SenderEmail})
		}
	}
	return items
}

// newAliasList creates a list model for the alias pane.
func newAliasList() list.Model {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetSpacing(0)
	delegate.Styles.NormalTitle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Padding(0, 0, 0, 1)
	delegate.Styles.SelectedTitle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(colorFocused).
		Foreground(lipgloss.Color("15"))

	l := list.New(nil, delegate, 1, 1)
	l.SetShowTitle(false)
	l.SetShowFilter(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()
	return l
}

// newSenderList creates a list model for the sender pane.
func newSenderList() list.Model {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetSpacing(0)
	delegate.Styles.NormalTitle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Padding(0, 0, 0, 1)
	delegate.Styles.SelectedTitle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(colorFocused).
		Foreground(lipgloss.Color("15"))

	l := list.New(nil, delegate, 1, 1)
	l.SetShowTitle(false)
	l.SetShowFilter(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()
	return l
}
