package account

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	bc "shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/hardcover"
	"shelfcheck/internal/libby"
)

// DigitalFormats are catalog formats borrowed online rather than from a shelf.
var DigitalFormats = map[string]bool{"EBOOK": true, "EAUDIOBOOK": true, "AB_ONLINE": true, "EMAG": true}

// WishOptions controls how wishlist titles are matched in the catalog.
type WishOptions struct {
	Home     string   // branch code whose shelf counts for print copies
	HomeName string   // its name, for WishMatch.AvailableAt
	Formats  []string // catalog format codes; default BK and EBOOK
	Language string   // catalog language code; "" for any
}

func (o WishOptions) withDefaults() WishOptions {
	if len(o.Formats) == 0 {
		o.Formats = []string{"BK", "EBOOK"}
	}
	if o.HomeName == "" {
		o.HomeName = "branch " + o.Home
	}
	return o
}

// WishItem is one wishlist title and the catalog copies available now.
type WishItem struct {
	Tagging libby.Tagging
	Matches []WishMatch
}

func (w WishItem) Available() bool { return len(w.Matches) > 0 }

// OnShelf reports whether a print copy is on the shelf at home.
func (w WishItem) OnShelf() bool {
	for _, m := range w.Matches {
		if !m.Online {
			return true
		}
	}
	return false
}

// OnlineNow reports whether a digital copy can be borrowed now.
func (w WishItem) OnlineNow() bool {
	for _, m := range w.Matches {
		if m.Online {
			return true
		}
	}
	return false
}

// WishMatch is a catalog record for a wishlist title that's available now.
type WishMatch struct {
	Bib         bc.Bib
	Online      bool
	AvailableAt string // home branch name, or "online"
}

// WishlistTitles returns the titles in a Libby tag (by name or kind) that
// could have a print or ebook copy: books, not magazines or audiobooks.
func WishlistTitles(ctx context.Context, lc *libby.Client, tagName string) ([]libby.Tagging, error) {
	tags, err := lc.Tags(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range tags {
		if !strings.EqualFold(t.Name, tagName) && !strings.EqualFold(t.Kind, tagName) {
			continue
		}
		items, err := lc.Taggings(ctx, t)
		var books []libby.Tagging
		for _, it := range items {
			if it.Format == "" || it.Format == "book" {
				books = append(books, it)
			}
		}
		return books, err
	}
	return nil, fmt.Errorf("no Libby tag named %q", tagName)
}

// WantToRead gathers want-to-read titles from every source set up: the
// Libby tag tagName and, when tagName is "want to read", the Hardcover
// library's Want to Read shelf. A title on both is listed once, as Libby has
// it. If a source fails, the others' titles come back with its error.
func (s *Store) WantToRead(ctx context.Context, tagName string) ([]libby.Tagging, error) {
	var out []libby.Tagging
	var errs []error
	seen := map[string]bool{}
	if s.HasLibby() {
		lc, err := s.LibbyClient()
		if err == nil {
			var items []libby.Tagging
			items, err = WishlistTitles(ctx, lc, tagName)
			for _, it := range items {
				seen[WorkKey(it.Title, it.Author)] = true
			}
			out = append(out, items...)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("libby: %w", err))
		}
	}
	if hc := s.HardcoverClient(); hc != nil && strings.EqualFold(tagName, "want to read") {
		lib, err := hc.Library(ctx)
		if err != nil {
			errs = append(errs, err)
		}
		for _, ub := range lib {
			if ub.Status != hardcover.WantToRead || seen[WorkKey(ub.Title, ub.Author)] {
				continue
			}
			seen[WorkKey(ub.Title, ub.Author)] = true
			it := libby.Tagging{Title: ub.Title, Author: ub.Author, Format: "book"}
			it.Tagged, _ = time.Parse("2006-01-02", ub.Added)
			out = append(out, it)
		}
	}
	return out, errors.Join(errs...)
}

