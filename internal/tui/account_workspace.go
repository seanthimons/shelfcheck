package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	bc "shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/hardcover"
)

const (
	accountOverview = iota
	accountLoans
	accountHolds
	accountHistory
	accountAllBooks
	accountReading
	accountWant
	accountRead
	accountPaused
	accountDNF
	accountIgnored
	accountSectionCount
)

var accountLabels = [accountSectionCount]string{
	"Overview", "Loans", "Holds", "Borrowing history", "All books",
	"Currently reading", "Want to read", "Read", "Paused", "Did not finish", "Ignored",
}
var accountStatuses = [accountSectionCount]hardcover.Status{
	accountReading: hardcover.CurrentlyReading, accountWant: hardcover.WantToRead,
	accountRead: hardcover.Read, accountPaused: hardcover.Paused,
	accountDNF: hardcover.DidNotFinish, accountIgnored: hardcover.Ignored,
}
var accountSortLabels = []string{"Recently added", "Title", "Author", "Rating", "Last read"}

type accountWorkspace struct {
	section   int
	sidebar   bool
	cursors   [accountSectionCount]int
	filters   [accountSectionCount]string
	sorts     [accountSectionCount]int
	filtering bool
	filter    textinput.Model
}

type acctRow struct {
	bib            bc.Bib
	hcID           int
	section        int
	source, status string
}

func (m Model) openAccount() (Model, tea.Cmd) {
	m.screen = accountScreen
	m.input.Blur()
	if m.acct == nil {
		return m, nil
	}
	var cmds []tea.Cmd
	if !m.acctLoading && !m.acctLoaded && (m.acct.HasCatalog() || m.acct.HasLibby()) {
		var cmd tea.Cmd
		m, cmd = m.loadAccount()
		cmds = append(cmds, cmd)
	}
	if !m.hcLibraryLoaded && !m.hcLibraryLoading {
		var cmd tea.Cmd
		m, cmd = m.loadAccountHardcover(false)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

// Library and Libby fail independently. Keep successful and partial data visible.
func (m Model) loadAccount() (Model, tea.Cmd) {
	if m.acct == nil || m.acctLoading {
		return m, nil
	}
	m.acctLoading, m.acctErr, m.libbyErr = true, nil, nil
	client, st := m.client, m.acct
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		var msg accountMsg
		if st.HasCatalog() {
			msg.err = st.WithSession(ctx, client, func(s *bc.Session) error {
				var err error
				if msg.checkouts, err = client.Checkouts(ctx, s); err != nil {
					return err
				}
				if msg.holds, err = client.Holds(ctx, s); err != nil {
					return err
				}
				hist, err := client.History(ctx, s)
				msg.recent = hist[:min(recentCount, len(hist))]
				return err
			})
		}
		msg.checkouts, msg.holds, msg.libbyErr = st.WithLibby(ctx, msg.checkouts, msg.holds)
		return msg
	}
}

func (m Model) loadAccountHardcover(refresh bool) (Model, tea.Cmd) {
	if m.acct == nil || m.hcLibraryLoading {
		return m, nil
	}
	hc := m.acct.HardcoverClient()
	if hc == nil {
		return m, nil
	}
	m.hcLibraryLoading, m.hcLibraryErr = true, nil
	m.hcLibrarySeq++
	seq := m.hcLibrarySeq
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if refresh {
			hc.RefreshMetadata(ctx)
		}
		lib, err := hc.Library(ctx)
		return hcLibraryMsg{seq: seq, lib: hardcover.Shelves(lib), err: err}
	}
}

func (m Model) accountBooks(section int, filter string, order int) []hardcover.UserBook {
	var books []hardcover.UserBook
	filter = strings.ToLower(strings.TrimSpace(filter))
	for _, b := range m.hcLib {
		if section != accountAllBooks && b.Status != accountStatuses[section] {
			continue
		}
		if filter != "" && !strings.Contains(strings.ToLower(b.Title+" "+b.Author), filter) {
			continue
		}
		books = append(books, b)
	}
	sort.Slice(books, func(i, j int) bool {
		a, b := books[i], books[j]
		switch order {
		case 0:
			if a.Added != b.Added {
				return a.Added > b.Added
			}
		case 2:
			if !strings.EqualFold(a.Author, b.Author) {
				return strings.ToLower(a.Author) < strings.ToLower(b.Author)
			}
		case 3:
			if a.Rating != b.Rating {
				return a.Rating > b.Rating
			}
		case 4:
			if a.LastRead != b.LastRead {
				return a.LastRead > b.LastRead
			}
		}
		if !strings.EqualFold(a.Title, b.Title) {
			return strings.ToLower(a.Title) < strings.ToLower(b.Title)
		}
		return a.ID < b.ID
	})
	return books
}

