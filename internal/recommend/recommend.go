// Package recommend suggests books a patron can get right now (print on the
// shelf at their home branch, or an ebook to borrow) from their reading
// record: the next book in series they're reading, more by authors they
// return to, wishlist titles, books Hardcover readers pair with ones they
// liked, and (without Hardcover) well-rated books in their genres.
package recommend

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"shelfcheck/internal/account"
	bc "shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/hardcover"
)

type Options struct {
	Home     string // home branch code
	HomeName string
	Language string // catalog language code; "" for any
	Limit    int    // picks to return; default 25

	MaxSeries  int // active series to look up; default 8
	MaxAuthors int // authors to look up; default 8
	MaxGenres  int // genres to search; default 3
	MaxSeeds   int // liked books to find readers' co-picks for; default 6
	MaxPicked  int // co-picked books to look for in the catalog; default 20

	// Wishlist, if non-nil, holds already-checked wishlist titles (from
	// account.CheckWish); available ones become candidates.
	Wishlist []account.WishItem

	// Hardcover, if set, supplies series order, which library records
	// often lack or get wrong, and what readers pick alongside books the
	// patron liked (prompts and lists), which replaces the genre picks.
	Hardcover *hardcover.Client

	// Debug, if set, receives a line per decision.
	Debug func(format string, args ...any)

	Now time.Time // for recency weighting; default time.Now()
}

func (o Options) withDefaults() Options {
	if o.Limit == 0 {
		o.Limit = 25
	}
	if o.MaxSeries == 0 {
		o.MaxSeries = 8
	}
	if o.MaxAuthors == 0 {
		o.MaxAuthors = 8
	}
	if o.MaxGenres == 0 {
		o.MaxGenres = 3
	}
	if o.MaxSeeds == 0 {
		o.MaxSeeds = 6
	}
	if o.MaxPicked == 0 {
		o.MaxPicked = 20
	}
	if o.HomeName == "" {
		o.HomeName = "branch " + o.Home
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	if o.Debug == nil {
		o.Debug = func(string, ...any) {}
	}
	return o
}

// Pick is one recommended work, with its available copies merged.
type Pick struct {
	Bib     bc.Bib // the print record when one is on the shelf, else the ebook
	Print   bool   // a print copy is on the shelf at home
	Ebook   bool   // an ebook can be borrowed now
	Score   float64
	Reasons []string
	// From is the Hardcover prompt or list behind a "readers also picked"
	// reason, as a URL.
	From string
	key  string
	why  map[string]reason // strongest reason of each kind
}

type reason struct {
	score float64
	text  string
	order int // first-added order, to keep reasons stable
}

// Profile is what the reading record says about the patron's taste.
type Profile struct {
	// UpNext lists the next book in active series when it isn't available
	// now (checked out, not owned, or not out yet).
	UpNext []UpNext
	// Seeds are the liked books whose readers' co-picks were looked up.
	Seeds   []string
	Series  []SeriesTaste
	Authors []AuthorTaste
	Genres  []GenreTaste
	Read    int
}

type UpNext struct {
	Series   string
	Position float64
	Title    string
	Author   string
}

type SeriesTaste struct {
	Name     string  // as written in the record, e.g. "The Murderbot Diaries"
	MaxOrder float64 // highest book read
	Count    int
	Last     time.Time
	Author   string
	Weight   float64
}

type AuthorTaste struct {
	Name   string // "Last, First"
	Count  int
	Weight float64
}

type GenreTaste struct {
	Name   string
	Weight float64
}

// Too broad to say anything about taste.
var genericGenres = map[string]bool{
	"fiction": true, "large print books": true, "audiobooks": true, "novels": true,
	"literature": true, "general": true, "electronic books": true, "juvenile fiction": true,
}

// recency weights a reading date: 1 now, halving every two years.
func recency(date string, now time.Time) (float64, time.Time) {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return 0.5, time.Time{}
	}
	years := now.Sub(t).Hours() / (24 * 365)
	return math.Pow(0.5, math.Max(years, 0)/2), t
}

