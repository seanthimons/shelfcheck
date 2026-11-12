package recommend

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"shelfcheck/internal/account"
	bc "shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/hardcover"
)

// New arrivals: books on a branch's physical "new" shelves now, ranked by
// how well they fit the patron. The catalog has no shelf facet, so recently
// acquired titles on the shelf at home are checked one by one for a copy
// shelved in a new-books section.

// arrivalsQuery orders candidates by acquisition. ponytail: it only looks
// at the last 180 days of acquisitions, so a book that has sat in a new
// section longer than that is missed; widen it if that turns out to matter.
const arrivalsQuery = "nw:[0 TO 180]"

// NewShelf reports whether a copy's collection is a browsable new-books
// section ("Main - 1st Floor South - New Arrivals", "... - New Non
// Fiction"), not one staff are still processing.
func NewShelf(collection string) bool {
	c := strings.ToLower(collection)
	return !strings.Contains(c, "technical services") && slices.Contains(strings.Fields(c), "new")
}

type ArrivalOptions struct {
	Home     string // branch code
	Audience string // catalog audience, e.g. "adult"; "" for any
	Language string // catalog language code; "" for any
	Scan     int    // recently acquired titles to check; default 75
	Limit    int    // arrivals to return; default 30

	// Hardcover, if set, adds its picks for the patron and what its
	// readers tag each book.
	Hardcover *hardcover.Client

	Now time.Time // for recency weighting; default time.Now()
}

// Arrival is one title with a copy available on a new shelf at home.
type Arrival struct {
	Bib        bc.Bib
	Shelf      string // the copy's collection, as the library words it
	CallNumber string
	Score      float64 // fit with the reading record; 0 without one
	Reasons    []string

	HardcoverID  int // 0 if it wasn't looked up or found
	HardcoverURL string
}

// ArrivalsResult is what a bounded scan found. It isn't a shelf inventory:
// see More and Failed.
type ArrivalsResult struct {
	Items   []Arrival
	Checked int      // titles whose copies were checked
	Failed  int      // titles whose copies couldn't be fetched: unknown, not absent
	Known   int      // arrivals left out as already read, borrowed or on hold
	More    bool     // recently acquired titles remain beyond the scan
	Notes   []string // what couldn't be scored, and why
}

// Partial reports whether new-shelf books may be missing from Items.
func (r ArrivalsResult) Partial() bool { return r.More || r.Failed > 0 }

// Arrivals scans recently acquired books on the shelf at home for copies in
// a new-books section, then ranks them against rec (which may be nil: the
// arrivals then come back in acquisition order). progress, if set, gets
// the result so far.
func Arrivals(ctx context.Context, client *bc.Client, rec *account.Record, opts ArrivalOptions, progress func(ArrivalsResult)) (ArrivalsResult, error) {
	if opts.Scan <= 0 {
		opts.Scan = 75
	}
	if opts.Limit <= 0 {
		opts.Limit = 30
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	var res ArrivalsResult
	report := func() {
		if progress != nil {
			snap := res
			snap.Items = slices.Clone(res.Items)
			progress(snap)
		}
	}

	q := bc.Query{Text: arrivalsQuery, Type: bc.Boolean, Sort: bc.SortNewlyAcquired}
	q.Toggle(bc.FacetStatus, opts.Home)
	q.Toggle(bc.FacetFormat, "BK")
	if opts.Audience != "" {
		q.Toggle(bc.FacetAudience, opts.Audience)
	}
	if opts.Language != "" {
		q.Toggle(bc.FacetLanguage, opts.Language)
	}
	seen := map[string]bool{}
scan:
	for page := 1; ; page++ {
		sr, err := client.Search(ctx, q, page)
		if err != nil {
			if ctx.Err() != nil || len(res.Items) == 0 {
				return res, err
			}
			res.More = true
			res.Notes = append(res.Notes, "stopped scanning early: "+err.Error())
			break
		}
		for _, b := range sr.Bibs {
			if seen[b.ID] {
				continue
			}
			seen[b.ID] = true
			if res.Checked+res.Failed >= opts.Scan {
				res.More = true
				break scan
			}
			av, err := client.Availability(ctx, b.ID)
			if err != nil {
				if ctx.Err() != nil {
					return res, ctx.Err()
				}
				res.Failed++
				continue
			}
			res.Checked++
			// One copy has to be at home, in, and on a new shelf: a copy in
			// at another branch says nothing about the one shelved here.
			for _, c := range av.Copies {
				if c.BranchCode == opts.Home && c.Available && NewShelf(c.Collection) {
					res.Items = append(res.Items, Arrival{Bib: b, Shelf: c.Collection, CallNumber: c.CallNumber})
					break
				}
			}
			report()
		}
		if page >= sr.Pages || len(sr.Bibs) == 0 {
			break
		}
	}

	var e *engine
	if rec != nil {
		e = &engine{prof: BuildProfile(rec, opts.Now)}
		e.scoreArrivals(&res, rec)
	}
	hcPicks := arrivalVibes(ctx, opts.Hardcover, &res)
	for i := range res.Items {
		a := &res.Items[i]
		if p, ok := hcPicks[account.WorkKey(a.Bib.Title, firstAuthor(a.Bib))]; ok {
			a.Score += p.score
			a.Reasons = append([]string{p.reason}, a.Reasons...)
			a.HardcoverID, a.HardcoverURL = p.book.ID, p.book.URL()
		}
	}
	rank := func() {
		sort.SliceStable(res.Items, func(i, j int) bool { return res.Items[i].Score > res.Items[j].Score })
	}
	rank()
	if len(res.Items) > opts.Limit {
		res.Items = res.Items[:opts.Limit]
	}
	report()
	if e != nil && opts.Hardcover != nil {
		e.hardcoverTags(ctx, opts.Hardcover, &res)
		rank()
	}
	return res, nil
}

func firstAuthor(b bc.Bib) string {
	if len(b.Authors) == 0 {
		return ""
	}
	return b.Authors[0]
}

// scoreArrivals drops arrivals the patron has read, has or is waiting on,
// and scores the rest: taste fit, authors and series they read, and their
// want-to-read list.
func (e *engine) scoreArrivals(res *ArrivalsResult, rec *account.Record) {
	want := map[string]bool{}
	for _, b := range rec.Books {
		if b.Shelf == account.ShelfWant {
			want[b.Key()] = true
		}
	}
	kept := res.Items[:0]
	for _, a := range res.Items {
		author := firstAuthor(a.Bib)
		if _, has := rec.Has(a.Bib.Title, author); has {
			res.Known++
			continue
		}
		a.Score = e.fit(a.Bib)
		if want[account.WorkKey(a.Bib.Title, author)] {
			a.Score += 4
			a.Reasons = append(a.Reasons, "on your want-to-read list")
		}
		if e.readsSeries(a.Bib) {
			a.Score += 3
			a.Reasons = append(a.Reasons, "in a series you read")
		}
		for _, at := range e.prof.Authors {
			if account.AuthorKey(at.Name) == account.AuthorKey(author) {
				a.Score += min(1+at.Weight, 3)
				a.Reasons = append(a.Reasons, fmt.Sprintf("you've read %d by %s", at.Count, displayName(at.Name)))
				break
			}
		}
		if names, _ := e.tastes(append(append([]string{}, a.Bib.Genres...), a.Bib.Subjects...)); len(names) > 0 {
			a.Reasons = append(a.Reasons, "your genres: "+strings.Join(names, ", "))
		}
		kept = append(kept, a)
	}
	res.Items = kept
}

// genreKey compares genre names across the catalog ("Fantasy fiction") and
// Hardcover ("Fantasy").
func genreKey(g string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.ToLower(strings.TrimSpace(g)), "fiction"))
}

