package recommend

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"shelfcheck/internal/account"
	"shelfcheck/internal/hardcover"
)

// A seed is a book the patron liked, whose readers' other picks on
// Hardcover become candidates.
type seed struct {
	book  account.Book
	score float64
}

// pickSeeds chooses books to start from: liked (rated 3.5+, or unrated and
// read), favoring recent reads, authors they return to and series they keep
// reading. A series is represented by the earliest book read in it, which
// has the most readers (and so the most co-picks); one seed per author, so
// the co-picks range across their taste.
func (e *engine) pickSeeds() []seed {
	authorCount := map[string]int{}
	for _, a := range e.prof.Authors {
		authorCount[account.AuthorKey(a.Name)] = a.Count
	}
	seriesCount := map[string]int{}
	for _, st := range e.prof.Series {
		seriesCount[seriesKey(st.Name)] = st.Count
	}
	order := func(b account.Book) float64 {
		if o, err := strconv.ParseFloat(b.SeriesOrder, 64); err == nil && o > 0 {
			return o
		}
		return math.Inf(1)
	}
	groups := map[string]*seed{}
	var keys []string
	for _, b := range e.rec.Books {
		if b.Shelf != account.ShelfRead && b.Shelf != account.ShelfCurrent {
			continue
		}
		if b.Stars > 0 && b.Stars < 3.5 {
			continue
		}
		w, _ := recency(b.Date, e.opts.Now)
		if b.Shelf == account.ShelfCurrent {
			w = 1
		}
		if b.Stars > 0 {
			w += (b.Stars - 3) / 2 // 5 stars: +1
		}
		if len(b.Authors) > 0 {
			w += 0.25 * math.Min(float64(authorCount[account.AuthorKey(b.Authors[0])]-1), 3)
		}
		k := "b:" + b.Key()
		if sk := seriesKey(b.Series); sk != "" {
			k = "s:" + sk
			if seriesCount[sk] >= 2 {
				w += 0.3
			}
		}
		g := groups[k]
		if g == nil {
			groups[k] = &seed{b, w}
			keys = append(keys, k)
			continue
		}
		g.score = math.Max(g.score, w)
		if order(b) < order(g.book) {
			g.book = b
		}
	}
	all := make([]seed, 0, len(keys))
	for _, k := range keys {
		all = append(all, *groups[k])
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].score > all[j].score })
	var out []seed
	seenAuthor := map[string]bool{}
	for _, s := range all {
		a := ""
		if len(s.book.Authors) > 0 {
			a = account.AuthorKey(s.book.Authors[0])
		}
		if seenAuthor[a] {
			continue
		}
		seenAuthor[a] = true
		out = append(out, s)
	}
	return out
}

// coPick is a candidate gathered across seeds.
type coPick struct {
	hardcover.CoPick
	sum    float64  // weight summed over seeds
	best   float64  // strongest single seed's weight
	seeds  []string // titles of the seeds behind it
	reason string
	from   string // URL of the prompt or list behind reason
}

