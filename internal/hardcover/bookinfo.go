package hardcover

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
)

// BookInfo is a book's metadata on Hardcover, for books the library
// doesn't have or describes thinly.
type BookInfo struct {
	Book
	Subtitle    string
	Description string
	Authors     []string // authors only, not illustrators or translators
	Year        int
	Pages       int
	Rating      float64 // average stars, 0-5
	Ratings     int
	ReadBy      int      // readers who've read it
	Genres      []string // most-tagged first
	Moods       []string
	CoverURL    string // its cover image, if it has one
}

// HasRating reports whether readers have rated it.
func (b BookInfo) HasRating() bool { return b.Ratings > 0 && b.Rating > 0 }

const bookInfoQuery = `query($id: Int!) {
  books_by_pk(id: $id) {
    ` + bookFields + `
    subtitle description release_year pages rating ratings_count users_read_count cached_tags cached_image
    authors: contributions(where: {_or: [{contribution: {_is_null: true}}, {contribution: {_eq: "Author"}}]}) { author { name } }
  }
}`

// Info fetches a book's metadata by Hardcover ID; ok is false if there's
// no such book.
func (c *Client) loadInfo(ctx context.Context, id int) (BookInfo, bool, error) {
	var raw struct {
		Book *struct {
			rawBook
			Subtitle    *string                    `json:"subtitle"`
			Description *string                    `json:"description"`
			Year        *int                       `json:"release_year"`
			Pages       *int                       `json:"pages"`
			Rating      *float64                   `json:"rating"`
			Ratings     int                        `json:"ratings_count"`
			ReadBy      int                        `json:"users_read_count"`
			Tags        map[string]json.RawMessage `json:"cached_tags"`
			Image       *struct {
				URL string `json:"url"`
			} `json:"cached_image"`
			Authors []struct {
				Author struct {
					Name string `json:"name"`
				} `json:"author"`
			} `json:"authors"`
		} `json:"books_by_pk"`
	}
	if err := c.Query(ctx, bookInfoQuery, map[string]any{"id": id}, &raw); err != nil {
		return BookInfo{}, false, err
	}
	r := raw.Book
	if r == nil {
		return BookInfo{}, false, nil
	}
	info := BookInfo{Book: r.rawBook.book(), Ratings: r.Ratings, ReadBy: r.ReadBy}
	deref := func(s *string) string {
		if s == nil {
			return ""
		}
		return strings.TrimSpace(*s)
	}
	info.Subtitle, info.Description = deref(r.Subtitle), deref(r.Description)
	if r.Image != nil {
		info.CoverURL = r.Image.URL
	}
	if r.Year != nil {
		info.Year = *r.Year
	}
	if r.Pages != nil {
		info.Pages = *r.Pages
	}
	if r.Rating != nil {
		info.Rating = *r.Rating
	}
	for _, a := range r.Authors {
		info.Authors = append(info.Authors, a.Author.Name)
	}
	info.Genres = topTags(r.Tags["Genre"], 6)
	info.Moods = topTags(r.Tags["Mood"], 5)
	return info, true, nil
}

// topTags returns up to n of a cached_tags category's tags, most used first.
func topTags(raw json.RawMessage, n int) []string {
	var tags []struct {
		Tag   string `json:"tag"`
		Count int    `json:"count"`
	}
	if json.Unmarshal(raw, &tags) != nil {
		return nil
	}
	sort.SliceStable(tags, func(i, j int) bool { return tags[i].Count > tags[j].Count })
	var out []string
	for _, t := range tags {
		if len(out) == n {
			break
		}
		if t.Tag != "" {
			out = append(out, t.Tag)
		}
	}
	return out
}

// Lookup finds a book's metadata by title and author: the most-read match.
func (c *Client) Lookup(ctx context.Context, title, author string) (BookInfo, bool, error) {
	b, ok, err := c.FindBook(ctx, title, author)
	if err != nil || !ok {
		return BookInfo{}, ok, err
	}
	return c.Info(ctx, b.ID)
}
