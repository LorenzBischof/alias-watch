package tui

import (
	"fmt"
	"net/mail"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lorenzbischof/email-monitoring/internal/db"
)

type mode int

const (
	modeBrowse mode = iota
	modeFilter
	modeAdd
	modeConfirmDelete
	modeDetail
	modeEditTitle
)

type pane int

const (
	paneLeft  pane = iota
	paneRight pane = iota
)

type senderListEntry struct {
	Sender   *db.KnownSender
	Domain   string
	IsDomain bool
}

// Model is the top-level Bubble Tea model for the TUI.
type Model struct {
	store  *db.Store
	width  int
	height int

	aliases     []db.Alias
	accounts    map[string][]string // alias email → account names
	lastSeen    map[string]time.Time
	senders     []db.KnownSender
	domains     map[string]db.KnownDomain // sender domain -> domain rule
	entries     []senderListEntry
	senderIndex map[string][]string // alias email -> sender emails

	focus      pane
	aliasList  list.Model
	senderList list.Model

	currentMode  mode
	formInputs   []textinput.Model
	formFocusIdx int
	filterInput  textinput.Model

	statusMsg string
	err       error

	keys KeyMap
}

// New creates and initialises a new TUI model.
func New(store *db.Store) (*Model, error) {
	m := &Model{
		store:       store,
		keys:        DefaultKeyMap(),
		aliasList:   newAliasList(),
		senderList:  newSenderList(),
		filterInput: newFilterInput(),
	}
	m.aliasList.Filter = m.aliasFilter
	if err := m.reloadAliases(); err != nil {
		return nil, err
	}
	if len(m.aliases) > 0 {
		if err := m.reloadSenders(); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizePanes()
		return m, nil

	case tea.KeyMsg:
		switch m.currentMode {
		case modeBrowse:
			return m.updateBrowse(msg)
		case modeFilter:
			return m.updateFilter(msg)
		case modeAdd:
			return m.updateAdd(msg)
		case modeConfirmDelete:
			return m.updateConfirmDelete(msg)
		case modeDetail:
			return m.updateDetail(msg)
		case modeEditTitle:
			return m.updateEditTitle(msg)
		}
	}
	return m, nil
}

func (m *Model) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.String() == "ctrl+c":
		return m, tea.Quit

	case msg.String() == "q":
		return m, tea.Quit

	case msg.String() == "tab":
		m.toggleFocus()
		return m, nil

	case msg.String() == "left":
		if m.focus != paneLeft {
			m.focus = paneLeft
		}
		return m, nil

	case msg.String() == "right":
		if m.focus != paneRight {
			m.focus = paneRight
		}
		return m, nil

	case msg.String() == "/" && m.focus == paneLeft:
		m.currentMode = modeFilter
		m.filterInput.Focus()
		return m, nil
	}

	if m.focus == paneLeft {
		if msg.String() == "enter" {
			m.toggleFocus()
			return m, nil
		}
		if msg.String() == "r" {
			alias := m.currentAlias()
			if alias == nil {
				return m, nil
			}
			ti := textinput.New()
			ti.Placeholder = "alias title"
			ti.CharLimit = 256
			ti.SetValue(alias.Title)
			ti.Focus()
			m.formInputs = []textinput.Model{ti}
			m.formFocusIdx = 0
			m.currentMode = modeEditTitle
			m.statusMsg = ""
			return m, textinput.Blink
		}
		prev := m.aliasList.Index()
		var cmd tea.Cmd
		m.aliasList, cmd = m.aliasList.Update(msg)
		if m.aliasList.Index() != prev {
			m.senderList.Select(0)
			if err := m.reloadSenders(); err != nil {
				m.err = err
			}
		}
		return m, cmd
	}

	// Right pane
	switch msg.String() {
	case "enter":
		if len(m.entries) > 0 {
			m.currentMode = modeDetail
		}
		return m, nil

	case "n":
		m.currentMode = modeAdd
		m.formInputs = newAddForm()
		m.formFocusIdx = 0
		m.statusMsg = ""
		return m, textinput.Blink

	case "d":
		if len(m.entries) == 0 {
			return m, nil
		}
		m.currentMode = modeConfirmDelete
		m.statusMsg = "Delete selected entry? [y/enter] confirm  [n/esc] cancel"
		return m, nil

	case "f":
		return m, m.toggleFlagged()

	case "e":
		return m, m.toggleDomainRule()

	default:
		var cmd tea.Cmd
		m.senderList, cmd = m.senderList.Update(msg)
		return m, cmd
	}
}