// alsoPicked looks up, for a few liked books, what Hardcover readers pick
// alongside them (in prompts like "favorite space opera" and on focused
// lists), then checks the strongest candidates against the catalog.
func (e *engine) alsoPicked(ctx context.Context) error {
	hc := e.opts.Hardcover
	if hc == nil {
		return nil
	}
	cands := map[int]*coPick{}
	tried, used := 0, 0
	for _, s := range e.pickSeeds() {
		if used >= e.opts.MaxSeeds || tried >= 2*e.opts.MaxSeeds {
			break
		}
		tried++
		title, author := s.book.Title, ""
		if len(s.book.Authors) > 0 {
			author = s.book.Authors[0]
		}
		id := s.book.HardcoverID
		if id == 0 {
			hb, ok, err := hc.FindBook(ctx, title, author)
			if err != nil {
				return err
			}
			// Too few readers to say what they also like.
			if !ok || hb.Readers < 50 {
				e.opts.Debug("seed %q: not on Hardcover or little read", title)
				continue
			}
			id = hb.ID
		}
		cps, err := hc.AlsoPicked(ctx, id)
		if err != nil {
			return err
		}
		cps = e.usable(cps)
		if len(cps) == 0 {
			e.opts.Debug("seed %q: no usable prompts or lists", title)
			continue
		}
		used++
		short := shortTitle(title)
		e.seeds = append(e.seeds, short)
		// Normalize so each seed's strongest co-pick counts at most 1: a
		// book with many prompts shouldn't drown out the others.
		norm := math.Max(cps[0].Weight, 1)
		e.opts.Debug("seed %q (Hardcover %d): %d co-picks, top %q %.2f", title, id, len(cps), cps[0].Title, cps[0].Weight)
		for _, cp := range cps {
			w := cp.Weight / norm
			if w < 0.05 {
				break // sorted; the rest is noise
			}
			c := cands[cp.ID]
			if c == nil {
				c = &coPick{CoPick: cp}
				cands[cp.ID] = c
			}
			// The most-read books turn up beside everything (and the
			// patron has surely heard of Dune), so lean toward discoveries.
			w *= math.Min(1, math.Pow(3000/math.Max(float64(cp.Readers), 1), 0.75))
			c.sum += w
			c.seeds = append(c.seeds, short)
			if w > c.best {
				c.best = w
				c.reason, c.from = sourceReason(cp, short), sourceURL(cp)
			}
		}
	}

	// A book beside several seeds is a better bet, but summing outright
	// would favor whatever turns up beside everything: count the strongest
	// seed and a little of the rest.
	weight := func(c *coPick) float64 { return c.best + 0.3*(c.sum-c.best) }
	var ranked []*coPick
	for _, c := range cands {
		ranked = append(ranked, c)
	}
	sort.Slice(ranked, func(i, j int) bool {
		if wi, wj := weight(ranked[i]), weight(ranked[j]); wi != wj {
			return wi > wj
		}
		return ranked[i].Readers > ranked[j].Readers
	})
	for i, c := range ranked {
		if i >= e.opts.MaxPicked {
			break
		}
		reason := c.reason
		if len(c.seeds) > 1 {
			reason = "readers who liked " + joinTitles(c.seeds) + " also picked it"
		}
		score := 1.5 + 2.5*math.Min(weight(c), 1)
		e.opts.Debug("  co-pick %.2f %q by %s (%d readers): %s", weight(c), c.Title, c.Author, c.Readers, reason)
		if p := e.picks[account.WorkKey(c.Title, c.Author)]; p != nil {
			p.note("picked", score, reason) // found already; no need to search again
			continue
		}
		found, err := e.findTitle(ctx, c.Title, c.Author, "picked", score, reason)
		if err != nil {
			return err
		}
		for _, p := range found {
			if p.From == "" {
				p.From = c.from
			}
		}
		e.picked += len(found)
	}
	return nil
}

// usable drops co-picks that aren't candidates (see candidate), keeping the
// order.
func (e *engine) usable(cps []hardcover.CoPick) []hardcover.CoPick {
	var out []hardcover.CoPick
	for _, c := range cps {
		if e.candidate(c.Book) {
			out = append(out, c)
		}
	}
	return out
}

// candidate reports whether a Hardcover book could be suggested: not read,
// had or on hold; not a later book in a series (the series stage handles
// ones the patron reads, and book 4 of one they haven't started is no
// pick); and named.
func (e *engine) candidate(b hardcover.Book) bool {
	if _, has := e.rec.Has(b.Title, b.Author); has {
		return false
	}
	if e.ahead[account.WorkKey(b.Title, b.Author)] || placeholderTitle(b.Title) {
		return false
	}
	return b.Series == "" || (b.SeriesPos <= 1 && !e.readsSeriesName(b.Series))
}

// sourceReason explains a single seed's co-pick.
func sourceReason(cp hardcover.CoPick, seedTitle string) string {
	switch {
	case cp.Prompt != nil:
		return fmt.Sprintf("readers who picked %s also picked it · %q", seedTitle, cp.Prompt.Question)
	case cp.List != nil:
		return fmt.Sprintf("on a list with %s · %q", seedTitle, cp.List.Name)
	}
	return "readers of " + seedTitle + " also picked it"
}

func sourceURL(cp hardcover.CoPick) string {
	switch {
	case cp.Prompt != nil:
		return cp.Prompt.URL()
	case cp.List != nil:
		return cp.List.URL()
	}
	return ""
}

// joinTitles lists up to three titles: "A, B and C".
func joinTitles(ts []string) string {
	if len(ts) > 3 {
		ts = ts[:3]
	}
	if len(ts) == 1 {
		return ts[0]
	}
	return strings.Join(ts[:len(ts)-1], ", ") + " and " + ts[len(ts)-1]
}

func (e *engine) readsSeriesName(name string) bool {
	k := seriesKey(name)
	for _, st := range e.prof.Series {
		if seriesKey(st.Name) == k {
			return true
		}
	}
	return false
}
