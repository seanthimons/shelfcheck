package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	bc "shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/hardcover"
	"shelfcheck/internal/libby"
	"shelfcheck/internal/recommend"
)

func editorModel(t *testing.T) Model {
	t.Helper()
	m := New(bc.New("test"), "1", false, "", nil)
	m.screen, m.width, m.height = detailScreen, 80, 24
	m.selectedHC = 42
	m.hcInfo[m.detailKey()] = &hcInfo{ok: true, info: hardcover.BookInfo{Book: hardcover.Book{ID: 42, Title: "Book"}}}
	m.hcEdit = hcEditor{active: true, loading: true, seq: 1}
	ub := hardcover.UserBook{Book: hardcover.Book{ID: 42, Title: "Book"}, UserBookID: 7, Status: hardcover.Read, Rating: 4.5, Review: "Original **review**.\nSecond line.", ReviewHasSpoilers: true}
	m, _ = m.handleHCEditLoaded(hcEditLoadedMsg{seq: 1, entry: ub, ok: true})
	return m
}

func pressEditor(m Model, code rune, text string) (Model, tea.Cmd) {
	next, cmd := m.update(tea.KeyPressMsg{Code: code, Text: text})
	return next.(Model), cmd
}

func TestHCEditorPrefillsAndPreservesUntouchedFields(t *testing.T) {
	m := editorModel(t)
	p, err := m.hcEdit.changes()
	if err != nil || p.Status != nil || p.Rating != nil || p.ClearRating || p.Review != nil || p.ReviewHasSpoilers != nil {
		t.Fatalf("prefill produced patch %+v, %v", p, err)
	}
	m.hcEdit.review.SetValue("Replacement review")
	p, err = m.hcEdit.changes()
	if err != nil || p.Review == nil || *p.Review != "Replacement review" || p.Status != nil || p.Rating != nil || p.ClearRating || p.ReviewHasSpoilers != nil {
		t.Fatalf("review patch=%+v,%v", p, err)
	}
}

func TestHCEditorRatingValidationAndClear(t *testing.T) {
	m := editorModel(t)
	for _, value := range []string{"0", "6", "3.7", "NaN", "Inf", "words"} {
		m.hcEdit.rating.SetValue(value)
		if _, err := m.hcEdit.changes(); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
	m.hcEdit.rating.SetValue("")
	p, err := m.hcEdit.changes()
	if err != nil || !p.ClearRating || p.Rating != nil {
		t.Fatalf("clear patch=%+v,%v", p, err)
	}
}

func TestHCEditorCapturesReviewShortcutsAndCancel(t *testing.T) {
	m := editorModel(t)
	m.hcEdit.field = 2
	m.hcEdit.review.Focus()
	m.hcEdit.review.SetValue("")
	for _, r := range "qwpmt?/" {
		m, _ = pressEditor(m, r, string(r))
	}
	if m.screen != detailScreen || m.showHelp || m.hcEdit.review.Value() != "qwpmt?/" {
		t.Fatalf("review shortcuts escaped editor: %+v", m.hcEdit)
	}
	m, cmd := pressEditor(m, tea.KeyEscape, "")
	if m.hcEdit.active || cmd != nil || m.hcLib[42].Review != "Original **review**.\nSecond line." {
		t.Fatal("cancel changed record or wrote data")
	}
}

func TestHCEditorIgnoresLateLoadsAndWaitsForSave(t *testing.T) {
	m := editorModel(t)
	m.hcEdit.saving = true
	m, _ = pressEditor(m, tea.KeyEscape, "")
	if !m.hcEdit.active {
		t.Fatal("canceled an in-flight save")
	}
	m, _ = m.handleHCEditSaved(hcEditSavedMsg{seq: 0, err: errors.New("stale")})
	if !m.hcEdit.saving || m.hcEdit.err != nil {
		t.Fatal("accepted stale save")
	}
	m.hcEdit.active = false
	m, _ = m.handleHCEditLoaded(hcEditLoadedMsg{seq: 1, err: errors.New("late")})
	if m.hcEdit.err != nil {
		t.Fatal("accepted canceled load")
	}
}

func TestHCEditorSaveRefreshesCachesAndRetainsRejectedDraft(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "ignored field"}[fail], func(t *testing.T) {
			m := editorModel(t)
			m.hc.shelves = map[int]hardcover.UserBook{}
			m.wishLoading, m.recLoading, m.recDone = true, true, true
			m.wishTitles = []libby.Tagging{{Title: "Old wishlist"}}
			m.recPicks = []recommend.Pick{{}}
			m.hcEdit.saving = true
			m.hcEdit.review.SetValue("Draft")
			ub := m.hcEdit.original
			ub.Rating = 5
			var err error
			if fail {
				err = errors.New("review ignored")
			} else {
				ub.Review = "Draft"
			}
			m, _ = m.handleHCEditSaved(hcEditSavedMsg{seq: 1, entry: ub, err: err})
			if m.hcLib[42].Rating != 5 || m.hc.shelves[42].Rating != 5 || m.wishLoading || m.recLoading || m.recDone || m.wishTitles != nil || m.recPicks != nil {
				t.Fatal("save left stale caches")
			}
			if fail {
				if !m.hcEdit.active || m.hcEdit.err == nil || m.hcEdit.review.Value() != "Draft" || m.hcEdit.original.Review == "Draft" {
					t.Fatal("failed save lost draft or saved baseline")
				}
			} else if m.hcEdit.active || m.flash != "Saved to Hardcover" || !strings.Contains(ansi.Strip(m.renderDetail()), "your rating: 5.0") {
				t.Fatal("saved values missing from detail")
			}
		})
	}
}

