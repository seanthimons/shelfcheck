package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"shelfcheck/internal/account"
	bc "shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/hardcover"
)

// The detail screen adds what Hardcover knows about a book: its rating and
// readers, the patron's own shelf, series, genres and moods, and the
// description when the library has none. Books with no library record
// (to buy, Libby-only, from a Hardcover list) open the same screen built
// from Hardcover alone.

type (
	hcInfoMsg struct {
		key  string
		info hardcover.BookInfo
		ok   bool
		lib  map[int]hardcover.UserBook // the patron's library, on first use
		err  error
	}
	// hcInfo is Hardcover's metadata for one detail-screen book.
	hcInfo struct {
		loading bool
		ok      bool // found on Hardcover
		info    hardcover.BookInfo
		err     error
	}
)

// detailKey identifies the selected book for hcInfo: its catalog ID, or
// its Hardcover ID or title and author when it has no catalog record.
func (m Model) detailKey() string {
	switch {
	case m.selected.ID != "":
		return m.selected.ID
	case m.selectedHC != 0:
		return "hc:" + strconv.Itoa(m.selectedHC)
	}
	return "t:" + account.WorkKey(m.selected.Title, firstOr(m.selected.Authors, ""))
}

// fetchHCInfo looks the selected book up on Hardcover, unless it has been.
func (m Model) fetchHCInfo() (Model, tea.Cmd) {
	var hc *hardcover.Client
	if m.acct != nil {
		hc = m.acct.HardcoverClient()
	}
	key := m.detailKey()
	if hc == nil || m.hcInfo[key] != nil {
		return m, nil
	}
	m.hcInfo[key] = &hcInfo{loading: true}
	b, id, needLib := m.selected, m.selectedHC, m.hcLib == nil
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		msg := hcInfoMsg{key: key}
		if id != 0 {
			msg.info, msg.ok, msg.err = hc.Info(ctx, id)
		} else {
			msg.info, msg.ok, msg.err = hc.Lookup(ctx, b.Title, firstOr(b.Authors, ""))
		}
		if needLib && msg.err == nil {
			if lib, err := hc.Library(ctx); err == nil {
				msg.lib = hardcover.Shelves(lib)
			}
		}
		return msg
	}
}

func (m Model) handleHCInfo(msg hcInfoMsg) (Model, tea.Cmd) {
	m.hcInfo[msg.key] = &hcInfo{ok: msg.ok, info: msg.info, err: msg.err}
	if msg.lib != nil {
		m.hcLib = msg.lib
	}
	if m.screen == detailScreen && m.detailKey() == msg.key {
		m.refreshDetail()
	}
	return m, nil
}

// openBook shows a book the library has no record of, from Hardcover;
// hcID is its Hardcover ID if known, else it's looked up by title and
// author.
func (m Model) openBook(title, author string, hcID int, from screen) (tea.Model, tea.Cmd) {
	m.selected = bc.Bib{Title: title}
	if author != "" {
		m.selected.Authors = []string{author}
	}
	m.selectedHC = hcID
	m.backScreen = from
	m.screen = detailScreen
	m, cmd := m.fetchHCInfo()
	m.refreshDetail()
	m.vp.GotoTop()
	return m, cmd
}

// searchSelected searches the catalog for the selected book by title, for
// one with no record here (it may be checked out, or under another title).
func (m Model) searchSelected() (tea.Model, tea.Cmd) {
	t := m.selected.Title
	if i := strings.IndexAny(t, ":("); i > 3 {
		t = t[:i]
	}
	m.input.SetValue(t)
	m.typeIdx = 1 // title
	m.searchSeq++
	m.searching = true
	m.q = bc.Query{Text: t, Type: bc.Title}
	m.screen = resultsScreen
	return m, m.doSearch(m.searchSeq, m.q, 1, false)
}

// renderHardcover is the detail screen's ON HARDCOVER section; desc says
// whether to include the description (when the library has none).
func (m Model) renderHardcover(w int, desc bool) []string {
	h := m.hcInfo[m.detailKey()]
	switch {
	case h == nil:
		return nil
	case h.loading:
		return []string{"", dimStyle.Render(m.spin.View() + " looking it up on Hardcover…")}
	case h.err != nil:
		return []string{"", dimStyle.Render("Couldn't reach Hardcover: " + h.err.Error())}
	case !h.ok:
		return []string{"", dimStyle.Render("Not found on Hardcover.")}
	}
	wrap := lipgloss.NewStyle().Width(w)
	in := h.info
	out := []string{"", headingStyle.Render("ON HARDCOVER")}
	var facts []string
	if in.HasRating() {
		facts = append(facts, starStyle.Render(stars(in.Rating))+fmt.Sprintf(" %.1f", in.Rating)+
			dimStyle.Render(fmt.Sprintf(" (%s)", plural(in.Ratings, "rating"))))
	}
	if in.ReadBy > 0 {
		facts = append(facts, fmt.Sprintf("read by %s", compact(in.ReadBy)))
	}
	if ub, ok := m.hcLib[in.ID]; ok {
		facts = append(facts, accentStyle.Render("on your "+ub.Status.String()+" shelf"))
	}
	if len(facts) > 0 {
		out = append(out, wrap.Render(strings.Join(facts, dimStyle.Render(" · "))))
	}
	if in.Series != "" {
		s := in.Series
		if in.SeriesPos > 0 {
			s += " #" + strconv.FormatFloat(in.SeriesPos, 'f', -1, 64)
		}
		out = append(out, dimStyle.Render("Series: ")+s)
	}
	if len(in.Genres) > 0 {
		out = append(out, wrap.Render(dimStyle.Render("Genres: ")+strings.Join(in.Genres, ", ")))
	}
	if len(in.Moods) > 0 {
		out = append(out, wrap.Render(dimStyle.Render("Moods: ")+strings.Join(in.Moods, ", ")))
	}
	if desc && in.Description != "" {
		out = append(out, "", wrap.Render(in.Description))
	}
	if u := in.URL(); u != "" {
		out = append(out, "", dimStyle.Render(u))
	}
	return out
}

// hcHeader is the title and facts line for a book with no catalog record,
// from Hardcover once it's loaded.
func (m Model) hcHeader() (title, meta string) {
	b := m.selected
	title, meta = b.Title, firstOr(b.Authors, "")
	h := m.hcInfo[m.detailKey()]
	if h == nil || !h.ok {
		return title, meta
	}
	in := h.info
	title = in.Title
	if in.Subtitle != "" {
		title += ": " + in.Subtitle
	}
	var facts []string
	if len(in.Authors) > 0 {
		facts = append(facts, strings.Join(in.Authors, ", "))
	} else if in.Author != "" {
		facts = append(facts, in.Author)
	}
	if in.Year > 0 {
		facts = append(facts, strconv.Itoa(in.Year))
	}
	if in.Pages > 0 {
		facts = append(facts, fmt.Sprintf("%d pages", in.Pages))
	}
	return title, strings.Join(facts, " · ")
}