// BuildProfile summarizes the record's read and current books.
func BuildProfile(rec *account.Record, now time.Time) Profile {
	var p Profile
	series := map[string]*SeriesTaste{}
	authors := map[string]*AuthorTaste{}
	genres := map[string]*GenreTaste{}
	for _, b := range rec.Books {
		if b.Shelf != account.ShelfRead && b.Shelf != account.ShelfCurrent {
			continue
		}
		p.Read++
		if b.Stars > 0 && b.Stars < 2.5 {
			continue // read it, didn't like it
		}
		w, when := recency(b.Date, now)
		if b.Shelf == account.ShelfCurrent {
			w, when = 1, now
		}
		if b.Stars >= 4 {
			w *= 1.5
		}
		if len(b.Authors) > 0 {
			k := account.AuthorKey(b.Authors[0])
			a := authors[k]
			if a == nil {
				a = &AuthorTaste{Name: b.Authors[0]}
				authors[k] = a
			}
			if strings.Contains(b.Authors[0], ",") && !strings.Contains(a.Name, ",") {
				a.Name = b.Authors[0] // prefer "Last, First" for catalog author search
			}
			a.Count++
			a.Weight += w
		}
		if b.Series != "" {
			k := seriesKey(b.Series)
			s := series[k]
			if s == nil {
				s = &SeriesTaste{Name: b.Series}
				if len(b.Authors) > 0 {
					s.Author = b.Authors[0]
				}
				series[k] = s
			}
			if o, err := strconv.ParseFloat(b.SeriesOrder, 64); err == nil && o > s.MaxOrder {
				s.MaxOrder = o
			}
			s.Count++
			s.Weight += w
			if when.After(s.Last) {
				s.Last = when
			}
		}
		terms := append(append([]string{}, b.Genres...), b.Subjects...)
		seen := map[string]bool{}
		for _, g := range terms {
			k := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(g), "Fiction")))
			k = strings.TrimSpace(k)
			if k == "" || genericGenres[strings.ToLower(g)] || genericGenres[k] || seen[k] {
				continue
			}
			seen[k] = true
			gt := genres[k]
			if gt == nil {
				gt = &GenreTaste{Name: g}
				genres[k] = gt
			}
			gt.Weight += w
		}
	}
	for _, s := range series {
		p.Series = append(p.Series, *s)
	}
	for _, a := range authors {
		p.Authors = append(p.Authors, *a)
	}
	for _, g := range genres {
		p.Genres = append(p.Genres, *g)
	}
	sort.Slice(p.Series, func(i, j int) bool { return p.Series[i].Weight > p.Series[j].Weight })
	sort.Slice(p.Authors, func(i, j int) bool { return p.Authors[i].Weight > p.Authors[j].Weight })
	sort.Slice(p.Genres, func(i, j int) bool { return p.Genres[i].Weight > p.Genres[j].Weight })
	return p
}

// seriesKey normalizes series names across sources ("The Murderbot
// Diaries" vs "Murderbot diaries").
func seriesKey(s string) string {
	k := strings.ToLower(s)
	k = strings.TrimPrefix(k, "the ")
	for _, suf := range []string{" series", " trilogy", " saga", " sequence", " cycle"} {
		k = strings.TrimSuffix(k, suf)
	}
	return strings.Join(strings.FieldsFunc(k, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127)
	}), " ")
}

// Progress reports a stage and the picks so far.
type Progress func(stage string, picks []Pick)

