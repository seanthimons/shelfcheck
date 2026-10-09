package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"shelfcheck/internal/hardcover"
	"shelfcheck/internal/recommend"
)

// The editor owns keyboard input, including navigation/quit letters, so
// ordinary review text never triggers app shortcuts. Esc cancels before
// saving; while a write is in flight it waits for the server's response.
type hcEditor struct {
	active, loading, saving bool
	seq                     int
	original                hardcover.UserBook
	status                  hardcover.Status
	rating                  textinput.Model
	review                  textarea.Model
	spoilers                bool
	field                   int
	err                     error
}

type hcEditLoadedMsg struct {
	seq   int
	entry hardcover.UserBook
	ok    bool
	err   error
}

type hcEditSavedMsg struct {
	seq   int
	entry hardcover.UserBook
	err   error
}

func (m Model) openHCEditor() (Model, tea.Cmd) {
	h := m.hcInfo[m.detailKey()]
	if m.acct == nil || !m.acct.HasHardcover() {
		m.flash = "Set HARDCOVER_TOKEN to edit your Hardcover record"
		return m, nil
	}
	if h == nil {
		m.flash = "Looking up this book on Hardcover; press e when it loads"
		return m.fetchHCInfo()
	}
	if h.loading {
		m.flash = "Hardcover lookup is still loading; press e when it finishes"
		return m, nil
	}
	if h.err != nil {
		m.flash = "Hardcover lookup failed: " + h.err.Error() + "; retrying now"
		delete(m.hcInfo, m.detailKey())
		return m.fetchHCInfo()
	}
	if !h.ok || h.info.ID == 0 {
		m.flash = "No matching book found on Hardcover; r retries the lookup"
		return m, nil
	}
	m.hcEdit = hcEditor{active: true, loading: true, seq: m.hcEdit.seq + 1}
	hc, id, seq := m.acct.HardcoverClient(), h.info.ID, m.hcEdit.seq
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		ub, ok, err := hc.UserBook(ctx, id)
		return hcEditLoadedMsg{seq: seq, entry: ub, ok: ok, err: err}
	}
}

func (m Model) handleHCEditLoaded(msg hcEditLoadedMsg) (Model, tea.Cmd) {
	if !m.hcEdit.active || msg.seq != m.hcEdit.seq {
		return m, nil
	}
	e := &m.hcEdit
	e.loading = false
	if msg.err != nil {
		e.err = msg.err
		return m, nil
	}
	if !msg.ok {
		e.err = fmt.Errorf("This book isn't in your Hardcover library. Add it on Hardcover first.")
		return m, nil
	}
	e.original, e.status, e.spoilers = msg.entry, msg.entry.Status, msg.entry.ReviewHasSpoilers
	e.rating = textinput.New()
	e.rating.Prompt = ""
	e.rating.Placeholder = "unrated"
	e.rating.CharLimit = 12
	if msg.entry.Rating != 0 {
		e.rating.SetValue(strconv.FormatFloat(msg.entry.Rating, 'f', -1, 64))
	}
	e.review = textarea.New()
	e.review.Prompt = ""
	e.review.ShowLineNumbers = false
	e.review.CharLimit = 0
	e.review.Placeholder = "Write your review…"
	e.review.SetValue(msg.entry.Review)
	m.styleHCEditor()
	m.resizeHCEditor()
	m.cacheHCEntry(msg.entry)
	m.refreshDetail()
	return m, nil
}

func (m *Model) resizeHCEditor() {
	if m.hcEdit.original.UserBookID == 0 {
		return
	}
	m.hcEdit.rating.SetWidth(max(5, min(20, m.width-4)))
	m.hcEdit.review.SetWidth(max(1, m.width-4))
	m.hcEdit.review.SetHeight(max(2, m.bodyHeight()-13))
}

func (e hcEditor) changes() (hardcover.UserBookChanges, error) {
	p := hardcover.UserBookChanges{}
	if e.status != e.original.Status {
		s := e.status
		p.Status = &s
	}
	r := strings.TrimSpace(e.rating.Value())
	if r == "" {
		p.ClearRating = e.original.Rating != 0
	} else {
		n, err := strconv.ParseFloat(r, 64)
		if err != nil || !(n >= 0.5 && n <= 5) || n*2 != float64(int(n*2)) {
			return p, fmt.Errorf("Rating must be 0.5–5 in half-star steps, or blank to clear")
		}
		if n != e.original.Rating {
			p.Rating = &n
		}
	}
	if e.review.Value() != e.original.Review {
		r := e.review.Value()
		p.Review = &r
	}
	if e.spoilers != e.original.ReviewHasSpoilers {
		s := e.spoilers
		p.ReviewHasSpoilers = &s
	}
	return p, nil
}

