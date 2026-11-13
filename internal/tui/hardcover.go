package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"shelfcheck/internal/account"
	bc "shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/hardcover"
	"shelfcheck/internal/libby"
)

// The Hardcover screen checks a Hardcover prompt's answers ("What is your
// favorite dark academia book?") or a list's books against the catalog,
// filling in like the wishlist. Paste a prompt or list URL into the search
// box, or press h on a "readers also picked" pick. + puts the selected book
// on (or takes it off) the patron's Hardcover Want to Read shelf, which the
// wishlist reads.

const hcBooks = 30 // books to check

type (
	hcCollectionMsg struct {
		seq     int
		col     hardcover.Collection
		shelves map[int]hardcover.UserBook
		err     error
	}
	// hcWantMsg is the result of a +, from any screen. It isn't tied to a
	// screen's seq: a write that went through must be recorded wherever
	// the patron has gone since.
	hcWantMsg struct {
		book    hardcover.Book
		entry   hardcover.UserBook // the entry added, removed or left alone
		removed bool
		kept    bool // on another shelf, so left alone
		err     error
	}
	hcCheckedMsg struct {
		seq  int
		i    int
		item account.WishItem
		err  error
	}
)

// hcState is the Hardcover screen's state.
type hcState struct {
	seq     int
	ref     hardcover.Ref
	loading bool
	err     error
	col     *hardcover.Collection
	shelves map[int]hardcover.UserBook // the patron's own Hardcover library, by book
	note    string                     // result of the last + (shown until the next key)
	items   []account.WishItem         // checked so far, in collection order
	failed  int
	all     bool
	cursor  int
	back    screen
}

// isHardcoverURL reports whether search text is a Hardcover prompt or list
// link rather than a query.
func isHardcoverURL(s string) bool {
	_, ok := hardcover.ParseRef(s)
	return ok && strings.Contains(s, "hardcover.app/")
}

func (m Model) openHardcover(url string, from screen) (Model, tea.Cmd) {
	ref, _ := hardcover.ParseRef(url)
	m.screen = hardcoverScreen
	m.input.Blur()
	if ref == m.hc.ref && (m.hc.loading || m.hc.col != nil) {
		m.hc.back = from
		return m, nil
	}
	m.hc = hcState{seq: m.hc.seq + 1, ref: ref, back: from}
	return m.loadHardcover()
}

func (m Model) loadHardcover() (Model, tea.Cmd) {
	var hc *hardcover.Client
	if m.acct != nil {
		hc = m.acct.HardcoverClient()
	}
	if hc == nil {
		m.hc.err = fmt.Errorf("set HARDCOVER_TOKEN to read Hardcover prompts and lists")
		return m, nil
	}
	m.hc.seq++
	m.hc.loading, m.hc.err = true, nil
	m.hc.col, m.hc.items, m.hc.failed, m.hc.cursor = nil, nil, 0, 0
	seq, ref := m.hc.seq, m.hc.ref
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		col, err := hc.Collection(ctx, ref, hcBooks)
		if err != nil {
			return hcCollectionMsg{seq: seq, err: err}
		}
		lib, _ := hc.Library(ctx) // only marks what they've read
		return hcCollectionMsg{seq: seq, col: col, shelves: hardcover.Shelves(lib)}
	}
}

func (m Model) checkHardcover(i int) tea.Cmd {
	client, b, seq := m.client, m.hc.col.Books[i], m.hc.seq
	opts := account.WishOptions{Home: m.home, HomeName: m.homeName}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		item, err := account.CheckWish(ctx, client, libby.Tagging{Title: b.Title, Author: b.Author, Format: "book"}, opts)
		return hcCheckedMsg{seq: seq, i: i, item: item, err: err}
	}
}

func (m Model) handleHardcoverMsg(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case hcCollectionMsg:
		if msg.seq != m.hc.seq {
			return m, nil
		}
		if msg.err != nil {
			m.hc.loading, m.hc.err = false, msg.err
			return m, nil
		}
		m.hc.col, m.hc.shelves = &msg.col, msg.shelves
		if len(msg.col.Books) == 0 {
			m.hc.loading = false
			return m, nil
		}
		return m, m.checkHardcover(0)
	case hcCheckedMsg:
		if msg.seq != m.hc.seq {
			return m, nil
		}
		if msg.err != nil {
			m.hc.failed++
		}
		m.hc.items = append(m.hc.items, msg.item)
		if next := msg.i + 1; next < len(m.hc.col.Books) {
			return m, m.checkHardcover(next)
		}
		m.hc.loading = false
	}
	return m, nil
}

