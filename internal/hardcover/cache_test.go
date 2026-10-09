package hardcover

import (
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"shelfcheck/internal/cache"
)

func TestLibraryCacheIsolationInvalidationAndFreshEditorReads(t *testing.T) {
	disk, err := cache.Open(filepath.Join(t.TempDir(), "cache.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { disk.Close() })
	var reads, writes atomic.Int32
	c := fakeHardcover(t, func(q string, vars map[string]any) string {
		if strings.Contains(q, "insert_user_book") {
			writes.Add(1)
			return `{"data":{"insert_user_book":{"id":8,"error":null}}}`
		}
		reads.Add(1)
		return `{"data":{"me":[{"user_books":[{"id":7,"status_id":3,"rating":4.5,"book":{"id":42,"title":"Book"}}]}]}}`
	})
	c.Disk = disk
	ctx := context.Background()
	if _, err = c.Library(ctx); err != nil {
		t.Fatal(err)
	}
	fresh := New("token")
	fresh.Endpoint = c.Endpoint
	fresh.Disk = disk
	if _, err = fresh.Library(ctx); err != nil {
		t.Fatal(err)
	}
	if reads.Load() != 1 {
		t.Fatal("library wasn't shared between clients")
	}
	for range 2 {
		if _, ok, err := fresh.UserBook(ctx, 42); err != nil || !ok {
			t.Fatalf("editor record: %v %v", ok, err)
		}
	}
	if reads.Load() != 3 {
		t.Fatal("editor read came from cache")
	}
	other := New("different-token")
	other.Endpoint = c.Endpoint
	other.Disk = disk
	if _, err = other.Library(ctx); err != nil {
		t.Fatal(err)
	}
	if reads.Load() != 4 {
		t.Fatal("private library crossed accounts")
	}
	if _, err = fresh.AddToShelf(ctx, 43, WantToRead); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Library(ctx); err != nil {
		t.Fatal(err)
	}
	if reads.Load() != 5 || writes.Load() != 1 {
		t.Fatal("mutation didn't invalidate cached library")
	}
	if _, err = other.Library(ctx); err != nil {
		t.Fatal(err)
	}
	if reads.Load() != 5 {
		t.Fatal("mutation invalidated unrelated account")
	}
}

func TestMetadataCacheAndExplicitRefresh(t *testing.T) {
	disk, err := cache.Open(filepath.Join(t.TempDir(), "cache.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { disk.Close() })
	var hits atomic.Int32
	c := fakeHardcover(t, func(q string, vars map[string]any) string {
		hits.Add(1)
		return `{"data":{"books_by_pk":{"id":42,"title":"Book","ratings_count":3}}}`
	})
	c.Disk = disk
	ctx := context.Background()
	for range 2 {
		if _, ok, err := c.Info(ctx, 42); err != nil || !ok {
			t.Fatalf("metadata: %v %v", ok, err)
		}
	}
	if hits.Load() != 1 {
		t.Fatal("metadata wasn't cached")
	}
	c.RefreshMetadata(ctx)
	if _, _, err = c.Info(ctx, 42); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatal("explicit refresh didn't bypass cache")
	}
}