// CheckWish searches the catalog for one wishlist title and returns the
// copies available now: print on the shelf at home, or digital available
// online. It makes one catalog request.
func CheckWish(ctx context.Context, client *bc.Client, it libby.Tagging, opts WishOptions) (WishItem, error) {
	opts = opts.withDefaults()
	w := WishItem{Tagging: it}
	q := bc.Query{Text: wishQuery(it.Title), Type: bc.Title}
	// STATUS values are ORed: on the shelf at home, or (for digital
	// formats) available online now.
	q.Toggle(bc.FacetStatus, opts.Home)
	for _, f := range opts.Formats {
		q.Toggle(bc.FacetFormat, f)
		if DigitalFormats[f] && !q.Has(bc.FacetStatus, "_online_") {
			q.Toggle(bc.FacetStatus, "_online_")
		}
	}
	if opts.Language != "" {
		q.Toggle(bc.FacetLanguage, opts.Language)
	}
	res, err := client.Search(ctx, q, 1)
	if err != nil {
		return w, fmt.Errorf("searching %q: %w", it.Title, err)
	}
	want := AuthorKey(it.Author)
	for _, b := range res.Bibs {
		if !SameTitle(b.Title, it.Title) {
			continue // same author, different book
		}
		for _, a := range b.Authors {
			if want != "" && AuthorKey(a) == want {
				m := WishMatch{Bib: b, AvailableAt: opts.HomeName}
				if b.Digital || DigitalFormats[b.Format] {
					m.Online, m.AvailableAt = true, "online"
				}
				w.Matches = append(w.Matches, m)
				break
			}
		}
	}
	return w, nil
}

var (
	seriesSuffixRe = regexp.MustCompile(`,\s*\d+$`)
	nonWordRe      = regexp.MustCompile(`[^\p{L}\p{N}]+`)
	seriesNumRe    = regexp.MustCompile(`(?i)^(.*?)[,;:]?\s*(?:bk\.?|book|vol\.?|volume|no\.?|#)\s*(\d+(?:\.\d+)?)\s*$`)
)

// wishQuery trims what trips up catalog title search: series numbers
// ("..., 1") and subtitles.
func wishQuery(title string) string {
	t := seriesSuffixRe.ReplaceAllString(title, "")
	if i := strings.IndexAny(t, ":;("); i > 3 {
		t = t[:i]
	}
	return strings.TrimSpace(t)
}

// AuthorKey is the first author's last name, lowercased, for matching
// "Last, First" (catalog, Libby) against "First Last" (Hardcover).
func AuthorKey(a string) string {
	a = strings.Split(a, ";")[0]
	last, _, hasComma := strings.Cut(a, ",")
	last = strings.ToLower(strings.TrimSpace(nonWordRe.ReplaceAllString(last, " ")))
	if !hasComma {
		if f := strings.Fields(last); len(f) > 0 {
			last = f[len(f)-1]
		}
	}
	return last
}

// normTitle lowercases a title, reduces punctuation to single spaces and
// drops a leading article.
func normTitle(t string) string {
	t = strings.TrimSpace(nonWordRe.ReplaceAllString(strings.ToLower(t), " "))
	for _, a := range []string{"the ", "a ", "an "} {
		t = strings.TrimPrefix(t, a)
	}
	return t
}

// SameTitle reports whether a catalog title names the same book as a Libby
// title. Libby runs subtitles into the title ("Cadillac Desert The American
// West...") while the catalog keeps them separate, so the catalog title must
// equal the Libby title or be a whole-word prefix of it. A longer catalog
// title ("Skyward Flight" for "Skyward") is a different book.
func SameTitle(catalog, libbyTitle string) bool {
	c, l := normTitle(catalog), normTitle(seriesSuffixRe.ReplaceAllString(libbyTitle, ""))
	return c != "" && (c == l || strings.HasPrefix(l, c+" "))
}