// tastes finds the patron's genres among terms: up to three names,
// strongest first, and their share of the profile's genre weight.
func (e *engine) tastes(terms []string) (names []string, share float64) {
	has := map[string]bool{}
	for _, t := range terms {
		has[genreKey(t)] = true
	}
	total, overlap := 0.0, 0.0
	for _, g := range e.prof.Genres {
		total += g.Weight
		if k := genreKey(g.Name); k != "" && has[k] {
			overlap += g.Weight
			if len(names) < 3 {
				names = append(names, g.Name)
			}
		}
	}
	if total == 0 {
		return nil, 0
	}
	return names, overlap / total
}

type hcPick struct {
	book   hardcover.Book
	score  float64
	reason string
}

// arrivalVibes loads Hardcover's picks for the patron, by work. Arrivals
// are matched against them by title and author, which costs no lookups.
func arrivalVibes(ctx context.Context, hc *hardcover.Client, res *ArrivalsResult) map[string]hcPick {
	if hc == nil || len(res.Items) == 0 {
		return nil
	}
	user, err := hc.Username(ctx)
	if err != nil {
		res.Notes = append(res.Notes, "Hardcover's picks for you weren't checked: "+err.Error())
		return nil
	}
	picks := map[string]hcPick{}
	for _, l := range vibeLists {
		v, _, err := hc.VibeBySlug(ctx, user, l.slug, 100)
		if err != nil {
			res.Notes = append(res.Notes, "Hardcover's picks for you weren't all checked: "+err.Error())
			continue
		}
		for rank, b := range v.Books {
			k := account.WorkKey(b.Title, b.Author)
			if _, dup := picks[k]; !dup {
				picks[k] = hcPick{b, l.base + 1.5*(1-float64(rank)/float64(len(v.Books))), fmt.Sprintf(l.reason, rank+1)}
			}
		}
	}
	return picks
}

// hardcoverTags looks each arrival up on Hardcover and scores what its
// readers say: their rating, and genres and moods the patron reads. A book
// that can't be looked up keeps its score; the failures are noted.
// ponytail: one or two throttled requests per uncached book (a second
// each), so only the arrivals already cut to Limit are looked up.
func (e *engine) hardcoverTags(ctx context.Context, hc *hardcover.Client, res *ArrivalsResult) {
	failed := 0
	for i := range res.Items {
		a := &res.Items[i]
		info, ok, err := hc.Lookup(ctx, a.Bib.Title, firstAuthor(a.Bib))
		if ctx.Err() != nil {
			res.Notes = append(res.Notes, fmt.Sprintf("ran out of time looking books up on Hardcover: %d of %d weren't checked", len(res.Items)-i, len(res.Items)))
			return
		}
		if err != nil {
			failed++
			continue
		}
		if !ok {
			continue
		}
		a.HardcoverID, a.HardcoverURL = info.ID, info.URL()
		if info.Ratings >= 20 && info.Rating > 0 {
			a.Score += info.Rating - 3.75
			if info.Rating >= 4 {
				a.Reasons = append(a.Reasons, fmt.Sprintf("%.1f★ on Hardcover", info.Rating))
			}
		}
		if names, share := e.tastes(append(append([]string{}, info.Genres...), info.Moods...)); len(names) > 0 {
			a.Score += 2 * share
			a.Reasons = append(a.Reasons, "Hardcover readers tag it "+strings.Join(names, ", "))
		}
	}
	if failed > 0 {
		res.Notes = append(res.Notes, fmt.Sprintf("%d books couldn't be looked up on Hardcover", failed))
	}
}
