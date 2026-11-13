package bibliocommons

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Search runs a catalog search. page is 1-based; results come 25 per page
// (the gateway ignores a smaller limit).
func (c *Client) Search(ctx context.Context, q Query, page int) (*SearchResult, error) {
	text := strings.TrimSpace(q.Text)
	if text == "" {
		return nil, errors.New("bibliocommons: empty search query")
	}
	if q.Sort == SortTopRated {
		return c.searchTopRated(ctx, q)
	}
	typ := q.Type
	if typ == "" {
		typ = Keyword
	}
	if page < 1 {
		page = 1
	}
	params := url.Values{"query": {text}, "searchType": {string(typ)}}
	if typ == Genre {
		// There's no genre searchType; the catalog's boolean ("bl") query
		// language has a genre field instead.
		params.Set("query", "genre:("+strings.NewReplacer("(", " ", ")", " ", `"`, " ").Replace(text)+")")
		params.Set("searchType", "bl")
	}
	for field, values := range q.Filters {
		vs := append([]string(nil), values...)
		sort.Strings(vs) // stable cache keys
		params["f_"+field] = vs
	}
	if q.Sort != "" {
		params.Set("sort", q.Sort)
	}
	if page > 1 {
		params.Set("page", strconv.Itoa(page))
	}

	var raw rawSearch
	if err := c.getJSON(ctx, "bibs/search", params, searchTTL, &raw); err != nil {
		return nil, err
	}
	p := raw.CatalogSearch.Pagination
	res := &SearchResult{Query: q, Page: p.Page, Pages: p.Pages, Total: p.Count}
	res.Facets = map[string][]Facet{}
	for _, f := range raw.CatalogSearch.Fields {
		for _, ff := range f.FieldFilters {
			res.Facets[f.ID] = append(res.Facets[f.ID], Facet{Value: ff.Value, Count: ff.Count})
		}
	}
	for _, r := range raw.CatalogSearch.Results {
		if b, ok := raw.Entities.Bibs[r.Representative]; ok {
			res.Bibs = append(res.Bibs, b.toBib())
		}
	}
	return res, nil
}

const (
	topRatedExact    = 50 // the top this many are guaranteed correctly ranked
	topRatedMaxPages = 10
)

// searchTopRated pages through the gateway's raw-average sort and re-ranks
// by Rating.Weighted. A title's weighted score always lies between its raw
// average and ratingPrior, so once both the last raw average seen and the
// prior fall below the topRatedExact-th best weighted score, no later title
// can make the cut and paging stops.
func (c *Client) searchTopRated(ctx context.Context, q Query) (*SearchResult, error) {
	raw := q.Clone()
	raw.Sort = SortRating
	var out *SearchResult
	var bibs []Bib
	for page := 1; page <= topRatedMaxPages; page++ {
		res, err := c.Search(ctx, raw, page)
		if err != nil {
			return nil, err
		}
		if out == nil {
			out = res
		}
		bibs = append(bibs, res.Bibs...)
		if page >= res.Pages || len(res.Bibs) == 0 {
			break
		}
		last := res.Bibs[len(res.Bibs)-1].Rating
		if last.Count == 0 {
			break // into the unrated titles
		}
		if s, ok := kthWeighted(bibs, topRatedExact); ok && math.Max(float64(last.Average), ratingPrior) < s {
			break
		}
	}
	sort.SliceStable(bibs, func(i, j int) bool {
		return bibs[i].Rating.Weighted() > bibs[j].Rating.Weighted()
	})
	out.Query, out.Bibs, out.Page, out.Pages = q, bibs, 1, 1
	out.Partial = len(bibs) < out.Total
	return out, nil
}

// kthWeighted returns the k-th highest weighted rating among bibs.
func kthWeighted(bibs []Bib, k int) (float64, bool) {
	if len(bibs) < k {
		return 0, false
	}
	ws := make([]float64, len(bibs))
	for i, b := range bibs {
		ws[i] = b.Rating.Weighted()
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(ws)))
	return ws[k-1], true
}