func (m *Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.filterInput.SetValue("")
		m.applyAliasFilter("")
		m.currentMode = modeBrowse
		m.filterInput.Blur()
		return m, nil
	case "enter":
		m.currentMode = modeBrowse
		m.filterInput.Blur()
		return m, nil
	case "tab":
		// Leave filter edit mode and execute the key in normal browse mode.
		m.currentMode = modeBrowse
		m.filterInput.Blur()
		return m.updateBrowse(msg)
	case "left", "right":
		// Leave filter edit mode and execute the key in normal browse mode.
		m.currentMode = modeBrowse
		m.filterInput.Blur()
		return m.updateBrowse(msg)
	case "up", "down":
		prev := m.aliasList.Index()
		var cmd tea.Cmd
		m.aliasList, cmd = m.aliasList.Update(msg)
		if m.aliasList.Index() != prev {
			m.senderList.Select(0)
			if err := m.reloadSenders(); err != nil {
				m.err = err
			}
		}
		return m, cmd
	}

	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	m.applyAliasFilter(m.filterInput.Value())
	return m, cmd
}

func (m *Model) updateAdd(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.currentMode = modeBrowse
		m.statusMsg = ""
		return m, nil

	case "enter":
		return m, m.submitAddForm()

	case "tab":
		m.formFocusIdx = (m.formFocusIdx + 1) % len(m.formInputs)
		return m, m.syncFormFocus()

	case "shift+tab":
		m.formFocusIdx = (m.formFocusIdx - 1 + len(m.formInputs)) % len(m.formInputs)
		return m, m.syncFormFocus()
	}

	var cmds []tea.Cmd
	var batchCmds []tea.Cmd
	m.formInputs, batchCmds = updateFormInputs(m.formInputs, m.formFocusIdx, msg)
	cmds = append(cmds, batchCmds...)
	return m, tea.Batch(cmds...)
}

func (m *Model) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "y":
		return m, m.confirmDelete()
	case "esc", "n":
		m.currentMode = modeBrowse
		m.statusMsg = ""
	}
	return m, nil
}

func (m *Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.currentMode = modeBrowse
	case "f":
		return m, m.toggleFlagged()
	case "e":
		return m, m.toggleDomainRule()
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) updateEditTitle(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.currentMode = modeBrowse
		m.statusMsg = ""
		return m, nil
	case "enter":
		return m, m.submitEditTitle()
	}
	var batchCmds []tea.Cmd
	m.formInputs, batchCmds = updateFormInputs(m.formInputs, 0, msg)
	return m, tea.Batch(batchCmds...)
}

func (m *Model) submitEditTitle() tea.Cmd {
	alias := m.currentAlias()
	if alias == nil {
		m.currentMode = modeBrowse
		return nil
	}
	aliasEmail := alias.Email
	desc := strings.TrimSpace(m.formInputs[0].Value())
	if err := m.store.UpdateAliasTitle(aliasEmail, desc); err != nil {
		m.err = err
		m.currentMode = modeBrowse
		return nil
	}
	m.err = nil
	m.statusMsg = "Title updated."
	m.currentMode = modeBrowse
	_ = m.reloadAliases()
	return nil
}

