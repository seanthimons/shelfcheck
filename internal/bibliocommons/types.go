package bibliocommons

import "time"

// Bib is one catalog record (a specific format/edition of a work).
type Bib struct {
	ID          string
	Title       string
	Subtitle    string
	Authors     []string
	Format      string // BiblioCommons format code, e.g. "BK", "EBOOK", "DVD"
	Digital     bool
	Year        string
	Edition     string
	CallNumber  string
	Description string
	Series      []string
	ISBNs       []string
	Genres      []string // e.g. "Science fiction", "Space operas (Fiction)"
	Subjects    []string // e.g. "Human-alien encounters Fiction"
	Language    string   // MARC code, e.g. "eng"; "und"/"zxx" when unknown or none
	Audiences   []string // "ADULT", "TEEN", "JUVENILE"
	ContentType string   // "FICTION", "NONFICTION", "UNDETERMINED"
	Rating      Rating
	CoverURL    string // medium-size jacket image, may be empty
	Summary     Summary
}

// Rating is the patron star rating. Average is 0-100 (78 = 3.9 of 5 stars);
// Count is 0 when nobody has rated the title.
type Rating struct {
	Average int
	Count   int
}

// Stars returns the average on a 0-5 scale.
func (r Rating) Stars() float64 { return float64(r.Average) / 20 }

// Summary is the system-wide availability rollup for a Bib.
type Summary struct {
	Status         string // "AVAILABLE", "UNAVAILABLE", "ON_ORDER", ...
	Available      int
	Total          int
	Holds          int
	OnOrder        int
	LibraryUseOnly bool
}

type SearchType string

const (
	Keyword SearchType = "keyword"
	Title   SearchType = "title"
	Author  SearchType = "author"
	Subject SearchType = "subject"
	Series  SearchType = "series"
)

type SearchResult struct {
	Query string
	Page  int // 1-based
	Pages int
	Total int
	Bibs  []Bib
}

// Availability is the per-copy detail for one Bib.
type Availability struct {
	ID             string
	Summary        Summary
	Copies         []Copy   // physical copies; empty for digital titles
	DigitalFormats []string // e.g. "KINDLE", "OVERDRIVE_READ"
}

// Copy is one physical item.
type Copy struct {
	ItemID        string
	BranchCode    string // matches Location.Code
	Branch        string
	Collection    string // shelf location, e.g. "Main - 1st Floor South - Sci-Fi"
	CallNumber    string
	Volume        string
	Available     bool
	Status        string    // normalized: "AVAILABLE", "UNAVAILABLE", ...
	LibraryStatus string    // the library's own wording, e.g. "CHECK SHELVES", "Checked out"
	Due           time.Time // zero if not checked out
}

type Location struct {
	Code    string
	Name    string
	Address string
	Phone   string
	Hours   []DayHours // Monday..Sunday
	WebURL  string
}

type DayHours struct {
	Day    string // "monday"
	Closed bool
	Open   string // "10:00"
	Close  string // "21:00"
}

// Details is the professional content for a Bib: trade reviews and an author
// bio, sourced by the library from Syndetics. Fields are plain text with the
// HTML stripped; any of them may be empty.
type Details struct {
	ID         string
	AuthorNote string
	Reviews    []Review
	Excerpt    string // often the opening chapter; can be long
}

type Review struct {
	Source string // e.g. "Publishers Weekly", "Library Journal"
	Text   string
}
