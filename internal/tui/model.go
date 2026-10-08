// Package tui is the Bubble Tea interface for browsing a library catalog.
package tui

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"shelfcheck/internal/account"
	bc "shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/libby"
	"shelfcheck/internal/recommend"
)

type screen int

const (
	searchScreen screen = iota
	resultsScreen
	detailScreen
	accountScreen
	wishlistScreen
	forYouScreen
	hardcoverScreen
)

var searchTypes = []bc.SearchType{bc.Keyword, bc.Title, bc.Author, bc.Subject, bc.Genre, bc.Series}

const requestTimeout = 20 * time.Second

// Model is the root Bubble Tea model.
type Model struct {
	client   *bc.Client
	home     string // home branch code
	homeOnly bool   // new searches start filtered to copies on the shelf at home
	homeName string
	branches map[string]string // code -> name

	screen        screen
	width, height int

	input   textinput.Model
	typeIdx int

	// Current search.
	q           bc.Query
	results     []bc.Bib
	facets      map[string][]bc.Facet // refinements for the current search
	total       int
	partial     bool // results are the best-ranked slice of total
	page, pages int
	cursor      int
	offset      int // first visible result row
	searchSeq   int // drops responses from superseded searches
	searching   bool
	loadingMore bool

	// Filter panel, opened with f on the results screen; see filters.go.
	panel     panelState
	draft     bc.Query
	catCursor int
	valCursor int

	// Per-bib data, shared by the results list (home-branch marker) and the
	// detail screen.
	avail        map[string]*bc.Availability
	availPending map[string]bool
	availErr     map[string]error
	details      map[string]*bc.Details
	detailsErr   map[string]error

	selected   bc.Bib
	backScreen screen // where esc on the detail screen returns to

	// Account and wishlist screens; see account.go. acct is nil when the
	// patron's files aren't available (e.g. over SSH).
	acct        *account.Store
	acctLoading bool
	acctLoaded  bool
	acctErr     error
	checkouts   []bc.Checkout
	holds       []bc.Hold
	recent      []bc.HistoryItem
	acctCursor  int

	wishSeq     int
	wishLoading bool
	wishErr     error
	wishTitles  []libby.Tagging
	wishItems   []account.WishItem // checked so far, in list order
	wishFailed  int
	wishAll     bool
	wishCursor  int

	// "For you" screen; see foryou.go.
	recSeq     int
	recLoading bool
	recDone    bool
	recStage   string
	recErr     error
	recPicks   []recommend.Pick
	recProfile recommend.Profile
	recCursor  int

	hc hcState // Hardcover prompt or list; see hardcover.go

	vp   viewport.Model
	spin spinner.Model
	err  error
}

type (
	searchMsg struct {
		seq  int
		more bool
		res  *bc.SearchResult
		err  error
	}
	availMsg struct {
		id  string
		a   *bc.Availability
		err error
	}
	detailsMsg struct {
		id  string
		d   *bc.Details
		err error
	}
	locationsMsg struct {
		locs []bc.Location
		err  error
	}
)

// New builds the model. If query is non-empty it is searched immediately.
// New builds the model. If homeOnly, searches start narrowed to titles on
// the shelf at the home branch.
//
// acct gives access to the patron's account and Libby wishlist; pass nil to
// leave those screens unavailable.
func New(client *bc.Client, home string, homeOnly bool, query string, acct *account.Store) Model {
	in := textinput.New()
	in.Prompt = "› "
	in.Placeholder = "title, author, subject…"
	in.CharLimit = 200
	in.SetValue(query)
	in.Focus()

	m := Model{
		client:       client,
		home:         home,
		homeOnly:     homeOnly,
		acct:         acct,
		homeName:     "branch " + home,
		branches:     map[string]string{},
		input:        in,
		avail:        map[string]*bc.Availability{},
		availPending: map[string]bool{},
		availErr:     map[string]error{},
		details:      map[string]*bc.Details{},
		detailsErr:   map[string]error{},
		vp:           viewport.New(),
		spin:         spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(accentStyle)),
	}
	if strings.TrimSpace(query) != "" {
		m.searchSeq = 1
		m.searching = true
		m.q = m.newQuery(strings.TrimSpace(query), bc.Keyword)
	}
	return m
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spin.Tick, m.fetchLocations()}
	if m.searching {
		cmds = append(cmds, m.doSearch(m.searchSeq, m.q, 1, false))
	}
	return tea.Batch(cmds...)
}