// Bibs looks up records by ID, in the order given. IDs the gateway doesn't
// return are skipped; an unknown ID can fail the whole request.
func (c *Client) Bibs(ctx context.Context, ids ...string) ([]Bib, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var raw struct {
		Entities struct {
			Bibs map[string]rawBib `json:"bibs"`
		} `json:"entities"`
	}
	params := url.Values{"metadataIds": {strings.Join(ids, ",")}}
	if err := c.getJSON(ctx, "bibs", params, searchTTL, &raw); err != nil {
		return nil, err
	}
	var out []Bib
	for _, id := range ids {
		if b, ok := raw.Entities.Bibs[id]; ok {
			out = append(out, b.toBib())
		}
	}
	return out, nil
}

// Availability returns per-copy availability for a bib ID from search results.
func (c *Client) Availability(ctx context.Context, bibID string) (*Availability, error) {
	var raw rawAvailability
	if err := c.getJSON(ctx, "bibs/"+url.PathEscape(bibID)+"/availability", nil, availabilityTTL, &raw); err != nil {
		return nil, err
	}
	a := &Availability{ID: bibID}
	if s, ok := raw.Entities.Availabilities[bibID]; ok {
		a.Summary = s.toSummary()
	}
	for _, f := range raw.Availability.DigitalFormats {
		a.DigitalFormats = append(a.DigitalFormats, f.Name)
	}
	for _, id := range raw.Availability.Items {
		if it, ok := raw.Entities.BibItems[id]; ok {
			a.Copies = append(a.Copies, it.toCopy())
		}
	}
	// On-shelf copies first, then soonest due, then by branch.
	sort.SliceStable(a.Copies, func(i, j int) bool {
		x, y := a.Copies[i], a.Copies[j]
		if x.Available != y.Available {
			return x.Available
		}
		if !x.Due.Equal(y.Due) {
			if x.Due.IsZero() || y.Due.IsZero() {
				return !x.Due.IsZero()
			}
			return x.Due.Before(y.Due)
		}
		return x.Branch < y.Branch
	})
	return a, nil
}

// Locations returns all branches, in the library's own order.
func (c *Client) Locations(ctx context.Context) ([]Location, error) {
	var out []Location
	pages := 1
	for page := 1; page <= pages; page++ {
		var raw rawLocations
		params := url.Values{"limit": {"100"}, "page": {strconv.Itoa(page)}}
		if err := c.getJSON(ctx, "locations", params, locationsTTL, &raw); err != nil {
			return nil, err
		}
		if page == 1 {
			pages = raw.Locations.Pagination.Pages
		}
		if pages > 1 && raw.Locations.Pagination.Page != page {
			return nil, fmt.Errorf("bibliocommons: requested locations page %d, got %d", page, raw.Locations.Pagination.Page)
		}
		for _, code := range raw.Locations.Results {
			if l, ok := raw.Entities.Locations[code]; ok && !l.IsHidden {
				out = append(out, l.toLocation())
			}
		}
	}
	return out, nil
}

// --- raw gateway shapes -----------------------------------------------------

type rawPagination struct {
	Count int `json:"count"`
	Page  int `json:"page"`
	Pages int `json:"pages"`
}

type rawSearch struct {
	CatalogSearch struct {
		Pagination rawPagination `json:"pagination"`
		Fields     []struct {
			ID           string `json:"id"`
			FieldFilters []struct {
				Value string `json:"value"`
				Count int    `json:"count"`
			} `json:"fieldFilters"`
		} `json:"fields"`
		Results []struct {
			Representative string   `json:"representative"`
			Manifestations []string `json:"manifestations"`
		} `json:"results"`
	} `json:"catalogSearch"`
	Entities struct {
		Bibs map[string]rawBib `json:"bibs"`
	} `json:"entities"`
}

type rawBib struct {
	ID        string `json:"id"`
	BriefInfo struct {
		Title       string   `json:"title"`
		Subtitle    string   `json:"subtitle"`
		Format      string   `json:"format"`
		CallNumber  string   `json:"callNumber"`
		Description string   `json:"description"`
		Authors     []string `json:"authors"`
		PubDate     string   `json:"publicationDate"`
		Edition     string   `json:"edition"`
		ISBNs       []string `json:"isbns"`
		Genres      []string `json:"genreForm"`
		Subjects    []string `json:"subjectHeadings"`
		Language    string   `json:"primaryLanguage"`
		Audiences   []string `json:"audiences"`
		ContentType string   `json:"contentType"`
		Rating      *struct {
			Average int `json:"averageRating"`
			Count   int `json:"totalCount"`
		} `json:"rating"`
		Series []struct {
			Name   string `json:"name"`
			Volume string `json:"volume"`
		} `json:"series"`
		Jacket struct {
			Medium string `json:"medium"`
		} `json:"jacket"`
	} `json:"briefInfo"`
	Policy struct {
		MaterialType string `json:"materialType"`
	} `json:"policy"`
	Availability rawSummary `json:"availability"`
}

