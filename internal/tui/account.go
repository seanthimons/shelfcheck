package tui

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"shelfcheck/internal/account"
	bc "shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/libby"
)

// The account workspace lives in account_workspace.go. This file shares
// navigation, wishlist checking and row formatting across account screens.

const recentCount = 10

type (
	accountMsg struct {
		checkouts []bc.Checkout
		holds     []bc.Hold
		recent    []bc.HistoryItem
		err       error
		libbyErr  error // Libby's loans and holds couldn't be added
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
	case "alt+n":
		mm, cmd := m.openArrivals()
		return mm, cmd, true
	case "alt+t":
		mm, cmd := m.cycleTheme()
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
	case "n":
		if m.screen != arrivalsScreen {
			mm, cmd := m.openArrivals()
			return mm, cmd, true
		}
	case "p":
		if m.screen != forYouScreen {
			mm, cmd := m.openForYou()
			return mm, cmd, true
		}
	case "t":
		mm, cmd := m.cycleTheme()
		return mm, cmd, true
	}
	return m, nil, false
}

// openBib shows b on the detail screen; esc returns to from.
func (m Model) openBib(b bc.Bib, from screen) (tea.Model, tea.Cmd) {
	m.selected, m.selectedHC = b, 0
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
	m, hcCmd := m.fetchHCInfo()
	cmds = append(cmds, hcCmd, m.fetchCover(m.coverURL()))
	m.refreshDetail()
	m.vp.GotoTop()
	return m, tea.Batch(cmds...)
}

// --- account ----------------------------------------------------------------

// --- wishlist ---------------------------------------------------------------

func (m Model) openWishlist() (Model, tea.Cmd) {
	m.screen = wishlistScreen
	m.input.Blur()
	if !m.hasWishlist() || m.wishLoading || m.wishTitles != nil {
		return m, nil
	}
	return m.loadWishlist()
}

// hasWishlist reports whether there's a want-to-read list to check: Libby's
// or Hardcover's.
func (m Model) hasWishlist() bool {
	return m.acct != nil && (m.acct.HasLibby() || m.acct.HasHardcover())
}

func (m Model) loadWishlist() (Model, tea.Cmd) {
	st := m.acct
	m.wishSeq++
	m.wishLoading = true
	m.wishErr = nil
	m.wishTitles, m.wishItems = nil, nil
	m.wishCursor = 0
	seq := m.wishSeq
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		titles, err := st.WantToRead(ctx, "want to read")
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
		// One source failing (say, Libby) still leaves the other's titles.
		m.wishErr = msg.err
		if msg.err != nil && len(msg.titles) == 0 {
			m.wishLoading = false
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
			if filtered, ok := filterWish(w, m.listFilters[wishlistScreen]); ok {
				out = append(out, filtered)
			}
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
		if m.hasWishlist() {
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
	case !m.hasWishlist():
		return m.notice(titleStyle.Render("No want-to-read list here.") +
			"\n\nLink Libby (in Libby: Menu > Copy To Another Device, then run\nshelfcheck libby link <8-digit code>), or set HARDCOVER_TOKEN for your Hardcover list.")
	case m.wishErr != nil && m.wishTitles == nil:
		return m.notice(errStyle.Render("Couldn't load your wishlist: "+m.wishErr.Error()) + "\n\nPress r to try again.")
	case m.wishTitles == nil:
		return m.notice(m.spin.View() + " Loading your want-to-read list…")
	}

	items := m.wishVisible()
	avail := 0
	for _, w := range m.wishItems {
		if filtered, ok := filterWish(w, m.listFilters[wishlistScreen]); ok && filtered.Available() {
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
	if m.wishErr != nil {
		head += errStyle.Render(" · " + m.wishErr.Error())
	}
	sub := "  " + dimStyle.Render("print on the shelf at "+m.homeName+", or ebooks you can borrow now")
	if m.wishAll {
		sub = "  " + dimStyle.Render("showing every title (A to show only available)")
	}
	if len(items) == 0 {
		rows := []string{head, sub}
		if !m.wishLoading {
			rows = append(rows, "", dimStyle.Render("  No titles match the current availability and filters. Press f to adjust or F to clear filters."))
		}
		return strings.Join(rows, "\n")
	}

	h := m.listHeight()
	start := max(0, min(m.wishCursor-h+1, len(items)-h))
	w := max(20, m.width-4)
	head = spread(head, dimStyle.Render(fmt.Sprintf("%d/%d", m.wishCursor+1, len(items))), m.width-2)
	rows := []string{head, sub, "  " + bookHeader(w, "")}
	var list []string
	for i := start; i < min(len(items), start+h); i++ {
		list = append(list, cursorRow(i == m.wishCursor, wishLine(items[i], w, m.homeName)))
	}
	return strings.Join(append(rows, withScrollbar(list, scrollbar(h, len(items), start), m.width)...), "\n")
}

func wishLine(it account.WishItem, w int, homeName string) string {
	b, ok := bestMatch(it)
	if !ok {
		b = bc.Bib{Title: it.Tagging.Title, Authors: []string{it.Tagging.Author}}
	}
	return bookLine(b, w, "", wishWhere(it))
}

// wishWhere says where a title can be had now.
func wishWhere(it account.WishItem) string {
	switch {
	case it.OnShelf() && it.OnlineNow():
		return okStyle.Render("on shelf") + dimStyle.Render(" + ") + accentStyle.Render("ebook")
	case it.OnShelf():
		return okStyle.Render("on shelf")
	case it.OnlineNow():
		return accentStyle.Render("ebook now")
	}
	return dimStyle.Render("not available now")
}

// --- shared row formatting --------------------------------------------------

const bookAuthorW, bookKindW, bookRatingW, bookStatusW = 22, 13, 11, 26

// bookCols sizes bookLine's columns: the title's width, and whether there's
// room for the author.
func bookCols(w int, kind string) (titleW int, showAuthor bool) {
	showAuthor = w >= 90
	fixed := bookRatingW + bookStatusW + 2
	if kind != "" {
		fixed += bookKindW + 1
	}
	if showAuthor {
		fixed += bookAuthorW + 1
	}
	return max(12, w-fixed), showAuthor
}

// bookHeader labels bookLine's columns; kind labels the kind column, if
// there is one (" " for one without a label).
func bookHeader(w int, kind string) string {
	titleW, showAuthor := bookCols(w, kind)
	cols := []string{colHead("TITLE", titleW)}
	if showAuthor {
		cols = append(cols, colHead("AUTHOR", bookAuthorW))
	}
	if kind != "" {
		cols = append(cols, colHead(kind, bookKindW))
	}
	return strings.Join(append(cols, colHead("RATING", bookRatingW), colHead("AVAILABLE", bookStatusW)), " ")
}

// bookLine renders title, author, a kind label, the rating and a status,
// fitted to width w.
func bookLine(b bc.Bib, w int, kind, status string) string {
	const authorW, kindW, ratingW, statusW = bookAuthorW, bookKindW, bookRatingW, bookStatusW
	titleW, showAuthor := bookCols(w, kind)
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
	today := startOfToday()
	days := int(math.Round(t.Sub(today).Hours() / 24)) // 23 or 25 hours across a clock change
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

// startOfToday is local midnight. (Truncating to a day works in UTC, which
// is tomorrow here by evening.)
func startOfToday() time.Time {
	y, mo, d := timeNow().Date()
	return time.Date(y, mo, d, 0, 0, 0, 0, time.Local)
}

// timeNow is time.Now, for tests to set the clock.
var timeNow = time.Now