// Recommend builds the profile, gathers available candidates and ranks them.
func Recommend(ctx context.Context, client *bc.Client, rec *account.Record, opts Options, progress Progress) ([]Pick, Profile, error) {
	opts = opts.withDefaults()
	prof := BuildProfile(rec, opts.Now)
	e := &engine{client: client, rec: rec, prof: prof, opts: opts, picks: map[string]*Pick{}, ahead: map[string]bool{}}
	if progress == nil {
		progress = func(string, []Pick) {}
	}

	stages := []struct {
		name string
		run  func(context.Context) error
	}{
		{"wishlist", e.wishlist},
		{"series you're reading", e.series},
		{"authors you come back to", e.authors},
		{"what readers also picked", e.alsoPicked},
		{"your genres", e.genres},
	}
	var errs []string
	for _, st := range stages {
		progress("checking "+st.name, e.ranked())
		if err := st.run(ctx); err != nil {
			if ctx.Err() != nil {
				return e.ranked(), prof, ctx.Err()
			}
			errs = append(errs, st.name+": "+err.Error())
			e.opts.Debug("%s failed: %v", st.name, err)
		}
	}
	picks := e.ranked()
	prof.UpNext, prof.Seeds = e.upNext, e.seeds
	progress("done", picks)
	if len(errs) > 0 && len(picks) == 0 {
		return nil, prof, fmt.Errorf("recommend: %s", strings.Join(errs, "; "))
	}
	return picks, prof, nil
}

type engine struct {
	client *bc.Client
	rec    *account.Record
	prof   Profile
	opts   Options
	picks  map[string]*Pick
	upNext []UpNext
	seeds  []string
	picked int // co-picks found in the catalog
	// ahead holds works further along a series than the patron's next
	// book (from Hardcover), which other stages shouldn't suggest.
	ahead map[string]bool
}

// availableQuery narrows a search to copies the patron can get now.
func (e *engine) availableQuery(text string, typ bc.SearchType) bc.Query {
	q := bc.Query{Text: text, Type: typ}
	q.Toggle(bc.FacetStatus, e.opts.Home)
	q.Toggle(bc.FacetStatus, "_online_")
	q.Toggle(bc.FacetFormat, "BK")
	q.Toggle(bc.FacetFormat, "EBOOK")
	if e.opts.Language != "" {
		q.Toggle(bc.FacetLanguage, e.opts.Language)
	}
	return q
}

// add merges a candidate copy into the picks unless it's already read. kind
// groups reasons ("series", "author", "wish", "genre"): a pick keeps only the
// strongest reason of each kind, so two records of one book don't double up.
func (e *engine) add(b bc.Bib, kind string, score float64, text string) *Pick {
	author := ""
	if len(b.Authors) > 0 {
		author = b.Authors[0]
	}
	if _, has := e.rec.Has(b.Title, author); has {
		return nil
	}
	k := account.WorkKey(b.Title, author)
	if e.ahead[k] && kind != "wish" {
		return nil // too far along a series they're reading
	}
	p := e.picks[k]
	if p == nil {
		p = &Pick{Bib: b, key: k, why: map[string]reason{}}
		e.picks[k] = p
	}
	digital := b.Digital || account.DigitalFormats[b.Format]
	if digital {
		p.Ebook = true
	} else {
		if !p.Print {
			p.Bib = b // prefer the print record
		}
		p.Print = true
	}
	if old, ok := p.why[kind]; !ok || score > old.score {
		order := len(p.why)
		if ok {
			order = old.order
		}
		p.why[kind] = reason{score, text, order}
	}
	return p
}

func (e *engine) wishlist(ctx context.Context) error {
	for _, w := range e.opts.Wishlist {
		for _, m := range w.Matches {
			e.add(m.Bib, "wish", 3, "on your want-to-read list")
		}
	}
	return nil
}

