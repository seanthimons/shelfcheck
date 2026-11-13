package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	bc "shelfcheck/internal/bibliocommons"
)

// The filter panel edits a draft query for search or the current discovery list.
// Changes apply when the panel closes.

type panelState int

const (
	panelClosed panelState = iota
	panelCategories
	panelValues
)

type category struct {
	field string // facet field; "" for sort
	name  string
}

var categories = []category{
	{bc.FacetStatus, "Available"},
	{bc.FacetFormat, "Format"},
	{bc.FacetGenre, "Genre"},
	{bc.FacetRating, "Rating"},
	{bc.FacetFiction, "Fiction"},
	{bc.FacetAudience, "Audience"},
	{bc.FacetNew, "Recently added"},
	{bc.FacetYear, "Year"},
	{bc.FacetLanguage, "Language"},
	{bc.FacetAuthor, "Author"},
	{"", "Sort"},
}

var sortOptions = []string{bc.SortRelevance, bc.SortTopRated, bc.SortPublished, bc.SortNewlyAcquired, bc.SortTitle, bc.SortAuthor}

var sortLabels = map[string]string{
	bc.SortRelevance:     "Relevance",
	bc.SortTopRated:      "Top rated",
	bc.SortPublished:     "Newest published",
	bc.SortNewlyAcquired: "Recently added",
	bc.SortTitle:         "Title",
	bc.SortAuthor:        "Author",
}

// option is one row in the values list. Count < 0 means unknown.
type option struct {
	value string
	count int
}

// options lists the values for a category: the facet values from the
// current results, plus anything selected in the draft that isn't among
// them (so it can still be unselected).
func (m Model) options(c category) []option {
	if c.field == "" {
		out := make([]option, len(sortOptions))
		for i, s := range sortOptions {
			out[i] = option{s, -1}
		}
		return out
	}
	var out []option
	seen := map[string]bool{}
	add := func(v string, n int) {
		if !seen[v] {
			seen[v] = true
			out = append(out, option{v, n})
		}
	}
	if c.field == bc.FacetStatus {
		// Home branch first, then the catch-alls, then other branches.
		add(m.home, m.facetCount(c.field, m.home))
		add("_physical_", m.facetCount(c.field, "_physical_"))
		add("_online_", m.facetCount(c.field, "_online_"))
	}
	for _, f := range m.panelFacets()[c.field] {
		add(f.Value, f.Count)
	}
	for _, v := range m.draft.Filters[c.field] {
		add(v, -1)
	}
	return out
}

func (m Model) facetCount(field, value string) int {
	for _, f := range m.panelFacets()[field] {
		if f.Value == value {
			return f.Count
		}
	}
	return -1
}

// visibleCategories hides categories with nothing to pick.
func (m Model) visibleCategories() []category {
	facets := m.panelFacets()
	var out []category
	for _, c := range categories {
		if m.localFilters() && (c.field == "" || c.field == bc.FacetStatus || c.field == bc.FacetNew) {
			continue
		}
		if c.field == "" || c.field == bc.FacetStatus || len(facets[c.field]) > 0 || len(m.draft.Filters[c.field]) > 0 {
			out = append(out, c)
		}
	}
	return out
}

func (m Model) valueLabel(field, v string) string {
	switch field {
	case "":
		return sortLabels[v]
	case bc.FacetStatus:
		switch v {
		case "_physical_":
			return "On a shelf anywhere"
		case "_online_":
			return "Available online"
		}
		name := m.branchName(v)
		if v == m.home {
			name += " (home)"
		}
		return "On shelf at " + name
	case bc.FacetFormat:
		return formatLabel(v)
	case bc.FacetRating:
		if n, ok := map[string]string{"FOUR_PLUS_STARS": "4", "THREE_PLUS_STARS": "3", "TWO_PLUS_STARS": "2", "ONE_PLUS_STARS": "1"}[v]; ok {
			return "★ " + n + "+"
		}
	case bc.FacetFiction:
		if v == "UNDETERMINED" {
			return "Not specified"
		}
	case bc.FacetAudience:
		if v == "juvenile" {
			return "Children"
		}
	case bc.FacetNew:
		if d, ok := strings.CutPrefix(v, "PAST_"); ok {
			return "Last " + strings.ToLower(strings.ReplaceAll(d, "_", " "))
		}
		if v == "OLDER_THAN_180_DAYS" {
			return "Older"
		}
	case bc.FacetLanguage:
		if l, ok := languages[v]; ok {
			return l
		}
	case bc.FacetGenre, bc.FacetYear, bc.FacetAuthor:
		return v
	}
	return titleCase(v)
}

