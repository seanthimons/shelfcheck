// Package libby is a small client for Libby's private, undocumented API
// (OverDrive's "sentry" account service and "vandal" tag service), used to
// read a patron's Libby loans, holds and tags.
//
// Access comes from linking this client as another device with an 8-digit
// "Copy To Another Device" code from the Libby app. The resulting identity
// token can act on the whole Libby account; this package only ever reads,
// apart from the link itself.
package libby

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	SentryURL  = "https://sentry.libbyapp.com"
	VandalURL  = "https://vandal.libbyapp.com"
	ThunderURL = "https://thunder.api.overdrive.com"

	// Libby's API turns away clients that don't look like the web app.
	userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
)

// ErrNotLinked means the saved identity no longer works; link again.
var ErrNotLinked = errors.New("libby: not linked (or the link was revoked); run shelfcheck libby link with a new code")

type Client struct {
	Identity string // bearer token from Link
	HTTP     *http.Client
	Sentry   string
	Vandal   string
	Thunder  string
}

func New(identity string) *Client {
	return &Client{
		Identity: identity,
		HTTP:     &http.Client{Timeout: 30 * time.Second},
		Sentry:   SentryURL, Vandal: VandalURL, Thunder: ThunderURL,
	}
}

type APIError struct {
	Status int
	URL    string
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("libby: %s returned %d: %s", e.URL, e.Status, e.Body)
}