func TestHCEditorViewFitsNormalTerminal(t *testing.T) {
	m := editorModel(t)
	view := ansi.Strip(m.viewHCEditor())
	if len(strings.Split(view, "\n")) > m.bodyHeight() || !strings.Contains(view, "Contains spoilers") {
		t.Fatalf("editor exceeds body: %s", view)
	}
}

func TestHCEditorSaveWithoutChangesAndInvalidRating(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		m := editorModel(t)
		if invalid {
			m.hcEdit.rating.SetValue("3.7")
		}
		next, cmd := m.update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
		m = next.(Model)
		if cmd != nil {
			t.Fatal("no-op or invalid save scheduled a write")
		}
		if invalid {
			if !m.hcEdit.active || m.hcEdit.err == nil {
				t.Fatal("invalid rating didn't keep editor open")
			}
		} else if m.hcEdit.active || m.flash != "No changes to save" {
			t.Fatal("no-op save didn't close")
		}
	}
}

func TestHCEditorFailedWriteKeepsOriginalAndDraft(t *testing.T) {
	m := editorModel(t)
	original := m.hcLib[42]
	m.hcEdit.review.SetValue("Draft")
	m.hcEdit.saving = true
	m, _ = m.handleHCEditSaved(hcEditSavedMsg{seq: 1, err: errors.New("token rejected")})
	if !m.hcEdit.active || m.hcEdit.saving || m.hcEdit.err == nil || m.hcLib[42].Review != original.Review || m.hcEdit.review.Value() != "Draft" {
		t.Fatal("failure lost original or draft")
	}
}

func TestHCEditorUnavailableRecordAndToken(t *testing.T) {
	m := editorModel(t)
	m.hcEdit.active = false
	m, cmd := m.openHCEditor()
	if cmd != nil || m.hcEdit.active || !strings.Contains(m.flash, "HARDCOVER_TOKEN") {
		t.Fatal("missing token wasn't explained")
	}
	m.hcEdit = hcEditor{active: true, loading: true, seq: 2}
	m, _ = m.handleHCEditLoaded(hcEditLoadedMsg{seq: 2, ok: false})
	if m.hcEdit.loading || m.hcEdit.original.UserBookID != 0 || m.hcEdit.err == nil || !strings.Contains(ansi.Strip(m.viewHCEditor()), "isn't in your Hardcover library") {
		t.Fatal("missing record wasn't explained")
	}
}

func TestSupersededRecommendationsAreDrained(t *testing.T) {
	m := editorModel(t)
	m.recSeq = 2
	ch := make(chan tea.Msg, 1)
	ch <- recDoneMsg{seq: 1}
	close(ch)
	m, cmd := m.handleRecMsg(recProgressMsg{seq: 1, next: ch, picks: []recommend.Pick{{}}})
	if cmd == nil || m.recPicks != nil {
		t.Fatal("superseded recommendations weren't drained")
	}
	if msg, ok := cmd().(recDoneMsg); !ok || msg.seq != 1 {
		t.Fatal("didn't drain old producer")
	}
}

func TestHCEditorResizeBeforeRecordLoads(t *testing.T) {
	m := editorModel(t)
	m.hcEdit = hcEditor{active: true, loading: true, seq: 2}
	next, _ := m.update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)
	if !m.hcEdit.loading || !strings.Contains(ansi.Strip(m.viewHCEditor()), "Loading your Hardcover record") {
		t.Fatal("resize changed loading state")
	}
}
