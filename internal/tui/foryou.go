package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	bc "shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/recommend"
)

// The "For you" screen (p) runs the recommender: it loads the reading record,
// then gathers picks stage by stage, showing them as they arrive.

type (
	recProgressMsg struct {
		seq   int
		stage string
		picks []recommend.Pick
		next  <-chan tea.Msg
	}
	recDoneMsg struct {
		seq     int
		picks   []recommend.Pick
		profile recommend.Profile
		err     error
	}
)

func (m Model) openForYou() (Model, tea.Cmd) {
	m.screen = forYouScreen
	m.input.Blur()
	if m.acct == nil || !m.hasRecord() || m.recLoading || m.recDone {
		return m, nil
	}
	return m.loadForYou()
}

// loadForYou starts the recommender in the background; its progress
// arrives as a chain of messages read from a channel.
func (m Model) loadForYou() (Model, tea.Cmd) {
	m.recSeq++
	m.recLoading, m.recDone = true, false
	m.recErr = nil
	m.recStage = "reading your history"
	seq := m.recSeq
	client, st := m.client, m.acct
	opts := recommend.Options{
		Home: m.home, HomeName: m.homeName, Language: "eng", Limit: 30,
		Hardcover: st.HardcoverClient(),
	}
	if !m.wishLoading {
		for _, w := range m.wishItems {
			if w.Available() {
				opts.Wishlist = append(opts.Wishlist, w)
			}
		}
	}
	ch := make(chan tea.Msg, 8)
	go func() {
		defer close(ch)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		rec, err := st.LoadRecord(ctx, client, client.Library)
		if err != nil {
			ch <- recDoneMsg{seq: seq, err: err}
			return
		}
		picks, prof, err := recommend.Recommend(ctx, client, rec, opts, func(stage string, picks []recommend.Pick) {
			ch <- recProgressMsg{seq: seq, stage: stage, picks: picks}
		})
		ch <- recDoneMsg{seq: seq, picks: picks, profile: prof, err: err}
	}()
	return m, waitRec(ch)
}

// hasRecord reports whether any source of reading history is set up.
func (m Model) hasRecord() bool {
	return m.acct.HasCatalog() || m.acct.HasLibby() || m.acct.HasHardcover()
}

func waitRec(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		if p, isProgress := msg.(recProgressMsg); isProgress {
			p.next = ch
			return p
		}
		return msg
	}
}

func (m Model) handleRecMsg(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case recProgressMsg:
		if msg.seq != m.recSeq {
			return m, nil
		}
		m.recStage = msg.stage
		if len(msg.picks) > 0 {
			m.recPicks = msg.picks
		}
		return m, waitRec(msg.next)
	case recDoneMsg:
		if msg.seq != m.recSeq {
			return m, nil
		}
		m.recLoading = false
		m.recErr = msg.err
		if msg.err == nil {
			m.recDone = true
			m.recPicks, m.recProfile = msg.picks, msg.profile
		}
		m.recCursor = max(0, min(m.recCursor, m.forYouRows()-1))
	}
	return m, nil
}

// forYouRows counts the selectable rows: picks, then Hardcover's picks the
// library has but not on the shelf now, then ones it doesn't have.
func (m Model) forYouRows() int {
	return len(m.recPicks) + len(m.recProfile.NotOnShelf) + len(m.recProfile.ToBuy)
}

// elsewhereAt returns the not-on-the-shelf or to-buy entry at row i, if
// row i is one.
func (m Model) elsewhereAt(i int) (recommend.Elsewhere, bool) {
	i -= len(m.recPicks)
	if i < 0 {
		return recommend.Elsewhere{}, false
	}
	if i < len(m.recProfile.NotOnShelf) {
		return m.recProfile.NotOnShelf[i], true
	}
	i -= len(m.recProfile.NotOnShelf)
	if i < len(m.recProfile.ToBuy) {
		return m.recProfile.ToBuy[i], true
	}
	return recommend.Elsewhere{}, false
}

func (m Model) updateForYou(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	n := m.forYouRows()
	h := max(1, m.listHeight()/2)
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "esc", "/", "s":
		return m, m.toSearch()
	case "r":
		if !m.recLoading && m.acct != nil {
			return m.loadForYou()
		}
	case "up", "k":
		m.recCursor--
	case "down", "j":
		m.recCursor++
	case "pgup":
		m.recCursor -= h
	case "pgdown", "space":
		m.recCursor += h
	case "home", "g":
		m.recCursor = 0
	case "end", "G":
		m.recCursor = n - 1
	case "enter", "right", "l":
		if m.recCursor < len(m.recPicks) {
			return m.openBib(m.recPicks[m.recCursor].Bib, forYouScreen)
		}
		if u, ok := m.elsewhereAt(m.recCursor); ok {
			if u.Bib.ID != "" {
				return m.openBib(u.Bib, forYouScreen) // its branches and holds
			}
			return m.openBook(u.Title, u.Author, u.HardcoverID, forYouScreen)
		}
	case "h":
		from := ""
		if m.recCursor < len(m.recPicks) {
			from = m.recPicks[m.recCursor].From
		} else if u, ok := m.elsewhereAt(m.recCursor); ok {
			from = u.From
		}
		if from != "" {
			return m.openHardcover(from, forYouScreen)
		}
	}
	m.recCursor = max(0, min(m.recCursor, n-1))
	return m, nil
}

