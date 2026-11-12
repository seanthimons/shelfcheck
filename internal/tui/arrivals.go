package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"shelfcheck/internal/account"
	"shelfcheck/internal/hardcover"
	"shelfcheck/internal/recommend"
)

// The "New arrivals" screen (n) lists books on the new-books shelves at the
// home branch now. With a reading record it ranks them by fit; without one
// (over SSH, say) they come in the order the library acquired them.

type (
	arrivalsProgressMsg struct {
		seq  int
		res  recommend.ArrivalsResult
		next <-chan tea.Msg
	}
	arrivalsDoneMsg struct {
		seq    int
		res    recommend.ArrivalsResult
		ranked bool
		recErr error // the reading record couldn't be loaded; the list is unranked
		err    error
	}
	arrivalsState struct {
		seq     int
		loading bool
		done    bool
		ranked  bool
		res     recommend.ArrivalsResult
		recErr  error
		err     error
		cursor  int
	}
)

func (m Model) openArrivals() (Model, tea.Cmd) {
	m.screen = arrivalsScreen
	m.input.Blur()
	if m.arr.loading || m.arr.done {
		return m, nil
	}
	return m.loadArrivals()
}

// loadArrivals scans in the background; progress arrives as a chain of
// messages read from a channel, as on the For you screen.
func (m Model) loadArrivals() (Model, tea.Cmd) {
	m.arr.seq++
	m.arr.loading, m.arr.done = true, false
	m.arr.err, m.arr.recErr = nil, nil
	m.arr.res = recommend.ArrivalsResult{}
	seq := m.arr.seq
	client, st := m.client, m.acct
	// ponytail: adult books in English only; make the audience a setting
	// when someone wants the juvenile or teen new shelves.
	opts := recommend.ArrivalOptions{Home: m.home, Audience: "adult", Language: "eng"}
	withRecord := st != nil && m.hasRecord()
	ch := make(chan tea.Msg, 8)
	go func() {
		defer close(ch)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		done := arrivalsDoneMsg{seq: seq}
		var rec *account.Record
		if withRecord {
			if rec, done.recErr = st.LoadRecord(ctx, client, client.Library); done.recErr != nil {
				rec = nil
			} else {
				opts.Hardcover = st.HardcoverClient()
			}
		}
		done.ranked = rec != nil
		done.res, done.err = recommend.Arrivals(ctx, client, rec, opts, func(res recommend.ArrivalsResult) {
			ch <- arrivalsProgressMsg{seq: seq, res: res}
		})
		ch <- done
	}()
	return m, waitArrivals(ch)
}

func waitArrivals(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		if p, isProgress := msg.(arrivalsProgressMsg); isProgress {
			p.next = ch
			return p
		}
		return msg
	}
}

func (m Model) handleArrivalsMsg(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case arrivalsProgressMsg:
		if msg.seq == m.arr.seq {
			m.arr.res = msg.res
		}
		// Superseded runs are drained too, so their producer can finish.
		return m, waitArrivals(msg.next)
	case arrivalsDoneMsg:
		if msg.seq != m.arr.seq {
			return m, nil
		}
		m.arr.loading = false
		m.arr.err, m.arr.recErr = msg.err, msg.recErr
		if msg.err == nil {
			m.arr.done, m.arr.ranked, m.arr.res = true, msg.ranked, msg.res
		}
		m.arr.cursor = max(0, min(m.arr.cursor, len(m.arr.res.Items)-1))
	}
	return m, nil
}

