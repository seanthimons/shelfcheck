package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"shelfcheck/internal/account"
	bc "shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/libby"
)

// The account screen (m) shows loans, holds and recent history from the
// logged-in catalog session; the wishlist screen (w) checks the Libby
// "want to read" tag against the catalog one title at a time.

const recentCount = 10

type (
	accountMsg struct {
		checkouts []bc.Checkout
		holds     []bc.Hold
		recent    []bc.HistoryItem
		err       error
	}
	wishTitlesMsg struct {
		seq    int
		titles []libby.Tagging
		err    error
	}
	wishCheckedMsg struct {
		seq  int
		i    int
		item account.WishItem
		err  error
	}
)

// acctRow is one selectable line on the account screen.
type acctRow struct {
	bib  bc.Bib
	line func(width int) string
}

// --- navigation -------------------------------------------------------------

// navKey handles the screen-switching keys shared by every screen. Plain
// letters work everywhere but the search box, where alt+letter does.
func (m Model) navKey(key string) (Model, tea.Cmd, bool) {
	switch key {
	case "alt+w":
		mm, cmd := m.openWishlist()
		return mm, cmd, true
	case "alt+m":
		mm, cmd := m.openAccount()
		return mm, cmd, true
	case "alt+p":
		mm, cmd := m.openForYou()
		return mm, cmd, true
	}
	if m.screen == searchScreen || m.panel != panelClosed {
		return m, nil, false
	}
	switch key {
	case "w":
		if m.screen != wishlistScreen {
			mm, cmd := m.openWishlist()
			return mm, cmd, true
		}
	case "m":
		if m.screen != accountScreen {
			mm, cmd := m.openAccount()
			return mm, cmd, true
		}
	case "p":
		if m.screen != forYouScreen {
			mm, cmd := m.openForYou()
			return mm, cmd, true
		}
	}
	return m, nil, false
}

// openBib shows b on the detail screen; esc returns to from.
func (m Model) openBib(b bc.Bib, from screen) (tea.Model, tea.Cmd) {
	m.selected = b
	m.backScreen = from
	m.screen = detailScreen
	id := b.ID
	var cmds []tea.Cmd
	if !m.availPending[id] {
		m.availPending[id] = true
		cmds = append(cmds, m.fetchAvail(id))
	}
	if m.details[id] == nil {
		delete(m.detailsErr, id)
		cmds = append(cmds, m.fetchDetails(id))
	}
	m.refreshDetail()
	m.vp.GotoTop()
	return m, tea.Batch(cmds...)
}

// --- account ----------------------------------------------------------------

func (m Model) openAccount() (Model, tea.Cmd) {
	m.screen = accountScreen
	m.input.Blur()
	if m.acct == nil || !m.acct.HasCatalog() || m.acctLoading || m.acctLoaded {
		return m, nil
	}
	return m.loadAccount()
}

func (m Model) loadAccount() (Model, tea.Cmd) {
	m.acctLoading = true
	m.acctErr = nil
	client, st := m.client, m.acct
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		var msg accountMsg
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
		return msg
	}
}

func (m Model) acctRows() []acctRow {
	var rows []acctRow
	for _, c := range m.checkouts {
		c := c
		rows = append(rows, acctRow{c.Bib, func(w int) string {
			due := dueIn(c.Due)
			if c.Status == "OVERDUE" {
				due = errStyle.Render("overdue · " + c.Due)
			}
			return bookLine(c.Bib, w, kindLabel(c.Bib, c.Digital), due)
		}})
	}
	for _, h := range m.holds {
		h := h
		rows = append(rows, acctRow{h.Bib, func(w int) string {
			return bookLine(h.Bib, w, kindLabel(h.Bib, h.Digital), holdStatus(h))
		}})
	}
	for _, r := range m.recent {
		r := r
		rows = append(rows, acctRow{r.Bib, func(w int) string {
			return bookLine(r.Bib, w, formatLabel(r.Bib.Format), dimStyle.Render("borrowed "+shortDate(r.CheckedOut)))
		}})
	}
	return rows
}