// hcVisible returns the indexes of the books the list shows: available
// ones, or all with A.
func (m Model) hcVisible() []int {
	var out []int
	for i, it := range m.hc.items {
		if _, ok := filterWish(it, m.listFilters[hardcoverScreen]); ok && (it.Available() || m.hc.all) {
			out = append(out, i)
		}
	}
	return out
}

func (m Model) updateHardcover(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	vis := m.hcVisible()
	h := max(1, m.listHeight())
	if !m.hcWantBusy {
		m.hc.note = ""
	}
	switch msg.String() {
	case "+":
		if m.hc.cursor < len(vis) {
			return m.toggleWant(m.hc.col.Books[vis[m.hc.cursor]].Book)
		}
	case "q":
		return m, tea.Quit
	case "esc":
		if m.hc.back != searchScreen {
			m.screen = m.hc.back
			return m, nil
		}
		return m, m.toSearch()
	case "/", "s":
		return m, m.toSearch()
	case "r":
		if !m.hc.loading {
			return m.loadHardcover()
		}
	case "A":
		m.hc.all = !m.hc.all
		m.hc.cursor = 0
	case "up", "k":
		m.hc.cursor--
	case "down", "j":
		m.hc.cursor++
	case "pgup":
		m.hc.cursor -= h
	case "pgdown", "space":
		m.hc.cursor += h
	case "home", "g":
		m.hc.cursor = 0
	case "end", "G":
		m.hc.cursor = len(vis) - 1
	case "enter", "right", "l":
		if m.hc.cursor < len(vis) {
			it, b := m.hc.items[vis[m.hc.cursor]], m.hc.col.Books[vis[m.hc.cursor]]
			it, _ = filterWish(it, m.listFilters[hardcoverScreen])
			if bib, ok := bestMatch(it); ok {
				return m.openBib(bib, hardcoverScreen)
			}
			// Unavailable: show it from Hardcover (c there searches the
			// catalog for holds and other branches).
			return m.openBook(b.Title, b.Author, b.ID, hardcoverScreen)
		}
	}
	m.hc.cursor = max(0, min(m.hc.cursor, len(vis)-1))
	return m, nil
}

// toggleWant puts b on the Hardcover Want to Read shelf, or takes it off if
// it's there. Books on another shelf (read, did not finish...) are left be.
// It asks Hardcover where the book stands first, so a stale or unloaded
// library can't add a second entry or remove the wrong one.
func (m Model) toggleWant(b hardcover.Book) (Model, tea.Cmd) {
	var hc *hardcover.Client
	if m.acct != nil {
		hc = m.acct.HardcoverClient()
	}
	switch {
	case hc == nil:
		m.wantNote(errStyle, "Set HARDCOVER_TOKEN to change your Hardcover shelves")
		return m, nil
	case b.ID == 0:
		return m, nil
	case m.hcWantBusy:
		m.wantNote(dimStyle, "Still updating Hardcover…")
		return m, nil
	}
	m.hcWantBusy = true
	m.wantNote(dimStyle, "Updating Hardcover…")
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		entry, has, err := hc.UserBook(ctx, b.ID)
		switch {
		case err != nil:
			return hcWantMsg{book: b, err: err}
		case has && entry.Status != hardcover.WantToRead:
			return hcWantMsg{book: b, entry: entry, kept: true}
		case has:
			return hcWantMsg{book: b, entry: entry, removed: true, err: hc.RemoveFromLibrary(ctx, entry.UserBookID)}
		}
		id, err := hc.AddToShelf(ctx, b.ID, hardcover.WantToRead)
		return hcWantMsg{book: b, err: err, entry: hardcover.UserBook{Book: b, UserBookID: id, Status: hardcover.WantToRead}}
	}
}

// wantNote shows the state of a +: under the Hardcover screen's heading,
// or in the footer anywhere else.
func (m *Model) wantNote(style lipgloss.Style, text string) {
	if m.screen == hardcoverScreen {
		m.hc.note = style.Render(text)
		return
	}
	m.flash = text
}

func (m Model) handleHCWant(msg hcWantMsg) Model {
	m.hcWantBusy = false
	switch {
	case msg.err != nil:
		// The write may have gone through all the same: reload the shelves
		// and what's worked out from them.
		m.hcLibrarySeq++
		m.hcLibraryLoading, m.hcLibraryLoaded = false, false
		m.dropWishlist()
		m.wantNote(errStyle, "Couldn't update Hardcover: "+msg.err.Error())
	case msg.kept:
		m.cacheHCEntry(msg.entry)
		m.wantNote(dimStyle, msg.book.Title+" is already on your Hardcover as "+msg.entry.Status.String())
	case msg.removed:
		m.dropHCEntry(msg.book.ID)
		m.dropWishlist()
		m.wantNote(okStyle, "Took "+msg.book.Title+" off your Hardcover want to read")
	default:
		m.cacheHCEntry(msg.entry)
		m.dropWishlist()
		m.wantNote(okStyle, "Added "+msg.book.Title+" to your Hardcover want to read")
	}
	if m.screen == detailScreen {
		m.refreshDetail()
	}
	return m
}

