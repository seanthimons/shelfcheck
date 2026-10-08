package bibliocommons

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fixtureServer serves testdata files captured from the real gateway.
func fixtureServer(t *testing.T, hits *atomic.Int32) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			hits.Add(1)
		}
		if r.Header.Get("User-Agent") != DefaultUserAgent {
			t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
		}
		p := strings.TrimPrefix(r.URL.Path, "/testlib/")
		var file string
		switch {
		case p == "bibs/search":
			file = "search.json"
		case p == "bibs/S170C3895239/availability":
			file = "availability_physical.json"
		case p == "bibs/S980C10216670/availability":
			file = "availability_digital.json"
		case p == "bibs/S170C3895239/evaluation":
			file = "evaluation_alienclay.json"
		case p == "bibs/S170C3599719/evaluation":
			file = "evaluation_hailmary.json"
		case p == "locations":
			file = "locations.json"
		default:
			http.NotFound(w, r)
			return
		}
		b, err := os.ReadFile(filepath.Join("testdata", file))
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	c := New("testlib")
	c.BaseURL = srv.URL
	return c
}

func TestSearch(t *testing.T) {
	c := fixtureServer(t, nil)
	res, err := c.Search(context.Background(), "Alien Clay", Keyword, 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 6 || res.Pages != 1 || len(res.Bibs) != 6 {
		t.Fatalf("got total=%d pages=%d bibs=%d", res.Total, res.Pages, len(res.Bibs))
	}
	b := res.Bibs[0]
	if b.ID != "S170C3895239" || b.Title != "Alien Clay" || b.Format != "BK" || b.Digital {
		t.Errorf("first bib = %+v", b)
	}
	if len(b.Authors) == 0 || b.Authors[0] != "Tchaikovsky, Adrian" {
		t.Errorf("authors = %v", b.Authors)
	}
	if b.Summary.Available != 2 || b.Summary.Total != 8 {
		t.Errorf("summary = %+v", b.Summary)
	}
	if b.CoverURL == "" {
		t.Error("expected a cover URL")
	}
	if b.Rating.Average != 78 || b.Rating.Count != 108 || b.Rating.Stars() != 3.9 {
		t.Errorf("rating = %+v (%.1f stars)", b.Rating, b.Rating.Stars())
	}
	if len(b.Genres) == 0 || b.Genres[0] != "Science fiction" || len(b.Subjects) == 0 {
		t.Errorf("genres = %v subjects = %v", b.Genres, b.Subjects)
	}
	if b.Language != "eng" || b.ContentType != "FICTION" || len(b.Audiences) == 0 || b.Audiences[0] != "ADULT" {
		t.Errorf("language=%q contentType=%q audiences=%v", b.Language, b.ContentType, b.Audiences)
	}
	var sawDigital bool
	for _, b := range res.Bibs {
		sawDigital = sawDigital || b.Digital
		// OverDrive descriptions arrive as HTML.
		if strings.ContainsAny(b.Description, "<>") {
			t.Errorf("%s description not cleaned: %.80q", b.ID, b.Description)
		}
	}
	if !sawDigital {
		t.Error("expected at least one digital bib")
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	c := fixtureServer(t, nil)
	if _, err := c.Search(context.Background(), "  ", Keyword, 1); err == nil {
		t.Fatal("expected error")
	}
}

func TestAvailabilityPhysical(t *testing.T) {
	c := fixtureServer(t, nil)
	a, err := c.Availability(context.Background(), "S170C3895239")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Copies) != 8 || a.Summary.Total != 8 {
		t.Fatalf("copies=%d summary=%+v", len(a.Copies), a.Summary)
	}
	// Available copies sort first, and have no due date.
	avail := 0
	for i, cp := range a.Copies {
		if cp.Available {
			avail++
			if i >= a.Summary.Available {
				t.Errorf("available copy at index %d sorted after unavailable ones", i)
			}
			if !cp.Due.IsZero() {
				t.Errorf("available copy has due date %v", cp.Due)
			}
		} else if cp.Due.IsZero() {
			t.Logf("unavailable copy without due date: %+v", cp)
		}
		if cp.Branch == "" || cp.BranchCode == "" {
			t.Errorf("copy missing branch: %+v", cp)
		}
	}
	if avail != 2 {
		t.Errorf("available copies = %d, want 2", avail)
	}
	for i := 1; i < len(a.Copies); i++ {
		p, q := a.Copies[i-1], a.Copies[i]
		if !p.Available && !q.Available && !p.Due.IsZero() && !q.Due.IsZero() && q.Due.Before(p.Due) {
			t.Errorf("due dates out of order: %v then %v", p.Due, q.Due)
		}
	}
}

func TestAvailabilityDigital(t *testing.T) {
	c := fixtureServer(t, nil)
	a, err := c.Availability(context.Background(), "S980C10216670")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Copies) != 0 || len(a.DigitalFormats) == 0 {
		t.Fatalf("copies=%d formats=%v", len(a.Copies), a.DigitalFormats)
	}
	if a.Summary.Total != 2 || a.Summary.Holds != 1 {
		t.Errorf("summary = %+v", a.Summary)
	}
}