func (m Model) updateAccount(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	rows := m.acctRows()
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "esc", "/", "s":
		return m, m.toSearch()
	case "r":
		if !m.acctLoading && m.acct != nil {
			return m.loadAccount()
		}
	case "up", "k":
		m.acctCursor--
	case "down", "j":
		m.acctCursor++
	case "home", "g":
		m.acctCursor = 0
	case "end", "G":
		m.acctCursor = len(rows) - 1
	case "enter", "right", "l":
		if m.acctCursor < len(rows) {
			return m.openBib(rows[m.acctCursor].bib, accountScreen)
		}
	}
	m.acctCursor = max(0, min(m.acctCursor, len(rows)-1))
	return m, nil
}

func (m Model) viewAccount() string {
	switch {
	case m.acct == nil || !m.acct.HasCatalog():
		return "\n  " + titleStyle.Render("Your account isn't set up here.") +
			"\n\n  Put your card number and PIN in ~/.config/shelfcheck/credentials.env\n  (SHELFCHECK_CARD=… and SHELFCHECK_PIN=…), then run: shelfcheck login"
	case m.acctLoading && !m.acctLoaded:
		return "\n  " + m.spin.View() + " Loading your loans and holds…"
	case m.acctErr != nil && !m.acctLoaded:
		return "\n  " + errStyle.Render("Couldn't load your account: "+m.acctErr.Error()) + "\n\n  Press r to try again."
	}
	w := max(20, m.width-4)
	rows := m.acctRows()
	var out []string
	i := 0
	section := func(title string, n int, empty string) {
		out = append(out, "", "  "+headingStyle.Render(title)+dimStyle.Render(fmt.Sprintf(" (%d)", n)))
		if n == 0 {
			out = append(out, dimStyle.Render("    "+empty))
		}
		for j := 0; j < n; j, i = j+1, i+1 {
			out = append(out, cursorRow(i == m.acctCursor, rows[i].line(w)))
		}
	}
	section("CHECKED OUT", len(m.checkouts), "Nothing checked out.")
	section("HOLDS", len(m.holds), "No holds.")
	section("RECENTLY BORROWED", len(m.recent), "No borrowing history.")

	// Scroll so the cursor's line stays visible.
	cursorLine := 0
	for n, l := range out {
		if strings.HasPrefix(l, accentStyle.Render("▸ ")) {
			cursorLine = n
		}
	}
	h := m.bodyHeight()
	start := max(0, min(cursorLine-h/2, len(out)-h))
	return strings.Join(out[start:], "\n")
}

// --- wishlist ---------------------------------------------------------------

func (m Model) openWishlist() (Model, tea.Cmd) {
	m.screen = wishlistScreen
	m.input.Blur()
	if m.acct == nil || !m.acct.HasLibby() || m.wishLoading || m.wishTitles != nil {
		return m, nil
	}
	return m.loadWishlist()
}

func (m Model) loadWishlist() (Model, tea.Cmd) {
	lc, err := m.acct.LibbyClient()
	if err != nil {
		m.wishErr = err
		return m, nil
	}
	m.wishSeq++
	m.wishLoading = true
	m.wishErr = nil
	m.wishTitles, m.wishItems = nil, nil
	m.wishCursor = 0
	seq := m.wishSeq
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		titles, err := account.WishlistTitles(ctx, lc, "want to read")
		return wishTitlesMsg{seq: seq, titles: titles, err: err}
	}
}

// checkWish checks wishlist title i; the reply triggers the next, so the
// list fills in as it goes.
func (m Model) checkWish(i int) tea.Cmd {
	client, it, seq := m.client, m.wishTitles[i], m.wishSeq
	opts := account.WishOptions{Home: m.home, HomeName: m.homeName}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		item, err := account.CheckWish(ctx, client, it, opts)
		return wishCheckedMsg{seq: seq, i: i, item: item, err: err}
	}
}