// wantSelected is + on the detail screen: the selected book, as Hardcover
// knows it, goes on (or comes off) Want to Read.
func (m Model) wantSelected() (Model, tea.Cmd) {
	h := m.hcInfo[m.detailKey()]
	switch {
	case m.acct == nil || m.acct.HardcoverClient() == nil:
		m.flash = "Set HARDCOVER_TOKEN to change your Hardcover shelves"
	case h == nil || h.loading:
		m.flash = "Still looking this book up on Hardcover; press + when it loads"
	case h.err != nil:
		m.flash = "Hardcover lookup failed: " + h.err.Error() + "; r retries it"
	case !h.ok || h.info.ID == 0:
		m.flash = "No matching book found on Hardcover; r retries the lookup"
	default:
		return m.toggleWant(h.info.Book)
	}
	return m, nil
}

func (m Model) viewHardcover() string {
	switch {
	case m.hc.err != nil && m.hc.col == nil:
		return m.notice(errStyle.Render("Couldn't load from Hardcover: "+m.hc.err.Error()) + "\n\nPress r to try again.")
	case m.hc.col == nil:
		return m.notice(m.spin.View() + " Loading from Hardcover…")
	}
	col := m.hc.col
	avail := 0
	for _, it := range m.hc.items {
		if filtered, ok := filterWish(it, m.listFilters[hardcoverScreen]); ok && filtered.Available() {
			avail++
		}
	}
	w := max(20, m.width-4)
	head := "  " + titleStyle.Render(ansi.Truncate(col.Title, max(10, w-30), "…")) + dimStyle.Render(fmt.Sprintf(" · %d available now", avail))
	if m.hc.loading {
		head += dimStyle.Render(fmt.Sprintf(" · checking %d of %d ", len(m.hc.items)+1, len(col.Books))) + m.spin.View()
	} else {
		head += dimStyle.Render(fmt.Sprintf(" of %d", len(col.Books)))
	}
	if m.hc.failed > 0 {
		head += errStyle.Render(fmt.Sprintf(" · %d couldn't be checked", m.hc.failed))
	}
	what := map[string]string{"prompt": "top answers", "list": "list", "vibe": "Hardcover's picks"}[col.Kind]
	sub := "  " + dimStyle.Render(what+" on "+col.URL+" · on the shelf at "+m.homeName+", or ebooks now")
	if m.hc.all {
		sub = "  " + dimStyle.Render("showing every book (A to show only available)")
	}
	if m.hc.note != "" {
		sub = "  " + m.hc.note
	}
	vis := m.hcVisible()
	if len(vis) == 0 {
		rows := []string{head, sub}
		if !m.hc.loading {
			rows = append(rows, "", dimStyle.Render("  No titles match the current availability and filters. Press f to adjust or F to clear filters; A includes unavailable titles."))
		}
		return strings.Join(rows, "\n")
	}
	h := m.listHeight()
	start := max(0, min(m.hc.cursor-h+1, len(vis)-h))
	head = spread(head, dimStyle.Render(fmt.Sprintf("%d/%d", m.hc.cursor+1, len(vis))), m.width-2)
	rows := []string{head, sub, "  " + bookHeader(w, " ")}
	var list []string
	for i := start; i < min(len(vis), start+h); i++ {
		list = append(list, cursorRow(i == m.hc.cursor, m.hcLine(vis[i], w)))
	}
	return strings.Join(append(rows, withScrollbar(list, scrollbar(h, len(vis), start), m.width)...), "\n")
}

// hcLine is a wishlist-style row, labeled with the patron's own Hardcover
// status ("read") or, for prompts, how many readers gave the book.
func (m Model) hcLine(i, w int) string {
	it, b := m.hc.items[i], m.hc.col.Books[i]
	label := " " // keeps the column when there's nothing to say
	switch ub, ok := m.hc.shelves[b.ID]; {
	case ok:
		label = ub.Status.String()
	case b.Votes == 1:
		label = "1 pick"
	case b.Votes > 1:
		label = fmt.Sprintf("%d picks", b.Votes)
	}
	it, _ = filterWish(it, m.listFilters[hardcoverScreen])
	bib, ok := bestMatch(it)
	if !ok {
		bib = bc.Bib{Title: it.Tagging.Title, Authors: []string{it.Tagging.Author}}
	}
	return bookLine(bib, w, label, wishWhere(it))
}
