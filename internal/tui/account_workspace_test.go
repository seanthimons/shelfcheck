package tui

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"shelfcheck/internal/account"
	bc "shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/hardcover"
)

func workspaceModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("HARDCOVER_TOKEN", "test-token")
	t.Setenv("SHELFCHECK_CARD", "")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	m := New(bc.New("test"), "1", false, "", account.DefaultStore(t.TempDir()))
	m.screen, m.width, m.height = accountScreen, 120, 30
	m.acctLoaded, m.hcLibraryLoaded = true, true
	m.checkouts = []bc.Checkout{{Bib: bc.Bib{ID: "loan", Title: "Piranesi", Authors: []string{"Susanna Clarke"}}, Due: "2026-10-15"}}
	m.holds = []bc.Hold{{Bib: bc.Bib{Title: "James", Authors: []string{"Percival Everett"}}, Position: 8, WaitDays: 21, Digital: true}}
	m.recent = []bc.HistoryItem{{Bib: bc.Bib{Title: "Borrowed but unfinished"}, CheckedOut: "2026-09-01"}}
	m.hcLib = map[int]hardcover.UserBook{
		1: {Book: hardcover.Book{ID: 1, Title: "Piranesi", Author: "Susanna Clarke"}, UserBookID: 11, Status: hardcover.CurrentlyReading, Added: "2026-10-09"},
		2: {Book: hardcover.Book{ID: 2, Title: "The Dispossessed", Author: "Ursula K. Le Guin"}, UserBookID: 12, Status: hardcover.Read, Rating: 5, Added: "2026-08-01", LastRead: "2026-09-28"},
		3: {Book: hardcover.Book{ID: 3, Title: "The Left Hand of Darkness", Author: "Ursula K. Le Guin"}, UserBookID: 13, Status: hardcover.Read, Rating: 4.5, Added: "2026-09-01", LastRead: "2026-08-14"},
	}
	return m
}

func accountPress(m Model, code rune, text string) (Model, tea.Cmd) {
	next, cmd := m.update(tea.KeyPressMsg{Code: code, Text: text})
	return next.(Model), cmd
}

func TestAccountSectionsSeparateBorrowingAndReading(t *testing.T) {
	m := workspaceModel(t)
	if rows := m.acctRows(); len(rows) != 3 || rows[0].section != accountLoans || rows[1].section != accountHolds || rows[2].hcID != 1 {
		t.Fatalf("overview=%+v", rows)
	}
	m.accountUI.section = accountHistory
	if rows := m.acctRows(); len(rows) != 1 || rows[0].bib.Title != "Borrowed but unfinished" || rows[0].hcID != 0 {
		t.Fatalf("history=%+v", rows)
	}
	m.accountUI.section = accountRead
	for _, row := range m.acctRows() {
		if row.bib.Title == "Borrowed but unfinished" || row.hcID == 0 {
			t.Fatalf("borrowing leaked into Read: %+v", row)
		}
	}
}

func TestAccountRemembersSectionCursorAndDetailOrigin(t *testing.T) {
	m := workspaceModel(t)
	m.accountUI.section = accountRead
	m.acctCursor = 1
	m, _ = accountPress(m, tea.KeyTab, "")
	m, _ = accountPress(m, tea.KeyUp, "")
	if m.accountUI.section != accountWant || m.acctCursor != 0 {
		t.Fatal("section cursor not switched")
	}
	m, _ = accountPress(m, tea.KeyDown, "")
	if m.accountUI.section != accountRead || m.acctCursor != 1 {
		t.Fatal("section cursor not restored")
	}
	m, _ = accountPress(m, tea.KeyEnter, "") // focus books
	m, _ = accountPress(m, tea.KeyEnter, "") // open selected book
	if m.screen != detailScreen || m.backScreen != accountScreen || m.selectedHC != 2 {
		t.Fatalf("wrong detail origin: screen=%v, back=%v, hc=%v", m.screen, m.backScreen, m.selectedHC)
	}
	m, _ = accountPress(m, tea.KeyEscape, "")
	if m.screen != accountScreen || m.accountUI.section != accountRead || m.acctCursor != 1 {
		t.Fatal("detail return lost section")
	}
}

func TestAccountFilterCapturesShortcutsAndCancel(t *testing.T) {
	m := workspaceModel(t)
	m.accountUI.section = accountAllBooks
	m, _ = accountPress(m, 'f', "f")
	for _, r := range "qwpmt?/" {
		m, _ = accountPress(m, r, string(r))
	}
	if m.screen != accountScreen || m.showHelp || m.accountUI.filter.Value() != "qwpmt?/" {
		t.Fatal("filter leaked global keys")
	}
	m, _ = accountPress(m, tea.KeyEscape, "")
	if m.accountUI.filters[accountAllBooks] != "" || m.accountUI.filtering {
		t.Fatal("cancel applied draft")
	}
	m, _ = accountPress(m, 'f', "f")
	m.accountUI.filter.SetValue("ursula")
	m, _ = accountPress(m, tea.KeyEnter, "")
	if len(m.acctRows()) != 2 || m.accountUI.filters[accountAllBooks] != "ursula" {
		t.Fatal("filter not applied")
	}
	m.accountUI.sorts[accountAllBooks] = 3
	if m.acctRows()[0].hcID != 2 {
		t.Fatal("rating sort not applied")
	}
}

