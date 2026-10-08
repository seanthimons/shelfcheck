package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"shelfcheck/internal/account"

	bc "shelfcheck/internal/bibliocommons"
)

func TestYearOf(t *testing.T) {
	for in, want := range map[string]string{"2024": "2024", "1978-1992": "1978", "20261006": "2026", "": "", "n.d.": "n.d."} {
		if got := yearOf(in); got != want {
			t.Errorf("yearOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPlural(t *testing.T) {
	for _, c := range []struct {
		n    int
		word string
		want string
	}{{1, "hold", "1 hold"}, {2, "hold", "2 holds"}, {3, "copy", "3 copies"}, {1, "copy", "1 copy"}} {
		if got := plural(c.n, c.word); got != c.want {
			t.Errorf("plural(%d, %q) = %q, want %q", c.n, c.word, got, c.want)
		}
	}
}

func TestFilterSummary(t *testing.T) {
	m := Model{home: "1", branches: map[string]string{"1": "Downtown Main Library"}}
	q := bc.Query{Text: "x", Sort: bc.SortTopRated}
	q.Toggle(bc.FacetFormat, "BK")
	q.Toggle(bc.FacetStatus, "1")
	q.Toggle(bc.FacetRating, "FOUR_PLUS_STARS")
	want := "On shelf at Downtown Main Library (home) · Book · ★ 4+ · sorted by top rated"
	if got := m.filterSummary(q); got != want {
		t.Errorf("filterSummary = %q, want %q", got, want)
	}
	if got := m.filterSummary(bc.Query{Text: "x"}); got != "" {
		t.Errorf("empty filterSummary = %q", got)
	}
}

func TestDueIn(t *testing.T) {
	today := time.Now()
	cases := map[string]string{
		today.AddDate(0, 0, 10).Format("2006-01-02"): "· 10d",
		today.AddDate(0, 0, 2).Format("2006-01-02"):  "· 2d",
		today.Format("2006-01-02"):                   "today",
		today.AddDate(0, 0, -3).Format("2006-01-02"): "overdue",
		"garbage": "garbage",
	}
	for in, want := range cases {
		if got := ansi.Strip(dueIn(in)); !strings.Contains(got, want) {
			t.Errorf("dueIn(%q) = %q, want it to contain %q", in, got, want)
		}
	}
}

func TestHoldStatus(t *testing.T) {
	for _, c := range []struct {
		h    bc.Hold
		want string
	}{
		{bc.Hold{Status: "SUSPENDED", Position: 1}, "paused · #1 in line"},
		{bc.Hold{Status: "NOT_YET_AVAILABLE", Position: 4}, "#4 in line"},
		{bc.Hold{Status: "READY_FOR_PICKUP"}, "ready for pickup"},
	} {
		if got := ansi.Strip(holdStatus(c.h)); got != c.want {
			t.Errorf("holdStatus(%+v) = %q, want %q", c.h, got, c.want)
		}
	}
}

func TestBestMatchPrefersPrint(t *testing.T) {
	w := account.WishItem{Matches: []account.WishMatch{
		{Bib: bc.Bib{ID: "e"}, Online: true},
		{Bib: bc.Bib{ID: "p"}},
	}}
	if b, ok := bestMatch(w); !ok || b.ID != "p" {
		t.Errorf("bestMatch = %q, want print copy", b.ID)
	}
	if _, ok := bestMatch(account.WishItem{}); ok {
		t.Error("bestMatch on unavailable item")
	}
}
