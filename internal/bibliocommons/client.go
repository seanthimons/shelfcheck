// Package bibliocommons is a small read-only client for the BiblioCommons
// gateway API (the undocumented JSON API behind *.bibliocommons.com catalogs).
//
// Everything this project knows about that API lives in this package, so if
// BiblioCommons changes it, this is the only place that needs fixing.
package bibliocommons

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	DefaultBaseURL   = "https://gateway.bibliocommons.com/v2/libraries"
	DefaultUserAgent = "shelfcheck/0.1 (personal catalog browser)"

	searchTTL       = 5 * time.Minute
	availabilityTTL = 1 * time.Minute
	locationsTTL    = 24 * time.Hour
	detailsTTL      = 24 * time.Hour

	// minInterval spaces out requests that miss the cache so we stay a
	// light, polite client of an API we don't own.
	minInterval = 250 * time.Millisecond
)

// Client talks to one library's catalog. It is safe for concurrent use.
type Client struct {
	BaseURL   string
	Site      string // catalog website for logins; defaults to https://<Library>.bibliocommons.com
	Library   string // e.g. "cincinnatilibrary"
	UserAgent string
	HTTP      *http.Client

	mu    sync.Mutex
	cache map[string]cacheEntry

	throttle sync.Mutex
	last     time.Time
}

type cacheEntry struct {
	body    []byte
	expires time.Time
}

// New returns a client for the given library slug (the subdomain of its
// bibliocommons.com catalog).
func New(library string) *Client {
	return &Client{
		BaseURL:   DefaultBaseURL,
		Library:   library,
		UserAgent: DefaultUserAgent,
		HTTP:      &http.Client{Timeout: 15 * time.Second},
		cache:     map[string]cacheEntry{},
	}
}

// APIError is returned for non-2xx responses.
type APIError struct {
	Status int
	URL    string
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("bibliocommons: %s returned %d: %s", e.URL, e.Status, e.Body)
}

// getJSON fetches path (relative to the library root) with query params and
// decodes the response into out, serving from cache when fresh.
func (c *Client) getJSON(ctx context.Context, path string, params url.Values, ttl time.Duration, out any) error {
	u := c.gatewayURL(path, params)
	if body, ok := c.cached(u); ok {
		return json.Unmarshal(body, out)
	}
	body, err := c.fetch(ctx, u, nil)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("bibliocommons: decoding %s: %w", u, err)
	}
	c.store(u, body, ttl)
	return nil
}

func (c *Client) gatewayURL(path string, params url.Values) string {
	if params == nil {
		params = url.Values{}
	}
	params.Set("locale", "en-US")
	return fmt.Sprintf("%s/%s/%s?%s", c.BaseURL, url.PathEscape(c.Library), path, params.Encode())
}

// fetch GETs u (throttled, uncached) with optional extra headers and returns
// the body of a 2xx response.
func (c *Client) fetch(ctx context.Context, u string, header http.Header) ([]byte, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bibliocommons: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("bibliocommons: reading %s: %w", u, err)
	}
	if resp.StatusCode/100 != 2 {
		snippet := string(body)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, &APIError{Status: resp.StatusCode, URL: u, Body: snippet}
	}
	return body, nil
}

func (c *Client) cached(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[key]
	if !ok || time.Now().After(e.expires) {
		delete(c.cache, key)
		return nil, false
	}
	return e.body, true
}

func (c *Client) store(key string, body []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cache == nil {
		c.cache = map[string]cacheEntry{}
	}
	c.cache[key] = cacheEntry{body: body, expires: time.Now().Add(ttl)}
}

// wait blocks until at least minInterval has passed since the previous request.
func (c *Client) wait(ctx context.Context) error {
	c.throttle.Lock()
	defer c.throttle.Unlock()
	if d := time.Until(c.last.Add(minInterval)); d > 0 {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
	c.last = time.Now()
	return nil
}
