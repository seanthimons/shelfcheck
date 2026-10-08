// Package hardcover is a small read-only client for the Hardcover GraphQL
// API (https://api.hardcover.app/v1/graphql), used for series order and
// community book data.
package hardcover

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const Endpoint = "https://api.hardcover.app/v1/graphql"

// ErrUnauthorized means the token was rejected.
var ErrUnauthorized = errors.New("hardcover: token rejected; check HARDCOVER_TOKEN")

type Client struct {
	Token    string
	Endpoint string
	HTTP     *http.Client
	// CacheDir, if set, keeps AlsoPicked results for a week: they change
	// slowly, and Hardcover's rate limit makes them slow to gather.
	CacheDir string

	throttle sync.Mutex
	last     time.Time
}

func New(token string) *Client {
	token = strings.TrimSpace(token)
	if token != "" && !strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = "Bearer " + token
	}
	return &Client{Token: token, Endpoint: Endpoint, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Hardcover allows about 60 requests a minute.
const minInterval = time.Second

// Query runs a GraphQL query and decodes its data into out. When Hardcover
// says to slow down, it waits as told and tries again, a few times.
func (c *Client) Query(ctx context.Context, query string, vars map[string]any, out any) error {
	for attempt := 0; ; attempt++ {
		err := c.query(ctx, query, vars, out)
		var rl *rateLimited
		if !errors.As(err, &rl) || attempt >= 4 {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(rl.wait):
		}
	}
}

// rateLimited is an HTTP 429 and how long to wait.
type rateLimited struct{ wait time.Duration }

func (r *rateLimited) Error() string {
	return fmt.Sprintf("hardcover: rate limited; retry in %v", r.wait)
}

var retryInRe = regexp.MustCompile(`(?i)try again in (\d+) seconds?`)

func retryAfter(resp *http.Response, body []byte) time.Duration {
	secs := 2
	if n, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
		secs = n
	} else if m := retryInRe.FindSubmatch(body); m != nil {
		secs, _ = strconv.Atoi(string(m[1]))
	}
	return time.Duration(min(max(secs, 1), 60))*time.Second + 250*time.Millisecond
}

func (c *Client) query(ctx context.Context, query string, vars map[string]any, out any) error {
	c.throttle.Lock()
	if d := time.Until(c.last.Add(minInterval)); d > 0 {
		select {
		case <-ctx.Done():
			c.throttle.Unlock()
			return ctx.Err()
		case <-time.After(d):
		}
	}
	c.last = time.Now()
	c.throttle.Unlock()

	body, _ := json.Marshal(map[string]any{"query": query, "variables": vars})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "shelfcheck/0.1 (personal)")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("hardcover: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		wait := retryAfter(resp, b)
		c.throttle.Lock()
		c.last = time.Now().Add(wait - minInterval) // hold back other callers too
		c.throttle.Unlock()
		return &rateLimited{wait}
	}
	if resp.StatusCode/100 != 2 {
		// Hardcover explains refusals (over capacity, bad token) in
		// "message".
		var e struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(b, &e) == nil && e.Message != "" {
			if strings.Contains(strings.ToLower(e.Message), "token") || strings.Contains(strings.ToLower(e.Message), "jwt") {
				return ErrUnauthorized
			}
			return fmt.Errorf("hardcover: HTTP %d: %s", resp.StatusCode, e.Message)
		}
		if resp.StatusCode == http.StatusForbidden {
			return ErrUnauthorized
		}
		return fmt.Errorf("hardcover: HTTP %d: %.200s", resp.StatusCode, b)
	}
	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return fmt.Errorf("hardcover: decoding: %w", err)
	}
	if len(env.Errors) > 0 {
		if strings.Contains(strings.ToLower(env.Errors[0].Message), "jwt") {
			return ErrUnauthorized
		}
		return fmt.Errorf("hardcover: %s", env.Errors[0].Message)
	}
	return json.Unmarshal(env.Data, out)
}

// SeriesBook is one book's place in a series.
type SeriesBook struct {
	Position float64
	Title    string
	Author   string
	Readers  int // Hardcover users who have it
}

// Series is a series in reading order.
type Series struct {
	ID    int
	Name  string
	Books []SeriesBook
}

// Next returns the first whole-numbered book after position after.
func (s Series) Next(after float64) (SeriesBook, bool) {
	for _, b := range s.Books {
		if b.Position > after && b.Position == math.Trunc(b.Position) {
			return b, true
		}
	}
	return SeriesBook{}, false
}