func (m *Model) renderEditTitle() string {
	alias := m.currentAlias()
	if alias == nil {
		return m.renderMainView()
	}

	inputLine := "> Title: " + m.formInputs[0].View()
	content := fmt.Sprintf("Edit alias: %s\n\n%s\n\n[Enter] save  [Esc] cancel",
		alias.Email, inputLine)

	popupW := 70
	if popupW > m.width-4 {
		popupW = m.width - 4
	}
	popup := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorFocused).
		Padding(1, 2).
		Width(popupW).
		Render(content)

	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		popup,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceBackground(colorBackdrop),
	)
}

func (m *Model) renderDetail() string {
	entry := m.currentEntry()
	if entry == nil {
		return ""
	}
	if entry.IsDomain {
		emails := m.senderEmailsForDomain(entry.Domain)
		emailLines := "  (none)"
		if len(emails) > 0 {
			var b strings.Builder
			limit := len(emails)
			if limit > 12 {
				limit = 12
			}
			for i := 0; i < limit; i++ {
				b.WriteString("  ")
				b.WriteString(emails[i])
				b.WriteString("\n")
			}
			if len(emails) > limit {
				b.WriteString(fmt.Sprintf("  ... and %d more", len(emails)-limit))
			}
			emailLines = strings.TrimRight(b.String(), "\n")
		}

		content := fmt.Sprintf(
			"Domain:       %s\nDomain Rule:  %s   [e] toggle\nEmails:\n%s\n\n[Esc] close",
			entry.Domain, m.domainRuleState(entry.Domain), emailLines,
		)
		popupW := 60
		if popupW > m.width-4 {
			popupW = m.width - 4
		}
		popup := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorFocused).
			Padding(1, 2).
			Width(popupW).
			Render(content)

		return lipgloss.Place(
			m.width,
			m.height,
			lipgloss.Center,
			lipgloss.Center,
			popup,
			lipgloss.WithWhitespaceChars(" "),
			lipgloss.WithWhitespaceBackground(colorBackdrop),
		)
	}
	ks := entry.Sender
	if ks == nil {
		return ""
	}

	flaggedStr := "no"
	if ks.Flagged {
		flaggedStr = "YES"
	}
	firstSeen := ks.FirstSeen.Format("2006-01-02")
	if ks.FirstSeen.IsZero() {
		firstSeen = "-"
	}
	lastSeen := ks.LastSeen.Format("2006-01-02")
	if ks.LastSeen.IsZero() {
		lastSeen = "-"
	}

	content := fmt.Sprintf(
		"Email:        %s\nDomain:       %s\nDomain Rule:  %s   [e] toggle\nFlagged:      %s   [f] toggle\nFirst Seen:   %s\nLast Seen:    %s\n\n[Esc] close",
		ks.SenderEmail, ks.SenderDomain, m.domainRuleState(ks.SenderDomain), flaggedStr, firstSeen, lastSeen,
	)

	popupW := 60
	if popupW > m.width-4 {
		popupW = m.width - 4
	}

	popup := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorFocused).
		Padding(1, 2).
		Width(popupW).
		Render(content)

	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		popup,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceBackground(colorBackdrop),
	)
}

// View implements tea.Model.
func (m *Model) View() string {
	if m.width == 0 {
		return "Loading…"
	}
	if m.currentMode == modeDetail {
		return m.renderDetail()
	}
	if m.currentMode == modeEditTitle {
		return m.renderEditTitle()
	}
	return m.renderMainView()
}