func (m Model) handleWishMsg(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case wishTitlesMsg:
		if msg.seq != m.wishSeq {
			return m, nil
		}
		if msg.err != nil {
			m.wishLoading, m.wishErr = false, msg.err
			return m, nil
		}
		m.wishTitles = msg.titles
		if len(m.wishTitles) == 0 {
			m.wishLoading = false
			return m, nil
		}
		return m, m.checkWish(0)
	case wishCheckedMsg:
		if msg.seq != m.wishSeq {
			return m, nil
		}
		if msg.err != nil {
			m.wishFailed++
		}
		m.wishItems = append(m.wishItems, msg.item)
		if next := msg.i + 1; next < len(m.wishTitles) {
			return m, m.checkWish(next)
		}
		m.wishLoading = false
	}
	return m, nil
}

// wishVisible is what the list shows: available titles, or all with A.
func (m Model) wishVisible() []account.WishItem {
	var out []account.WishItem
	for _, w := range m.wishItems {
		if w.Available() || m.wishAll {
			out = append(out, w)
		}
	}
	return out
}

func (m Model) updateWishlist(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	items := m.wishVisible()
	h := max(1, m.listHeight())
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "esc", "/", "s":
		return m, m.toSearch()
	case "r":
		if m.acct != nil && m.acct.HasLibby() {
			return m.loadWishlist()
		}
	case "A":
		m.wishAll = !m.wishAll
		m.wishCursor = 0
	case "up", "k":
		m.wishCursor--
	case "down", "j":
		m.wishCursor++
	case "pgup":
		m.wishCursor -= h
	case "pgdown", "space":
		m.wishCursor += h
	case "home", "g":
		m.wishCursor = 0
	case "end", "G":
		m.wishCursor = len(items) - 1
	case "enter", "right", "l":
		if m.wishCursor < len(items) {
			if b, ok := bestMatch(items[m.wishCursor]); ok {
				return m.openBib(b, wishlistScreen)
			}
			// Unavailable: search for it so its holds and branches show.
			it := items[m.wishCursor].Tagging
			m.input.SetValue(it.Title)
			m.typeIdx = 1 // title
			m.searchSeq++
			m.searching = true
			m.q = bc.Query{Text: it.Title, Type: bc.Title}
			m.screen = resultsScreen
			return m, m.doSearch(m.searchSeq, m.q, 1, false)
		}
	}
	m.wishCursor = max(0, min(m.wishCursor, len(items)-1))
	return m, nil
}

// bestMatch prefers a print copy on the home shelf over an ebook.
func bestMatch(w account.WishItem) (bc.Bib, bool) {
	for _, mt := range w.Matches {
		if !mt.Online {
			return mt.Bib, true
		}
	}
	if len(w.Matches) > 0 {
		return w.Matches[0].Bib, true
	}
	return bc.Bib{}, false
}

func (m Model) viewWishlist() string {
	switch {
	case m.acct == nil || !m.acct.HasLibby():
		return "\n  " + titleStyle.Render("Libby isn't linked here.") +
			"\n\n  In Libby: Menu > Copy To Another Device, then run:\n  shelfcheck libby link <8-digit code>"
	case m.wishErr != nil && m.wishItems == nil:
		return "\n  " + errStyle.Render("Couldn't load your wishlist: "+m.wishErr.Error()) + "\n\n  Press r to try again."
	case m.wishTitles == nil:
		return "\n  " + m.spin.View() + " Loading your Libby want-to-read list…"
	}

	items := m.wishVisible()
	avail := 0
	for _, w := range m.wishItems {
		if w.Available() {
			avail++
		}
	}
	head := "  " + titleStyle.Render("Want to read") + dimStyle.Render(fmt.Sprintf(" · %d available now", avail))
	if m.wishLoading {
		head += dimStyle.Render(fmt.Sprintf(" · checking %d of %d ", len(m.wishItems)+1, len(m.wishTitles))) + m.spin.View()
	} else {
		head += dimStyle.Render(fmt.Sprintf(" of %d", len(m.wishTitles)))
	}
	if m.wishFailed > 0 {
		head += errStyle.Render(fmt.Sprintf(" · %d couldn't be checked", m.wishFailed))
	}
	sub := "  " + dimStyle.Render("print on the shelf at "+m.homeName+", or ebooks you can borrow now")
	if m.wishAll {
		sub = "  " + dimStyle.Render("showing every title (A to show only available)")
	}
	rows := []string{head, sub}
	if len(items) == 0 {
		if !m.wishLoading {
			rows = append(rows, "", dimStyle.Render("  Nothing on your list is available right now."))
		}
		return strings.Join(rows, "\n")
	}

	h := m.listHeight()
	start := max(0, min(m.wishCursor-h+1, len(items)-h))
	w := max(20, m.width-4)
	for i := start; i < min(len(items), start+h); i++ {
		rows = append(rows, cursorRow(i == m.wishCursor, wishLine(items[i], w, m.homeName)))
	}
	return strings.Join(rows, "\n")
}

