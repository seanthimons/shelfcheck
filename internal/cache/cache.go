// Package cache stores disposable API responses across runs. Cache failures
// fall back to the network; credentials and failed responses aren't stored.
package cache

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
	_ "modernc.org/sqlite"
)

type Store struct {
	db      *sql.DB
	flights singleflight.Group
}

var defaultOnce sync.Once
var defaultStore *Store

func Default() *Store {
	defaultOnce.Do(func() {
		dir, err := os.UserCacheDir()
		if err == nil {
			defaultStore, _ = Open(filepath.Join(dir, "shelfcheck", "cache.sqlite"))
		}
	})
	return defaultStore
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: abs}
	q := url.Values{"_pragma": {"busy_timeout(2000)"}}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{
		`PRAGMA journal_mode=WAL`,
		`CREATE TABLE IF NOT EXISTS responses_v1 (scope TEXT NOT NULL, key TEXT NOT NULL, body BLOB NOT NULL, expires INTEGER NOT NULL, stored INTEGER NOT NULL, PRIMARY KEY(scope,key))`,
		`CREATE TABLE IF NOT EXISTS generations_v1 (scope TEXT PRIMARY KEY, generation INTEGER NOT NULL)`,
	} {
		if _, err = db.Exec(stmt); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Fetch returns a fresh cached response or coalesces identical requests.
// The loader supplies its TTL so negative matches can expire sooner.
func (s *Store) Fetch(ctx context.Context, scope, key string, load func() ([]byte, time.Duration, error)) ([]byte, error) {
	if s == nil {
		body, _, err := load()
		return body, err
	}
	read := func() ([]byte, bool) {
		var body []byte
		err := s.db.QueryRowContext(ctx, `SELECT body FROM responses_v1 WHERE scope=? AND key=? AND expires>?`, scope, key, time.Now().UnixNano()).Scan(&body)
		return body, err == nil
	}
	if body, ok := read(); ok {
		return body, nil
	}
	var generation int64
	_ = s.db.QueryRowContext(ctx, `SELECT generation FROM generations_v1 WHERE scope=?`, scope).Scan(&generation)
	result := s.flights.DoChan(scope+"\x00"+key+"\x00"+strconv.FormatInt(generation, 10), func() (any, error) {
		if body, ok := read(); ok {
			return body, nil
		}
		body, ttl, err := load()
		if err != nil {
			return nil, err
		}
		if ttl > 0 {
			now := time.Now()
			// An invalidation in another client/process prevents an older in-flight
			// response from repopulating the cache after a write.
			_, _ = s.db.ExecContext(ctx, `INSERT INTO responses_v1(scope,key,body,expires,stored) SELECT ?,?,?,?,? WHERE COALESCE((SELECT generation FROM generations_v1 WHERE scope=?),0)=? ON CONFLICT(scope,key) DO UPDATE SET body=excluded.body,expires=excluded.expires,stored=excluded.stored`, scope, key, body, now.Add(ttl).UnixNano(), now.UnixNano(), scope, generation)
			_, _ = s.db.ExecContext(ctx, `DELETE FROM responses_v1 WHERE expires<=? OR rowid IN (SELECT rowid FROM (SELECT rowid, SUM(length(body)) OVER (ORDER BY stored DESC) AS bytes, ROW_NUMBER() OVER (ORDER BY stored DESC) AS n FROM responses_v1) WHERE bytes>67108864 OR n>2000)`, now.UnixNano())
		}
		return body, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-result:
		if r.Err != nil {
			return nil, r.Err
		}
		return r.Val.([]byte), nil
	}
}

func (s *Store) Invalidate(ctx context.Context, scope string) {
	if s == nil {
		return
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO generations_v1(scope,generation) VALUES(?,1) ON CONFLICT(scope) DO UPDATE SET generation=generation+1`, scope); err != nil {
		return
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM responses_v1 WHERE scope=?`, scope); err != nil {
		return
	}
	_ = tx.Commit()
}