func (e *engine) series(ctx context.Context) error {
	if e.opts.Hardcover != nil {
		return e.hardcoverSeries(ctx)
	}
	n := 0
	for _, s := range e.prof.Series {
		if n >= e.opts.MaxSeries {
			break
		}
		if e.opts.Now.Sub(s.Last) > 4*365*24*time.Hour {
			continue // not active
		}
		n++
		res, err := e.client.Search(ctx, e.availableQuery(s.Name, bc.Series), 1)
		if err != nil {
			return err
		}
		want := seriesKey(s.Name)
		for _, b := range res.Bibs {
			order, inSeries := bibSeriesOrder(b, want)
			if !inSeries {
				continue
			}
			read := fmt.Sprintf("you've read %d", s.Count)
			if s.MaxOrder > 0 {
				read = "you've read up to #" + trimNum(s.MaxOrder)
			}
			whole := order == math.Trunc(order)
			next := math.Floor(s.MaxOrder) + 1
			switch {
			case order == 0:
				// Some records (often ebooks) don't number the series.
				e.add(b, "series", 1.5, fmt.Sprintf("in %s · %s", s.Name, read))
			case !whole:
				e.add(b, "series", 1, fmt.Sprintf("novella #%s in %s", trimNum(order), s.Name))
			case order < next:
				// An earlier book they skipped.
				e.add(b, "series", 2.5, fmt.Sprintf("#%s in %s · you skipped it", trimNum(order), s.Name))
			case order == next:
				e.add(b, "series", 7, fmt.Sprintf("next in %s · %s", s.Name, read))
			default:
				e.add(b, "series", 0.5, fmt.Sprintf("#%s in %s · you're on #%s", trimNum(order), s.Name, trimNum(next)))
			}
		}
	}
	return nil
}

// hardcoverSeries uses Hardcover's series order to find the next book the
// patron hasn't read in each active series (and one they skipped), then
// checks whether the library has it available now.
func (e *engine) hardcoverSeries(ctx context.Context) error {
	n := 0
	for _, s := range e.prof.Series {
		if n >= e.opts.MaxSeries {
			break
		}
		if e.opts.Now.Sub(s.Last) > 4*365*24*time.Hour {
			continue
		}
		n++
		hs, ok, err := e.opts.Hardcover.FindSeries(ctx, s.Name, s.Author)
		if err != nil {
			return err
		}
		if !ok {
			e.opts.Debug("series %q: not found on Hardcover", s.Name)
			continue
		}
		e.opts.Debug("series %q -> Hardcover %q (%d books)", s.Name, hs.Name, len(hs.Books))
		// Furthest book read, by Hardcover's numbering.
		maxRead := s.MaxOrder
		for _, b := range hs.Books {
			if _, has := e.rec.Has(b.Title, b.Author); has && b.Position > maxRead {
				maxRead = b.Position
			}
		}
		if maxRead == 0 {
			continue
		}
		read := "you've read up to #" + trimNum(maxRead)
		if next, ok := hs.Next(maxRead); ok {
			for _, b := range hs.Books {
				if b.Position > next.Position {
					e.ahead[account.WorkKey(b.Title, b.Author)] = true
				}
			}
		}
		var targets []struct {
			b      hardcover.SeriesBook
			score  float64
			reason string
		}
		skipped := false
		for _, b := range hs.Books {
			if b.Position != math.Trunc(b.Position) || placeholderTitle(b.Title) {
				continue // novellas, unannounced books
			}
			if _, has := e.rec.Has(b.Title, b.Author); has {
				continue
			}
			if b.Position < maxRead && !skipped {
				skipped = true
				targets = append(targets, struct {
					b      hardcover.SeriesBook
					score  float64
					reason string
				}{b, 2.5, fmt.Sprintf("#%s in %s · you skipped it", trimNum(b.Position), hs.Name)})
			}
			if b.Position > maxRead {
				targets = append(targets, struct {
					b      hardcover.SeriesBook
					score  float64
					reason string
				}{b, 7, fmt.Sprintf("next in %s (#%s) · %s", hs.Name, trimNum(b.Position), read)})
				break
			}
		}
		for _, t := range targets {
			e.opts.Debug("  read up to #%v; looking for #%v %q by %s", maxRead, t.b.Position, t.b.Title, t.b.Author)
			found, err := e.findTitle(ctx, t.b.Title, t.b.Author, "series", t.score, t.reason)
			if err != nil {
				return err
			}
			if len(found) == 0 && t.b.Position > maxRead {
				e.upNext = append(e.upNext, UpNext{Series: hs.Name, Position: t.b.Position, Title: t.b.Title, Author: t.b.Author})
			}
		}
	}
	return nil
}

