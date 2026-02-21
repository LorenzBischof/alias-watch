package tui

import (
	"fmt"
	"net/mail"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
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
	allAliases  []db.Alias
	accounts    map[string][]string // alias email → account names
	lastSeen    map[string]time.Time
	senders     []db.KnownSender
	domains     map[string]db.KnownDomain // sender domain -> domain rule
	entries     []senderListEntry
	senderIndex map[string][]string // alias email -> sender emails

	focus       pane
	aliasTable  table.Model
	senderTable table.Model

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
		aliasTable:  newAliasTable(),
		senderTable: newSenderTable(),
		filterInput: newFilterInput(),
	}
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
		prev := m.aliasTable.Cursor()
		var cmd tea.Cmd
		m.aliasTable, cmd = m.aliasTable.Update(msg)
		if m.aliasTable.Cursor() != prev {
			m.senderTable.GotoTop()
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

	case "a":
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
		m.senderTable, cmd = m.senderTable.Update(msg)
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
	case "up", "down":
		prev := m.aliasTable.Cursor()
		var cmd tea.Cmd
		m.aliasTable, cmd = m.aliasTable.Update(msg)
		if m.aliasTable.Cursor() != prev {
			m.senderTable.GotoTop()
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
	return m.renderMainView()
}

func (m *Model) renderMainView() string {
	// Border overhead: 2 cols per pane (left+right border), 1 col gutter
	leftW, rightW := m.paneWidths()

	leftTitle := "Aliases"
	rightTitle := "Known Senders"

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

	leftContent := leftBorder.
		Width(leftW).
		Height(innerH).
		Render(fmt.Sprintf(" %s\n%s", leftTitle, m.aliasTable.View()))

	// Truncate rightTitle so it never wraps inside the box.
	rightTitle = truncate(rightTitle, rightW-2) // -2 for the leading space + margin

	meta := m.renderAliasMeta()

	var rightInner string
	switch m.currentMode {
	case modeAdd:
		rightInner = fmt.Sprintf(" %s\n%s\n\nAdd Sender:\n%s",
			rightTitle, meta, renderForm(m.formInputs, m.formFocusIdx))
	case modeConfirmDelete:
		rightInner = fmt.Sprintf(" %s\n%s\n%s", rightTitle, meta, m.senderTable.View())
	default:
		rightInner = fmt.Sprintf(" %s\n%s\n%s", rightTitle, meta, m.senderTable.View())
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

	help := styleHelp.Render("Tab/Enter: switch pane | / filter | [a]dd [d]el | Enter: detail | q: quit")
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

func (m *Model) renderAliasMeta() string {
	if len(m.aliases) == 0 {
		return " Alias: -\n Title: -\n Active: -\n Accounts: -\n Last Seen: -"
	}
	idx := m.aliasTable.Cursor()
	if idx < 0 || idx >= len(m.aliases) {
		return " Alias: -\n Title: -\n Active: -\n Accounts: -\n Last Seen: -"
	}

	a := m.aliases[idx]
	title := strings.TrimSpace(a.Description)
	if title == "" {
		title = "-"
	}
	active := "yes"
	if !a.Active {
		active = "no"
	}

	accs := strings.Join(m.accounts[a.Email], ", ")
	if accs == "" {
		accs = "UNMAPPED"
	}

	lastSeen := "-"
	if ts := m.lastSeen[a.Email]; !ts.IsZero() {
		lastSeen = ts.Local().Format("2006-01-02 15:04")
	}

	return fmt.Sprintf(
		" Alias: %s\n Title: %s\n Active: %s\n Accounts: %s\n Last Seen: %s",
		a.Email,
		title,
		active,
		accs,
		lastSeen,
	)
}

// --- helpers ---

func (m *Model) toggleFocus() {
	if m.focus == paneLeft {
		m.focus = paneRight
		m.aliasTable.Blur()
		m.senderTable.Focus()
	} else {
		m.focus = paneLeft
		m.senderTable.Blur()
		m.aliasTable.Focus()
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

func senderColumnWidth(rightPaneWidth int) int {
	// Reserve space for table rendering so content does not touch/overflow
	// pane borders.
	w := rightPaneWidth - 3
	if w < 1 {
		return 1
	}
	return w
}

func (m *Model) resizePanes() {
	leftW, rightW := m.paneWidths()
	leftTableH := m.height - 6 // borders + title row + status + help
	if leftTableH < 1 {
		leftTableH = 1
	}

	// Left pane: single alias email column.
	emailW := leftW
	m.aliasTable.SetColumns([]table.Column{
		{Title: "Alias Email", Width: emailW},
	})
	m.aliasTable.SetWidth(leftW)
	if leftTableH > 1 {
		leftTableH--
	}
	m.aliasTable.SetHeight(leftTableH)

	// Right pane: reserve rows for alias metadata shown above the sender table.
	rightTableH := m.height - 12
	if rightTableH < 1 {
		rightTableH = 1
	}
	senderColW := senderColumnWidth(rightW)
	m.senderTable.SetColumns([]table.Column{
		{Title: "Sender / Domain", Width: senderColW},
	})
	m.senderTable.SetWidth(rightW)
	m.senderTable.SetHeight(rightTableH)
}

func (m *Model) reloadAliases() error {
	var prevAlias string
	if len(m.aliases) > 0 {
		idx := m.aliasTable.Cursor()
		if idx >= 0 && idx < len(m.aliases) {
			prevAlias = m.aliases[idx].Email
		}
	}

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
	m.allAliases = aliases
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

	m.applyAliasFilter(m.filterInput.Value())
	if prevAlias != "" && len(m.aliases) > 0 {
		for i, a := range m.aliases {
			if a.Email == prevAlias {
				m.aliasTable.SetCursor(i)
				break
			}
		}
	}
	return nil
}

func (m *Model) reloadSenders() error {
	if len(m.aliases) == 0 {
		m.senders = nil
		m.domains = nil
		m.entries = nil
		m.senderTable.SetRows(nil)
		return nil
	}
	idx := m.aliasTable.Cursor()
	if idx < 0 || idx >= len(m.aliases) {
		return nil
	}
	senders, err := m.store.KnownSendersForAlias(m.aliases[idx].Email)
	if err != nil {
		return err
	}
	domains, err := m.store.KnownDomainsForAlias(m.aliases[idx].Email)
	if err != nil {
		return err
	}
	m.domains = make(map[string]db.KnownDomain, len(domains))
	for _, d := range domains {
		m.domains[d.SenderDomain] = d
	}
	m.senders = senders
	aliasEmail := m.aliases[idx].Email
	m.senderIndex[aliasEmail] = make([]string, 0, len(senders))
	for _, s := range senders {
		m.senderIndex[aliasEmail] = append(m.senderIndex[aliasEmail], s.SenderEmail)
	}
	m.entries = m.buildSenderListEntries(senders)
	_, rightW := m.paneWidths()
	m.senderTable.SetRows(buildSenderRows(m.entries, senderColumnWidth(rightW)))
	return nil
}

func (m *Model) currentEntry() *senderListEntry {
	idx := m.senderTable.Cursor()
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
	cursor := m.senderTable.Cursor()
	_ = m.reloadSenders()
	m.senderTable.SetCursor(cursor)
	return nil
}

func (m *Model) toggleDomainRule() tea.Cmd {
	entry := m.currentEntry()
	if entry == nil {
		return nil
	}
	if len(m.aliases) == 0 {
		return nil
	}
	aliasIdx := m.aliasTable.Cursor()
	if aliasIdx < 0 || aliasIdx >= len(m.aliases) {
		return nil
	}
	aliasEmail := m.aliases[aliasIdx].Email
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
	cursor := m.senderTable.Cursor()
	_ = m.reloadSenders()
	m.senderTable.SetCursor(cursor)
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
	prevAlias := ""
	if len(m.aliases) > 0 {
		idx := m.aliasTable.Cursor()
		if idx >= 0 && idx < len(m.aliases) {
			prevAlias = m.aliases[idx].Email
		}
	}

	q := strings.TrimSpace(query)
	if q == "" {
		m.aliases = append([]db.Alias(nil), m.allAliases...)
	} else {
		var senderMatches []db.Alias
		var aliasOnlyMatches []db.Alias
		for _, a := range m.allAliases {
			aliasMatch := fuzzyMatch(a.Email, q)
			senderMatch := false
			for _, senderEmail := range m.senderIndex[a.Email] {
				if fuzzyMatch(senderEmail, q) {
					senderMatch = true
					break
				}
			}
			if !aliasMatch && !senderMatch {
				continue
			}
			if senderMatch {
				senderMatches = append(senderMatches, a)
			} else {
				aliasOnlyMatches = append(aliasOnlyMatches, a)
			}
		}
		m.aliases = append(senderMatches, aliasOnlyMatches...)
	}

	m.aliasTable.SetRows(buildAliasRows(m.aliases))
	if len(m.aliases) == 0 {
		m.senders = nil
		m.entries = nil
		m.senderTable.SetRows(nil)
		return
	}

	cursor := 0
	if prevAlias != "" {
		for i, a := range m.aliases {
			if a.Email == prevAlias {
				cursor = i
				break
			}
		}
	}
	m.aliasTable.SetCursor(cursor)
	m.senderTable.GotoTop()
	_ = m.reloadSenders()
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
	m.senderTable.GotoTop()
	if entry.IsDomain {
		if len(m.aliases) == 0 {
			return nil
		}
		aliasIdx := m.aliasTable.Cursor()
		if aliasIdx < 0 || aliasIdx >= len(m.aliases) {
			return nil
		}
		aliasEmail := m.aliases[aliasIdx].Email
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
	aliasIdx := m.aliasTable.Cursor()
	if aliasIdx < 0 || aliasIdx >= len(m.aliases) {
		return nil
	}
	aliasEmail := m.aliases[aliasIdx].Email

	now := time.Now()
	ks := db.KnownSender{
		AliasEmail:   aliasEmail,
		SenderEmail:  senderEmail,
		SenderDomain: domain,
		FirstSeen:    now,
		LastSeen:     now,
	}

	m.senderTable.GotoTop()
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