func TestDetails(t *testing.T) {
	c := fixtureServer(t, nil)
	ctx := context.Background()

	d, err := c.Details(ctx, "S170C3895239")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(d.AuthorNote, "Adrian Tchaikovsky was born") {
		t.Errorf("author note = %.80q", d.AuthorNote)
	}
	if len(d.Reviews) != 2 {
		t.Fatalf("reviews = %d, want 2", len(d.Reviews))
	}
	sources := map[string]bool{}
	for _, r := range d.Reviews {
		sources[r.Source] = true
		if r.Text == "" || strings.ContainsAny(r.Text, "<>") || strings.Contains(r.Text, "&#") {
			t.Errorf("review text not cleaned: %.120q", r.Text)
		}
	}
	if !sources["Publishers Weekly"] || !sources["Library Journal"] {
		t.Errorf("review sources = %v", sources)
	}
	if d.Excerpt != "" {
		t.Errorf("Alien Clay has no excerpt, got %.40q", d.Excerpt)
	}

	d, err = c.Details(ctx, "S170C3599719")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(d.Excerpt, "Chapter 1") || !strings.Contains(d.Excerpt, `"What's two plus two?"`) {
		t.Errorf("excerpt = %.80q", d.Excerpt)
	}
}

func TestPlainText(t *testing.T) {
	in := "<p> One &amp; two&#39;s.</p><p>Three<br/>four   five </p>"
	want := "One & two's.\n\nThree\n\nfour five"
	if got := plainText(in); got != want {
		t.Errorf("plainText = %q, want %q", got, want)
	}
}

func TestLocations(t *testing.T) {
	c := fixtureServer(t, nil)
	locs, err := c.Locations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) < 40 {
		t.Fatalf("got %d locations, want ~41", len(locs))
	}
	var main *Location
	for i := range locs {
		if locs[i].Code == "1" {
			main = &locs[i]
		}
	}
	if main == nil {
		t.Fatal("Main Library (code 1) missing; branch codes must match Copy.BranchCode")
	}
	if len(main.Hours) != 7 || main.Address == "" {
		t.Errorf("main = %+v", *main)
	}
}

func TestCacheAvoidsSecondRequest(t *testing.T) {
	var hits atomic.Int32
	c := fixtureServer(t, &hits)
	ctx := context.Background()
	for range 3 {
		if _, err := c.Search(ctx, "Alien Clay", Keyword, 1); err != nil {
			t.Fatal(err)
		}
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("server hit %d times, want 1", n)
	}
}

func TestAPIError(t *testing.T) {
	c := fixtureServer(t, nil)
	_, err := c.Availability(context.Background(), "NOPE")
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Status != 404 {
		t.Fatalf("err = %v", err)
	}
}

// TestLive hits the real gateway. Run with: SHELFCHECK_LIVE=1 go test -run Live -v ./...
func TestLive(t *testing.T) {
	if os.Getenv("SHELFCHECK_LIVE") == "" {
		t.Skip("set SHELFCHECK_LIVE=1 to run against the real API")
	}
	c := New("cincinnatilibrary")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for _, typ := range []SearchType{Keyword, Title, Author, Subject, Series} {
		q := map[SearchType]string{Keyword: "alien clay", Title: "dune", Author: "tchaikovsky", Subject: "beekeeping", Series: "discworld"}[typ]
		res, err := c.Search(ctx, q, typ, 1)
		if err != nil {
			t.Fatalf("%s search: %v", typ, err)
		}
		t.Logf("%-8s %-12q -> %d results, %d pages", typ, q, res.Total, res.Pages)
		if res.Total == 0 {
			t.Errorf("%s search %q returned nothing", typ, q)
		}
	}

	res, err := c.Search(ctx, "alien clay", Keyword, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range res.Bibs[:min(3, len(res.Bibs))] {
		a, err := c.Availability(ctx, b.ID)
		if err != nil {
			t.Fatalf("availability %s: %v", b.ID, err)
		}
		d, err := c.Details(ctx, b.ID)
		if err != nil {
			t.Fatalf("details %s: %v", b.ID, err)
		}
		t.Logf("%s rating %.1f★ (%d), genres %v, %d reviews, author note %d chars, excerpt %d chars",
			b.ID, b.Rating.Stars(), b.Rating.Count, b.Genres, len(d.Reviews), len(d.AuthorNote), len(d.Excerpt))
		t.Logf("%s [%s] %q: %d/%d available, %d holds, %d copies, formats %v",
			b.ID, b.Format, b.Title, a.Summary.Available, a.Summary.Total, a.Summary.Holds, len(a.Copies), a.DigitalFormats)
		for _, cp := range a.Copies {
			due := ""
			if !cp.Due.IsZero() {
				due = "due " + cp.Due.Format("Jan 2")
			}
			t.Logf("    %-14s %-38s %-14s %s", cp.Branch, cp.Collection, cp.LibraryStatus, due)
		}
	}

	locs, err := c.Locations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d locations", len(locs))
}
