package bibliocommons

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
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
	var raw rawLocations
	params := url.Values{"limit": {"100"}}
	if err := c.getJSON(ctx, "locations", params, locationsTTL, &raw); err != nil {
		return nil, err
	}
	if p := raw.Locations.Pagination; p.Pages > 1 {
		return nil, fmt.Errorf("bibliocommons: %d locations span %d pages; paging not implemented", p.Count, p.Pages)
	}
	var out []Location
	for _, code := range raw.Locations.Results {
		if l, ok := raw.Entities.Locations[code]; ok && !l.IsHidden {
			out = append(out, l.toLocation())
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