const seriesSearchQuery = `query($q: String!) {
  search(query: $q, query_type: "Series", per_page: 5, page: 1) { results }
}`

const seriesByIDQuery = `query($id: Int!) {
  series_by_pk(id: $id) {
    id name
    book_series(where: {compilation: {_eq: false}, position: {_is_null: false}}, order_by: {position: asc}, limit: 100) {
      position
      book { title users_count contributions(limit: 1) { author { name } } }
    }
  }
}`

// FindSeries looks a series up by name (via Hardcover's search) and returns
// its books in order, one per position (the most read edition of each),
// without compilations. author, if given, picks between same-named series.
// ok is false if nothing matched.
func (c *Client) FindSeries(ctx context.Context, name, author string) (Series, bool, error) {
	var found struct {
		Search struct {
			Results struct {
				Hits []struct {
					Document struct {
						ID           json.Number `json:"id"`
						Name         string      `json:"name"`
						AuthorName   string      `json:"author_name"`
						BooksCount   int         `json:"books_count"`
						ReadersCount int         `json:"readers_count"`
					} `json:"document"`
				} `json:"hits"`
			} `json:"results"`
		} `json:"search"`
	}
	if err := c.Query(ctx, seriesSearchQuery, map[string]any{"q": name}, &found); err != nil {
		return Series{}, false, err
	}
	authorLast := lastName(author)
	want := normName(name)
	bestID, bestScore := "", -1
	for _, h := range found.Search.Results.Hits {
		d := h.Document
		if d.BooksCount == 0 {
			continue
		}
		// Same name beats same author ("The Expanse" over "The Expanse
		// Graphic Novels"), which beats popularity.
		score := d.ReadersCount
		if d.ReadersCount == 0 {
			score -= 1 << 30 // stray duplicates and translations
		}
		if authorLast != "" && strings.Contains(strings.ToLower(d.AuthorName), authorLast) {
			score += 1 << 28
		}
		if normName(d.Name) == want {
			score += 1 << 29
		}
		if score > bestScore {
			bestID, bestScore = d.ID.String(), score
		}
	}
	if bestID == "" {
		return Series{}, false, nil
	}
	id, err := strconv.Atoi(bestID)
	if err != nil {
		return Series{}, false, fmt.Errorf("hardcover: series id %q: %w", bestID, err)
	}
	var raw struct {
		Series *struct {
			ID         int    `json:"id"`
			Name       string `json:"name"`
			BookSeries []struct {
				Position float64 `json:"position"`
				Book     struct {
					Title         string `json:"title"`
					UsersCount    int    `json:"users_count"`
					Contributions []struct {
						Author struct {
							Name string `json:"name"`
						} `json:"author"`
					} `json:"contributions"`
				} `json:"book"`
			} `json:"book_series"`
		} `json:"series_by_pk"`
	}
	if err := c.Query(ctx, seriesByIDQuery, map[string]any{"id": id}, &raw); err != nil {
		return Series{}, false, err
	}
	if raw.Series == nil {
		return Series{}, false, nil
	}
	out := Series{ID: raw.Series.ID, Name: raw.Series.Name}
	byPos := map[float64]SeriesBook{}
	for _, bs := range raw.Series.BookSeries {
		b := SeriesBook{Position: bs.Position, Title: bs.Book.Title, Readers: bs.Book.UsersCount}
		if len(bs.Book.Contributions) > 0 {
			b.Author = bs.Book.Contributions[0].Author.Name
		}
		if old, ok := byPos[b.Position]; !ok || b.Readers > old.Readers {
			byPos[b.Position] = b
		}
	}
	for _, b := range byPos {
		out.Books = append(out.Books, b)
	}
	sort.Slice(out.Books, func(i, j int) bool { return out.Books[i].Position < out.Books[j].Position })
	return out, true, nil
}

// normName lowercases a series name and drops a leading "the" and trailing
// "series"/"trilogy" for comparison.
func normName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "the ")
	for _, suf := range []string{" series", " trilogy", " saga", " cycle"} {
		s = strings.TrimSuffix(s, suf)
	}
	return strings.Join(strings.Fields(s), " ")
}

// lastName lowercases the last name from "Last, First" or "First Last".
func lastName(a string) string {
	a = strings.TrimSpace(a)
	if last, _, ok := strings.Cut(a, ","); ok {
		return strings.ToLower(strings.TrimSpace(last))
	}
	f := strings.Fields(a)
	if len(f) == 0 {
		return ""
	}
	return strings.ToLower(f[len(f)-1])
}