func (m Model) updateArrivals(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	n := len(m.arr.res.Items)
	h := max(1, m.listHeight()/2)
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "esc", "/", "s":
		return m, m.toSearch()
	case "r":
		if !m.arr.loading {
			return m.loadArrivals()
		}
	case "up", "k":
		m.arr.cursor--
	case "down", "j":
		m.arr.cursor++
	case "pgup":
		m.arr.cursor -= h
	case "pgdown", "space":
		m.arr.cursor += h
	case "home", "g":
		m.arr.cursor = 0
	case "end", "G":
		m.arr.cursor = n - 1
	case "+":
		if m.arr.cursor < n {
			a := m.arr.res.Items[m.arr.cursor]
			if a.HardcoverID == 0 {
				// ponytail: no lookup from the list; the detail view already does one.
				m.flash = "Not matched on Hardcover yet; press enter, then + on the book's details"
				return m, nil
			}
			b := hardcover.Book{ID: a.HardcoverID, Title: a.Bib.Title}
			if len(a.Bib.Authors) > 0 {
				b.Author = a.Bib.Authors[0]
			}
			return m.toggleWant(b)
		}
	case "enter", "right", "l":
		if m.arr.cursor < n {
			return m.openBib(m.arr.res.Items[m.arr.cursor].Bib, arrivalsScreen)
		}
	}
	m.arr.cursor = max(0, min(m.arr.cursor, n-1))
	return m, nil
}

// shelfName is the section from a collection: "New Arrivals" from "Main -
// 1st Floor South - New Arrivals".
func shelfName(collection string) string {
	if i := strings.LastIndex(collection, " - "); i >= 0 {
		return collection[i+3:]
	}
	return collection
}

func (m Model) viewArrivals() string {
	res := m.arr.res
	switch {
	case m.arr.err != nil:
		return m.notice(errStyle.Render("Couldn't scan the new arrivals: "+m.arr.err.Error()) + "\n\nPress r to try again.")
	case len(res.Items) == 0 && m.arr.loading:
		return m.notice(m.spin.View() + fmt.Sprintf(" Checking recently added books for copies on the new shelves… %d checked", res.Checked))
	case len(res.Items) == 0:
		return m.notice(titleStyle.Render("Nothing found on the new shelves at "+m.homeName+".") +
			"\n\n" + dimStyle.Render(m.arrivalsScope()) + "\n\nPress r to scan again.")
	}

	head := "  " + titleStyle.Render("New arrivals") + dimStyle.Render(" · on the new shelves at "+m.homeName+" · "+m.arrivalsScope())
	if m.arr.loading {
		head += " " + m.spin.View()
	}
	w := max(20, m.width-4)
	var lines []string
	cursorLine := 0
	for i, a := range res.Items {
		if i == m.arr.cursor {
			cursorLine = len(lines)
		}
		lines = append(lines, cursorRow(i == m.arr.cursor, bookLine(a.Bib, w, "", okStyle.Render(shelfName(a.Shelf)))))
		why := strings.Join(a.Reasons, " · ")
		if why == "" {
			why = a.CallNumber
		}
		lines = append(lines, "    "+dimStyle.Render(ansi.Truncate(why, max(10, w-4), "…")))
	}
	switch {
	case m.arr.loading:
	case m.arr.recErr != nil:
		lines = append(lines, "", "  "+errStyle.Render("Not ranked: couldn't load your reading record: "+m.arr.recErr.Error()))
	case !m.arr.ranked:
		lines = append(lines, "", "  "+dimStyle.Render("Newest first. Set up a library login, Libby or Hardcover to rank these by what you read."))
	}
	for _, n := range res.Notes {
		lines = append(lines, "  "+errStyle.Render(capitalize(n)))
	}

	h := m.bodyHeight() - 2
	start := max(0, min(cursorLine-h/3, len(lines)-h))
	return head + "\n\n" + strings.Join(withScrollbar(lines[start:], scrollbar(h, len(lines), start), m.width), "\n")
}

// arrivalsScope says how much of the catalog the list covers: it's a
// bounded scan, not an inventory of the shelf.
func (m Model) arrivalsScope() string {
	res := m.arr.res
	s := fmt.Sprintf("%s checked", plural(res.Checked, "recent title"))
	if res.Failed > 0 {
		s += fmt.Sprintf(", %d couldn't be", res.Failed)
	}
	if res.Known > 0 {
		s += fmt.Sprintf(" · %d you've read or have left out", res.Known)
	}
	if res.More {
		s += " · more not scanned"
	}
	return s
}
