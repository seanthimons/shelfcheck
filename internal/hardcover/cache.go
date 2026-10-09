package hardcover

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"shelfcheck/internal/cache"
)

func (c *Client) diskCache() *cache.Store {
	if c.Disk != nil {
		return c.Disk
	}
	if c.Endpoint == Endpoint {
		return cache.Default()
	}
	return nil
}

func (c *Client) cacheScope() string {
	// Include the endpoint and a token fingerprint; private snapshots never
	// cross credentials, and no token is written to the cache database.
	return fmt.Sprintf("hardcover:%x", sha256.Sum256([]byte(c.Endpoint+"\x00"+c.Token)))
}

func cachedResult[T any](ctx context.Context, c *Client, key string, ttl time.Duration, load func() (T, bool, error)) (T, bool, error) {
	type result struct {
		Value T
		Found bool
	}
	var out result
	body, err := c.diskCache().Fetch(ctx, c.cacheScope(), key, func() ([]byte, time.Duration, error) {
		value, found, err := load()
		if err != nil {
			return nil, 0, err
		}
		if !found {
			ttl = 10 * time.Minute
		}
		body, err := json.Marshal(result{Value: value, Found: found})
		return body, ttl, err
	})
	if err == nil {
		err = json.Unmarshal(body, &out)
	}
	return out.Value, out.Found, err
}

// Info caches public metadata for a day across runs.
func (c *Client) Info(ctx context.Context, id int) (BookInfo, bool, error) {
	return cachedResult(ctx, c, fmt.Sprintf("info:%d", id), 24*time.Hour, func() (BookInfo, bool, error) { return c.loadInfo(ctx, id) })
}

// FindBook caches successful catalog-to-Hardcover matches for a week.
func (c *Client) FindBook(ctx context.Context, title, author string) (Book, bool, error) {
	key, _ := json.Marshal([]string{normName(shortTitle(title)), lastName(author)})
	return cachedResult(ctx, c, "match:"+string(key), 7*24*time.Hour, func() (Book, bool, error) { return c.loadFindBook(ctx, title, author) })
}

// Library caches complete private snapshots for five minutes. Editor reads
// and mutation verification use UserBook, which always goes to the server.
func (c *Client) Library(ctx context.Context) ([]UserBook, error) {
	lib, _, err := cachedResult(ctx, c, "library", 5*time.Minute, func() ([]UserBook, bool, error) { lib, err := c.loadLibrary(ctx); return lib, true, err })
	return lib, err
}

// RefreshMetadata explicitly bypasses metadata/match caches for a user retry.
func (c *Client) RefreshMetadata(ctx context.Context) {
	c.diskCache().Invalidate(ctx, c.cacheScope())
}