// do sends a request and decodes JSON into out (if non-nil).
func (c *Client) do(ctx context.Context, method, u string, body any, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://libbyapp.com/")
	req.Header.Set("Origin", "https://libbyapp.com")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Identity != "" {
		req.Header.Set("Authorization", "Bearer "+c.Identity)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("libby: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrNotLinked
	}
	if resp.StatusCode/100 != 2 {
		s := string(b)
		if len(s) > 200 {
			s = s[:200]
		}
		return &APIError{Status: resp.StatusCode, URL: u, Body: s}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("libby: decoding %s: %w", u, err)
	}
	return nil
}

// GetRaw fetches u and returns the raw JSON (for inspection and saving).
func (c *Client) GetRaw(ctx context.Context, u string) (json.RawMessage, error) {
	var raw json.RawMessage
	err := c.do(ctx, http.MethodGet, u, nil, &raw)
	return raw, err
}

var codeRe = regexp.MustCompile(`^\d{8}$`)

// Link turns an 8-digit "Copy To Another Device" code into an identity
// token: get a fresh device identity, attach the code to it, then fetch the
// identity again now that it's signed in. It returns the new token and also
// stores it on c.
func (c *Client) Link(ctx context.Context, code string) (string, error) {
	if !codeRe.MatchString(code) {
		return "", errors.New("libby: the code should be 8 digits")
	}
	var chip struct {
		Identity string `json:"identity"`
	}
	c.Identity = ""
	if err := c.do(ctx, http.MethodPost, c.Sentry+"/chip?client=dewey", nil, &chip); err != nil {
		return "", err
	}
	if chip.Identity == "" {
		return "", errors.New("libby: no identity from /chip")
	}
	c.Identity = chip.Identity

	var clone struct {
		Result string `json:"result"`
	}
	if err := c.do(ctx, http.MethodPost, c.Sentry+"/chip/clone/code", map[string]string{"code": code}, &clone); err != nil {
		return "", err
	}
	if clone.Result != "cloned" {
		return "", fmt.Errorf("libby: link failed (%q); codes expire quickly, so get a fresh one and try again", clone.Result)
	}

	chip.Identity = ""
	if err := c.do(ctx, http.MethodPost, c.Sentry+"/chip?client=dewey", nil, &chip); err != nil {
		return "", err
	}
	if chip.Identity == "" {
		return "", errors.New("libby: no identity after linking")
	}
	c.Identity = chip.Identity
	return c.Identity, nil
}

// SyncURL is the account state: cards, loans and holds.
func (c *Client) SyncURL() string { return c.Sentry + "/chip/sync" }

// TagsURL lists the patron's tags.
func (c *Client) TagsURL() string { return c.Vandal + "/tags" }

// TagURL lists titles [start, end) of one tag.
func (c *Client) TagURL(id, name string, start, end int) string {
	enc := url.PathEscape(base64.StdEncoding.EncodeToString([]byte(name)))
	q := url.Values{"enc": {"1"}, "sort": {"newest"}, "range": {fmt.Sprintf("%d...%d", start, end)}}
	return fmt.Sprintf("%s/tag/%s/%s?%s", c.Vandal, url.PathEscape(id), enc, q.Encode())
}

// --- tags ---------------------------------------------------------------------

// Tag is one of the patron's Libby tags. Kind is set for Libby's automatic
// tags: "borrowed" (🧾), "sampled" (🍰) or "notify-me" (🔔).
type Tag struct {
	Name        string
	UUID        string
	Description string
	Kind        string
	Total       int
}

// Tagging is a title in a tag.
type Tagging struct {
	TitleID  string // Libby/OverDrive title ID
	Title    string
	Author   string
	Format   string // "book", "audiobook", "magazine"
	Subjects []string
	Tagged   time.Time // when it was tagged; for "borrowed", roughly the borrow date
}

// Tags lists the patron's tags.
func (c *Client) Tags(ctx context.Context) ([]Tag, error) {
	var raw struct {
		Tags []struct {
			Name          string                     `json:"name"`
			UUID          string                     `json:"uuid"`
			Description   string                     `json:"description"`
			TotalTaggings int                        `json:"totalTaggings"`
			Behaviors     map[string]json.RawMessage `json:"behaviors"`
		} `json:"tags"`
	}
	if err := c.do(ctx, http.MethodGet, c.TagsURL(), nil, &raw); err != nil {
		return nil, err
	}
	var out []Tag
	for _, t := range raw.Tags {
		tag := Tag{Name: t.Name, UUID: t.UUID, Description: t.Description, Total: t.TotalTaggings}
		for _, k := range []string{"borrowed", "sampled", "notify-me"} {
			if _, ok := t.Behaviors[k]; ok {
				tag.Kind = k
			}
		}
		out = append(out, tag)
	}
	return out, nil
}

const tagPage = 50

// Taggings returns every title in a tag, newest first.
func (c *Client) Taggings(ctx context.Context, t Tag) ([]Tagging, error) {
	var out []Tagging
	for start := 0; start < max(t.Total, 1) && start < 5000; start += tagPage {
		var raw struct {
			Tag struct {
				Taggings []struct {
					TitleID       string      `json:"titleId"`
					SortTitle     string      `json:"sortTitle"`
					SortAuthor    string      `json:"sortAuthor"`
					TitleFormat   string      `json:"titleFormat"`
					CreateTime    json.Number `json:"createTime"`
					TitleSubjects []struct {
						Name string `json:"name"`
					} `json:"titleSubjects"`
				} `json:"taggings"`
			} `json:"tag"`
		}
		if err := c.do(ctx, http.MethodGet, c.TagURL(t.UUID, t.Name, start, start+tagPage), nil, &raw); err != nil {
			return out, err
		}
		for _, x := range raw.Tag.Taggings {
			tg := Tagging{TitleID: x.TitleID, Title: x.SortTitle, Author: x.SortAuthor, Format: x.TitleFormat}
			for _, s := range x.TitleSubjects {
				tg.Subjects = append(tg.Subjects, s.Name)
			}
			if ms, err := x.CreateTime.Int64(); err == nil {
				tg.Tagged = time.UnixMilli(ms)
			}
			out = append(out, tg)
		}
		if len(raw.Tag.Taggings) < tagPage {
			break
		}
		time.Sleep(250 * time.Millisecond) // be gentle with a private API
	}
	return out, nil
}

// --- saved identity -----------------------------------------------------------

type Saved struct {
	Identity string    `json:"identity"`
	Linked   time.Time `json:"linked"`
}

func Save(path string, s Saved) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func Load(path string) (Saved, error) {
	var s Saved
	b, err := os.ReadFile(path)
	if err != nil {
		return s, fmt.Errorf("%w (%v)", ErrNotLinked, err)
	}
	err = json.Unmarshal(b, &s)
	return s, err
}

// --- catalog media (thunder) --------------------------------------------------

// Media is a Libby/OverDrive catalog record.
type Media struct {
	ID          string
	Title       string
	Subtitle    string
	Authors     []string
	Type        string // "ebook", "audiobook", "magazine"
	Series      string
	SeriesOrder string // reading order within Series, e.g. "8"; may be empty
	Subjects    []string
	ISBN        string
	PublishYear string
	Publisher   string
	Available   bool
	Copies      int
	Holds       int
}

const mediaBatch = 25

// Media looks up titles in a library's Libby collection (by the library's
// advantage key, e.g. "cincinnatilibrary"), in batches. It needs no login.
func (c *Client) Media(ctx context.Context, library string, ids []string) (map[string]Media, error) {
	out := map[string]Media{}
	for start := 0; start < len(ids); start += mediaBatch {
		batch := ids[start:min(start+mediaBatch, len(ids))]
		u := fmt.Sprintf("%s/v2/libraries/%s/media/bulk?%s", c.Thunder, url.PathEscape(library),
			url.Values{"titleIds": {strings.Join(batch, ",")}, "x-client-id": {"dewey"}}.Encode())
		var raw struct {
			Items []struct {
				ID       string `json:"id"`
				Title    string `json:"title"`
				Subtitle string `json:"subtitle"`
				Type     struct {
					ID string `json:"id"`
				} `json:"type"`
				Creators []struct {
					Name string `json:"name"`
					Role string `json:"role"`
				} `json:"creators"`
				Series         string `json:"series"`
				DetailedSeries struct {
					ReadingOrder string `json:"readingOrder"`
				} `json:"detailedSeries"`
				Subjects []struct {
					Name string `json:"name"`
				} `json:"subjects"`
				Formats []struct {
					Identifiers []struct {
						Type  string `json:"type"`
						Value string `json:"value"`
					} `json:"identifiers"`
				} `json:"formats"`
				PublishDate string `json:"publishDate"`
				Publisher   struct {
					Name string `json:"name"`
				} `json:"publisher"`
				IsAvailable bool `json:"isAvailable"`
				OwnedCopies int  `json:"ownedCopies"`
				HoldsCount  int  `json:"holdsCount"`
			} `json:"items"`
		}
		if err := c.do(ctx, http.MethodGet, u, nil, &raw); err != nil {
			return out, err
		}
		for _, it := range raw.Items {
			m := Media{
				ID: it.ID, Title: it.Title, Subtitle: it.Subtitle, Type: it.Type.ID,
				Series: it.Series, SeriesOrder: it.DetailedSeries.ReadingOrder,
				Publisher: it.Publisher.Name, Available: it.IsAvailable, Copies: it.OwnedCopies, Holds: it.HoldsCount,
			}
			for _, cr := range it.Creators {
				if cr.Role == "Author" || cr.Role == "" {
					m.Authors = append(m.Authors, cr.Name)
				}
			}
			for _, s := range it.Subjects {
				m.Subjects = append(m.Subjects, s.Name)
			}
		isbn:
			for _, f := range it.Formats {
				for _, id := range f.Identifiers {
					if id.Type == "ISBN" && id.Value != "" {
						m.ISBN = id.Value
						break isbn
					}
				}
			}
			if len(it.PublishDate) >= 4 {
				m.PublishYear = it.PublishDate[:4]
			}
			out[it.ID] = m
		}
		if start+mediaBatch < len(ids) {
			time.Sleep(250 * time.Millisecond)
		}
	}
	return out, nil
}
