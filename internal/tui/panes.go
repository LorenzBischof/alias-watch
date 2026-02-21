package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/table"

	"github.com/lorenzbischof/email-monitoring/internal/db"
)

// aliasColumns defines the left-pane column layout.
var aliasColumns = []table.Column{
	{Title: "Alias Email", Width: 40},
}

// buildAliasRows converts aliases into table rows.
func buildAliasRows(aliases []db.Alias) []table.Row {
	rows := make([]table.Row, 0, len(aliases))
	for _, a := range aliases {
		rows = append(rows, table.Row{a.Email})
	}
	return rows
}

// buildSenderRows converts sender/domain entries into table rows.
func buildSenderRows(entries []senderListEntry, width int) []table.Row {
	rows := make([]table.Row, 0, len(entries))
	for _, entry := range entries {
		if width < 1 {
			width = 1
		}
		if entry.IsDomain {
			label := entry.Domain
			if width > 2 {
				usable := width - 1
				label = truncate(label, usable)
				pad := usable - len([]rune(label))
				if pad < 1 {
					pad = 1
				}
				label = label + strings.Repeat(" ", pad) + "@"
			}
			rows = append(rows, table.Row{label})
			continue
		}
		if entry.Sender != nil {
			label := entry.Sender.SenderEmail
			label = truncate(label, width)
			rows = append(rows, table.Row{label})
		}
	}
	return rows
}

// newAliasTable creates a new table model for the alias pane.
func newAliasTable() table.Model {
	t := table.New(
		table.WithColumns(aliasColumns),
		table.WithFocused(true),
	)
	s := table.DefaultStyles()
	t.SetStyles(s)
	return t
}

// newSenderTable creates a new table model for the sender pane.
// The column width is a placeholder; resizePanes sets the real width on WindowSizeMsg.
func newSenderTable() table.Model {
	t := table.New(
		table.WithColumns([]table.Column{{Title: "Sender / Domain", Width: 40}}),
		table.WithFocused(false),
	)
	s := table.DefaultStyles()
	t.SetStyles(s)
	return t
}
