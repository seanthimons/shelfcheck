package recommend

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shelfcheck/internal/account"
	bc "shelfcheck/internal/bibliocommons"
)

func TestNewShelf(t *testing.T) {
	for shelf, want := range map[string]bool{
		"Main - 1st Floor South - New Arrivals":          true,
		"Main - 1st Floor South - Juvenile New Arrivals": true,
		"Main - 2nd Floor South - New Non Fiction":       true,
		"Technical Services New Arrivals":                false,
		"Main - 1st Floor South - Sci-Fi":                false,
		"Newport - Fiction":                              false,
		"":                                               false,
	} {
		if got := NewShelf(shelf); got != want {
			t.Errorf("NewShelf(%q) = %v, want %v", shelf, got, want)
		}
	}
}

// arrivalsCatalog serves one page of recently acquired titles and each
// one's copies, as "branch|collection|status" (or a 500 for "fail").
func arrivalsCatalog(t *testing.T, order []string, copies map[string][]string) *bc.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/testlib/")
		if p == "bibs/search" {
			if q := r.URL.Query(); q.Get("query") != arrivalsQuery || q.Get("searchType") != "bl" || q.Get("sort") != bc.SortNewlyAcquired || q.Get("f_STATUS") != "1" {
				t.Errorf("search params = %v", q)
			}
			var results, bibs []string
			for _, id := range order {
				results = append(results, fmt.Sprintf(`{"representative":%q}`, id))
				bibs = append(bibs, fmt.Sprintf(`%q:{"id":%q,"briefInfo":{"title":%q,"authors":["Author, An"],"genreForm":["Fantasy fiction"]}}`, id, id, id))
			}
			fmt.Fprintf(w, `{"catalogSearch":{"pagination":{"count":%d,"page":1,"pages":1},"results":[%s]},"entities":{"bibs":{%s}}}`,
				len(order), strings.Join(results, ","), strings.Join(bibs, ","))
			return
		}
		id := strings.TrimSuffix(strings.TrimPrefix(p, "bibs/"), "/availability")
		if len(copies[id]) == 1 && copies[id][0] == "fail" {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		var ids, items []string
		for i, c := range copies[id] {
			f := strings.Split(c, "|")
			item := fmt.Sprintf("%s-%d", id, i)
			ids = append(ids, fmt.Sprintf("%q", item))
			items = append(items, fmt.Sprintf(`%q:{"itemId":%q,"collection":%q,"branch":{"code":%q},"availability":{"status":%q}}`, item, item, f[1], f[0], f[2]))
		}
		fmt.Fprintf(w, `{"availability":{"items":[%s]},"entities":{"bibItems":{%s}}}`, strings.Join(ids, ","), strings.Join(items, ","))
	}))
	t.Cleanup(srv.Close)
	c := bc.New("testlib")
	c.BaseURL = srv.URL
	return c
}

func TestArrivals(t *testing.T) {
	const shelf = "Main - 1st Floor South - New Arrivals"
	client := arrivalsCatalog(t, []string{"on-shelf", "split", "processing", "broken", "read", "beyond"}, map[string][]string{
		"on-shelf": {"1|" + shelf + "|AVAILABLE"},
		// In at another branch, and the new-shelf copy at home is out.
		"split":      {"2|Branch - Fiction|AVAILABLE", "1|" + shelf + "|UNAVAILABLE"},
		"processing": {"1|Technical Services New Arrivals|AVAILABLE"},
		"broken":     {"fail"},
		"read":       {"1|" + shelf + "|AVAILABLE"},
		"beyond":     {"1|" + shelf + "|AVAILABLE"},
	})
	rec := &account.Record{Books: []account.Book{
		{Title: "read", Authors: []string{"Author, An"}, Shelf: account.ShelfRead, Genres: []string{"Fantasy fiction"}},
	}}
	res, err := Arrivals(context.Background(), client, rec, ArrivalOptions{Home: "1", Scan: 5}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].Bib.ID != "on-shelf" || res.Items[0].Shelf != shelf {
		t.Fatalf("items = %+v", res.Items)
	}
	if res.Checked != 4 || res.Failed != 1 || res.Known != 1 || !res.More || !res.Partial() {
		t.Errorf("checked %d, failed %d, known %d, more %v", res.Checked, res.Failed, res.Known, res.More)
	}
	// It shares the read book's author and genre.
	if a := res.Items[0]; a.Score <= 0 || !strings.Contains(strings.Join(a.Reasons, "; "), "you've read 1 by An Author") ||
		!strings.Contains(strings.Join(a.Reasons, "; "), "your genres: Fantasy fiction") {
		t.Errorf("score %v, reasons %q", a.Score, a.Reasons)
	}

	// Without a reading record nothing is dropped or scored.
	res, err = Arrivals(context.Background(), client, nil, ArrivalOptions{Home: "1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 3 || res.More || res.Items[0].Score != 0 {
		t.Errorf("unscored items = %+v, more %v", res.Items, res.More)
	}
}