// findTitle searches the catalog for a specific book, available now, and adds
// matching copies, returning the picks they went to.
func (e *engine) findTitle(ctx context.Context, title, author, kind string, score float64, reason string) ([]*Pick, error) {
	q := e.availableQuery(shortTitle(title), bc.Title)
	res, err := e.client.Search(ctx, q, 1)
	if err != nil {
		return nil, err
	}
	want := account.AuthorKey(author)
	var found []*Pick
	for _, b := range res.Bibs {
		if len(b.Authors) == 0 || account.AuthorKey(b.Authors[0]) != want {
			continue
		}
		if account.WorkKey(b.Title, b.Authors[0]) != account.WorkKey(title, author) && !account.SameTitle(b.Title, title) {
			continue
		}
		if subtitlesClash(b.Subtitle, title) {
			continue // "Mistborn: Secret History" isn't "Mistborn: The Final Empire"
		}
		if p := e.add(b, kind, score, reason); p != nil {
			found = append(found, p)
		}
	}
	e.opts.Debug("    catalog: %d results, %d matched", len(res.Bibs), len(found))
	return found, nil
}

// subtitlesClash reports whether a catalog subtitle and the subtitle in
// title (after a colon) name different books. Either missing, or a generic
// one ("A Novel"), is no clash.
func subtitlesClash(catalogSub, title string) bool {
	_, wantSub, ok := strings.Cut(title, ":")
	norm := func(s string) string {
		s = strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127)
		}), " ")
		if s == "a novel" || strings.HasPrefix(s, "a novel ") || strings.HasSuffix(s, " novel") {
			return ""
		}
		return s
	}
	c, w := norm(catalogSub), norm(wantSub)
	if !ok || c == "" || w == "" {
		return false
	}
	return !strings.HasPrefix(c, w) && !strings.HasPrefix(w, c)
}

// shortTitle drops what trips up a catalog title search: a subtitle
// ("Babel, or The Necessity of Violence: An Arcane History...") or a
// parenthetical series note.
func shortTitle(t string) string {
	if i := strings.IndexAny(t, ":("); i > 3 {
		t = t[:i]
	}
	if i := strings.Index(strings.ToLower(t), ", or "); i > 3 {
		t = t[:i]
	}
	return strings.TrimSpace(t)
}

// placeholderTitle spots announced-but-unnamed books ("Untitled", "TBA").
func placeholderTitle(t string) bool {
	t = strings.ToLower(strings.TrimSpace(t))
	return t == "" || strings.HasPrefix(t, "untitled") || t == "tba" || t == "tbd" || strings.Contains(t, "untitled book")
}

// bibSeriesOrder finds a catalog record's position in the series with key
// want. inSeries is false if it's not in that series at all.
func bibSeriesOrder(b bc.Bib, want string) (order float64, inSeries bool) {
	for _, s := range b.Series {
		name, num := account.SplitSeries(s)
		if seriesKey(name) != want && !strings.Contains(seriesKey(name), want) && !strings.Contains(want, seriesKey(name)) {
			continue
		}
		o, _ := strconv.ParseFloat(num, 64)
		return o, true
	}
	return 0, false
}