func (m Model) updateHCEditor(msg tea.Msg) (Model, tea.Cmd) {
	e := &m.hcEdit
	if key, ok := msg.(tea.KeyPressMsg); ok {
		if key.String() == "esc" && !e.saving {
			e.active = false
			return m, nil
		}
		if e.loading || e.saving || e.original.UserBookID == 0 {
			return m, nil
		}
		switch key.String() {
		case "tab", "shift+tab":
			step := 1
			if key.String() == "shift+tab" {
				step = -1
			}
			e.field = (e.field + step + 4) % 4
			e.rating.Blur()
			e.review.Blur()
			if e.field == 1 {
				return m, e.rating.Focus()
			}
			if e.field == 2 {
				return m, e.review.Focus()
			}
			return m, nil
		case "ctrl+s":
			p, err := e.changes()
			if err != nil {
				e.err = err
				return m, nil
			}
			if p.Status == nil && p.Rating == nil && !p.ClearRating && p.Review == nil && p.ReviewHasSpoilers == nil {
				e.active = false
				m.flash = "No changes to save"
				return m, nil
			}
			var hc *hardcover.Client
			if m.acct != nil {
				hc = m.acct.HardcoverClient()
			}
			if hc == nil {
				e.err = fmt.Errorf("Set HARDCOVER_TOKEN to save")
				return m, nil
			}
			e.saving, e.err = true, nil
			seq, id := e.seq, e.original.UserBookID
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
				defer cancel()
				ub, err := hc.UpdateUserBook(ctx, id, p)
				return hcEditSavedMsg{seq: seq, entry: ub, err: err}
			}
		}
		if e.field == 0 {
			switch key.String() {
			case "left", "up":
				e.status = hardcover.Status((int(e.status)+4)%6 + 1)
			case "right", "down", " ", "space", "enter":
				e.status = hardcover.Status(int(e.status)%6 + 1)
			}
			return m, nil
		}
		if e.field == 3 {
			switch key.String() {
			case " ", "space", "enter", "left", "right":
				e.spoilers = !e.spoilers
			}
			return m, nil
		}
	}
	if e.loading || e.saving || e.original.UserBookID == 0 {
		return m, nil
	}
	var cmd tea.Cmd
	if e.field == 1 {
		e.rating, cmd = e.rating.Update(msg)
	}
	if e.field == 2 {
		e.review, cmd = e.review.Update(msg)
	}
	return m, cmd
}

func (m Model) handleHCEditSaved(msg hcEditSavedMsg) (Model, tea.Cmd) {
	if !m.hcEdit.active || msg.seq != m.hcEdit.seq {
		return m, nil
	}
	m.hcEdit.saving = false
	if msg.entry.UserBookID != 0 {
		m.cacheHCEntry(msg.entry)
		// Use the returned record as the baseline if Hardcover ignored one
		// field but applied others; retain the draft for correction/retry.
		m.hcEdit.original = msg.entry
	}
	// Invalidate derived data even on ambiguous errors: the server might
	// have saved a write whose response was lost. Drop old async results.
	m.wishSeq++
	m.wishLoading = false
	m.wishTitles, m.wishItems, m.wishErr = nil, nil, nil
	m.recSeq++
	m.recLoading, m.recDone = false, false
	m.recPicks, m.recErr = nil, nil
	m.recProfile = recommend.Profile{}
	m.recCursor = 0
	if msg.err != nil {
		m.hcEdit.err = msg.err
		m.refreshDetail()
		return m, nil
	}
	m.hcEdit.active = false
	m.flash = "Saved to Hardcover"
	m.refreshDetail()
	return m, nil
}

func (m *Model) cacheHCEntry(ub hardcover.UserBook) {
	if m.hcLib == nil {
		m.hcLib = map[int]hardcover.UserBook{}
	}
	m.hcLib[ub.ID] = ub
	if m.hc.shelves != nil {
		m.hc.shelves[ub.ID] = ub
	}
}

func (m Model) viewHCEditor() string {
	e := m.hcEdit
	if e.loading {
		return m.notice(m.spin.View() + " Loading your Hardcover record…\n\nesc cancels")
	}
	if e.original.UserBookID == 0 {
		return m.notice(errStyle.Render(e.err.Error()) + "\n\nesc goes back")
	}
	label := func(n int, text string) string {
		if e.field == n {
			return accentStyle.Render("▸ " + text)
		}
		return dimStyle.Render("  " + text)
	}
	spoiler := "no"
	if e.spoilers {
		spoiler = "yes"
	}
	lines := []string{
		"  " + titleStyle.Render("Edit your Hardcover record"),
		"  " + ansi.Truncate(e.original.Title, max(1, m.width-4), "…"), "",
		label(0, "Status: ") + e.status.String() + dimStyle.Render("  ← → change"),
		label(1, "Rating: ") + e.rating.View() + dimStyle.Render("  0.5–5; blank clears"), "",
		label(2, "Review (Markdown):"), e.review.View(),
		label(3, "Contains spoilers: ") + spoiler, "",
	}
	if e.saving {
		lines = append(lines, dimStyle.Render(m.spin.View()+" Saving to Hardcover…"))
	} else if e.err != nil {
		lines = append(lines, lipgloss.NewStyle().Width(max(1, m.width-4)).Render(errStyle.Render(e.err.Error())))
	}
	return strings.Join(lines, "\n")
}