func (b rawBib) toBib() Bib {
	bi := b.BriefInfo
	out := Bib{
		ID:          b.ID,
		Title:       bi.Title,
		Subtitle:    bi.Subtitle,
		Authors:     bi.Authors,
		Format:      bi.Format,
		Digital:     b.Policy.MaterialType == "DIGITAL" || b.Availability.BibType == "DIGITAL",
		Year:        bi.PubDate,
		Edition:     bi.Edition,
		CallNumber:  bi.CallNumber,
		Description: plainText(bi.Description),
		ISBNs:       bi.ISBNs,
		Genres:      bi.Genres,
		Subjects:    bi.Subjects,
		Language:    bi.Language,
		Audiences:   bi.Audiences,
		ContentType: bi.ContentType,
		CoverURL:    bi.Jacket.Medium,
		Summary:     b.Availability.toSummary(),
	}
	if bi.Rating != nil {
		out.Rating = Rating{Average: bi.Rating.Average, Count: bi.Rating.Count}
	}
	for _, s := range bi.Series {
		name := s.Name
		if s.Volume != "" {
			name += ", " + s.Volume
		}
		out.Series = append(out.Series, name)
	}
	return out
}

type rawSummary struct {
	BibType         string `json:"bibType"`
	Status          string `json:"status"`
	LibraryUseOnly  bool   `json:"libraryUseOnly"`
	HeldCopies      int    `json:"heldCopies"`
	AvailableCopies int    `json:"availableCopies"`
	TotalCopies     int    `json:"totalCopies"`
	OnOrderCopies   int    `json:"onOrderCopies"` // null for digital; decodes as 0
}

func (s rawSummary) toSummary() Summary {
	return Summary{
		Status:         s.Status,
		Available:      s.AvailableCopies,
		Total:          s.TotalCopies,
		Holds:          s.HeldCopies,
		OnOrder:        s.OnOrderCopies,
		LibraryUseOnly: s.LibraryUseOnly,
	}
}

type rawAvailability struct {
	Availability struct {
		DigitalFormats []struct {
			Name string `json:"name"`
		} `json:"digitalFormats"`
		Items []string `json:"items"`
	} `json:"availability"`
	Entities struct {
		Availabilities map[string]rawSummary `json:"availabilities"`
		BibItems       map[string]rawItem    `json:"bibItems"`
	} `json:"entities"`
}

type rawItem struct {
	ItemID     string  `json:"itemId"`
	Collection string  `json:"collection"`
	CallNumber string  `json:"callNumber"`
	Volume     *string `json:"volume"`
	DueDate    string  `json:"dueDate"`
	Branch     struct {
		Name string `json:"name"`
		Code string `json:"code"`
	} `json:"branch"`
	Availability struct {
		Status        string `json:"status"`
		LibraryStatus string `json:"libraryStatus"`
	} `json:"availability"`
}

func (it rawItem) toCopy() Copy {
	c := Copy{
		ItemID:        it.ItemID,
		BranchCode:    it.Branch.Code,
		Branch:        it.Branch.Name,
		Collection:    it.Collection,
		CallNumber:    it.CallNumber,
		Available:     it.Availability.Status == "AVAILABLE",
		Status:        it.Availability.Status,
		LibraryStatus: it.Availability.LibraryStatus,
	}
	if it.Volume != nil {
		c.Volume = *it.Volume
	}
	if it.DueDate != "" {
		if t, err := time.ParseInLocation("2006-01-02", it.DueDate, time.Local); err == nil {
			c.Due = t
		}
	}
	return c
}

type rawLocations struct {
	Locations struct {
		Results    []string      `json:"results"`
		Pagination rawPagination `json:"pagination"`
	} `json:"locations"`
	Entities struct {
		Locations map[string]rawLocation `json:"locations"`
	} `json:"entities"`
}

