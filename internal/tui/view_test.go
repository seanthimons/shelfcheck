package tui

import (
	"testing"

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
