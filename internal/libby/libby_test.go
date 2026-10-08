package libby

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakeLibby(t *testing.T) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /chip", func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Authorization") {
		case "":
			fmt.Fprint(w, `{"identity":"anon"}`)
		case "Bearer anon":
			fmt.Fprint(w, `{"identity":"linked"}`)
		default:
			t.Errorf("unexpected auth %q", r.Header.Get("Authorization"))
		}
	})
	mux.HandleFunc("POST /chip/clone/code", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer anon" {
			t.Errorf("clone auth = %q", r.Header.Get("Authorization"))
		}
		buf := make([]byte, 100)
		n, _ := r.Body.Read(buf)
		if !strings.Contains(string(buf[:n]), `"12345678"`) {
			fmt.Fprint(w, `{"result":"invalid_code"}`)
			return
		}
		fmt.Fprint(w, `{"result":"cloned"}`)
	})
	mux.HandleFunc("GET /tags", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer linked" {
			http.Error(w, "{}", http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"tags":[
{"name":"🧾","uuid":"u1","description":"Titles I've borrowed in Libby.","totalTaggings":60,"behaviors":{"borrowed":{"type":"autonomous"}}},
{"name":"want to read","uuid":"u2","totalTaggings":0,"behaviors":{}}]}`)
	})
	mux.HandleFunc("GET /tag/u1/{name}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("name") != base64.StdEncoding.EncodeToString([]byte("🧾")) {
			t.Errorf("tag name segment = %q", r.PathValue("name"))
		}
		var start, end int
		fmt.Sscanf(r.URL.Query().Get("range"), "%d...%d", &start, &end)
		var items []string
		for i := start; i < min(end, 60); i++ {
			items = append(items, fmt.Sprintf(`{"titleId":"%d","sortTitle":"Book %d","sortAuthor":"A","titleFormat":"book","createTime":1789964068175,"titleSubjects":[{"id":"80","name":"Science Fiction"}]}`, i, i))
		}
		fmt.Fprintf(w, `{"tag":{"taggings":[%s]}}`, strings.Join(items, ","))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New("")
	c.Sentry, c.Vandal = srv.URL, srv.URL
	return c
}

func TestLinkTagsAndPaging(t *testing.T) {
	c := fakeLibby(t)
	ctx := context.Background()
	if _, err := c.Link(ctx, "1234"); err == nil {
		t.Error("short code accepted")
	}
	id, err := c.Link(ctx, "12345678")
	if err != nil || id != "linked" {
		t.Fatalf("Link = %q, %v", id, err)
	}
	tags, err := c.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[0].Kind != "borrowed" || tags[1].Kind != "" {
		t.Fatalf("tags = %+v", tags)
	}
	items, err := c.Taggings(ctx, tags[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 60 || items[59].Title != "Book 59" || items[0].Subjects[0] != "Science Fiction" ||
		items[0].Tagged.Year() != 2026 {
		t.Errorf("got %d items, first %+v", len(items), items[0])
	}
}

func TestLinkBadCode(t *testing.T) {
	c := fakeLibby(t)
	if _, err := c.Link(context.Background(), "87654321"); err == nil || !strings.Contains(err.Error(), "invalid_code") {
		t.Errorf("err = %v", err)
	}
}

func TestNotLinked(t *testing.T) {
	c := fakeLibby(t)
	c.Identity = "stale"
	if _, err := c.Tags(context.Background()); err != ErrNotLinked {
		t.Errorf("err = %v", err)
	}
}