type rawLocation struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	IsHidden bool   `json:"isHidden"`
	WebURL   string `json:"webUrl"`
	Address  struct {
		Number string `json:"number"`
		Street string `json:"street"`
		City   string `json:"city"`
		State  string `json:"state"`
		Zip    string `json:"zip"`
	} `json:"address"`
	BranchContacts []struct {
		ContactType string `json:"contactType"`
		Value       string `json:"value"`
	} `json:"branchContacts"`
	SortedHours []struct {
		Day   string `json:"day"`
		Hours []struct {
			OpenTime  string `json:"openTime"`
			CloseTime string `json:"closeTime"`
		} `json:"hours"`
	} `json:"sortedHours"`
}

func (l rawLocation) toLocation() Location {
	a := l.Address
	street := strings.TrimSpace(strings.TrimSpace(a.Number) + " " + strings.TrimSpace(a.Street))
	out := Location{
		Code:    l.ID,
		Name:    l.Name,
		Address: strings.TrimSpace(fmt.Sprintf("%s, %s, %s %s", street, a.City, a.State, a.Zip)),
		WebURL:  l.WebURL,
	}
	for _, c := range l.BranchContacts {
		if c.ContactType == "phone" {
			out.Phone = c.Value
			break
		}
	}
	for _, d := range l.SortedHours {
		dh := DayHours{Day: d.Day, Closed: len(d.Hours) == 0}
		if !dh.Closed {
			// A day can list several ranges; span first open to last close.
			dh.Open = strings.TrimPrefix(d.Hours[0].OpenTime, "T")
			dh.Close = strings.TrimPrefix(d.Hours[len(d.Hours)-1].CloseTime, "T")
		}
		out.Hours = append(out.Hours, dh)
	}
	return out
}

// Details fetches trade reviews, the author bio and an excerpt for a bib.
func (c *Client) Details(ctx context.Context, bibID string) (*Details, error) {
	var raw rawEvaluation
	if err := c.getJSON(ctx, "bibs/"+url.PathEscape(bibID)+"/evaluation", nil, detailsTTL, &raw); err != nil {
		return nil, err
	}
	en := raw.Entities
	d := &Details{ID: bibID}
	if raw.AuthorNote.ID != nil {
		d.AuthorNote = plainText(en.AuthorNotes[raw.AuthorNote.ID.String()].Notes)
	}
	for _, id := range raw.Reviews.Items {
		r, ok := en.Reviews[id.String()]
		if !ok {
			continue
		}
		src := r.Source
		if src == "" {
			src = r.ProviderName
		}
		d.Reviews = append(d.Reviews, Review{Source: src, Text: plainText(r.Contents)})
	}
	if raw.Excerpt.ID != nil {
		var text string
		if json.Unmarshal(en.Excerpts[raw.Excerpt.ID.String()].Excerpt, &text) == nil {
			d.Excerpt = plainText(text)
		}
	}
	return d, nil
}

type rawRef struct {
	ID *json.Number `json:"id"`
}

type rawEvaluation struct {
	AuthorNote rawRef `json:"authorNote"`
	Excerpt    rawRef `json:"excerpt"`
	Reviews    struct {
		Items []json.Number `json:"items"`
	} `json:"reviews"`
	Entities struct {
		AuthorNotes map[string]struct {
			Notes string `json:"notes"`
		} `json:"authorNotes"`
		Reviews map[string]struct {
			Source       string `json:"source"`
			ProviderName string `json:"providerName"`
			Contents     string `json:"contents"`
		} `json:"reviews"`
		Excerpts map[string]struct {
			// A string when present; an {"id": null} placeholder otherwise.
			Excerpt json.RawMessage `json:"excerpt"`
		} `json:"excerpts"`
	} `json:"entities"`
}

var (
	breakTags  = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/li)\s*/?>`)
	anyTag     = regexp.MustCompile(`<[^>]*>`)
	spaceRuns  = regexp.MustCompile(`[ \t\r\f\v]+`)
	blankLines = regexp.MustCompile(`\n\s*\n+`)
)

// plainText turns the gateway's HTML snippets into terminal-friendly text,
// keeping paragraph breaks.
func plainText(s string) string {
	s = breakTags.ReplaceAllString(s, "\n\n")
	s = anyTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = spaceRuns.ReplaceAllString(s, " ")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	s = blankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")
	return strings.TrimSpace(s)
}