func (m Model) acctRows() []acctRow {
	section := m.accountUI.section
	var rows []acctRow
	source := func(digital bool) string {
		if digital {
			return "Libby/OverDrive"
		}
		return "Library"
	}
	if section == accountOverview || section == accountLoans {
		for _, c := range m.checkouts {
			due := dueIn(c.Due)
			if c.Status == "OVERDUE" {
				due = errStyle.Render("overdue · " + c.Due)
			}
			if c.Due == "" {
				due = "Due date unavailable"
			}
			rows = append(rows, acctRow{bib: c.Bib, section: accountLoans, source: source(c.Digital), status: due})
		}
	}
	if section == accountOverview || section == accountHolds {
		for _, h := range m.holds {
			status := holdStatus(h)
			if h.WaitDays > 0 && h.Status != "READY" && h.Status != "READY_FOR_PICKUP" {
				status += fmt.Sprintf(" · ~%dd wait", h.WaitDays)
			}
			rows = append(rows, acctRow{bib: h.Bib, section: accountHolds, source: source(h.Digital), status: status})
		}
	}
	if section == accountHistory {
		for _, r := range m.recent {
			status := "borrowed " + shortDate(r.CheckedOut)
			if r.Returned != "" {
				status += " · returned " + shortDate(r.Returned)
			}
			rows = append(rows, acctRow{bib: r.Bib, section: accountHistory, source: "Library", status: status})
		}
	}
	if section == accountOverview || section >= accountAllBooks {
		shelf := section
		if shelf == accountOverview {
			shelf = accountReading
		}
		for _, b := range m.accountBooks(shelf, m.accountUI.filters[section], m.accountUI.sorts[section]) {
			status := b.Status.String()
			if b.LastRead != "" {
				status += " · finished " + shortDate(b.LastRead)
			}
			if b.Added != "" {
				status += " · added " + shortDate(b.Added)
			}
			if b.Rating > 0 {
				status = fmt.Sprintf("%.1f ★ · %s", b.Rating, status)
			}
			rows = append(rows, acctRow{bib: bc.Bib{Title: b.Title, Authors: []string{b.Author}}, hcID: b.ID, section: shelf, source: "Hardcover", status: status})
		}
	}
	return rows
}

func (m Model) updateAccount(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	u := &m.accountUI
	if u.filtering {
		switch msg.String() {
		case "esc":
			u.filtering = false
			return m, nil
		case "enter":
			u.filters[u.section] = u.filter.Value()
			u.cursors[u.section] = 0
			m.acctCursor = 0
			u.filtering = false
			return m, nil
		}
		var cmd tea.Cmd
		u.filter, cmd = u.filter.Update(msg)
		return m, cmd
	}
	rows := m.acctRows()
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "esc", "/", "s":
		return m, m.toSearch()
	case "tab", "shift+tab":
		u.sidebar = !u.sidebar
	case "left", "h":
		u.sidebar = true
	case "r":
		var cmds []tea.Cmd
		if u.section <= accountHistory && m.acct != nil && (m.acct.HasCatalog() || m.acct.HasLibby()) {
			var cmd tea.Cmd
			m, cmd = m.loadAccount()
			cmds = append(cmds, cmd)
		}
		if u.section == accountOverview || u.section >= accountAllBooks {
			var cmd tea.Cmd
			m, cmd = m.loadAccountHardcover(true)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	case "f":
		if u.section >= accountAllBooks {
			u.filter = textinput.New()
			u.filter.Prompt = "Filter: "
			u.filter.Placeholder = "title or author"
			u.filter.SetValue(u.filters[u.section])
			filterWidth := m.width - 10
			if m.width >= 90 {
				filterWidth -= 30
			}
			u.filter.SetWidth(max(1, filterWidth))
			u.filtering = true
			u.sidebar = false
			return m, u.filter.Focus()
		}
	case "o":
		if u.section >= accountAllBooks {
			u.sorts[u.section] = (u.sorts[u.section] + 1) % len(accountSortLabels)
			m.acctCursor = 0
		}
	case "up", "k", "down", "j", "home", "g", "end", "G", "pgup", "pgdown", "space":
		step := 1
		if msg.String() == "up" || msg.String() == "k" {
			step = -1
		}
		if u.sidebar {
			u.cursors[u.section] = m.acctCursor
			switch msg.String() {
			case "home", "g":
				u.section = 0
			case "end", "G":
				u.section = accountSectionCount - 1
			default:
				u.section = max(0, min(accountSectionCount-1, u.section+step))
			}
			m.acctCursor = u.cursors[u.section]
		} else {
			switch msg.String() {
			case "home", "g":
				m.acctCursor = 0
			case "end", "G":
				m.acctCursor = len(rows) - 1
			case "pgup":
				m.acctCursor -= max(1, (m.bodyHeight()-5)/4)
			case "pgdown", "space":
				m.acctCursor += max(1, (m.bodyHeight()-5)/4)
			default:
				m.acctCursor += step
			}
		}
	case "enter", "right", "l":
		if u.sidebar {
			u.sidebar = false
			return m, nil
		}
		if m.acctCursor >= 0 && m.acctCursor < len(rows) {
			row := rows[m.acctCursor]
			if row.bib.ID == "" {
				return m.openBook(row.bib.Title, firstOr(row.bib.Authors, ""), row.hcID, accountScreen)
			}
			return m.openBib(row.bib, accountScreen)
		}
	case "e":
		if !u.sidebar && m.acctCursor >= 0 && m.acctCursor < len(rows) && rows[m.acctCursor].hcID != 0 && m.acct != nil && m.acct.HasHardcover() {
			return m.openHCEditorFor(rows[m.acctCursor].hcID)
		}
	}
	m.acctCursor = max(0, min(m.acctCursor, len(m.acctRows())-1))
	m.accountUI.cursors[m.accountUI.section] = m.acctCursor
	return m, nil
}