func trimNum(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func (e *engine) authors(ctx context.Context) error {
	for i, a := range e.prof.Authors {
		if i >= e.opts.MaxAuthors || a.Count < 2 {
			break // one book isn't a pattern
		}
		q := e.availableQuery(a.Name, bc.Author)
		q.Sort = bc.SortRelevance
		res, err := e.client.Search(ctx, q, 1)
		if err != nil {
			return err
		}
		want := account.AuthorKey(a.Name)
		reason := fmt.Sprintf("by %s · you've read %d of theirs", displayName(a.Name), a.Count)
		for _, b := range res.Bibs {
			if len(b.Authors) == 0 || account.AuthorKey(b.Authors[0]) != want {
				continue
			}
			// Don't push book 5 of a series they haven't started.
			if order := firstSeriesOrder(b); order > 1 && !e.readsSeries(b) {
				continue
			}
			e.add(b, "author", 1+math.Min(a.Weight, 3), reason)
		}
	}
	return nil
}

func firstSeriesOrder(b bc.Bib) float64 {
	if len(b.Series) == 0 {
		return 0
	}
	_, num := account.SplitSeries(b.Series[0])
	o, _ := strconv.ParseFloat(num, 64)
	return o
}

func (e *engine) readsSeries(b bc.Bib) bool {
	for _, s := range b.Series {
		name, _ := account.SplitSeries(s)
		k := seriesKey(name)
		for _, st := range e.prof.Series {
			if seriesKey(st.Name) == k {
				return true
			}
		}
	}
	return false
}

// genres runs when Hardcover can't say what readers pair with the patron's
// books: top-rated catalog titles in their genres are a weak stand-in.
func (e *engine) genres(ctx context.Context) error {
	if e.picked > 0 {
		return nil
	}
	n := 0
	for _, g := range e.prof.Genres {
		if n >= e.opts.MaxGenres {
			break
		}
		n++
		q := e.availableQuery(g.Name, bc.Genre)
		q.Toggle(bc.FacetAudience, "adult") // top-rated genre lists skew young otherwise
		q.Sort = bc.SortTopRated
		res, err := e.client.Search(ctx, q, 1)
		if err != nil {
			return err
		}
		for i, b := range res.Bibs {
			if i >= 30 {
				break
			}
			if b.Rating.Count < 10 {
				continue
			}
			if order := firstSeriesOrder(b); order > 1 && !e.readsSeries(b) {
				continue
			}
			e.add(b, "genre", 0.5, "top rated in "+strings.ToLower(g.Name))
		}
	}
	return nil
}

// perAuthor caps picks by one author so a long series can't crowd out
// everything else.
const perAuthor = 2

// ranked scores every pick and returns the best, highest first.
func (e *engine) ranked() []Pick {
	var all []Pick
	for _, p := range e.picks {
		pp := *p
		rs := make([]reason, 0, len(p.why))
		for _, r := range p.why {
			pp.Score += r.score
			rs = append(rs, r)
		}
		sort.Slice(rs, func(i, j int) bool { return rs[i].score > rs[j].score })
		pp.Reasons = nil
		for _, r := range rs {
			pp.Reasons = append(pp.Reasons, r.text)
		}
		pp.Score += e.fit(p.Bib)
		all = append(all, pp)
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Score != all[j].Score {
			return all[i].Score > all[j].Score
		}
		return all[i].Bib.Title < all[j].Bib.Title
	})
	var out []Pick
	byAuthor := map[string]int{}
	for _, p := range all {
		a := ""
		if len(p.Bib.Authors) > 0 {
			a = account.AuthorKey(p.Bib.Authors[0])
		}
		if byAuthor[a] >= perAuthor {
			continue
		}
		byAuthor[a]++
		out = append(out, p)
		if len(out) >= e.opts.Limit {
			break
		}
	}
	return out
}

// fit is the taste score every candidate gets: genre overlap and rating.
func (e *engine) fit(b bc.Bib) float64 {
	score := 0.0
	if b.Rating.Count >= 5 {
		score += (b.Rating.Weighted()/20 - 3.75) * 2 // ~ -1..+2
	}
	total := 0.0
	for _, g := range e.prof.Genres {
		total += g.Weight
	}
	if total > 0 {
		overlap := 0.0
		terms := map[string]bool{}
		for _, g := range append(append([]string{}, b.Genres...), b.Subjects...) {
			terms[strings.ToLower(g)] = true
		}
		for _, g := range e.prof.Genres {
			if terms[strings.ToLower(g.Name)] {
				overlap += g.Weight
			}
		}
		score += 2 * overlap / total
	}
	return score
}

// displayName turns "Wells, Martha" into "Martha Wells".
func displayName(a string) string {
	last, first, ok := strings.Cut(a, ",")
	if !ok {
		return a
	}
	return strings.TrimSpace(first) + " " + strings.TrimSpace(last)
}