// --- commands ---------------------------------------------------------------

func (m Model) doSearch(seq int, q bc.Query, page int, more bool) tea.Cmd {
	c := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		res, err := c.Search(ctx, q, page)
		return searchMsg{seq: seq, more: more, res: res, err: err}
	}
}

func (m Model) fetchAvail(id string) tea.Cmd {
	c := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		a, err := c.Availability(ctx, id)
		return availMsg{id: id, a: a, err: err}
	}
}

func (m Model) fetchDetails(id string) tea.Cmd {
	c := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		d, err := c.Details(ctx, id)
		return detailsMsg{id: id, d: d, err: err}
	}
}

func (m Model) fetchLocations() tea.Cmd {
	c := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		locs, err := c.Locations(ctx)
		return locationsMsg{locs: locs, err: err}
	}
}

// --- update -----------------------------------------------------------------

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.SetWidth(max(10, min(60, m.width-8)))
		m.vp.SetWidth(m.width)
		m.vp.SetHeight(m.bodyHeight())
		m.clampScroll()
		if m.screen == detailScreen {
			m.refreshDetail()
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case locationsMsg:
		for _, l := range msg.locs {
			m.branches[l.Code] = l.Name
			if l.Code == m.home {
				m.homeName = l.Name
			}
		}
		return m, nil

	case searchMsg:
		if msg.seq != m.searchSeq {
			return m, nil
		}
		m.searching, m.loadingMore = false, false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		if msg.more {
			m.results = append(m.results, msg.res.Bibs...)
		} else {
			m.results = msg.res.Bibs
			m.facets = msg.res.Facets
			m.cursor, m.offset = 0, 0
			m.screen = resultsScreen
			m.input.Blur()
		}
		m.total, m.page, m.pages = msg.res.Total, msg.res.Page, msg.res.Pages
		m.partial = msg.res.Partial
		return m, nil

	case availMsg:
		delete(m.availPending, msg.id)
		if msg.err != nil {
			m.availErr[msg.id] = msg.err
		} else {
			delete(m.availErr, msg.id)
			m.avail[msg.id] = msg.a
		}
		if m.screen == detailScreen && m.selected.ID == msg.id {
			m.refreshDetail()
		}
		return m, nil

	case detailsMsg:
		if msg.err != nil {
			m.detailsErr[msg.id] = msg.err
		} else {
			m.details[msg.id] = msg.d
		}
		if m.screen == detailScreen && m.selected.ID == msg.id {
			m.refreshDetail()
		}
		return m, nil

	case accountMsg:
		m.acctLoading = false
		m.acctErr = msg.err
		if msg.err == nil {
			m.acctLoaded = true
			m.checkouts, m.holds, m.recent = msg.checkouts, msg.holds, msg.recent
			m.acctCursor = min(m.acctCursor, max(0, len(m.acctRows())-1))
		}
		return m, nil

	case wishTitlesMsg, wishCheckedMsg:
		return m.handleWishMsg(msg)

	case recProgressMsg, recDoneMsg:
		return m.handleRecMsg(msg)

	case hcCollectionMsg, hcCheckedMsg:
		return m.handleHardcoverMsg(msg)

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if mm, cmd, ok := m.navKey(msg.String()); ok {
			return mm, cmd
		}
		switch m.screen {
		case accountScreen:
			return m.updateAccount(msg)
		case wishlistScreen:
			return m.updateWishlist(msg)
		case forYouScreen:
			return m.updateForYou(msg)
		case hardcoverScreen:
			return m.updateHardcover(msg)
		case searchScreen:
			return m.updateSearch(msg)
		case resultsScreen:
			return m.updateResults(msg)
		case detailScreen:
			return m.updateDetail(msg)
		}
	}

	if m.screen == searchScreen {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) updateSearch(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		q := strings.TrimSpace(m.input.Value())
		if q == "" || m.searching {
			return m, nil
		}
		if isHardcoverURL(q) {
			return m.openHardcover(q, searchScreen)
		}
		m.searchSeq++
		m.searching = true
		m.err = nil
		m.q = m.newQuery(q, searchTypes[m.typeIdx])
		return m, m.doSearch(m.searchSeq, m.q, 1, false)
	case "tab":
		m.typeIdx = (m.typeIdx + 1) % len(searchTypes)
		return m, nil
	case "shift+tab":
		m.typeIdx = (m.typeIdx + len(searchTypes) - 1) % len(searchTypes)
		return m, nil
	case "esc":
		if len(m.results) > 0 {
			m.screen = resultsScreen
			m.input.Blur()
			return m, nil
		}
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) updateResults(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.panel != panelClosed {
		return m.updatePanel(msg)
	}
	page := max(1, m.listHeight())
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "f":
		return m.openPanel(), nil
	case "F":
		return m.runQuery(bc.Query{Text: m.q.Text, Type: m.q.Type})
	case "a":
		return m.toggleHome()
	case "o":
		return m.cycleSort()
	case "/", "s", "esc":
		return m, m.toSearch()
	case "up", "k":
		m.cursor--
	case "down", "j":
		m.cursor++
	case "pgup", "ctrl+u":
		m.cursor -= page
	case "pgdown", "ctrl+d", "space":
		m.cursor += page
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = len(m.results) - 1
	case "enter", "right", "l":
		if len(m.results) > 0 {
			return m.openDetail()
		}
		return m, nil
	default:
		return m, nil
	}
	m.clampScroll()
	return m, m.maybeLoadMore()
}

