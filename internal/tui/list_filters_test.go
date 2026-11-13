package tui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"shelfcheck/internal/account"
	bc "shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/hardcover"
	"shelfcheck/internal/libby"
	"shelfcheck/internal/recommend"
)

func filterEditions() (bc.Bib, bc.Bib) {
	print := bc.Bib{ID: "print", Title: "Both formats", Authors: []string{"Author"}, Format: "BK", Language: "eng", Genres: []string{"Fantasy"}, Year: "2024", Audiences: []string{"ADULT"}, ContentType: "FICTION", Rating: bc.Rating{Average: 90, Count: 10}}
	ebook := print
	ebook.ID, ebook.Format, ebook.Digital = "ebook", "EBOOK", true
	return print, ebook
}

func TestDiscoveryFilterPanelAndEditionSelection(t *testing.T) {
	for _, target := range []screen{wishlistScreen, forYouScreen, hardcoverScreen} {
		t.Run(map[screen]string{wishlistScreen: "wishlist", forYouScreen: "for you", hardcoverScreen: "hardcover"}[target], func(t *testing.T) {
			m := workspaceModel(t)
			m.screen = target
			print, ebook := filterEditions()
			onlyPrint := print
			onlyPrint.ID = "print-only"
			onlyPrint.Title = "Print only"
			wishes := []account.WishItem{
				{Tagging: libby.Tagging{Title: onlyPrint.Title}, Matches: []account.WishMatch{{Bib: onlyPrint}}},
				{Tagging: libby.Tagging{Title: print.Title}, Matches: []account.WishMatch{{Bib: print}, {Bib: ebook, Online: true}}},
			}
			m.wishTitles = []libby.Tagging{{Title: onlyPrint.Title}, {Title: print.Title}}
			m.wishItems = wishes
			m.recDone = true
			m.recPicks = []recommend.Pick{{Bib: onlyPrint, Print: true, Editions: []bc.Bib{onlyPrint}}, {Bib: print, Print: true, Ebook: true, Editions: []bc.Bib{print, ebook}}}
			m.hc.items = wishes
			m.hc.col = &hardcover.Collection{Title: "Collection", Books: []hardcover.PromptAnswer{{Book: hardcover.Book{Title: onlyPrint.Title}}, {Book: hardcover.Book{Title: print.Title}}}}
			m.wishCursor, m.recCursor, m.hc.cursor = 1, 1, 1
			// Drive the same panel keys a user uses: Format -> Ebook -> apply.
			var cmd tea.Cmd
			m, cmd = accountPress(m, 'f', "f")
			if cmd != nil || m.panel != panelCategories {
				t.Fatal("f did not open local panel")
			}
			if out := ansi.Strip(m.View().Content); !strings.Contains(out, "Format") {
				t.Fatalf("panel missing: %s", out)
			}
			m, _ = accountPress(m, tea.KeyEnter, "")
			m, _ = accountPress(m, tea.KeyDown, "")
			m, _ = accountPress(m, tea.KeySpace, " ")
			if m.listFilters[target].Narrowed() {
				t.Fatal("draft applied early")
			}
			m, _ = accountPress(m, tea.KeyEscape, "")
			m, cmd = accountPress(m, tea.KeyEscape, "")
			if cmd != nil || m.panel != panelClosed || !m.listFilters[target].Has(bc.FacetFormat, "EBOOK") {
				t.Fatalf("filters not applied: %+v", m.listFilters)
			}
			if m.wishCursor != 0 && target == wishlistScreen || m.recCursor != 0 && target == forYouScreen || m.hc.cursor != 0 && target == hardcoverScreen {
				t.Fatal("cursor not reset")
			}
			out := ansi.Strip(m.View().Content)
			if !strings.Contains(out, "Both formats") || strings.Contains(out, "Print only") || !strings.Contains(out, "Filters: eBook") {
				t.Fatalf("filtered view: %s", out)
			}
			next, _ := accountPress(m, tea.KeyEnter, "")
			if next.screen != detailScreen || next.selected.ID != "ebook" {
				t.Fatalf("Enter opened %v %q", next.screen, next.selected.ID)
			}
			if len(m.wishItems[1].Matches) != 2 || len(m.recPicks) != 2 {
				t.Fatal("filter destroyed source data")
			}
			m, _ = accountPress(m, 'F', "F")
			if m.activeFilters().Narrowed() || !strings.Contains(ansi.Strip(m.View().Content), "Print only") {
				t.Fatal("clear did not restore results")
			}
		})
	}
}

func TestMetadataFiltersMatchOneEdition(t *testing.T) {
	print, ebook := filterEditions()
	ebook.Language = "fre"
	wish := account.WishItem{Matches: []account.WishMatch{{Bib: print}, {Bib: ebook, Online: true}}}
	q := bc.Query{}
	q.Toggle(bc.FacetFormat, "EBOOK")
	q.Toggle(bc.FacetLanguage, "eng")
	if _, ok := filterWish(wish, q); ok {
		t.Fatal("combined metadata matched across two editions")
	}
	q.Toggle(bc.FacetLanguage, "fre") // OR within a field
	filtered, ok := filterWish(wish, q)
	if !ok || len(filtered.Matches) != 1 || filtered.Matches[0].Bib.ID != "ebook" {
		t.Fatal("OR language choices did not match ebook")
	}
	for field, value := range map[string]string{bc.FacetGenre: "Fantasy", bc.FacetAudience: "adult", bc.FacetYear: "2024", bc.FacetFiction: "FICTION", bc.FacetAuthor: "Author", bc.FacetRating: "FOUR_PLUS_STARS"} {
		q.Toggle(field, value)
	}
	if _, ok := filterWish(wish, q); !ok {
		t.Fatal("metadata filters excluded known matching edition")
	}
	if matchesFilters(bc.Bib{}, q) {
		t.Fatal("unknown metadata matched filters")
	}
	if matchesFilters(bc.Bib{Rating: bc.Rating{Average: 100}}, bc.Query{Filters: map[string][]string{bc.FacetRating: {"FOUR_PLUS_STARS"}}}) {
		t.Fatal("unrated book matched rating")
	}
}

