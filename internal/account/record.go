package account

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	bc "shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/hardcover"
	"shelfcheck/internal/libby"
)

// Shelf says where a book sits in the patron's reading life.
type Shelf string

const (
	ShelfRead    Shelf = "read"              // borrowed in the past (print history, Libby 🧾, Libby "Read", Hardcover "Read")
	ShelfCurrent Shelf = "currently-reading" // checked out now, or Hardcover "Currently Reading"
	ShelfDNF     Shelf = "did-not-finish"    // Hardcover "Did Not Finish" or "Ignored": not for them
	ShelfHold    Shelf = "on-hold"           // waiting on a hold
	ShelfWant    Shelf = "to-read"           // Libby or Hardcover "want to read"
	ShelfSampled Shelf = "sampled"           // Libby 🍰, never borrowed
)

// Book is one title in the reading record, merged across print and Libby.
type Book struct {
	Title       string
	Authors     []string // "Last, First" where known
	ISBN        string
	Year        string
	Publisher   string
	Format      string // "book", "ebook", "audiobook", ...
	Shelf       Shelf
	Date        string // YYYY-MM-DD: borrowed, checked out, hold placed or tagged
	Series      string
	SeriesOrder string
	Genres      []string
	Subjects    []string
	Rating      bc.Rating
	BibID       string  // catalog record, if from the library catalog
	LibbyID     string  // Libby title, if from Libby
	HardcoverID int     // Hardcover book, if from Hardcover
	Stars       float64 // the patron's own rating on Hardcover (0.5-5); 0 if unrated
}

// Key identifies a work across editions and formats.
func (b Book) Key() string { return WorkKey(b.Title, firstAuthor(b.Authors)) }

// WorkKey normalizes a title and first author into a comparable key. It
// keeps the title up to a subtitle or an alternative title ("Babel, or The
// Necessity of Violence: ..." and "Babel: Or the Necessity..." are both
// "babel").
func WorkKey(title, author string) string {
	t := normTitle(seriesSuffixRe.ReplaceAllString(title, ""))
	if loc := mainTitleEndRe.FindStringIndex(title); loc != nil && loc[0] > 3 {
		t = normTitle(title[:loc[0]])
	}
	return t + "|" + AuthorKey(author)
}

// mainTitleEndRe finds where a main title ends: a colon, or ", or" / "; or".
var mainTitleEndRe = regexp.MustCompile(`(?i):|[,;]\s*or\b`)

func firstAuthor(as []string) string {
	if len(as) == 0 {
		return ""
	}
	return as[0]
}

// Record is everything known about what the patron has read and wants.
type Record struct {
	Books []Book // one per work; Shelf is the most significant one
	Notes []string
}

// Has reports whether the patron has read, has, or is waiting on a work.
func (r *Record) Has(title, author string) (Shelf, bool) {
	k := WorkKey(title, author)
	for _, b := range r.Books {
		if b.Key() == k && b.Shelf != ShelfWant && b.Shelf != ShelfSampled {
			return b.Shelf, true
		}
	}
	return "", false
}

// shelfRank orders shelves so merges keep the strongest signal.
var shelfRank = map[Shelf]int{ShelfCurrent: 6, ShelfDNF: 5, ShelfRead: 4, ShelfHold: 3, ShelfWant: 2, ShelfSampled: 1}

// LoadRecord gathers the reading record from the catalog session (history,
// checkouts, holds), Libby (borrowed, Read, want to read, sampled) and the
// Hardcover library (with the patron's ratings), as far as each is set up.
// library is the catalog/Libby library key.
func (s *Store) LoadRecord(ctx context.Context, client *bc.Client, library string) (*Record, error) {
	r := &Record{}
	byKey := map[string]int{}
	add := func(b Book) {
		k := b.Key()
		i, ok := byKey[k]
		if !ok {
			byKey[k] = len(r.Books)
			r.Books = append(r.Books, b)
			return
		}
		old := &r.Books[i]
		if shelfRank[b.Shelf] > shelfRank[old.Shelf] {
			old.Shelf = b.Shelf
		}
		// Keep the earliest date for read books, fill gaps from the other copy.
		if b.Date != "" && (old.Date == "" || (old.Shelf == ShelfRead && b.Date < old.Date)) {
			old.Date = b.Date
		}
		fill := func(dst *string, src string) {
			if *dst == "" {
				*dst = src
			}
		}
		fill(&old.ISBN, b.ISBN)
		fill(&old.Year, b.Year)
		fill(&old.Publisher, b.Publisher)
		fill(&old.Series, b.Series)
		fill(&old.SeriesOrder, b.SeriesOrder)
		fill(&old.BibID, b.BibID)
		fill(&old.LibbyID, b.LibbyID)
		if old.HardcoverID == 0 {
			old.HardcoverID = b.HardcoverID
		}
		if old.Stars == 0 {
			old.Stars = b.Stars
		}
		if len(old.Genres) == 0 {
			old.Genres = b.Genres
		}
		if len(old.Subjects) == 0 {
			old.Subjects = b.Subjects
		}
		if old.Rating.Count == 0 {
			old.Rating = b.Rating
		}
	}

	if s.HasCatalog() {
		err := s.WithSession(ctx, client, func(sess *bc.Session) error {
			hist, err := client.History(ctx, sess)
			if err != nil {
				return err
			}
			for _, h := range hist {
				add(fromBib(h.Bib, ShelfRead, h.CheckedOut))
			}
			cos, err := client.Checkouts(ctx, sess)
			if err != nil {
				return err
			}
			for _, c := range cos {
				add(fromBib(c.Bib, ShelfCurrent, ""))
			}
			holds, err := client.Holds(ctx, sess)
			if err != nil {
				return err
			}
			for _, h := range holds {
				add(fromBib(h.Bib, ShelfHold, h.Placed))
			}
			return nil
		})
		if err != nil {
			r.Notes = append(r.Notes, "library account: "+err.Error())
		}
	}

	if s.HasLibby() {
		if err := s.loadLibby(ctx, library, add); err != nil {
			r.Notes = append(r.Notes, "libby: "+err.Error())
		}
	}

	if hc := s.HardcoverClient(); hc != nil {
		if err := loadHardcover(ctx, hc, add); err != nil {
			r.Notes = append(r.Notes, "hardcover: "+err.Error())
		}
	}
	sort.SliceStable(r.Books, func(i, j int) bool { return r.Books[i].Date > r.Books[j].Date })
	return r, nil
}

