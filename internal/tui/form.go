package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// newAddForm creates one textinput model for sender_email.
func newAddForm() []textinput.Model {
	emailInput := textinput.New()
	emailInput.Placeholder = "sender@example.com"
	emailInput.CharLimit = 256
	emailInput.Focus()

	return []textinput.Model{emailInput}
}

// renderForm renders the add-sender form as a string.
func renderForm(inputs []textinput.Model, focusIdx int) string {
	var sb strings.Builder
	labels := []string{"Sender Email: "}
	for i, inp := range inputs {
		prefix := "  "
		if i == focusIdx {
			prefix = "> "
		}
		sb.WriteString(prefix + labels[i] + inp.View() + "\n")
	}
	return sb.String()
}

// updateFormInputs routes a tea.Msg to the focused input and returns updated
// inputs and any commands.
func updateFormInputs(inputs []textinput.Model, focusIdx int, msg tea.Msg) ([]textinput.Model, []tea.Cmd) {
	cmds := make([]tea.Cmd, len(inputs))
	for i := range inputs {
		inputs[i], cmds[i] = inputs[i].Update(msg)
	}
	return inputs, cmds
}
