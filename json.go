package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"shelfcheck/internal/account"
	bc "shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/libby"
)

const jsonUsage = `usage: shelfcheck json <command> [flags] [args]

Machine-readable catalog access for scripts and agents. Output is JSON on
stdout; errors go to stderr with a non-zero exit. Flags go before arguments.

commands:
  search [flags] <query...>   search the catalog
  book [flags] <id...>        full record, copies and reviews for bib IDs from search
  branches                    branch codes, addresses and hours
  history                     your borrowing history, newest first    (needs shelfcheck login)
  checkouts                   your current loans, incl. Libby/OverDrive (needs shelfcheck login)
  holds                       your holds, incl. Libby/OverDrive        (needs shelfcheck login)
  libby                       your Libby tags: borrowed, sampled, wishlists (needs shelfcheck libby link)
  wishlist                    which "want to read" titles you can get now: print on the shelf at -home, or ebooks (needs libby link)

Run "shelfcheck json <command> -h" for a command's flags.
`

func jsonCmd(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "help" {
		fmt.Fprint(os.Stderr, jsonUsage)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var err error
	switch args[0] {
	case "search":
		err = jsonSearch(ctx, args[1:])
	case "book":
		err = jsonBook(ctx, args[1:])
	case "branches":
		err = jsonBranches(ctx, args[1:])
	case "history", "checkouts", "holds":
		err = jsonPatron(ctx, args[0], args[1:])
	case "libby":
		err = jsonLibby(ctx, args[1:])
	case "wishlist":
		err = jsonWishlist(ctx, args[1:])
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], jsonUsage)
	}
	if errors.Is(err, flag.ErrHelp) {
		return nil // usage already printed
	}
	return err
}

func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// listFlag collects a repeatable, comma-separated flag.
type listFlag []string

func (l *listFlag) String() string { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error {
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			*l = append(*l, p)
		}
	}
	return nil
}

// --- search -----------------------------------------------------------------

var formatAliases = map[string]string{
	"book": "BK", "books": "BK", "large-print": "LPRINT", "largeprint": "LPRINT",
	"ebook": "EBOOK", "eaudiobook": "EAUDIOBOOK", "audiobook": "AB", "audiobook-cd": "AB",
	"graphic-novel": "GRAPHIC_NOVEL", "comic": "GRAPHIC_NOVEL", "dvd": "DVD", "bluray": "BLURAY",
	"music-cd": "MUSIC_CD", "vinyl": "LP", "sheet-music": "MN",
}

var sortAliases = map[string]string{
	"relevance": bc.SortRelevance, "top-rated": bc.SortTopRated, "newest": bc.SortPublished,
	"added": bc.SortNewlyAcquired, "title": bc.SortTitle, "author": bc.SortAuthor,
	"raw-rating": bc.SortRating,
}

var ratingValues = map[int]string{1: "ONE_PLUS_STARS", 2: "TWO_PLUS_STARS", 3: "THREE_PLUS_STARS", 4: "FOUR_PLUS_STARS"}

type searchOut struct {
	Query   searchQueryOut        `json:"query"`
	Total   int                   `json:"total"`
	Page    int                   `json:"page"`
	Pages   int                   `json:"pages"`
	Partial bool                  `json:"partial,omitempty"`
	Note    string                `json:"note,omitempty"`
	Results []bookOut             `json:"results"`
	Facets  map[string][]facetOut `json:"facets,omitempty"`
}

type searchQueryOut struct {
	Text    string              `json:"text"`
	Type    string              `json:"type"`
	Filters map[string][]string `json:"filters,omitempty"`
	Sort    string              `json:"sort,omitempty"`
}

type facetOut struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