func (s *Store) loadLibby(ctx context.Context, library string, add func(Book)) error {
	lc, err := s.LibbyClient()
	if err != nil {
		return err
	}
	tags, err := lc.Tags(ctx)
	if err != nil {
		return err
	}
	type tagged struct {
		t     libby.Tagging
		shelf Shelf
	}
	var all []tagged
	for _, t := range tags {
		var shelf Shelf
		switch {
		case t.Kind == "borrowed", strings.EqualFold(t.Name, "read"):
			shelf = ShelfRead
		case strings.EqualFold(t.Name, "want to read"):
			shelf = ShelfWant
		case t.Kind == "sampled":
			shelf = ShelfSampled
		default:
			continue
		}
		items, err := lc.Taggings(ctx, t)
		if err != nil {
			return fmt.Errorf("tag %q: %w", t.Name, err)
		}
		for _, it := range items {
			all = append(all, tagged{it, shelf})
		}
	}
	var ids []string
	seen := map[string]bool{}
	for _, a := range all {
		if !seen[a.t.TitleID] {
			seen[a.t.TitleID] = true
			ids = append(ids, a.t.TitleID)
		}
	}
	media, err := lc.Media(ctx, library, ids)
	if err != nil {
		return fmt.Errorf("looking up titles: %w", err)
	}
	for _, a := range all {
		b := Book{
			Title: a.t.Title, Authors: splitAuthors(a.t.Author), Format: a.t.Format,
			Shelf: a.shelf, Subjects: a.t.Subjects, LibbyID: a.t.TitleID,
		}
		if !a.t.Tagged.IsZero() {
			b.Date = a.t.Tagged.Format("2006-01-02")
		}
		if m, ok := media[a.t.TitleID]; ok {
			b.Title = m.Title
			if m.Subtitle != "" {
				b.Title += ": " + m.Subtitle
			}
			b.ISBN, b.Year, b.Publisher = m.ISBN, m.PublishYear, m.Publisher
			b.Series, b.SeriesOrder = m.Series, m.SeriesOrder
			if b.Format == "book" {
				b.Format = "ebook"
			}
		}
		add(b)
	}
	return nil
}

// hardcoverShelf maps Hardcover statuses to shelves; Paused counts as read
// (they've had it in hand) and Ignored as not for them.
var hardcoverShelf = map[hardcover.Status]Shelf{
	hardcover.WantToRead: ShelfWant, hardcover.CurrentlyReading: ShelfCurrent, hardcover.Read: ShelfRead,
	hardcover.Paused: ShelfRead, hardcover.DidNotFinish: ShelfDNF, hardcover.Ignored: ShelfDNF,
}

func loadHardcover(ctx context.Context, hc *hardcover.Client, add func(Book)) error {
	lib, err := hc.Library(ctx)
	if err != nil {
		return err
	}
	for _, ub := range lib {
		shelf, ok := hardcoverShelf[ub.Status]
		if !ok {
			continue
		}
		b := Book{Title: ub.Title, Shelf: shelf, HardcoverID: ub.ID, Stars: ub.Rating, Format: "book"}
		if ub.Author != "" {
			b.Authors = []string{ub.Author}
		}
		if ub.Series != "" && ub.SeriesPos > 0 {
			b.Series, b.SeriesOrder = ub.Series, strconv.FormatFloat(ub.SeriesPos, 'f', -1, 64)
		}
		// Hardcover's date added is often the import date, so it only
		// dates a wish; a read book's date is its recorded finish, if any.
		switch shelf {
		case ShelfWant:
			b.Date = ub.Added
		case ShelfRead, ShelfDNF:
			b.Date = ub.LastRead
		}
		add(b)
	}
	return nil
}

func fromBib(b bc.Bib, shelf Shelf, date string) Book {
	bk := Book{
		Title: b.Title, Authors: b.Authors, Year: b.Year, Shelf: shelf, Date: date,
		Genres: b.Genres, Subjects: b.Subjects, Rating: b.Rating, BibID: b.ID,
		Format: strings.ToLower(b.Format),
	}
	if b.Subtitle != "" {
		bk.Title += ": " + b.Subtitle
	}
	if bk.Format == "bk" {
		bk.Format = "book"
	}
	if len(b.ISBNs) > 0 {
		bk.ISBN = b.ISBNs[0]
	}
	if len(b.Series) > 0 {
		bk.Series, bk.SeriesOrder = SplitSeries(b.Series[0])
	}
	return bk
}

// splitAuthors splits Libby's "Last, First; Last2, First2".
func splitAuthors(s string) []string {
	var out []string
	for _, a := range strings.Split(s, ";") {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}

// SplitSeries splits a catalog series heading like "Murderbot diaries, bk. 2"
// or "Expanse, bk. 9" into name and number.
func SplitSeries(s string) (name, order string) {
	m := seriesNumRe.FindStringSubmatch(s)
	if m == nil {
		return strings.TrimSpace(s), ""
	}
	return strings.TrimSpace(m[1]), m[2]
}