func (m *Model) renderMainView() string {
	// Border overhead: 2 cols per pane (left+right border), 1 col gutter
	leftW, rightW := m.paneWidths()

	leftBorder := styleUnfocusedBorder
	rightBorder := styleUnfocusedBorder
	if m.focus == paneLeft {
		leftBorder = styleFocusedBorder
	} else {
		rightBorder = styleFocusedBorder
	}

	innerH := m.height - 4 // account for borders + status bar + help line
	if m.currentMode == modeFilter || strings.TrimSpace(m.filterInput.Value()) != "" {
		innerH--
	}
	if innerH < 1 {
		innerH = 1
	}

	aliasListView := strings.TrimPrefix(m.aliasList.View(), "\n")
	leftContent := leftBorder.
		Width(leftW).
		Height(innerH).
		Render(fmt.Sprintf(" %s\n%s", styleLeftTitle.Render("Aliases"), aliasListView))

	selectedAlias := m.currentAlias()

	// Right panel header mirrors left list format: title (white), email (grey).
	aliasTitle := "-"
	aliasEmail := "-"
	if selectedAlias != nil {
		if t := strings.TrimSpace(selectedAlias.Title); t != "" {
			aliasTitle = t
		}
		if selectedAlias.Email != "" {
			aliasEmail = selectedAlias.Email
		}
	}
	rightInnerW := max(1, rightW-2)
	headerPrefix := " "
	headerGap := " "
	headerOverhead := len([]rune(headerPrefix)) + len([]rune(headerGap))
	maxTitle := rightInnerW - headerOverhead - len([]rune(aliasEmail))
	if maxTitle < 1 {
		maxTitle = 1
	}
	aliasTitle = clampRunes(aliasTitle, maxTitle)
	maxEmail := rightInnerW - headerOverhead - len([]rune(aliasTitle))
	if maxEmail < 1 {
		maxEmail = 1
	}
	aliasEmail = clampRunes(aliasEmail, maxEmail)
	aliasHeaderText := headerPrefix + styleAliasHeader.Render(aliasTitle) + headerGap + styleAliasMeta.Render(aliasEmail)
	aliasHeader := lipgloss.NewStyle().MaxWidth(rightW - 2).Render(aliasHeaderText)

	meta := styleAliasMeta.Render(m.renderAliasMeta(rightInnerW))
	senderListView := strings.TrimPrefix(m.senderList.View(), "\n")

	var rightInner string
	switch m.currentMode {
	case modeAdd:
		rightInner = fmt.Sprintf("%s\n\nAdd Sender:\n%s\n%s",
			aliasHeader, renderForm(m.formInputs, m.formFocusIdx), meta)
	case modeConfirmDelete:
		rightInner = fmt.Sprintf("%s\n%s\n%s", aliasHeader, senderListView, meta)
	default:
		rightInner = fmt.Sprintf("%s\n%s\n%s", aliasHeader, senderListView, meta)
	}

	rightContent := rightBorder.
		Width(rightW).
		Height(innerH).
		Render(rightInner)

	paneRow := lipgloss.JoinHorizontal(lipgloss.Top, leftContent, " ", rightContent)

	status := ""
	if m.err != nil {
		status = styleStatusBar.Foreground(colorFlagged).Render("Error: " + m.err.Error())
	} else if m.statusMsg != "" {
		status = styleStatusBar.Render(m.statusMsg)
	}

	help := styleHelp.Render(m.helpText())
	if m.currentMode == modeFilter {
		cmdline := styleStatusBar.Render(" / " + m.filterInput.View())
		return strings.Join([]string{paneRow, status, cmdline, help}, "\n")
	}
	if q := strings.TrimSpace(m.filterInput.Value()); q != "" {
		cmdline := styleHelp.Render(" / " + q)
		return strings.Join([]string{paneRow, status, cmdline, help}, "\n")
	}
	return strings.Join([]string{paneRow, status, help}, "\n")
}

func (m *Model) renderAliasMeta(width int) string {
	labelWidth := len(" Last Seen: ")
	valueWidth := width - labelWidth
	if valueWidth < 1 {
		valueWidth = 1
	}
	a := m.currentAlias()
	if a == nil {
		return " Title: -\n Email: -\n Active: -\n Last Seen: -"
	}

	title := strings.TrimSpace(a.Title)
	if title == "" {
		title = "-"
	}

	active := "yes"
	if !a.Active {
		active = "no"
	}

	lastSeen := "-"
	if ts := m.lastSeen[a.Email]; !ts.IsZero() {
		lastSeen = ts.Local().Format("2006-01-02 15:04")
	}

	return fmt.Sprintf(
		" Title: %s\n Email: %s\n Active: %s\n Last Seen: %s",
		clampRunes(title, valueWidth),
		clampRunes(a.Email, valueWidth),
		clampRunes(active, valueWidth),
		clampRunes(lastSeen, valueWidth),
	)
}