func jsonSearch(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("json search", flag.ContinueOnError)
	var o options
	o.registerCatalog(fs)
	typ := fs.String("type", "keyword", "keyword, title, author, subject, genre or series")
	homeOnly := fs.Bool("home-only", false, "only titles with a copy on the shelf at -home right now")
	var formats, genres, filters listFlag
	fs.Var(&formats, "format", "format(s): book, large-print, ebook, eaudiobook, audiobook-cd, graphic-novel, dvd, ... or a raw code like BK (repeatable, comma-separated; ORed)")
	fs.Var(&genres, "genre", `genre heading(s) exactly as in facets, e.g. "Science fiction" (repeatable; ORed)`)
	fs.Var(&filters, "filter", "raw facet filter FIELD=VALUE, e.g. AUDIENCE=teen (repeatable)")
	rating := fs.Int("min-stars", 0, "only titles whose raw average is at least 1-4 stars (weak on its own: one rating counts)")
	fiction := fs.Bool("fiction", false, "fiction only")
	nonfiction := fs.Bool("nonfiction", false, "nonfiction only")
	added := fs.Int("added-within", 0, "only titles added in the last 30, 60, 90 or 180 days")
	sortBy := fs.String("sort", "relevance", "relevance, top-rated (weighted by number of ratings), newest, added, title or author")
	page := fs.Int("page", 1, "page of 25 results (ignored for top-rated, which returns one ranked page)")
	limit := fs.Int("limit", 0, "return at most this many results (0 = all on the page)")
	brief := fs.Bool("brief", false, "omit descriptions to keep output short")
	facets := fs.Bool("facets", false, "include refinement facets (genres, formats, years, ...) with counts")
	if err := fs.Parse(args); err != nil {
		return err
	}
	text := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(text) == "" {
		return errors.New("missing search query")
	}

	q := bc.Query{Text: text, Type: bc.SearchType(*typ)}
	switch q.Type {
	case bc.Keyword, bc.Title, bc.Author, bc.Subject, bc.Genre, bc.Series:
	default:
		return fmt.Errorf("unknown -type %q", *typ)
	}
	s, ok := sortAliases[*sortBy]
	if !ok {
		return fmt.Errorf("unknown -sort %q", *sortBy)
	}
	q.Sort = s
	if *homeOnly {
		q.Toggle(bc.FacetStatus, o.home)
	}
	for _, f := range formats {
		code, ok := formatAliases[strings.ToLower(f)]
		if !ok {
			code = strings.ToUpper(f)
		}
		q.Toggle(bc.FacetFormat, code)
	}
	for _, g := range genres {
		q.Toggle(bc.FacetGenre, g)
	}
	if *rating != 0 {
		v, ok := ratingValues[*rating]
		if !ok {
			return errors.New("-min-stars must be 1-4")
		}
		q.Toggle(bc.FacetRating, v)
	}
	if *fiction {
		q.Toggle(bc.FacetFiction, "FICTION")
	}
	if *nonfiction {
		q.Toggle(bc.FacetFiction, "NONFICTION")
	}
	if *added != 0 {
		switch *added {
		case 30, 60, 90, 180:
			q.Toggle(bc.FacetNew, fmt.Sprintf("PAST_%d_DAYS", *added))
		default:
			return errors.New("-added-within must be 30, 60, 90 or 180")
		}
	}
	for _, f := range filters {
		field, value, ok := strings.Cut(f, "=")
		if !ok {
			return fmt.Errorf("-filter %q: want FIELD=VALUE", f)
		}
		q.Toggle(strings.ToUpper(field), value)
	}

	client := bc.New(o.library)
	res, err := client.Search(ctx, q, *page)
	if err != nil {
		return err
	}
	out := searchOut{
		Query:   searchQueryOut{Text: q.Text, Type: string(q.Type), Filters: q.Filters, Sort: *sortBy},
		Total:   res.Total,
		Page:    res.Page,
		Pages:   res.Pages,
		Partial: res.Partial,
		Results: []bookOut{},
	}
	if res.Partial {
		out.Note = fmt.Sprintf("top-rated returns the best %d of %d, ranked by weighted rating", len(res.Bibs), res.Total)
	}
	bibs := res.Bibs
	if *limit > 0 && len(bibs) > *limit {
		bibs = bibs[:*limit]
	}
	for _, b := range bibs {
		bo := newBookOut(o.library, b)
		if *brief {
			bo.Description = ""
		}
		out.Results = append(out.Results, bo)
	}
	if *facets {
		out.Facets = map[string][]facetOut{}
		for field, fs := range res.Facets {
			for _, f := range fs {
				out.Facets[field] = append(out.Facets[field], facetOut{f.Value, f.Count})
			}
		}
	}
	return writeJSON(out)
}

// --- book -------------------------------------------------------------------

type bookOut struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	Subtitle     string     `json:"subtitle,omitempty"`
	Authors      []string   `json:"authors,omitempty"`
	Format       string     `json:"format"`
	Digital      bool       `json:"digital,omitempty"`
	Year         string     `json:"year,omitempty"`
	Edition      string     `json:"edition,omitempty"`
	Series       []string   `json:"series,omitempty"`
	Genres       []string   `json:"genres,omitempty"`
	Subjects     []string   `json:"subjects,omitempty"`
	Audience     []string   `json:"audience,omitempty"`
	ContentType  string     `json:"content_type,omitempty"`
	Rating       *ratingOut `json:"rating,omitempty"`
	Availability summaryOut `json:"availability"`
	CallNumber   string     `json:"call_number,omitempty"`
	ISBNs        []string   `json:"isbns,omitempty"`
	Description  string     `json:"description,omitempty"`
	URL          string     `json:"url"`
}