func (m Model) updateDetail(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "esc", "backspace", "left", "h":
		m.screen = m.backScreen
		return m, nil
	case "/", "s":
		return m, m.toSearch()
	case "r":
		// The client caches availability for a minute, so this is cheap to mash.
		if !m.availPending[m.selected.ID] {
			m.availPending[m.selected.ID] = true
			m.refreshDetail()
			return m, m.fetchAvail(m.selected.ID)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

// newQuery starts a fresh search, carrying over only the home-branch preference.
func (m Model) newQuery(text string, typ bc.SearchType) bc.Query {
	q := bc.Query{Text: text, Type: typ}
	if m.homeOnly {
		q.Toggle(bc.FacetStatus, m.home)
	}
	return q
}

func (m *Model) toSearch() tea.Cmd {
	m.screen = searchScreen
	m.input.CursorEnd()
	return m.input.Focus()
}

func (m Model) openDetail() (tea.Model, tea.Cmd) {
	return m.openBib(m.results[m.cursor], resultsScreen)
}

func (m *Model) refreshDetail() {
	m.vp.SetContent(m.renderDetail())
}

// maybeLoadMore fetches the next page when the cursor nears the end.
func (m *Model) maybeLoadMore() tea.Cmd {
	if m.loadingMore || m.searching || m.page >= m.pages || m.cursor < len(m.results)-5 {
		return nil
	}
	m.loadingMore = true
	return m.doSearch(m.searchSeq, m.q, m.page+1, true)
}

func (m *Model) clampScroll() {
	m.cursor = max(0, min(m.cursor, len(m.results)-1))
	h := max(1, m.listHeight())
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	m.offset = max(0, min(m.offset, len(m.results)-h))
}