func clampRunes(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	r := []rune(strings.ReplaceAll(s, "\n", " "))
	if len(r) <= maxWidth {
		return string(r)
	}
	if maxWidth == 1 {
		return "…"
	}
	return string(r[:maxWidth-1]) + "…"
}

// --- helpers ---

func (m *Model) helpText() string {
	switch m.currentMode {
	case modeFilter:
		return "Type to filter | Enter: done | Esc: clear | Tab/←/→: switch pane | q: quit"
	case modeAdd:
		return "Tab/Shift+Tab: switch field | Enter: submit | Esc: cancel"
	case modeConfirmDelete:
		return "Delete selected entry? Enter/y: confirm | Esc/n: cancel"
	case modeEditTitle:
		return "Enter: save | Esc: cancel"
	case modeDetail:
		entry := m.currentEntry()
		parts := []string{"Esc/q: close", "e: toggle domain rule", "q: quit"}
		if entry != nil && !entry.IsDomain {
			parts = append(parts, "f: toggle flagged")
		}
		return strings.Join(parts, " | ")
	}

	if m.focus == paneLeft {
		return "Tab/→/Enter: switch pane | /: filter | r: rename | q: quit"
	}

	parts := []string{"Tab/←: switch pane", "n: add sender"}
	if len(m.entries) > 0 {
		parts = append(parts, "Enter: detail", "d: delete", "e: toggle domain rule")
		if entry := m.currentEntry(); entry != nil && !entry.IsDomain {
			parts = append(parts, "f: toggle flagged")
		}
	}
	parts = append(parts, "q: quit")
	return strings.Join(parts, " | ")
}

func (m *Model) toggleFocus() {
	if m.focus == paneLeft {
		m.focus = paneRight
	} else {
		m.focus = paneLeft
	}
}

func (m *Model) paneWidths() (left, right int) {
	// Two rounded borders × 2 chars each = 4 total horizontal overhead,
	// plus one explicit gutter between panes and one safety column to avoid
	// clipping the right-most border at the terminal edge.
	usable := m.width - 6
	if usable < 2 {
		return 1, 1
	}
	left = usable * 2 / 5
	right = usable - left
	return
}

func newFilterInput() textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "search alias or sender (/)"
	ti.CharLimit = 120
	return ti
}

func (m *Model) resizePanes() {
	leftW, rightW := m.paneWidths()
	leftTableH := m.height - 6 // borders + title row + status + help
	if leftTableH < 1 {
		leftTableH = 1
	}

	// Left pane list: title on first line, alias email on second line.
	m.aliasList.SetSize(leftW, leftTableH)

	// Right pane: reserve rows for header + metadata + separator rows.
	// Keep this in sync with rightInner format: header + "\n" + sender list + "\n" + meta.
	rightTableH := m.height - 11
	if rightTableH < 1 {
		rightTableH = 1
	}
	m.senderList.SetSize(rightW, rightTableH)
}

func (m *Model) reloadAliases() error {
	prevAlias := m.currentAliasEmail()

	aliases, err := m.store.AllAliases()
	if err != nil {
		return err
	}

	lastSeen, err := m.store.AliasLastSeen()
	if err != nil {
		return err
	}

	sort.SliceStable(aliases, func(i, j int) bool {
		iSeen := lastSeen[aliases[i].Email]
		jSeen := lastSeen[aliases[j].Email]
		if iSeen.Equal(jSeen) {
			return aliases[i].Email < aliases[j].Email
		}
		if iSeen.IsZero() {
			return false
		}
		if jSeen.IsZero() {
			return true
		}
		return iSeen.After(jSeen)
	})

	m.aliases = aliases
	m.lastSeen = lastSeen

	// Pre-load accounts for all aliases
	m.accounts = make(map[string][]string, len(aliases))
	for _, a := range aliases {
		accs, err := m.store.AccountsForAlias(a.Email)
		if err != nil {
			return err
		}
		m.accounts[a.Email] = accs
	}
	m.senderIndex = make(map[string][]string, len(aliases))
	known, err := m.store.AllKnownSenders()
	if err != nil {
		return err
	}
	for _, ks := range known {
		m.senderIndex[ks.AliasEmail] = append(m.senderIndex[ks.AliasEmail], ks.SenderEmail)
	}

	m.aliasList.SetItems(buildAliasItems(m.aliases))
	m.applyAliasFilter(m.filterInput.Value())
	m.selectAliasByEmail(prevAlias)
	return nil
}

