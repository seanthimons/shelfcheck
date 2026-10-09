package tui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"shelfcheck/internal/account"
	bc "shelfcheck/internal/bibliocommons"
)

func TestHardcoverMetadataArrivesBeforeLibrarySnapshot(t *testing.T) {
	t.Setenv("HARDCOVER_TOKEN", "test-token")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "books_by_pk") {
			io.WriteString(w, `{"data":{"books_by_pk":{"id":42,"title":"Book","ratings_count":1}}}`)
		} else {
			io.WriteString(w, `{"data":{"me":[{"user_books":[{"id":7,"status_id":3,"book":{"id":42,"title":"Book"}}]}]}}`)
		}
	}))
	defer srv.Close()
	st := account.DefaultStore(t.TempDir())
	st.HardcoverClient().Endpoint = srv.URL
	m := New(bc.New("testlib"), "1", false, "", st)
	m.screen, m.selectedHC = detailScreen, 42
	m, cmd := m.fetchHCInfo()
	msg, ok := cmd().(hcInfoMsg)
	if !ok || msg.err != nil || !msg.ok || hits.Load() != 1 {
		t.Fatalf("metadata waited for library: %+v, hits=%d", msg, hits.Load())
	}
	m, cmd = m.handleHCInfo(msg)
	if m.hcInfo[m.detailKey()].loading || !m.hcLibraryLoading || cmd == nil {
		t.Fatal("metadata wasn't ready before library command")
	}
	lib, ok := cmd().(hcLibraryMsg)
	if !ok || lib.err != nil {
		t.Fatalf("library response=%+v", lib)
	}
	m, _ = m.handleHCLibrary(lib)
	if !m.hcLibraryLoaded || m.hcLib[42].UserBookID != 7 {
		t.Fatal("background library wasn't applied")
	}
}
