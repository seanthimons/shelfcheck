package tui

import (
	"sort"
	"strings"

	"shelfcheck/internal/account"
	bc "shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/recommend"
)

func (m Model) localFilters() bool {
	switch m.screen {
	case wishlistScreen, forYouScreen, hardcoverScreen, arrivalsScreen:
		return true
	}
	return false
}

func (m Model) activeFilters() bc.Query {
	if m.localFilters() {
		return m.listFilters[m.screen]
	}
	return m.q
}

func (m Model) filterTitle() string {
	if m.localFilters() {
		return map[screen]string{wishlistScreen: "Wishlist", forYouScreen: "For you", hardcoverScreen: "Hardcover", arrivalsScreen: "New arrivals"}[m.screen]
	}
	return m.q.Text
}

func (m Model) applyListFilters(q bc.Query) Model {
	if m.listFilters == nil {
		m.listFilters = map[screen]bc.Query{}
	}
	m.listFilters[m.screen] = q.Clone()
	switch m.screen {
	case wishlistScreen:
		m.wishCursor = 0
	case forYouScreen:
		m.recCursor = 0
	case hardcoverScreen:
		m.hc.cursor = 0
	case arrivalsScreen:
		m.arr.cursor = 0
	}
	return m
}

// Values come from catalog editions. Unknown metadata cannot satisfy a filter.
func bibFilterValues(b bc.Bib) map[string][]string {
	values := map[string][]string{
		bc.FacetFormat: {b.Format}, bc.FacetGenre: b.Genres,
		bc.FacetFiction: {b.ContentType}, bc.FacetYear: {b.Year},
		bc.FacetLanguage: {b.Language}, bc.FacetAuthor: b.Authors,
	}
	for _, a := range b.Audiences {
		values[bc.FacetAudience] = append(values[bc.FacetAudience], strings.ToLower(a))
	}
	if b.Rating.Count > 0 {
		for n, v := range []string{"ONE_PLUS_STARS", "TWO_PLUS_STARS", "THREE_PLUS_STARS", "FOUR_PLUS_STARS"} {
			if b.Rating.Stars() >= float64(n+1) {
				values[bc.FacetRating] = append(values[bc.FacetRating], v)
			}
		}
	}
	return values
}

func matchesFilters(b bc.Bib, q bc.Query) bool {
	if !q.Narrowed() {
		return true
	}
	values := bibFilterValues(b)
	for field, wanted := range q.Filters {
		if len(wanted) == 0 {
			continue
		}
		matched := false
		for _, want := range wanted {
			for _, v := range values[field] {
				if v != "" && strings.EqualFold(v, want) {
					matched = true
				}
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func filterWish(w account.WishItem, q bc.Query) (account.WishItem, bool) {
	if !q.Narrowed() {
		return w, true
	}
	hadMatches := len(w.Matches) > 0
	var matches []account.WishMatch
	for _, match := range w.Matches {
		if matchesFilters(match.Bib, q) {
			matches = append(matches, match)
		}
	}
	w.Matches = matches
	if len(matches) > 0 {
		return w, true
	}
	if hadMatches {
		return w, false
	}
	// Source titles without catalog matches have only title/author metadata.
	return w, matchesFilters(bc.Bib{Title: w.Tagging.Title, Authors: []string{w.Tagging.Author}}, q)
}

func pickEditions(p recommend.Pick) []bc.Bib {
	if len(p.Editions) > 0 {
		return p.Editions
	}
	return []bc.Bib{p.Bib}
}

func (m Model) visibleRecommendations() ([]recommend.Pick, recommend.Profile) {
	q := m.listFilters[forYouScreen]
	if !q.Narrowed() {
		return m.recPicks, m.recProfile
	}
	var picks []recommend.Pick
	for _, p := range m.recPicks {
		var editions []bc.Bib
		for _, b := range pickEditions(p) {
			if matchesFilters(b, q) {
				editions = append(editions, b)
			}
		}
		if len(editions) == 0 {
			continue
		}
		p.Bib, p.Editions = editions[0], editions
		p.Print, p.Ebook = false, false
		for _, b := range editions {
			if b.Digital || account.DigitalFormats[b.Format] {
				p.Ebook = true
			} else {
				p.Print = true
				p.Bib = b
			}
		}
		picks = append(picks, p)
	}
	profile := m.recProfile
	filterElsewhere := func(items []recommend.Elsewhere) []recommend.Elsewhere {
		var out []recommend.Elsewhere
		for _, it := range items {
			b := it.Bib
			if b.ID == "" {
				b = bc.Bib{Title: it.Title, Authors: []string{it.Author}}
			}
			if matchesFilters(b, q) {
				out = append(out, it)
			}
		}
		return out
	}
	profile.NotOnShelf = filterElsewhere(profile.NotOnShelf)
	profile.ToBuy = filterElsewhere(profile.ToBuy)
	// Series notices have no edition metadata; don't present them as filtered matches.
	profile.UpNext = nil
	return picks, profile
}

func (m Model) visibleArrivals() []recommend.Arrival {
	var out []recommend.Arrival
	for _, a := range m.arr.res.Items {
		if matchesFilters(a.Bib, m.listFilters[arrivalsScreen]) {
			out = append(out, a)
		}
	}
	return out
}

// Local facet counts describe loaded catalog editions, independent of the draft.
func (m Model) panelFacets() map[string][]bc.Facet {
	if !m.localFilters() {
		return m.facets
	}
	var bibs []bc.Bib
	switch m.screen {
	case wishlistScreen, hardcoverScreen:
		items := m.wishItems
		if m.screen == hardcoverScreen {
			items = m.hc.items
		}
		for _, it := range items {
			if len(it.Matches) == 0 {
				bibs = append(bibs, bc.Bib{Authors: []string{it.Tagging.Author}})
			}
			for _, match := range it.Matches {
				bibs = append(bibs, match.Bib)
			}
		}
	case forYouScreen:
		for _, p := range m.recPicks {
			bibs = append(bibs, pickEditions(p)...)
		}
		for _, list := range [][]recommend.Elsewhere{m.recProfile.NotOnShelf, m.recProfile.ToBuy} {
			for _, it := range list {
				b := it.Bib
				if b.ID == "" {
					b.Authors = []string{it.Author}
				}
				bibs = append(bibs, b)
			}
		}
	case arrivalsScreen:
		for _, a := range m.arr.res.Items {
			bibs = append(bibs, a.Bib)
		}
	}
	counts := map[string]map[string]int{bc.FacetFormat: {"BK": 0}}
	if m.screen != arrivalsScreen {
		counts[bc.FacetFormat]["EBOOK"] = 0
	}
	for _, b := range bibs {
		for field, values := range bibFilterValues(b) {
			if counts[field] == nil {
				counts[field] = map[string]int{}
			}
			seen := map[string]bool{}
			for _, v := range values {
				if v != "" && !seen[v] {
					counts[field][v]++
					seen[v] = true
				}
			}
		}
	}
	facets := map[string][]bc.Facet{}
	for field, values := range counts {
		for value, count := range values {
			facets[field] = append(facets[field], bc.Facet{Value: value, Count: count})
		}
		sort.Slice(facets[field], func(i, j int) bool { return facets[field][i].Value < facets[field][j].Value })
	}
	return facets
}