func (m *Model) reloadSenders() error {
	alias := m.currentAlias()
	if alias == nil {
		m.senders = nil
		m.domains = nil
		m.entries = nil
		m.senderList.SetItems(nil)
		return nil
	}
	senders, err := m.store.KnownSendersForAlias(alias.Email)
	if err != nil {
		return err
	}
	domains, err := m.store.KnownDomainsForAlias(alias.Email)
	if err != nil {
		return err
	}
	m.domains = make(map[string]db.KnownDomain, len(domains))
	for _, d := range domains {
		m.domains[d.SenderDomain] = d
	}
	m.senders = senders
	aliasEmail := alias.Email
	m.senderIndex[aliasEmail] = make([]string, 0, len(senders))
	for _, s := range senders {
		m.senderIndex[aliasEmail] = append(m.senderIndex[aliasEmail], s.SenderEmail)
	}
	m.entries = m.buildSenderListEntries(senders)
	m.senderList.SetItems(buildSenderItems(m.entries))
	return nil
}

func (m *Model) currentEntry() *senderListEntry {
	idx := m.senderList.Index()
	if idx < 0 || idx >= len(m.entries) {
		return nil
	}
	entry := m.entries[idx]
	return &entry
}

func (m *Model) currentSender() *db.KnownSender {
	entry := m.currentEntry()
	if entry == nil || entry.IsDomain || entry.Sender == nil {
		return nil
	}
	ks := *entry.Sender
	return &ks
}

func (m *Model) toggleFlagged() tea.Cmd {
	ks := m.currentSender()
	if ks == nil {
		m.statusMsg = "Flagging is only available for sender email entries."
		return nil
	}
	ks.Flagged = !ks.Flagged
	ks.LastSeen = ks.LastSeen // preserve
	if err := m.store.UpdateKnownSender(*ks); err != nil {
		m.err = err
		return nil
	}
	m.err = nil
	if ks.Flagged {
		m.statusMsg = "Flagged sender."
	} else {
		m.statusMsg = "Unflagged sender."
	}
	cursor := m.senderList.Index()
	_ = m.reloadSenders()
	m.senderList.Select(cursor)
	return nil
}

func (m *Model) toggleDomainRule() tea.Cmd {
	entry := m.currentEntry()
	if entry == nil {
		return nil
	}
	alias := m.currentAlias()
	if alias == nil {
		return nil
	}
	aliasEmail := alias.Email
	domain := entry.Domain
	if domain == "" && entry.Sender != nil {
		domain = entry.Sender.SenderDomain
	}
	if domain == "" {
		return nil
	}

	rule, exists := m.domains[domain]
	if exists && rule.Enabled {
		if err := m.store.DeleteKnownDomain(aliasEmail, domain); err != nil {
			m.err = err
			return nil
		}
		m.statusMsg = fmt.Sprintf("Domain rule disabled for %s.", domain)
	} else {
		now := time.Now()
		if err := m.store.UpsertKnownDomain(db.KnownDomain{
			AliasEmail:   aliasEmail,
			SenderDomain: domain,
			Enabled:      true,
			CreatedAt:    now,
			UpdatedAt:    now,
		}); err != nil {
			m.err = err
			return nil
		}
		m.statusMsg = fmt.Sprintf("Domain rule enabled for %s.", domain)
	}

	m.err = nil
	cursor := m.senderList.Index()
	_ = m.reloadSenders()
	m.senderList.Select(cursor)
	return nil
}

