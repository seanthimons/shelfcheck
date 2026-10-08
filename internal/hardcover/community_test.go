package hardcover

import "testing"

func TestParseRef(t *testing.T) {
	cases := []struct {
		in   string
		want Ref
		ok   bool
	}{
		{"https://hardcover.app/prompts/what-is-your-favorite-dark-academia-book", Ref{Kind: "prompt", Slug: "what-is-your-favorite-dark-academia-book"}, true},
		{"hardcover.app/prompts/favorite-space-opera-books?tab=books", Ref{Kind: "prompt", Slug: "favorite-space-opera-books"}, true},
		{"https://hardcover.app/@emma/lists/hugo-award-for-best-novel-1953-to-2026", Ref{Kind: "list", Owner: "emma", Slug: "hugo-award-for-best-novel-1953-to-2026"}, true},
		{"what-is-your-favorite-dark-academia-book", Ref{Kind: "prompt", Slug: "what-is-your-favorite-dark-academia-book"}, true},
		{`"https://hardcover.app/prompts/what-is-your-favorite-dark-academia-book"`, Ref{Kind: "prompt", Slug: "what-is-your-favorite-dark-academia-book"}, true},
		{"<https://hardcover.app/@emma/lists/hugo-winners>", Ref{Kind: "list", Owner: "emma", Slug: "hugo-winners"}, true},
		{"\u200bhttps://hardcover.app/prompts/what-is-your-favorite-dark-academia-book\n", Ref{Kind: "prompt", Slug: "what-is-your-favorite-dark-academia-book"}, true},
		{"try https://hardcover.app/prompts/favorite-space-opera-books please", Ref{Kind: "prompt", Slug: "favorite-space-opera-books"}, true},
		{"https://hardcover.app/books/a-memory-called-empire", Ref{}, false},
		{"https://example.com/prompts/x-y", Ref{}, false},
		{"murderbot", Ref{}, false},
		{"dark academia", Ref{}, false},
	}
	for _, c := range cases {
		got, ok := ParseRef(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("ParseRef(%q) = %+v, %v; want %+v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestTopical(t *testing.T) {
	for q, want := range map[string]bool{
		"What is your favorite dark academia book?":                     true,
		"Favorite Space Opera books":                                    true,
		"What are your favorite books of all time?":                     true,
		"What are you most looking forward to reading in 2026?":         false,
		"Which new release are you most looking forward to reading?":    false,
		"What were your favorite childhood books?":                      false,
		"Need some shorter page turners to finish out my 52 book goal!": false,
		"Which books had the best film adaptations?":                    false,
	} {
		if got := (Prompt{Question: q}).Topical(); got != want {
			t.Errorf("Topical(%q) = %v, want %v", q, got, want)
		}
	}
	for name, personal := range map[string]bool{
		"On Deck": true, "Owned": true, "Kindle TBR": true, "My favorites of 2024": true, "2026 Releases": true,
		"Hugo Award for Best Novel (1953 to 2026)": false, "Sci-fi Written by Women": false,
		"NPR Top 50 Science Fiction and Fantasy Books of the 2010s": false,
	} {
		if got := personalListRe.MatchString(name); got != personal {
			t.Errorf("list %q skipped = %v, want %v", name, got, personal)
		}
	}
}

func TestCoPickedLift(t *testing.T) {
	// 100 readers gave 300 answers (3 each on average). Dune is everyone's
	// answer (half the readers gave it); Ancillary Justice is rarer.
	p := &Prompt{Users: 100, Books: 50, Votes: 300}
	votes := map[int]int{1: 50, 2: 5}
	dune, aj := Book{ID: 1, Title: "Dune"}, Book{ID: 2, Title: "Ancillary Justice"}
	// Four readers who gave the seed: two also gave Dune, two Ancillary
	// Justice; each gave three books besides the seed.
	other := func(id int) Book { return Book{ID: id, Title: "filler"} }
	answers := []answer{
		{1, dune}, {1, other(10)}, {1, other(11)},
		{2, dune}, {2, other(12)}, {2, other(13)},
		{3, aj}, {3, other(14)}, {3, other(15)},
		{4, aj}, {4, other(16)}, {4, other(17)},
	}
	got := map[int]float64{}
	for _, cp := range coPicked(p, answers, votes) {
		got[cp.ID] = cp.Weight
	}
	if got[2] <= got[1] {
		t.Errorf("Ancillary Justice (%.2f) should outrank Dune (%.2f): same co-picks, far less common", got[2], got[1])
	}
	// Expected Dune co-picks by chance: 12 answers × 0.5 share / 3 per
	// reader = 2, which is what was seen: no lift.
	if got[1] != 0 {
		t.Errorf("Dune got %.2f, want 0 (picked exactly as often as chance)", got[1])
	}
}
