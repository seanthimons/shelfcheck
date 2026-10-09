package bibliocommons

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"

	"shelfcheck/internal/cache"
)

func TestCatalogCachePersistsAcrossClients(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.sqlite")
	disk, err := cache.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var hits atomic.Int32
	c := fixtureServer(t, &hits)
	c.Disk = disk
	ctx := context.Background()
	q := Query{Text: "Alien Clay"}
	if _, err = c.Search(ctx, q, 1); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Details(ctx, "S170C3895239"); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Availability(ctx, "S170C3895239"); err != nil {
		t.Fatal(err)
	}
	disk.Close()
	disk, err = cache.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { disk.Close() })
	fresh := New("testlib")
	fresh.BaseURL = c.BaseURL
	fresh.Disk = disk
	if res, err := fresh.Search(ctx, q, 1); err != nil || res.Total != 6 {
		t.Fatalf("search=%+v,%v", res, err)
	}
	if _, err = fresh.Details(ctx, "S170C3895239"); err != nil {
		t.Fatal(err)
	}
	if _, err = fresh.Availability(ctx, "S170C3895239"); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 3 {
		t.Fatalf("warm catalog made network requests: %d", hits.Load())
	}
	changed := Query{Text: "Alien Clay", Sort: SortTitle}
	if _, err = fresh.Search(ctx, changed, 1); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 4 {
		t.Fatal("different query reused cached search")
	}
}
