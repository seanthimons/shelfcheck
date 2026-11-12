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

// Weighting for Rating.Weighted: every title is scored as if it also had
// ratingWeight ratings of ratingPrior (3.75 stars), so a handful of ratings
// can't outrank a long track record.
const (
	ratingPrior  = 75.0
	ratingWeight = 10.0
)

// Weighted returns a Bayesian-average score (0-100) that pulls titles with
// few ratings toward a typical rating: one 5-star rating scores about 77,
// while 4.45 stars from 211 ratings scores about 88. Unrated titles score 0.
func (r Rating) Weighted() float64 {
	if r.Count == 0 {
		return 0
	}
	n := float64(r.Count)
	return (ratingPrior*ratingWeight + float64(r.Average)*n) / (ratingWeight + n)
}

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
	Genre   SearchType = "genre" // not a gateway searchType; sent as a genre:(...) query
	Boolean SearchType = "bl"    // the catalog's boolean query language, e.g. "nw:[0 TO 180]"
)

// Query describes a search.
type Query struct {
	Text string
	Type SearchType
	// Filters narrows results by facet field (e.g. FacetFormat) to the given
	// values, as they appear in SearchResult.Facets. Values within a field
	// are ORed; fields are ANDed.
	Filters map[string][]string
	Sort    string // "" for relevance, or one of the Sort* constants
}

// Facet fields usable in Query.Filters.
const (
	FacetStatus   = "STATUS" // a branch code (on shelf there), "_physical_" or "_online_"
	FacetFormat   = "FORMAT"
	FacetGenre    = "GENRE_HEADINGS"
	FacetRating   = "UGC_RATING" // "FOUR_PLUS_STARS", "THREE_PLUS_STARS", ...
	FacetFiction  = "FICTION_TYPE"
	FacetAudience = "AUDIENCE"
	FacetNew      = "NEWLY_ACQUIRED" // "PAST_30_DAYS", ...
	FacetYear     = "PUBLISHED_DATE"
	FacetLanguage = "PRIMARY_LANGUAGE"
	FacetAuthor   = "AUTHOR"
)

const (
	SortRelevance = ""
	// SortTopRated ranks by Rating.Weighted. The gateway can't do this, so
	// the client fetches the best raw averages and re-ranks them; results
	// come back as a single page.
	SortTopRated = "top_rated"
	SortRating   = "ugc_rating" // the gateway's raw average; 1 five-star rating ranks first

	SortPublished     = "published_date"
	SortNewlyAcquired = "newly_acquired"
	SortTitle         = "title"
	SortAuthor        = "author"
)

// Clone returns a copy whose Filters can be changed without affecting q.
func (q Query) Clone() Query {
	c := q
	c.Filters = make(map[string][]string, len(q.Filters))
	for k, v := range q.Filters {
		c.Filters[k] = append([]string(nil), v...)
	}
	return c
}

// Has reports whether value is selected for field.
func (q Query) Has(field, value string) bool {
	for _, v := range q.Filters[field] {
		if v == value {
			return true
		}
	}
	return false
}

// Toggle adds or removes value for field. Call it on a Clone.
func (q *Query) Toggle(field, value string) {
	if q.Filters == nil {
		q.Filters = map[string][]string{}
	}
	vs := q.Filters[field]
	for i, v := range vs {
		if v == value {
			q.Filters[field] = append(vs[:i:i], vs[i+1:]...)
			if len(q.Filters[field]) == 0 {
				delete(q.Filters, field)
			}
			return
		}
	}
	q.Filters[field] = append(vs, value)
}

// Narrowed reports whether any filter is set.
func (q Query) Narrowed() bool { return len(q.Filters) > 0 }

type SearchResult struct {
	Query  Query
	Page   int // 1-based
	Pages  int
	Total  int
	Bibs   []Bib
	Facets map[string][]Facet // by facet field; each lists its most common values
	// Partial is set when Bibs holds only the best-ranked part of Total
	// (see SortTopRated).
	Partial bool
}

// Facet is one value of a search refinement and how many results have it.
type Facet struct {
	Value string
	Count int
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
