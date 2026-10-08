package bibliocommons

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeLibrary imitates the catalog's login form and the gateway endpoints
// a session uses.
func fakeLibrary(t *testing.T, posts *atomic.Int32) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /user/login", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "_live_bcui_session_id", Value: "pre", Path: "/"})
		w.Write([]byte(`<html><form action="/search"><input type="hidden" name="authenticity_token" value="WRONG"></form>
<form accept-charset="UTF-8" action="/user/login" data-js="login_form" method="post">
<input name="utf8" type="hidden" value="&#x2713;" /><input type="hidden" name="authenticity_token" value="tok123" />
<input name="name" type="text" /><input name="user_pin" type="password" />
<input name="remember_me" type="checkbox" value="true" /><input name="local" type="hidden" value="false" />
</form></html>`))
	})
	mux.HandleFunc("POST /user/login", func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		r.ParseForm()
		if r.Form.Get("authenticity_token") != "tok123" || r.Form.Get("utf8") != "✓" || r.Form.Get("local") != "false" {
			t.Errorf("form = %v", r.Form)
		}
		if c, err := r.Cookie("_live_bcui_session_id"); err != nil || c.Value != "pre" {
			t.Errorf("login page cookie not sent back")
		}
		if r.Form.Get("name") != "1234" || r.Form.Get("user_pin") != "9999" {
			w.Write([]byte(`<div class="alert alert-danger">Your library card number or PIN is incorrect.</div>`))
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "session_id", Value: "sess", Path: "/"})
		http.SetCookie(w, &http.Cookie{Name: "bc_access_token", Value: "acc", Path: "/"})
		http.Redirect(w, r, "/", http.StatusFound)
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("home")) })
	authed := func(r *http.Request) bool {
		return r.Header.Get("X-Access-Token") == "acc" && r.Header.Get("X-Session-Id") == "sess" &&
			strings.Contains(r.Header.Get("Cookie"), "bc_access_token=acc")
	}
	mux.HandleFunc("GET /gw/testlib/sessions/current", func(w http.ResponseWriter, r *http.Request) {
		if !authed(r) {
			w.Write([]byte(`{"auth":{},"entities":{}}`))
			return
		}
		w.Write([]byte(`{"auth":{"currentUserId":2233},"entities":{"users":{"2233":{"accounts":[2234]}},"accounts":{"2234":{"preferredLocations":["1"]}}}}`))
	})
	mux.HandleFunc("GET /gw/testlib/borrowinghistory", func(w http.ResponseWriter, r *http.Request) {
		if !authed(r) {
			http.Error(w, `{"error":{"message":"Unauthorized"}}`, http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("accountId") != "2234" {
			t.Errorf("accountId = %q", r.URL.Query().Get("accountId"))
		}
		if r.URL.Query().Get("page") == "2" {
			w.Write([]byte(`{"borrowing":{"borrowingHistory":{"results":["h3"],"pagination":{"page":2,"pages":2}}},
"entities":{"borrowingHistory":{"h3":{"metadataId":"B3","title":"Third","author":"C","format":"BK","checkedoutDate":"2023-01-01"}}}}`))
			return
		}
		w.Write([]byte(`{"borrowing":{"borrowingHistory":{"results":["h1","h2"],"pagination":{"page":1,"pages":2}}},
"entities":{"bibs":{"B1":{"id":"B1","briefInfo":{"title":"First","authors":["A"],"format":"BK","genreForm":["Fantasy fiction"]}}},
"borrowingHistory":{"h1":{"metadataId":"B1","title":"First","checkedoutDate":"2024-05-01"},"h2":{"metadataId":"B2","title":"Second","author":"B","format":"EBOOK","checkedoutDate":"2024-04-01"}}}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New("testlib")
	c.Site = srv.URL
	c.BaseURL = srv.URL + "/gw"
	return c
}

func TestLoginAndHistory(t *testing.T) {
	var posts atomic.Int32
	c := fakeLibrary(t, &posts)
	ctx := context.Background()

	s, err := c.Login(ctx, " 1234 ", "9999")
	if err != nil {
		t.Fatal(err)
	}
	if s.UserID != "2233" || s.AccountID != "2234" || s.HomeBranch != "1" || s.Cookies["session_id"] != "sess" {
		t.Fatalf("session = %+v", s)
	}

	path := filepath.Join(t.TempDir(), "session.json")
	if err := SaveSession(path, s); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("session file mode = %v", fi.Mode().Perm())
	}
	loaded, err := LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Check(ctx, loaded); err != nil {
		t.Fatal(err)
	}

	h, err := c.History(ctx, loaded)
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 3 || h[0].Bib.Title != "First" || h[0].Bib.Genres[0] != "Fantasy fiction" ||
		h[1].Bib.Authors[0] != "B" || h[1].Bib.Format != "EBOOK" || h[2].CheckedOut != "2023-01-01" {
		t.Errorf("history = %+v", h)
	}
}

func TestLoginWrongPIN(t *testing.T) {
	var posts atomic.Int32
	c := fakeLibrary(t, &posts)
	_, err := c.Login(context.Background(), "1234", "0000")
	if !errors.Is(err, ErrLoginFailed) || !strings.Contains(err.Error(), "PIN is incorrect") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "0000") {
		t.Error("error message leaks the PIN")
	}
	if n := posts.Load(); n != 1 {
		t.Errorf("login posted %d times, want exactly 1", n)
	}
}

func TestExpiredSession(t *testing.T) {
	var posts atomic.Int32
	c := fakeLibrary(t, &posts)
	stale := &Session{Library: "testlib", UserID: "2233", AccountID: "2234", Cookies: map[string]string{"session_id": "old", "bc_access_token": "old"}}
	if err := c.Check(context.Background(), stale); !errors.Is(err, ErrSessionExpired) {
		t.Errorf("Check err = %v", err)
	}
	if _, err := c.History(context.Background(), stale); !errors.Is(err, ErrSessionExpired) {
		t.Errorf("History err = %v", err)
	}
}