type ratingOut struct {
	Stars    float64 `json:"stars"`    // raw average, 0-5
	Count    int     `json:"count"`    // number of ratings behind it
	Weighted float64 `json:"weighted"` // 0-5, pulled toward 3.75 when Count is small
}

type summaryOut struct {
	Available      int  `json:"available"`
	Total          int  `json:"total"`
	Holds          int  `json:"holds"`
	OnOrder        int  `json:"on_order,omitempty"`
	LibraryUseOnly bool `json:"library_use_only,omitempty"`
}

func newBookOut(library string, b bc.Bib) bookOut {
	out := bookOut{
		ID: b.ID, Title: b.Title, Subtitle: b.Subtitle, Authors: b.Authors,
		Format: formatName(b.Format), Digital: b.Digital, Year: b.Year, Edition: b.Edition,
		Series: b.Series, Genres: b.Genres, Subjects: b.Subjects, Audience: b.Audiences,
		CallNumber: b.CallNumber, ISBNs: b.ISBNs, Description: b.Description,
		URL: fmt.Sprintf("https://%s.bibliocommons.com/v2/record/%s", library, b.ID),
		Availability: summaryOut{
			Available: b.Summary.Available, Total: b.Summary.Total, Holds: b.Summary.Holds,
			OnOrder: b.Summary.OnOrder, LibraryUseOnly: b.Summary.LibraryUseOnly,
		},
	}
	if b.ContentType != "UNDETERMINED" {
		out.ContentType = strings.ToLower(b.ContentType)
	}
	if b.Rating.Count > 0 {
		out.Rating = &ratingOut{
			Stars:    round1(b.Rating.Stars()),
			Count:    b.Rating.Count,
			Weighted: round1(b.Rating.Weighted() / 20),
		}
	}
	return out
}

func formatName(code string) string {
	for name, c := range formatAliases {
		if c == code && !strings.HasSuffix(name, "s") && name != "comic" && name != "largeprint" && name != "audiobook" {
			return name
		}
	}
	return strings.ToLower(code)
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }

type bookDetailOut struct {
	bookOut
	Copies         []copyOut   `json:"copies,omitempty"`
	OnShelfAtHome  int         `json:"on_shelf_at_home"`
	DigitalFormats []string    `json:"digital_formats,omitempty"`
	Reviews        []reviewOut `json:"reviews,omitempty"`
	AboutAuthor    string      `json:"about_author,omitempty"`
	Excerpt        string      `json:"excerpt,omitempty"`
	Errors         []string    `json:"errors,omitempty"`
}

type copyOut struct {
	Branch     string `json:"branch"`
	BranchCode string `json:"branch_code"`
	Home       bool   `json:"home,omitempty"`
	Shelf      string `json:"shelf"`
	CallNumber string `json:"call_number,omitempty"`
	Volume     string `json:"volume,omitempty"`
	OnShelf    bool   `json:"on_shelf"`
	Status     string `json:"status"`
	Due        string `json:"due,omitempty"`
}

type reviewOut struct {
	Source string `json:"source"`
	Text   string `json:"text"`
}