func (m *Model) domainRuleState(domain string) string {
	rule, ok := m.domains[domain]
	if !ok || !rule.Enabled {
		return "disabled"
	}
	return "enabled"
}

func (m *Model) buildSenderListEntries(senders []db.KnownSender) []senderListEntry {
	enabledDomains := make(map[string]struct{})
	for domain, rule := range m.domains {
		if rule.Enabled {
			enabledDomains[domain] = struct{}{}
		}
	}

	var domains []string
	for domain := range enabledDomains {
		domains = append(domains, domain)
	}
	sort.Strings(domains)

	entries := make([]senderListEntry, 0, len(domains)+len(senders))
	for _, domain := range domains {
		entries = append(entries, senderListEntry{
			Domain:   domain,
			IsDomain: true,
		})
	}

	for i := range senders {
		s := senders[i]
		if _, hiddenByDomainRule := enabledDomains[s.SenderDomain]; hiddenByDomainRule {
			continue
		}
		sCopy := s
		entries = append(entries, senderListEntry{
			Sender: &sCopy,
			Domain: sCopy.SenderDomain,
		})
	}
	return entries
}

func (m *Model) senderEmailsForDomain(domain string) []string {
	var out []string
	for _, s := range m.senders {
		if s.SenderDomain == domain {
			out = append(out, s.SenderEmail)
		}
	}
	sort.Strings(out)
	return out
}

func (m *Model) applyAliasFilter(query string) {
	prevAlias := m.currentAliasEmail()
	q := strings.TrimSpace(query)
	m.filterInput.SetValue(q)
	if q == "" {
		m.aliasList.ResetFilter()
	} else {
		m.aliasList.SetFilterText(q)
	}
	if prevAlias != "" {
		m.selectAliasByEmail(prevAlias)
	}
	if m.currentAlias() == nil {
		m.senders = nil
		m.domains = nil
		m.entries = nil
		m.senderList.SetItems(nil)
		return
	}
	m.senderList.Select(0)
	_ = m.reloadSenders()
}

func (m *Model) aliasFilter(term string, _ []string) []list.Rank {
	q := strings.TrimSpace(strings.ToLower(term))
	if q == "" {
		return nil
	}
	type scoredRank struct {
		rank  list.Rank
		score int
	}
	ranks := make([]scoredRank, 0, len(m.aliases))
	for i, a := range m.aliases {
		score := aliasMatchScore(a, q)
		if senderScore := senderMatchScore(m.senderIndex[a.Email], q); senderScore > score {
			score = senderScore
		}
		if score < 0 {
			continue
		}
		ranks = append(ranks, scoredRank{
			rank: list.Rank{Index: i},
			score: score,
		})
	}
	sort.SliceStable(ranks, func(i, j int) bool {
		return ranks[i].score > ranks[j].score
	})
	out := make([]list.Rank, len(ranks))
	for i := range ranks {
		out[i] = ranks[i].rank
	}
	return out
}

func aliasMatchScore(a db.Alias, q string) int {
	best := -1
	if score := matchScore(strings.ToLower(strings.TrimSpace(a.Title)), q, 420, 390, 360, 320); score > best {
		best = score
	}
	if score := matchScore(strings.ToLower(strings.TrimSpace(a.Email)), q, 280, 250, 220, 180); score > best {
		best = score
	}
	return best
}

func senderMatchScore(senders []string, q string) int {
	best := -1
	for _, sender := range senders {
		score := matchScore(strings.ToLower(sender), q, 340, 320, 290, 240)
		if score > best {
			best = score
		}
	}
	return best
}

func matchScore(value, q string, exact, prefix, contains, fuzzy int) int {
	switch {
	case value == q:
		return exact
	case strings.HasPrefix(value, q):
		return prefix
	case strings.Contains(value, q):
		return contains
	case fuzzyMatch(value, q):
		return fuzzy
	default:
		return -1
	}
}