func TestListFilterEmptyStateAndScreenIsolation(t *testing.T) {
	m := workspaceModel(t)
	print, _ := filterEditions()
	m.screen = wishlistScreen
	m.wishTitles = []libby.Tagging{{Title: print.Title}}
	m.wishItems = []account.WishItem{{Matches: []account.WishMatch{{Bib: print}}}, {Tagging: libby.Tagging{Title: "Unknown"}}}
	q := bc.Query{Filters: map[string][]string{bc.FacetFormat: {"EBOOK"}}}
	m = m.applyListFilters(q)
	m.wishAll = true
	if len(m.wishVisible()) != 0 {
		t.Fatal("show all bypassed format filter")
	}
	if out := ansi.Strip(m.viewWishlist()); !strings.Contains(out, "No titles match") {
		t.Fatal(out)
	}
	// Empty result sets still offer Format, and selected values can be cleared.
	m.wishItems = nil
	m = m.openPanel()
	if len(m.visibleCategories()) == 0 {
		t.Fatal("empty list has no filters")
	}
	m.panel = panelClosed
	m.screen = forYouScreen
	m.recPicks = []recommend.Pick{{Bib: print, Print: true}}
	if m.activeFilters().Narrowed() || m.forYouRows() != 1 {
		t.Fatal("wishlist filters leaked into For you")
	}
	m = m.applyListFilters(q)
	m.recDone = true
	if out := ansi.Strip(m.viewForYou()); !strings.Contains(out, "No recommendations match") || strings.Contains(out, "half a minute") {
		t.Fatal(out)
	}
	m.screen = resultsScreen
	m.q = bc.Query{Text: "test", Filters: map[string][]string{bc.FacetLanguage: {"eng"}}}
	if !m.activeFilters().Has(bc.FacetLanguage, "eng") || m.activeFilters().Has(bc.FacetFormat, "EBOOK") {
		t.Fatal("list filters changed search")
	}
}

func TestForYouElsewhereAndArrivalsUseFilteredIndexes(t *testing.T) {
	m := workspaceModel(t)
	print, ebook := filterEditions()
	m.screen = forYouScreen
	m.recDone = true
	m.recProfile.NotOnShelf = []recommend.Elsewhere{{Bib: print}, {Bib: ebook}}
	m.recProfile.ToBuy = []recommend.Elsewhere{{Title: "Unknown"}}
	m = m.applyListFilters(bc.Query{Filters: map[string][]string{bc.FacetFormat: {"EBOOK"}}})
	if m.forYouRows() != 1 {
		t.Fatal("wrong filtered row count")
	}
	if it, ok := m.elsewhereAt(0); !ok || it.Bib.ID != "ebook" {
		t.Fatal("wrong elsewhere row")
	}
	next, _ := accountPress(m, tea.KeyEnter, "")
	if next.selected.ID != "ebook" {
		t.Fatal("opened wrong elsewhere book")
	}
	m.screen = arrivalsScreen
	other := print
	other.ID = "other"
	other.Genres = []string{"History"}
	m.arr.done = true
	m.arr.res.Items = []recommend.Arrival{{Bib: other}, {Bib: print}}
	m = m.applyListFilters(bc.Query{Filters: map[string][]string{bc.FacetGenre: {"Fantasy"}}})
	if got := m.visibleArrivals(); len(got) != 1 || got[0].Bib.ID != "print" {
		t.Fatal("wrong arrivals")
	}
	next, _ = accountPress(m, tea.KeyEnter, "")
	if next.selected.ID != "print" {
		t.Fatal("opened unfiltered arrivals index")
	}
}

func TestSearchFiltersStillQueryCatalog(t *testing.T) {
	payload, err := os.ReadFile("../bibliocommons/testdata/search.json")
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.URL.Query().Get("f_FORMAT"); got != "EBOOK" {
			t.Errorf("format=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(payload)
	}))
	defer srv.Close()
	m := New(bc.New("test"), "1", false, "", nil)
	m.client.BaseURL = srv.URL
	m.screen = resultsScreen
	m.q = bc.Query{Text: "Example"}
	m.facets = map[string][]bc.Facet{bc.FacetFormat: {{Value: "EBOOK", Count: 1}}}
	m, _ = accountPress(m, 'f', "f")
	m, _ = accountPress(m, tea.KeyDown, "") // Available -> Format
	m, _ = accountPress(m, tea.KeyEnter, "")
	m, _ = accountPress(m, tea.KeySpace, " ")
	m, _ = accountPress(m, tea.KeyEscape, "")
	m, cmd := accountPress(m, tea.KeyEscape, "")
	if cmd == nil || !m.searching || !m.q.Has(bc.FacetFormat, "EBOOK") {
		t.Fatal("applying search filter did not start search")
	}
	next, _ := m.update(cmd())
	m = next.(Model)
	if m.err != nil || requests != 1 || m.searching || len(m.results) == 0 {
		t.Fatalf("search failed: %v, %d requests", m.err, requests)
	}
}