func wishLine(it account.WishItem, w int, homeName string) string {
	where := dimStyle.Render("not available now")
	switch {
	case it.OnShelf() && it.OnlineNow():
		where = okStyle.Render("on shelf") + dimStyle.Render(" + ") + accentStyle.Render("ebook")
	case it.OnShelf():
		where = okStyle.Render("on shelf")
	case it.OnlineNow():
		where = accentStyle.Render("ebook now")
	}
	b, ok := bestMatch(it)
	if !ok {
		b = bc.Bib{Title: it.Tagging.Title, Authors: []string{it.Tagging.Author}}
	}
	return bookLine(b, w, "", where)
}

// --- shared row formatting --------------------------------------------------

// bookLine renders title, author, a kind label, the rating and a status,
// fitted to width w.
func bookLine(b bc.Bib, w int, kind, status string) string {
	const authorW, kindW, ratingW, statusW = 22, 13, 11, 26
	showAuthor := w >= 90
	fixed := ratingW + statusW + 2
	if kind != "" {
		fixed += kindW + 1
	}
	if showAuthor {
		fixed += authorW + 1
	}
	titleW := max(12, w-fixed)
	title := b.Title
	if b.Subtitle != "" {
		title += ": " + b.Subtitle
	}
	cols := []string{pad(title, titleW)}
	if showAuthor {
		cols = append(cols, pad(firstOr(b.Authors, ""), authorW))
	}
	if kind != "" {
		cols = append(cols, pad(kind, kindW))
	}
	cols = append(cols, pad(ratingShort(b.Rating), ratingW), ansi.Truncate(status, statusW, "…"))
	return strings.Join(cols, " ")
}

func kindLabel(b bc.Bib, digital bool) string {
	l := formatLabel(b.Format)
	if b.Format == "" {
		l = "Book"
		if digital {
			l = "eBook"
		}
	}
	if digital {
		l += " (Libby)"
	}
	return l
}

func holdStatus(h bc.Hold) string {
	switch h.Status {
	case "READY_FOR_PICKUP", "READY":
		return okStyle.Render("ready for pickup")
	case "IN_TRANSIT":
		return warnStyle.Render("in transit")
	case "SUSPENDED":
		return dimStyle.Render(fmt.Sprintf("paused · #%d in line", h.Position))
	}
	if h.Position > 0 {
		return fmt.Sprintf("#%d in line", h.Position)
	}
	return strings.ToLower(strings.ReplaceAll(h.Status, "_", " "))
}

// dueIn describes a YYYY-MM-DD due date relative to today.
func dueIn(date string) string {
	t, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return date
	}
	today := time.Now().Truncate(24 * time.Hour)
	days := int(t.Sub(today).Hours()/24 + 0.5)
	label := "due " + t.Format("Jan 2")
	switch {
	case days < 0:
		return errStyle.Render("overdue · " + t.Format("Jan 2"))
	case days == 0:
		return warnStyle.Render(label + " · today")
	case days <= 3:
		return warnStyle.Render(fmt.Sprintf("%s · %dd", label, days))
	}
	return fmt.Sprintf("%s · %dd", label, days)
}

func shortDate(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	if t.Year() == time.Now().Year() {
		return t.Format("Jan 2")
	}
	return t.Format("Jan 2006")
}
