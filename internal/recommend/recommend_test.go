package recommend

import (
	"testing"

	"shelfcheck/internal/account"
	bc "shelfcheck/internal/bibliocommons"
)

func TestSubtitlesClash(t *testing.T) {
	cases := []struct {
		catTitle, sub, title string
		clash                bool
	}{
		{"Mistborn", "Secret History", "Mistborn: The Final Empire", true},
		{"Mistborn: Secret History", "", "Mistborn: The Final Empire", true},
		{"Mistborn", "", "Mistborn: The Final Empire", false},
		{"Mistborn", "The Final Empire", "Mistborn: The Final Empire", false},
		{"A Deadly Education", "A Novel", "A Deadly Education", false},
		{"Upgrade", "A Novel", "Upgrade: A Novel", false},
		{"Babel, Or, The Necessity of Violence", "An Arcane History of the Oxford Translators' Revolution", "Babel, or The Necessity of Violence: An Arcane History of the Oxford Translators' Revolution", false},
		{"Ogres", "A Dystopian Novella by the 'British Master of Science Fiction'", "Ogres", false},
	}
	for _, c := range cases {
		if got := subtitlesClash(c.catTitle, c.sub, c.title); got != c.clash {
			t.Errorf("subtitlesClash(%q, %q, %q) = %v, want %v", c.catTitle, c.sub, c.title, got, c.clash)
		}
	}
}

func TestElsewhereStatus(t *testing.T) {
	cases := []struct {
		u    Elsewhere
		want string
	}{
		{Elsewhere{}, "not at the library"},
		{Elsewhere{Bib: bc.Bib{ID: "1", Format: "BK", Summary: bc.Summary{Available: 10, Total: 16}}}, "10 in at other branches"},
		{Elsewhere{Bib: bc.Bib{ID: "1", Format: "BK", Summary: bc.Summary{Total: 3, Holds: 2}}}, "all 3 copies out · 2 holds"},
		{Elsewhere{Bib: bc.Bib{ID: "1", Format: "EBOOK", Summary: bc.Summary{Total: 4}}}, "all 4 ebook copies out"},
		{Elsewhere{Bib: bc.Bib{ID: "1", Format: "BK", Summary: bc.Summary{Total: 6, Holds: 1}}}, "all 6 copies out · 1 hold"},
		{Elsewhere{Bib: bc.Bib{ID: "1", Format: "BK", Summary: bc.Summary{OnOrder: 2}}}, "on order"},
	}
	for _, c := range cases {
		if got := c.u.Status(); got != c.want {
			t.Errorf("Status(%+v) = %q, want %q", c.u.Bib.Summary, got, c.want)
		}
	}
}

func TestPickRetainsAvailableEditions(t *testing.T) {
	e := &engine{rec: &account.Record{}, picks: map[string]*Pick{}, ahead: map[string]bool{}}
	print := bc.Bib{ID: "print", Title: "Example", Authors: []string{"Author"}, Format: "BK"}
	ebook := print
	ebook.ID, ebook.Format, ebook.Digital = "ebook", "EBOOK", true
	e.add(ebook, "genre", 1, "genre")
	p := e.add(print, "author", 2, "author")
	e.add(ebook, "wish", 3, "wishlist")
	if !p.Print || !p.Ebook || p.Bib.ID != "print" || len(p.Editions) != 2 {
		t.Fatalf("merged pick lost editions: %+v", p)
	}
}
