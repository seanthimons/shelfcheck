package tui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	bc "shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/hardcover"
	"shelfcheck/internal/recommend"
)

func TestArrivalsScreenShowsScanScope(t *testing.T) {
	m := New(bc.New("test"), "1", false, "", nil)
	m.width, m.height, m.homeName = 120, 30, "Main Library"
	m.screen = arrivalsScreen
	m.arr.seq, m.arr.loading = 1, true
	res := recommend.ArrivalsResult{Checked: 9, Failed: 1, More: true, Items: []recommend.Arrival{
		{Bib: bc.Bib{ID: "a", Title: "Ghost Barn"}, Shelf: "Main - 1st Floor South - New Arrivals", Reasons: []string{"your genres: Horror"}},
	}}
	// A superseded run's result is dropped.
	m, _ = m.handleArrivalsMsg(arrivalsDoneMsg{seq: 0, res: recommend.ArrivalsResult{Checked: 99}})
	if !m.arr.loading || m.arr.res.Checked != 0 {
		t.Fatal("stale arrivals result applied")
	}
	m, _ = m.handleArrivalsMsg(arrivalsDoneMsg{seq: 1, res: res})
	out := ansi.Strip(m.viewArrivals())
	for _, want := range []string{"Ghost Barn", "New Arrivals", "your genres: Horror", "9 recent titles checked, 1 couldn't be", "more not scanned", "Newest first"} {
		if !strings.Contains(out, want) {
			t.Errorf("view lacks %q:\n%s", want, out)
		}
	}
	next, _ := m.update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m = next.(Model); m.screen != detailScreen || m.selected.ID != "a" || m.backScreen != arrivalsScreen {
		t.Fatalf("enter opened screen %v, book %q", m.screen, m.selected.ID)
	}
}

func TestWantToReadFromDetail(t *testing.T) {
	m := workspaceModel(t)
	var shelved bool // on Want to Read, per the fake server
	var writes []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case strings.Contains(string(body), "insert_user_book"):
			writes, shelved = append(writes, "insert"), true
			io.WriteString(w, `{"data":{"insert_user_book":{"id":77,"error":null}}}`)
		case strings.Contains(string(body), "delete_user_book"):
			writes, shelved = append(writes, "delete"), false
			io.WriteString(w, `{"data":{"delete_user_book":{"id":77}}}`)
		case shelved:
			io.WriteString(w, `{"data":{"me":[{"user_books":[{"id":77,"status_id":1,"book":{"id":9,"title":"Ghost Barn"}}]}]}}`)
		default:
			io.WriteString(w, `{"data":{"me":[{"user_books":[]}]}}`)
		}
	}))
	defer srv.Close()
	m.acct.HardcoverClient().Endpoint = srv.URL
	m.screen = detailScreen
	m.selected = bc.Bib{ID: "bib", Title: "Ghost Barn"}

	// Not looked up yet: nothing is written.
	m, cmd := accountPress(m, '+', "+")
	if cmd != nil || !strings.Contains(m.flash, "Still looking") {
		t.Fatalf("+ before the lookup: flash %q, cmd %v", m.flash, cmd != nil)
	}
	m.hcInfo["bib"] = &hcInfo{ok: true, info: hardcover.BookInfo{Book: hardcover.Book{ID: 9, Title: "Ghost Barn"}}}

	m, cmd = accountPress(m, '+', "+")
	if cmd == nil || !m.hcWantBusy {
		t.Fatal("+ didn't start a save")
	}
	if again, cmd2 := accountPress(m, '+', "+"); cmd2 != nil || !again.hcWantBusy {
		t.Fatal("a second + while saving started another write")
	}
	next, _ := m.update(cmd())
	m = next.(Model)
	if ub := m.hcLib[9]; m.hcWantBusy || ub.UserBookID != 77 || ub.Status != hardcover.WantToRead || !strings.Contains(m.flash, "Added Ghost Barn") {
		t.Fatalf("after add: entry %+v, flash %q", ub, m.flash)
	}
	if !strings.Contains(ansi.Strip(m.renderDetail()), "on your want to read shelf") {
		t.Error("detail doesn't show the new shelf")
	}

	// Again takes it off, going by what Hardcover says rather than hcLib.
	delete(m.hcLib, 9)
	m, cmd = accountPress(m, '+', "+")
	next, _ = m.update(cmd())
	m = next.(Model)
	if _, has := m.hcLib[9]; has || !strings.Contains(m.flash, "Took Ghost Barn off") || strings.Join(writes, ",") != "insert,delete" {
		t.Fatalf("after remove: flash %q, writes %v", m.flash, writes)
	}
}