func (m Model) accountCount(section int) int {
	switch section {
	case accountOverview:
		return -1
	case accountLoans:
		return len(m.checkouts)
	case accountHolds:
		return len(m.holds)
	case accountHistory:
		return len(m.recent)
	default:
		n := 0
		for _, b := range m.hcLib {
			if section == accountAllBooks || b.Status == accountStatuses[section] {
				n++
			}
		}
		return n
	}
}

func (m Model) viewAccount() string {
	if m.acct == nil {
		return m.notice("Personal accounts are unavailable in this session.")
	}
	const sidebarWidth = 27
	if m.width < 90 {
		if m.accountUI.sidebar {
			return m.accountSidebar(max(1, m.width), m.bodyHeight())
		}
		return m.accountContent(max(1, m.width), m.bodyHeight())
	}
	contentWidth := m.width - sidebarWidth - 3
	side := m.accountSidebar(sidebarWidth, m.bodyHeight())
	body := m.accountContent(contentWidth, m.bodyHeight())
	rule := make([]string, m.bodyHeight())
	for i := range rule {
		rule[i] = ruleStyle.Render(" │ ")
	}
	if m.accountUI.sidebar {
		for i := range rule {
			rule[i] = accentStyle.Render(" ┃ ")
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, side, strings.Join(rule, "\n"), body)
}

func (m Model) accountSidebar(w, h int) string {
	lines := []string{headingStyle.Render("ACCOUNT"), ""}
	selected := 0
	for section, label := range accountLabels {
		if section == accountLoans {
			lines = append(lines, "", dimStyle.Render("LIBRARY & LIBBY"))
		}
		if section == accountAllBooks {
			lines = append(lines, "", dimStyle.Render("HARDCOVER"))
		}
		n := m.accountCount(section)
		count := ""
		if n >= 0 {
			count = fmt.Sprint(n)
			if section <= accountHistory {
				if m.acctLoading {
					count = "…"
				} else if m.acctErr != nil || m.libbyErr != nil {
					count += "!"
				} else if !m.acctLoaded {
					count = "—"
				}
			} else {
				if m.hcLibraryLoading {
					count = "…"
				} else if m.hcLibraryErr != nil {
					count += "!"
				} else if !m.hcLibraryLoaded {
					count = "—"
				}
			}
		}
		line := "  " + spread(label, count, max(1, w-3))
		if section == m.accountUI.section {
			selected = len(lines)
			if m.accountUI.sidebar {
				line = accentStyle.Render("▸ " + spread(label, count, max(1, w-3)))
			} else {
				line = headingStyle.Render("› " + spread(label, count, max(1, w-3)))
			}
		}
		lines = append(lines, line)
	}
	start := max(0, min(selected-h+1, len(lines)-h))
	return fitAccountLines(lines[start:min(len(lines), start+h)], w, h)
}

// Account rows wrap their status independently, so dates and hold information
// remain visible even when the terminal cannot fit table columns.
func (m Model) accountContent(w, h int) string {
	u := m.accountUI
	title := accountLabels[u.section]
	if u.section >= accountAllBooks {
		title = "Hardcover › " + title
	}
	if u.section > accountOverview && u.section <= accountHistory {
		title = "Library & Libby › " + title
	}
	focus := " · Tab: sections"
	if u.sidebar {
		focus = " · Enter: books"
	}
	heading := titleStyle.Render(title)
	if !u.sidebar {
		heading = accentStyle.Render(title)
	}
	header := []string{heading, dimStyle.Render(strings.TrimSpace(focus))}
	if u.section >= accountAllBooks {
		filter := u.filters[u.section]
		if filter == "" {
			filter = "all books"
		}
		sub := fmt.Sprintf("%d of %d books · Sort: %s", len(m.acctRows()), m.accountCount(u.section), accountSortLabels[u.sorts[u.section]])
		header = append(header, dimStyle.Render(sub))
		sub = "Filter: " + filter
		if u.filtering {
			sub = u.filter.View() + " · Enter applies · Esc cancels"
		}
		header = append(header, sub)
	}
	var lines []string
	wrap := lipgloss.NewStyle().Width(max(1, w-4))
	note := func(s string) { lines = append(lines, strings.Split(wrap.Render(s), "\n")...) }
	if u.section <= accountHistory {
		if !m.acct.HasCatalog() && !m.acct.HasLibby() {
			note("Connect a library account with shelfcheck login, or link Libby with shelfcheck libby link <code>.")
		}
		if u.section == accountHistory && !m.acct.HasCatalog() {
			note("Borrowing history requires a library login.")
		}
		if m.acctLoading {
			note(m.spin.View() + " Loading library and Libby…")
		}
		if m.acctErr != nil {
			note(errStyle.Render("Library: " + m.acctErr.Error() + " · r retries"))
		}
		if m.libbyErr != nil {
			note(errStyle.Render("Libby: " + m.libbyErr.Error() + " · r retries"))
		}
	}
	if u.section == accountOverview || u.section >= accountAllBooks {
		if !m.acct.HasHardcover() {
			note("Set HARDCOVER_TOKEN to browse your Hardcover shelves.")
		}
		if m.hcLibraryLoading {
			note(m.spin.View() + " Loading Hardcover…")
		}
		if m.hcLibraryErr != nil {
			note(errStyle.Render("Hardcover: " + m.hcLibraryErr.Error() + " · r retries"))
		}
	}
	rows := m.acctRows()
	cursorLine := 0
	lastSection := -1
	for i, row := range rows {
		if u.section == accountOverview && row.section != lastSection {
			lines = append(lines, "", sectionHead(strings.ToUpper(accountLabels[row.section]), "", w))
			lastSection = row.section
		}
		if i == m.acctCursor {
			cursorLine = len(lines)
		}
		title := ansi.Truncate(row.bib.Title, max(1, w-3), "…")
		if !u.sidebar {
			title = cursorRow(i == m.acctCursor, title)
		} else {
			title = "  " + title
		}
		lines = append(lines, title)
		author := firstOr(row.bib.Authors, "")
		meta := row.source
		if author != "" {
			meta = author + " · " + meta
		}
		lines = append(lines, "  "+ansi.Truncate(dimStyle.Render(meta), max(1, w-3), "…"))
		for _, line := range strings.Split(wrap.Render(row.status), "\n") {
			lines = append(lines, "  "+line)
		}
		lines = append(lines, "")
	}
	if len(rows) == 0 {
		if m.acctLoading || m.hcLibraryLoading {
			note("Books will appear as each source loads.")
		} else {
			note("No books to show in this section.")
		}
	}
	if u.section == accountHistory {
		note(fmt.Sprintf("Showing up to %d recent library borrowing entries.", recentCount))
	}
	available := max(1, h-len(header)-1)
	start := max(0, min(cursorLine-max(0, available-4), len(lines)-available))
	header = append(header, "")
	header = append(header, lines[start:min(len(lines), start+available)]...)
	return fitAccountLines(header, w, h)
}

func fitAccountLines(lines []string, w, h int) string {
	out := make([]string, h)
	for i := range out {
		line := ""
		if i < len(lines) {
			line = ansi.Truncate(lines[i], w, "…")
		}
		out[i] = line + strings.Repeat(" ", max(0, w-ansi.StringWidth(line)))
	}
	return strings.Join(out, "\n")
}

func (m *Model) clampAccountCursor() {
	m.acctCursor = max(0, min(m.acctCursor, len(m.acctRows())-1))
	m.accountUI.cursors[m.accountUI.section] = m.acctCursor
}