var languages = map[string]string{
	"eng": "English", "spa": "Spanish", "fre": "French", "ger": "German", "ita": "Italian",
	"rus": "Russian", "chi": "Chinese", "jpn": "Japanese", "kor": "Korean", "vie": "Vietnamese",
	"ara": "Arabic", "por": "Portuguese", "lat": "Latin", "heb": "Hebrew", "hin": "Hindi",
	"und": "Undetermined", "zxx": "No language", "mul": "Multiple",
}

func (m Model) branchName(code string) string {
	if n, ok := m.branches[code]; ok {
		return n
	}
	return "branch " + code
}

// summary describes a category's current draft selection.
func (m Model) summary(q bc.Query, c category) string {
	if c.field == "" {
		return sortLabels[q.Sort]
	}
	var labels []string
	for _, v := range q.Filters[c.field] {
		labels = append(labels, m.valueLabel(c.field, v))
	}
	return strings.Join(labels, ", ")
}

// filterSummary is the one-line description of q's filters and sort shown
// above the results.
func (m Model) filterSummary(q bc.Query) string {
	var parts []string
	for _, c := range categories {
		if c.field == "" {
			continue
		}
		if s := m.summary(q, c); s != "" {
			parts = append(parts, s)
		}
	}
	if q.Sort != "" {
		parts = append(parts, "sorted by "+strings.ToLower(sortLabels[q.Sort]))
	}
	return strings.Join(parts, " · ")
}

// --- update -----------------------------------------------------------------

func (m Model) openPanel() Model {
	m.draft = m.activeFilters().Clone()
	m.panel = panelCategories
	m.catCursor = 0
	return m
}

func (m Model) updatePanel(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	cats := m.visibleCategories()
	m.catCursor = min(m.catCursor, len(cats)-1)
	cat := cats[m.catCursor]
	key := msg.String()

	if key == "q" {
		return m, tea.Quit
	}

	if m.panel == panelCategories {
		switch key {
		case "up", "k":
			m.catCursor = max(0, m.catCursor-1)
		case "down", "j":
			m.catCursor = min(len(cats)-1, m.catCursor+1)
		case "enter", "right", "l", "space":
			m.panel, m.valCursor = panelValues, 0
		case "x", "backspace":
			if cat.field == "" {
				m.draft.Sort = ""
			} else {
				delete(m.draft.Filters, cat.field)
			}
		case "c":
			m.draft.Filters, m.draft.Sort = nil, ""
		case "esc", "f":
			m.panel = panelClosed
			if m.localFilters() {
				return m.applyListFilters(m.draft), nil
			}
			return m.runQuery(m.draft)
		}
		return m, nil
	}

	opts := m.options(cat)
	switch key {
	case "up", "k":
		m.valCursor = max(0, m.valCursor-1)
	case "down", "j":
		m.valCursor = min(len(opts)-1, m.valCursor+1)
	case "pgup":
		m.valCursor = max(0, m.valCursor-m.panelListHeight())
	case "pgdown":
		m.valCursor = min(len(opts)-1, m.valCursor+m.panelListHeight())
	case "enter", "space":
		if len(opts) == 0 {
			return m, nil
		}
		v := opts[m.valCursor].value
		if cat.field == "" {
			m.draft.Sort = v
			m.panel = panelCategories
		} else {
			m.draft.Toggle(cat.field, v)
		}
	case "esc", "left", "h", "backspace":
		m.panel = panelCategories
	}
	return m, nil
}

