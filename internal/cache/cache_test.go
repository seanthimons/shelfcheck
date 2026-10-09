package cache

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func openTest(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPersistenceExpiryAndScopes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	calls := 0
	loader := func() ([]byte, time.Duration, error) { calls++; return []byte("cached"), time.Hour, nil }
	if _, err = s.Fetch(ctx, "account-a", "book", loader); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s = openTest(t, path)
	if _, err = s.Fetch(ctx, "account-a", "book", loader); err != nil || calls != 1 {
		t.Fatalf("cache not persisted: calls=%d,err=%v", calls, err)
	}
	s.Fetch(ctx, "account-b", "book", loader)
	if calls != 2 {
		t.Fatal("account scopes shared data")
	}
	if _, err = s.db.Exec(`UPDATE responses_v1 SET expires=0 WHERE scope='account-a'`); err != nil {
		t.Fatal(err)
	}
	s.Fetch(ctx, "account-a", "book", loader)
	if calls != 3 {
		t.Fatal("expired data was reused")
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0600 {
		t.Fatalf("cache permissions: %v %v", fi, err)
	}
}

func TestCoalescesRequestsAndDoesNotCacheErrors(t *testing.T) {
	s := openTest(t, filepath.Join(t.TempDir(), "cache.sqlite"))
	ctx := context.Background()
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	loader := func() ([]byte, time.Duration, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return []byte("one"), time.Hour, nil
	}
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			body, err := s.Fetch(ctx, "scope", "key", loader)
			if err != nil || string(body) != "one" {
				t.Errorf("Fetch=%q,%v", body, err)
			}
		})
	}
	<-started
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("calls=%d", calls.Load())
	}
	errorsSeen := 0
	fail := func() ([]byte, time.Duration, error) { errorsSeen++; return nil, 0, errors.New("unauthorized") }
	s.Fetch(ctx, "scope", "error", fail)
	s.Fetch(ctx, "scope", "error", fail)
	if errorsSeen != 2 {
		t.Fatal("failed response was cached")
	}
}

func TestInvalidationRejectsOlderInflightResponse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.sqlite")
	s := openTest(t, path)
	writer := openTest(t, path)
	ctx := context.Background()
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		s.Fetch(ctx, "account", "library", func() ([]byte, time.Duration, error) { close(started); <-release; return []byte("old"), time.Hour, nil })
	}()
	<-started
	writer.Invalidate(ctx, "account")
	close(release)
	<-done
	calls := 0
	body, err := s.Fetch(ctx, "account", "library", func() ([]byte, time.Duration, error) { calls++; return []byte("new"), time.Hour, nil })
	if err != nil || calls != 1 || string(body) != "new" {
		t.Fatalf("old response repopulated cache: %q %d %v", body, calls, err)
	}
}

func TestCanceledFollowerAndUnavailableCache(t *testing.T) {
	s := openTest(t, filepath.Join(t.TempDir(), "cache.sqlite"))
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	loader := func() ([]byte, time.Duration, error) { close(started); <-release; return []byte("ok"), time.Hour, nil }
	go func() { defer close(done); s.Fetch(context.Background(), "s", "k", loader) }()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Fetch(ctx, "s", "k", loader); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled follower=%v", err)
	}
	close(release)
	<-done
	s.Close()
	body, err := s.Fetch(context.Background(), "s", "uncached", func() ([]byte, time.Duration, error) { return []byte("network"), time.Hour, nil })
	if err != nil || string(body) != "network" {
		t.Fatalf("broken cache blocked network: %q %v", body, err)
	}
}