func TestAccountLayoutRetainsHoldDetailsAtEveryWidth(t *testing.T) {
	m := workspaceModel(t)
	m.accountUI.section = accountHolds
	for _, w := range []int{32, 60, 89, 90, 120, 160} {
		m.width = w
		out := m.viewAccount()
		if !strings.Contains(ansi.Strip(out), "#8 in line · ~21d wait") {
			t.Errorf("lost hold detail at width %d: %s", w, ansi.Strip(out))
		}
		lines := strings.Split(out, "\n")
		if len(lines) != m.bodyHeight() {
			t.Errorf("height=%d", len(lines))
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > w {
				t.Errorf("overflow width %d: %q", w, line)
			}
		}
		m.accountUI.sidebar = true
		out = m.viewAccount()
		if !strings.Contains(out, "Holds") {
			t.Fatal("section picker missing selected section")
		}
		m.accountUI.sidebar = false
	}
}

func TestAccountHardcoverWithoutCatalogAndDirectEditor(t *testing.T) {
	m := workspaceModel(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "limit: 1") {
			io.WriteString(w, `{"data":{"me":[{"user_books":[{"id":11,"status_id":2,"book":{"id":1,"title":"Piranesi"}}]}]}}`)
		} else {
			io.WriteString(w, `{"data":{"me":[{"user_books":[{"id":11,"status_id":2,"book":{"id":1,"title":"Piranesi"}}]}]}}`)
		}
	}))
	defer srv.Close()
	m.acct.HardcoverClient().Endpoint = srv.URL
	m.hcLibraryLoaded = false
	m, _ = m.openAccount()
	if !m.hcLibraryLoading || m.acctLoading {
		t.Fatal("Hardcover-only account required catalog")
	}
	m.hcLibraryLoading = false
	var cmd tea.Cmd
	m, cmd = m.loadAccountHardcover(false)
	msg := cmd().(hcLibraryMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	m, _ = m.handleHCLibrary(msg)
	m.accountUI.section = accountReading
	m, cmd = accountPress(m, 'e', "e")
	if !m.hcEdit.active || !m.hcEdit.loading || m.screen != accountScreen || cmd == nil {
		t.Fatal("direct editor did not open")
	}
	loaded := cmd().(hcEditLoadedMsg)
	if loaded.err != nil || loaded.entry.ID != 1 {
		t.Fatalf("loaded=%+v", loaded)
	}
	m, _ = m.handleHCEditLoaded(loaded)
	m, _ = accountPress(m, tea.KeyEscape, "")
	if m.hcEdit.active || m.screen != accountScreen {
		t.Fatal("editor cancel lost account")
	}
}

func TestAccountSourceFailureLeavesOtherSourceVisible(t *testing.T) {
	m := workspaceModel(t)
	m, _ = m.handleHCLibrary(hcLibraryMsg{seq: m.hcLibrarySeq, err: errors.New("Hardcover unavailable")})
	m.accountUI.section = accountLoans
	if !strings.Contains(m.viewAccount(), "Piranesi") {
		t.Fatal("Hardcover failure hid loans")
	}
	m.accountUI.section = accountReading
	next, _ := m.update(accountMsg{err: errors.New("library unavailable")})
	m = next.(Model)
	if !strings.Contains(m.viewAccount(), "Piranesi") || m.acctErr == nil || m.hcLibraryErr == nil {
		t.Fatal("library failure hid Hardcover or its error")
	}
	m, _ = m.handleHCLibrary(hcLibraryMsg{seq: m.hcLibrarySeq - 1, lib: map[int]hardcover.UserBook{}})
	if len(m.hcLib) != 3 {
		t.Fatal("stale snapshot replaced library")
	}
}

func TestAccountEditorSaveMovesBookAndClampsSelection(t *testing.T) {
	m := workspaceModel(t)
	m.accountUI.section = accountReading
	m.hcEdit = hcEditor{active: true, saving: true, seq: 1}
	entry := m.hcLib[1]
	entry.Status = hardcover.Read
	m, _ = m.handleHCEditSaved(hcEditSavedMsg{seq: 1, entry: entry})
	if m.hcEdit.active || m.screen != accountScreen || len(m.acctRows()) != 0 || m.acctCursor != 0 {
		t.Fatal("save did not return to updated shelf")
	}
	m.accountUI.section = accountRead
	if len(m.acctRows()) != 3 {
		t.Fatal("saved record not visible on new shelf")
	}
}