func (m Model) viewForYou() string {
	switch {
	case m.acct == nil || !m.hasRecord():
		return "\n  " + titleStyle.Render("Recommendations need your reading history.") +
			"\n\n  Set up your library login (shelfcheck login), Libby (shelfcheck libby link <code>)\n  and/or Hardcover (HARDCOVER_TOKEN)."
	case m.recErr != nil && len(m.recPicks) == 0:
		return "\n  " + errStyle.Render("Couldn't build recommendations: "+m.recErr.Error()) + "\n\n  Press r to try again."
	case len(m.recPicks) == 0:
		return "\n  " + m.spin.View() + " " + capitalize(m.recStage) + "…" +
			dimStyle.Render("\n\n  This takes about half a minute the first time.")
	}

	head := "  " + titleStyle.Render("For you") + dimStyle.Render(" · on the shelf at "+m.homeName+", or ebooks to borrow now")
	if m.recLoading {
		head += dimStyle.Render(" · "+m.recStage+" ") + m.spin.View()
	}
	w := max(20, m.width-4)
	var lines []string
	cursorLine := 0
	for i, p := range m.recPicks {
		if i == m.recCursor {
			cursorLine = len(lines)
		}
		lines = append(lines, cursorRow(i == m.recCursor, bookLine(p.Bib, w, "", pickWhere(p))))
		why := strings.Join(p.Reasons, " · ")
		lines = append(lines, "    "+dimStyle.Render(ansi.Truncate(why, max(10, w-4), "…")))
	}
	if !m.recLoading {
		i := len(m.recPicks)
		section := func(title string, us []recommend.Elsewhere, where func(recommend.Elsewhere) (string, string)) {
			if len(us) == 0 {
				return
			}
			lines = append(lines, "", "  "+headingStyle.Render(title))
			for _, u := range us {
				if i == m.recCursor {
					cursorLine = len(lines)
				}
				b := u.Bib
				if b.ID == "" {
					b = bc.Bib{Title: u.Title, Authors: []string{u.Author}}
				}
				status, detail := where(u)
				lines = append(lines, cursorRow(i == m.recCursor, bookLine(b, w, "", status)),
					"    "+dimStyle.Render(ansi.Truncate(detail+" · "+u.Reason, max(10, w-4), "…")))
				i++
			}
		}
		section("HARDCOVER RECOMMENDS · AT THE LIBRARY, NOT ON THE SHELF NOW", m.recProfile.NotOnShelf,
			func(u recommend.Elsewhere) (string, string) {
				if u.Bib.Summary.Available > 0 {
					return okStyle.Render("other branches"), u.Status()
				}
				return warnStyle.Render("checked out"), u.Status() + " · enter for branches and holds"
			})
		section("HARDCOVER RECOMMENDS · NOT AT THE LIBRARY, TO BUY", m.recProfile.ToBuy,
			func(u recommend.Elsewhere) (string, string) {
				return accentStyle.Render("to buy"), u.URL
			})
	}
	if len(m.recProfile.UpNext) > 0 && !m.recLoading {
		lines = append(lines, "", "  "+headingStyle.Render("UP NEXT IN YOUR SERIES, NOT AVAILABLE NOW"))
		for _, u := range m.recProfile.UpNext {
			lines = append(lines, "    "+fmt.Sprintf("%s #%v: ", u.Series, u.Position)+titleStyle.Render(u.Title)+dimStyle.Render(" · search it to place a hold"))
		}
	}
	if m.recErr != nil {
		lines = append(lines, "", "  "+errStyle.Render("Some checks failed: "+m.recErr.Error()))
	}

	h := m.bodyHeight() - 2
	start := max(0, min(cursorLine-h/3, len(lines)-h))
	return head + "\n\n" + strings.Join(lines[start:], "\n")
}

func pickWhere(p recommend.Pick) string {
	switch {
	case p.Print && p.Ebook:
		return okStyle.Render("on shelf") + dimStyle.Render(" + ") + accentStyle.Render("ebook")
	case p.Print:
		return okStyle.Render("on shelf")
	case p.Ebook:
		return accentStyle.Render("ebook now")
	}
	return ""
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