// runQuery reruns the current search with q's filters and sort, if they
// differ from what's showing.
func (m Model) runQuery(q bc.Query) (tea.Model, tea.Cmd) {
	if m.filterSummary(q) == m.filterSummary(m.q) {
		return m, nil
	}
	m.q = q
	// Whatever the home filter ends up as becomes the default for new searches.
	m.homeOnly = q.Has(bc.FacetStatus, m.home)
	m.searchSeq++
	m.searching = true
	m.loadingMore = false
	m.err = nil
	return m, m.doSearch(m.searchSeq, m.q, 1, false)
}

// toggleHome flips the "on shelf at home branch" filter.
func (m Model) toggleHome() (tea.Model, tea.Cmd) {
	q := m.q.Clone()
	q.Toggle(bc.FacetStatus, m.home)
	return m.runQuery(q)
}

// cycleSort steps through the most useful sort orders.
func (m Model) cycleSort() (tea.Model, tea.Cmd) {
	order := []string{bc.SortRelevance, bc.SortTopRated, bc.SortPublished, bc.SortNewlyAcquired}
	next := bc.SortRelevance
	for i, s := range order {
		if s == m.q.Sort {
			next = order[(i+1)%len(order)]
		}
	}
	q := m.q.Clone()
	q.Sort = next
	return m.runQuery(q)
}

// --- view -------------------------------------------------------------------

// The panel is a box over the faded results, panelW wide inside its
// border, with two lines of heading over its list.
func (m Model) panelW() int          { return max(30, min(72, m.width-8)) }
func (m Model) panelListHeight() int { return max(1, m.bodyHeight()-6) }

func (m Model) viewPanel() string {
	cats := m.visibleCategories()
	cat := cats[min(m.catCursor, len(cats)-1)]
	w := m.panelW()
	nameW := 16

	if m.panel == panelCategories {
		rows := []string{ansi.Truncate("Filter "+titleStyle.Render(fmt.Sprintf("%q", m.filterTitle())), w, "…"), ""}
		for i, c := range cats {
			line := pad(c.name, nameW) + " " + accentStyle.Render(pad(m.summary(m.draft, c), max(10, w-nameW-3)))
			rows = append(rows, cursorRow(i == m.catCursor, line))
		}
		return box(strings.Join(rows, "\n"), accentColor)
	}

	rows := []string{ansi.Truncate(titleStyle.Render(cat.name)+dimStyle.Render(fmt.Sprintf(" · filtering %q", m.filterTitle())), w, "…"), ""}
	opts := m.options(cat)
	h := min(len(opts), m.panelListHeight())
	start := max(0, min(m.valCursor-h+1, len(opts)-h))
	labelW := max(10, w-2-4-8-2)
	var list []string
	for i := start; i < min(len(opts), start+h); i++ {
		o := opts[i]
		selected := m.draft.Has(cat.field, o.value)
		if cat.field == "" {
			selected = m.draft.Sort == o.value
		}
		mark := "[ ] "
		switch {
		case cat.field == "" && selected:
			mark = okStyle.Render("(•) ")
		case cat.field == "":
			mark = "( ) "
		case selected:
			mark = okStyle.Render("[✓] ")
		}
		count := ""
		if o.count >= 0 {
			count = fmt.Sprint(o.count)
		}
		line := pad(m.valueLabel(cat.field, o.value), labelW) + " " + fmt.Sprintf("%7s", count)
		list = append(list, cursorRow(i == m.valCursor, mark+line))
	}
	rows = append(rows, withScrollbar(list, scrollbar(h, len(opts), start), w)...)
	if len(opts) == 0 {
		rows = append(rows, dimStyle.Render("Nothing to filter by in these results."))
	}
	return box(strings.Join(rows, "\n"), accentColor)
}