func jsonBook(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("json book", flag.ContinueOnError)
	var o options
	o.registerCatalog(fs)
	reviews := fs.Bool("reviews", true, "include trade reviews and the author bio (one extra request per book)")
	excerpt := fs.Bool("excerpt", false, "include the excerpt (often a whole opening chapter)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ids := fs.Args()
	if len(ids) == 0 {
		return errors.New("missing bib ID(s); get them from json search")
	}

	client := bc.New(o.library)
	bibs, err := client.Bibs(ctx, ids...)
	if err != nil {
		return fmt.Errorf("looking up %s: %w", strings.Join(ids, ", "), err)
	}
	out := []bookDetailOut{}
	for _, b := range bibs {
		d := bookDetailOut{bookOut: newBookOut(o.library, b)}
		if a, err := client.Availability(ctx, b.ID); err != nil {
			d.Errors = append(d.Errors, "availability: "+err.Error())
		} else {
			d.DigitalFormats = a.DigitalFormats
			for _, c := range a.Copies {
				co := copyOut{
					Branch: c.Branch, BranchCode: c.BranchCode, Home: c.BranchCode == o.home,
					Shelf: c.Collection, CallNumber: c.CallNumber, Volume: c.Volume,
					OnShelf: c.Available, Status: c.LibraryStatus,
				}
				if !c.Due.IsZero() {
					co.Due = c.Due.Format("2006-01-02")
				}
				if co.Home && co.OnShelf {
					d.OnShelfAtHome++
				}
				d.Copies = append(d.Copies, co)
			}
		}
		if *reviews || *excerpt {
			if det, err := client.Details(ctx, b.ID); err != nil {
				d.Errors = append(d.Errors, "details: "+err.Error())
			} else {
				if *reviews {
					for _, r := range det.Reviews {
						d.Reviews = append(d.Reviews, reviewOut{r.Source, r.Text})
					}
					d.AboutAuthor = det.AuthorNote
				}
				if *excerpt {
					d.Excerpt = det.Excerpt
				}
			}
		}
		out = append(out, d)
	}
	if len(out) < len(ids) {
		fmt.Fprintf(os.Stderr, "shelfcheck json book: found %d of %d IDs\n", len(out), len(ids))
	}
	return writeJSON(out)
}

// --- branches ---------------------------------------------------------------

type branchOut struct {
	Code    string   `json:"code"`
	Name    string   `json:"name"`
	Address string   `json:"address"`
	Phone   string   `json:"phone,omitempty"`
	Hours   []string `json:"hours"`
	URL     string   `json:"url,omitempty"`
}

func jsonBranches(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("json branches", flag.ContinueOnError)
	var o options
	o.registerCatalog(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	locs, err := bc.New(o.library).Locations(ctx)
	if err != nil {
		return err
	}
	out := []branchOut{}
	for _, l := range locs {
		b := branchOut{Code: l.Code, Name: l.Name, Address: l.Address, Phone: l.Phone, URL: l.WebURL}
		for _, h := range l.Hours {
			if h.Closed {
				b.Hours = append(b.Hours, h.Day+" closed")
			} else {
				b.Hours = append(b.Hours, fmt.Sprintf("%s %s-%s", h.Day, h.Open, h.Close))
			}
		}
		out = append(out, b)
	}
	return writeJSON(out)
}

// --- patron (logged in) -----------------------------------------------------

type patronItemOut struct {
	bookOut
	CheckedOut string `json:"checked_out,omitempty"`
	Returned   string `json:"returned,omitempty"`
	Due        string `json:"due,omitempty"`
	Status     string `json:"status,omitempty"`
	Position   int    `json:"hold_position,omitempty"`
	Placed     string `json:"hold_placed,omitempty"`
	Renewed    int    `json:"times_renewed,omitempty"`
}

func jsonPatron(ctx context.Context, what string, args []string) error {
	fs := flag.NewFlagSet("json "+what, flag.ContinueOnError)
	var o options
	o.registerCatalog(fs)
	st := registerStore(fs)
	full := fs.Bool("full", false, "include descriptions and subjects")
	if err := fs.Parse(args); err != nil {
		return err
	}
	client := bc.New(o.library)
	out := []patronItemOut{}
	item := func(b bc.Bib) patronItemOut {
		bo := newBookOut(o.library, b)
		if !*full {
			bo.Description, bo.Subjects, bo.ISBNs = "", nil, nil
		}
		return patronItemOut{bookOut: bo}
	}
	err := st.WithSession(ctx, client, func(s *bc.Session) error {
		out = out[:0]
		switch what {
		case "history":
			items, err := client.History(ctx, s)
			for _, h := range items {
				it := item(h.Bib)
				it.CheckedOut, it.Returned = h.CheckedOut, h.Returned
				out = append(out, it)
			}
			return err
		case "checkouts":
			items, err := client.Checkouts(ctx, s)
			for _, c := range items {
				it := item(c.Bib)
				it.Due, it.Status, it.Renewed, it.Digital = c.Due, c.Status, c.Renewed, c.Digital || it.Digital
				out = append(out, it)
			}
			return err
		default:
			items, err := client.Holds(ctx, s)
			for _, h := range items {
				it := item(h.Bib)
				it.Status, it.Position, it.Placed, it.Digital = h.Status, h.Position, h.Placed, h.Digital || it.Digital
				out = append(out, it)
			}
			return err
		}
	})
	if err != nil {
		return err
	}
	return writeJSON(out)
}

// --- libby ------------------------------------------------------------------

type libbyTagOut struct {
	Name        string          `json:"name"`
	Kind        string          `json:"kind,omitempty"`
	Description string          `json:"description,omitempty"`
	Count       int             `json:"count"`
	Titles      []libbyTitleOut `json:"titles"`
}

type libbyTitleOut struct {
	Title    string   `json:"title"`
	Author   string   `json:"author,omitempty"`
	Format   string   `json:"format,omitempty"`
	Tagged   string   `json:"tagged,omitempty"`
	Subjects []string `json:"subjects,omitempty"`
	LibbyID  string   `json:"libby_id"`
}

func jsonLibby(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("json libby", flag.ContinueOnError)
	only := fs.String("tag", "", `only this tag, by name or kind ("borrowed", "sampled", "notify-me", "want to read")`)
	subjects := fs.Bool("subjects", true, "include Libby's subject labels")
	if err := fs.Parse(args); err != nil {
		return err
	}
	saved, err := libby.Load(libbyPath())
	if err != nil {
		return err
	}
	c := libby.New(saved.Identity)
	tags, err := c.Tags(ctx)
	if err != nil {
		return err
	}
	out := struct {
		Tags []libbyTagOut `json:"tags"`
	}{Tags: []libbyTagOut{}}
	for _, t := range tags {
		if *only != "" && !strings.EqualFold(*only, t.Name) && !strings.EqualFold(*only, t.Kind) {
			continue
		}
		to := libbyTagOut{Name: t.Name, Kind: t.Kind, Description: t.Description, Count: t.Total, Titles: []libbyTitleOut{}}
		if t.Total > 0 {
			items, err := c.Taggings(ctx, t)
			if err != nil {
				return fmt.Errorf("tag %q: %w", t.Name, err)
			}
			for _, it := range items {
				lo := libbyTitleOut{Title: it.Title, Author: it.Author, Format: it.Format, LibbyID: it.TitleID}
				if !it.Tagged.IsZero() {
					lo.Tagged = it.Tagged.Format("2006-01-02")
				}
				if *subjects {
					lo.Subjects = it.Subjects
				}
				to.Titles = append(to.Titles, lo)
			}
		}
		out.Tags = append(out.Tags, to)
	}
	if *only != "" && len(out.Tags) == 0 {
		return fmt.Errorf("no tag named %q", *only)
	}
	return writeJSON(out)
}

// --- wishlist ---------------------------------------------------------------

type wishOut struct {
	Title     string         `json:"title"`
	Author    string         `json:"author,omitempty"`
	Tagged    string         `json:"tagged,omitempty"`
	Available bool           `json:"available_now"`
	Matches   []wishMatchOut `json:"matches,omitempty"`
}

type wishMatchOut struct {
	bookOut
	AvailableAt string `json:"available_at"` // the home branch's name, or "online"
}

func jsonWishlist(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("json wishlist", flag.ContinueOnError)
	var o options
	o.registerCatalog(fs)
	tagName := fs.String("tag", "want to read", "Libby tag to check")
	var formats listFlag
	fs.Var(&formats, "format", "catalog formats to accept (default book, ebook); print must be on the shelf at -home, digital available to borrow now")
	all := fs.Bool("all", false, "list every title, not just the ones available now")
	lang := fs.String("lang", "eng", `catalog language code to accept ("" for any)`)
	if err := fs.Parse(args); err != nil {
		return err
	}
	opts := account.WishOptions{Home: o.home, Language: *lang}
	for _, f := range formats {
		code, ok := formatAliases[strings.ToLower(f)]
		if !ok {
			code = strings.ToUpper(f)
		}
		opts.Formats = append(opts.Formats, code)
	}
	lc, err := account.DefaultStore(configDir()).LibbyClient()
	if err != nil {
		return err
	}
	items, err := account.WishlistTitles(ctx, lc, *tagName)
	if err != nil {
		return err
	}
	client := bc.New(o.library)
	if locs, err := client.Locations(ctx); err == nil {
		for _, l := range locs {
			if l.Code == o.home {
				opts.HomeName = l.Name
			}
		}
	}
	out := []wishOut{}
	for _, it := range items {
		w, err := account.CheckWish(ctx, client, it, opts)
		if err != nil {
			return err
		}
		wo := wishOut{Title: it.Title, Author: it.Author, Available: w.Available()}
		if !it.Tagged.IsZero() {
			wo.Tagged = it.Tagged.Format("2006-01-02")
		}
		for _, m := range w.Matches {
			bo := newBookOut(o.library, m.Bib)
			bo.Description, bo.Subjects, bo.ISBNs = "", nil, nil
			wo.Matches = append(wo.Matches, wishMatchOut{bookOut: bo, AvailableAt: m.AvailableAt})
		}
		if wo.Available || *all {
			out = append(out, wo)
		}
	}
	return writeJSON(out)
}
