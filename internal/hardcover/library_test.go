package hardcover

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeHardcover answers each GraphQL request with reply(query, variables).
func fakeHardcover(t *testing.T, reply func(q string, vars map[string]any) string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		b, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(b, &req); err != nil {
			t.Errorf("bad request body: %v", err)
		}
		io.WriteString(w, reply(req.Query, req.Variables))
	}))
	t.Cleanup(srv.Close)
	c := New("token")
	c.Endpoint = srv.URL
	return c
}

func TestAddToShelfAndRemove(t *testing.T) {
	c := fakeHardcover(t, func(q string, vars map[string]any) string {
		switch {
		case strings.Contains(q, "insert_user_book"):
			if vars["b"] != float64(42) || vars["s"] != float64(WantToRead) {
				t.Errorf("insert vars = %v", vars)
			}
			return `{"data":{"insert_user_book":{"id":7,"error":null}}}`
		case strings.Contains(q, "delete_user_book"):
			if vars["id"] != float64(7) {
				t.Errorf("delete vars = %v", vars)
			}
			return `{"data":{"delete_user_book":{"id":7}}}`
		}
		t.Errorf("unexpected query %q", q)
		return `{}`
	})
	id, err := c.AddToShelf(context.Background(), 42, WantToRead)
	if err != nil || id != 7 {
		t.Fatalf("AddToShelf = %d, %v; want 7, nil", id, err)
	}
	if err := c.RemoveFromLibrary(context.Background(), id); err != nil {
		t.Fatalf("RemoveFromLibrary: %v", err)
	}
}

func TestAddToShelfReportsError(t *testing.T) {
	c := fakeHardcover(t, func(string, map[string]any) string {
		return `{"data":{"insert_user_book":{"id":null,"error":"Book already in library"}}}`
	})
	if _, err := c.AddToShelf(context.Background(), 1, WantToRead); err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("err = %v, want Hardcover's error", err)
	}
}