func fuzzyMatch(value, query string) bool {
	v := strings.ToLower(value)
	q := strings.ToLower(query)
	if q == "" {
		return true
	}
	if strings.Contains(v, q) {
		return true
	}
	qr := []rune(q)
	i := 0
	for _, r := range v {
		if r == qr[i] {
			i++
			if i == len(qr) {
				return true
			}
		}
	}
	return false
}

func (m *Model) confirmDelete() tea.Cmd {
	entry := m.currentEntry()
	m.currentMode = modeBrowse
	m.statusMsg = ""
	if entry == nil {
		return nil
	}
	m.senderList.Select(0)
	if entry.IsDomain {
		alias := m.currentAlias()
		if alias == nil {
			return nil
		}
		aliasEmail := alias.Email
		if err := m.store.DeleteKnownDomain(aliasEmail, entry.Domain); err != nil {
			m.err = err
			return nil
		}
		m.statusMsg = fmt.Sprintf("Disabled domain rule %s.", entry.Domain)
	} else {
		ks := entry.Sender
		if ks == nil {
			return nil
		}
		if err := m.store.DeleteKnownSender(ks.ID); err != nil {
			m.err = err
			return nil
		}
		m.statusMsg = fmt.Sprintf("Deleted sender %s.", ks.SenderEmail)
	}
	m.err = nil
	_ = m.reloadSenders()
	return nil
}

func (m *Model) syncFormFocus() tea.Cmd {
	cmds := make([]tea.Cmd, len(m.formInputs))
	for i := range m.formInputs {
		if i == m.formFocusIdx {
			cmds[i] = m.formInputs[i].Focus()
		} else {
			m.formInputs[i].Blur()
		}
	}
	return tea.Batch(cmds...)
}

func (m *Model) submitAddForm() tea.Cmd {
	senderEmail := strings.TrimSpace(m.formInputs[0].Value())

	if senderEmail == "" {
		m.statusMsg = "Sender email cannot be empty."
		return nil
	}

	// Validate email format
	addr, err := mail.ParseAddress(senderEmail)
	if err != nil {
		m.statusMsg = fmt.Sprintf("Invalid email address: %v", err)
		return nil
	}
	senderEmail = addr.Address

	// Extract domain
	parts := strings.SplitN(senderEmail, "@", 2)
	domain := ""
	if len(parts) == 2 {
		domain = parts[1]
	}

	if len(m.aliases) == 0 {
		return nil
	}
	alias := m.currentAlias()
	if alias == nil {
		return nil
	}
	aliasEmail := alias.Email

	now := time.Now()
	ks := db.KnownSender{
		AliasEmail:   aliasEmail,
		SenderEmail:  senderEmail,
		SenderDomain: domain,
		FirstSeen:    now,
		LastSeen:     now,
	}

	m.senderList.Select(0)
	_, err = m.store.UpsertKnownSender(ks)
	if err != nil {
		m.err = err
		return nil
	}
	m.err = nil
	m.statusMsg = fmt.Sprintf("Added sender %s.", senderEmail)
	m.currentMode = modeBrowse
	_ = m.reloadSenders()
	return nil
}

func (m *Model) currentAliasEmail() string {
	alias := m.currentAlias()
	if alias == nil {
		return ""
	}
	return alias.Email
}

func (m *Model) currentAlias() *db.Alias {
	item, ok := m.aliasList.SelectedItem().(aliasListItem)
	if !ok {
		return nil
	}
	for i := range m.aliases {
		if m.aliases[i].Email == item.email {
			return &m.aliases[i]
		}
	}
	return nil
}

func (m *Model) selectAliasByEmail(email string) {
	if email == "" {
		return
	}
	visible := m.aliasList.VisibleItems()
	for i := range visible {
		item, ok := visible[i].(aliasListItem)
		if !ok {
			continue
		}
		if item.email == email {
			m.aliasList.Select(i)
			return
		}
	}
}

// truncate shortens s to at most maxRunes runes, appending "…" if truncated.
func truncate(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	if maxRunes < 1 {
		return ""
	}
	return string(runes[:maxRunes-1]) + "…"
}
