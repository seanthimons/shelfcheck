package tui

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"regexp"
	"runtime/debug"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	bc "shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/theme"
)

// Styles, set from the current theme by ApplyTheme. The default theme
// uses ANSI palette colors, so the UI follows the terminal's own.
var (
	accentStyle, titleStyle, headingStyle, dimStyle, okStyle, warnStyle,
	errStyle, starStyle, selStyle, tabOnStyle, barStyle, barWarnStyle,
	sigilStyle, ruleStyle, footKeyStyle, footDescStyle lipgloss.Style

	// Colors for borders, the cursor row's tint and the footer bar; nil
	// when the theme leaves them to the terminal.
	accentColor, borderColor, selBG, barBG color.Color
)

func init() { ApplyTheme(theme.Theme{Name: theme.Terminal}) }

// The body is what's between the header (a line and its rule) and the
// footer; lists in it have two lines of heading and a row of column labels.
func (m Model) bodyHeight() int { return max(1, m.height-3) }
func (m Model) listHeight() int {
	height := m.bodyHeight() - 3
	if m.localFilters() && m.activeFilters().Narrowed() {
		height--
	}
	return max(1, height)
}

func (m Model) View() tea.View {
	var body string
	switch m.screen {
	case searchScreen:
		body = m.viewSearch()
	case resultsScreen:
		body = m.viewResults()
	case detailScreen:
		body = m.viewDetail()
	case accountScreen:
		body = m.viewAccount()
	case wishlistScreen:
		body = m.viewWishlist()
	case forYouScreen:
		body = m.viewForYou()
	case hardcoverScreen:
		body = m.viewHardcover()
	case arrivalsScreen:
		body = m.viewArrivals()
	}
	if m.localFilters() {
		if summary := m.filterSummary(m.activeFilters()); summary != "" {
			body = "  " + accentStyle.Render(ansi.Truncate("Filters: "+summary+" · F clears", max(1, m.width-4), "…")) + "\n" + body
		}
		if m.panel != panelClosed {
			body = m.overlay(body, m.viewPanel())
		}
	}
	if m.hcEdit.active {
		body = m.viewHCEditor()
	}
	body = lipgloss.NewStyle().Height(m.bodyHeight()).MaxHeight(m.bodyHeight()).Render(body)
	if m.showHelp {
		body = m.overlay(body, m.viewHelp())
	}
	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, m.viewHeader(), body, m.viewFooter()))
	v.AltScreen = true
	v.WindowTitle = "shelfcheck"
	if t := m.theme(); t.BG != "" {
		v.BackgroundColor = lipgloss.Color(t.BG)
	}
	if t := m.theme(); t.FG != "" {
		v.ForegroundColor = lipgloss.Color(t.FG)
	}
	return v
}

// buildStamp names the running build, so a stale copy is easy to spot: the
// commit (with * when built from uncommitted changes) and when the binary
// was built (its file time), e.g. "6abcd97* 16:43".
var buildStamp = func() string {
	var rev string
	dirty := false
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, st := range bi.Settings {
			switch st.Key {
			case "vcs.revision":
				rev = st.Value[:min(7, len(st.Value))]
			case "vcs.modified":
				dirty = st.Value == "true"
			}
		}
	}
	if dirty {
		rev += "*"
	}
	if exe, err := os.Executable(); err == nil {
		if fi, err := os.Stat(exe); err == nil {
			t := fi.ModTime()
			layout := "Jan 2 15:04"
			if t.Format(time.DateOnly) == time.Now().Format(time.DateOnly) {
				layout = "15:04" // built today
			}
			rev = strings.TrimSpace(rev + " " + t.Format(layout))
		}
	}
	if rev == "" {
		return "dev"
	}
	return rev
}()

// --- search -----------------------------------------------------------------

func (m Model) viewSearch() string {
	var tabs []string
	for i, t := range searchTypes {
		label := strings.ToUpper(string(t)[:1]) + string(t)[1:]
		if i == m.typeIdx {
			tabs = append(tabs, tabOnStyle.Render(label))
		} else {
			tabs = append(tabs, dimStyle.Render(label))
		}
	}
	w := min(m.width-4, 66)
	input := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Width(w)
	if accentColor != nil {
		input = input.BorderForeground(accentColor)
	}
	status := dimStyle.Render("or paste a Hardcover prompt or list link")
	if m.searching {
		status = m.spin.View() + " Searching…"
	}
	block := lipgloss.JoinVertical(lipgloss.Center,
		titleStyle.Render("Search the catalog")+dimStyle.Render(" · "+m.homeName),
		"",
		strings.Join(tabs, "   "),
		input.Render(m.input.View()),
		status,
	)
	// A little above center reads as centered.
	h := m.bodyHeight()
	top := max(0, (h-lipgloss.Height(block))*2/5)
	return strings.Repeat("\n", top) + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, block)
}

// --- results ----------------------------------------------------------------

func (m Model) viewResults() string {
	if m.searching {
		return m.notice(m.spin.View() + " Searching…")
	}
	list := m.viewResultList()
	if m.panel != panelClosed {
		return m.overlay(list, m.viewPanel())
	}
	return list
}

func (m Model) viewResultList() string {
	head := fmt.Sprintf("%s for %s", plural(m.total, "result"), titleStyle.Render(fmt.Sprintf("%q", m.q.Text)))
	if m.q.Type != "" && m.q.Type != bc.Keyword {
		head += dimStyle.Render(" (" + string(m.q.Type) + ")")
	}
	if m.partial {
		head += dimStyle.Render(fmt.Sprintf(" · showing the top %d", len(m.results)))
	}
	filters := m.filterSummary(m.q)
	if len(m.results) == 0 {
		msg := "No results for " + titleStyle.Render(fmt.Sprintf("%q", m.q.Text))
		if m.q.Has(bc.FacetStatus, m.home) {
			return m.notice(msg + " on the shelf at " + m.homeName + ".\nPress a to look at every branch.")
		}
		if filters != "" {
			return m.notice(msg + " with " + accentStyle.Render(filters) + ".\nPress f to change filters or F to clear them.")
		}
		return m.notice(msg + ". Press / to search again.")
	}
	count := m.total
	if m.partial {
		count = len(m.results)
	}
	head = spread("  "+head, dimStyle.Render(fmt.Sprintf("%d/%d", m.cursor+1, count)), m.width-2)

	// Columns: marker, title, author, format, year, rating, availability,
	// home; the last two cells on the right are for the scrollbar.
	const formatW, yearW, ratingW, availW, homeW = 12, 4, 11, 15, 6
	authorW := 22
	if m.width < 100 {
		authorW = 0
	}
	fixed := 2 + formatW + yearW + ratingW + availW + homeW + 6 + 2
	if authorW > 0 {
		fixed += authorW + 1
	}
	titleW := max(12, m.width-fixed)

	labels := []string{colHead("TITLE", titleW)}
	if authorW > 0 {
		labels = append(labels, colHead("AUTHOR", authorW))
	}
	labels = append(labels, colHead("FORMAT", formatW), colHead("YEAR", yearW), colHead("RATING", ratingW),
		colHead("AVAILABLE", availW), colHead("HOME", homeW))

	rows := []string{head, "  " + accentStyle.Render(ansi.Truncate(filters, max(1, m.width-4), "…")),
		"  " + strings.Join(labels, " ")}
	var list []string
	end := min(len(m.results), m.offset+m.listHeight())
	for i := m.offset; i < end; i++ {
		b := m.results[i]
		title := b.Title
		if b.Subtitle != "" {
			title += ": " + b.Subtitle
		}
		cols := []string{pad(title, titleW)}
		if authorW > 0 {
			cols = append(cols, pad(firstOr(b.Authors, ""), authorW))
		}
		cols = append(cols,
			pad(formatLabel(b.Format), formatW),
			pad(yearOf(b.Year), yearW),
			pad(ratingShort(b.Rating), ratingW),
			pad(availShort(b.Summary), availW),
			pad(m.homeMarker(b), homeW),
		)
		line := strings.Join(cols, " ")
		switch {
		case i == m.cursor && selBG == nil:
			list = append(list, accentStyle.Render("▸ ")+selStyle.Render(ansi.Strip(line)))
		case i == m.cursor:
			list = append(list, cursorRow(true, line))
		default:
			list = append(list, "  "+m.styleRow(b, line))
		}
	}
	list = withScrollbar(list, scrollbar(m.listHeight(), count, m.offset), m.width)
	return strings.Join(append(rows, list...), "\n")
}

func (m Model) styleRow(b bc.Bib, line string) string {
	if b.Summary.Available == 0 {
		return dimStyle.Render(line)
	}
	return line
}

// homeMarker says whether a physical copy is on the shelf at the home branch.
func (m Model) homeMarker(b bc.Bib) string {
	if b.Digital || b.Summary.Available == 0 {
		return ""
	}
	// Only titles whose copies we've already looked up (by opening them) get
	// a marker; checking every row would cost a request each.
	a := m.avail[b.ID]
	if a == nil {
		return ""
	}
	for _, c := range a.Copies {
		if c.Available && c.BranchCode == m.home {
			return okStyle.Render("⌂ here")
		}
	}
	return ""
}

// --- detail -----------------------------------------------------------------

// viewDetail is the scrolling detail page, with a scrollbar when it's
// longer than the screen.
func (m Model) viewDetail() string {
	bar := scrollbar(m.vp.Height(), m.vp.TotalLineCount(), m.vp.YOffset())
	if bar == nil {
		return m.vp.View()
	}
	return strings.Join(withScrollbar(strings.Split(m.vp.View(), "\n"), bar, m.width), "\n")
}

// The detail page puts availability in a box beside the rest when there's
// room for both, and above it when there isn't.
const (
	sideW     = 50  // the availability box
	sideAtMin = 130 // the narrowest screen with room for it
)

func (m Model) renderDetail() string {
	b := m.selected
	total := max(20, m.width-6) // less the indent and the scrollbar
	side := m.width >= sideAtMin && b.ID != ""
	w := min(total, 100)
	if side {
		w = min(total-sideW-3, 100)
	}
	wrap := lipgloss.NewStyle().Width(w)
	var out []string
	add := func(lines ...string) { out = append(out, lines...) }

	// The title block sits beside the jacket, if there is one.
	jacket := m.coverLines()
	head := wrap
	if jacket != nil {
		head = lipgloss.NewStyle().Width(max(10, w-coverW-2))
	}
	beside := func(top []string) {
		add("")
		if jacket != nil {
			add(lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(jacket, "\n"), "  ", strings.Join(top, "\n")))
		} else {
			add(top...)
		}
	}

	if b.ID == "" {
		// No library record: Hardcover is all there is.
		title, meta := m.hcHeader()
		top := []string{head.Render(titleStyle.Render(title))}
		if meta != "" {
			top = append(top, head.Render(meta))
		}
		top = append(top, "", head.Render(dimStyle.Render("No record at the library here · c searches the catalog for it (other titles, holds)")))
		beside(top)
		add(m.renderHardcover(w, true)...)
		add("")
		return indent(strings.Join(out, "\n"), 2)
	}

	var top []string
	title := b.Title
	if b.Subtitle != "" {
		title += ": " + b.Subtitle
	}
	top = append(top, head.Render(titleStyle.Render(title)))
	meta := []string{}
	if len(b.Authors) > 0 {
		meta = append(meta, strings.Join(b.Authors, "; "))
	}
	meta = append(meta, formatLabel(b.Format))
	if b.Year != "" {
		meta = append(meta, b.Year)
	}
	if b.Edition != "" {
		meta = append(meta, b.Edition)
	}
	top = append(top, head.Render(strings.Join(meta, " · ")))
	if b.Rating.Count > 0 {
		top = append(top, starStyle.Render(stars(b.Rating.Stars()))+fmt.Sprintf(" %.1f", b.Rating.Stars())+
			dimStyle.Render(fmt.Sprintf(" (%s)", plural(b.Rating.Count, "rating"))))
	}
	if len(b.Series) > 0 {
		top = append(top, head.Render(dimStyle.Render("Series: ")+strings.Join(b.Series, "; ")))
	}
	// Terminal hyperlinks open on the reader's machine, including over SSH.
	// Keep the URL visible for terminals that don't support hyperlinks.
	catalogURL := m.client.RecordURL(b.ID)
	top = append(top, "", ansi.SetHyperlink(catalogURL)+accentStyle.Underline(true).Render("Open in library catalog")+ansi.ResetHyperlink(),
		head.Render(ansi.SetHyperlink(catalogURL)+catalogURL+ansi.ResetHyperlink()),
		head.Render(dimStyle.Render("Place a hold in your browser; choose pickup and confirm there.")))
	beside(top)

	if !side {
		add("")
		add(m.renderAvailability(w, false)...)
	}

	if b.Description != "" || len(b.Genres) > 0 {
		add("", sectionHead("ABOUT", "", w))
		if b.Description != "" {
			add(wrap.Render(b.Description))
		}
		if len(b.Genres) > 0 {
			add("", wrap.Render(dimStyle.Render("Genres: ")+strings.Join(b.Genres, ", ")))
		}
		if len(b.Subjects) > 0 {
			add(wrap.Render(dimStyle.Render("Subjects: ") + strings.Join(b.Subjects, ", ")))
		}
	}

	// Hardcover's description fills in when the library has none.
	add(m.renderHardcover(w, b.Description == "")...)

	d, dErr := m.details[b.ID], m.detailsErr[b.ID]
	switch {
	case dErr != nil:
		add("", errStyle.Render("Couldn't load reviews: "+dErr.Error()))
	case d == nil:
		add("", dimStyle.Render(m.spin.View()+" loading reviews…"))
	default:
		for _, r := range d.Reviews {
			add("", sectionHead("REVIEW", dimStyle.Render(" · "+r.Source), w), wrap.Render(r.Text))
		}
		if d.AuthorNote != "" {
			add("", sectionHead("ABOUT THE AUTHOR", "", w), wrap.Render(d.AuthorNote))
		}
		if d.Excerpt != "" {
			add("", sectionHead("EXCERPT", "", w), wrap.Render(d.Excerpt))
		}
	}
	add("")
	page := strings.Join(out, "\n")
	if side {
		avail := box(strings.Join(m.renderAvailability(sideW-4, true), "\n"), borderColor)
		page = lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(w).Render(page), "   ", "\n"+avail)
	}
	return indent(page, 2)
}

// renderAvailability lists where the selected title's copies are, w wide.
// boxed puts the summary under the heading, for the narrow box beside the
// description.
func (m Model) renderAvailability(w int, boxed bool) []string {
	b := m.selected
	a := m.avail[b.ID]
	if a == nil {
		if err := m.availErr[b.ID]; err != nil {
			return []string{sectionHead("AVAILABILITY", "", w), errStyle.Render("Couldn't load: " + err.Error())}
		}
		return []string{sectionHead("AVAILABILITY", "", w), dimStyle.Render(m.spin.View() + " checking shelves…")}
	}
	s := a.Summary
	summary := availLong(s)
	if m.availPending[b.ID] {
		summary += " " + m.spin.View()
	}
	out := []string{sectionHead("AVAILABILITY", "  "+summary, w)}
	if boxed {
		out = []string{headingStyle.Render("AVAILABILITY"), lipgloss.NewStyle().Width(w).Render(summary)}
	}

	if b.Digital || len(a.Copies) == 0 {
		if len(a.DigitalFormats) > 0 {
			out = append(out, lipgloss.NewStyle().Width(w).Render(dimStyle.Render("Formats: ")+strings.Join(mapStrings(a.DigitalFormats, digitalLabel), ", ")))
		}
		if b.Digital {
			out = append(out, dimStyle.Render("Borrow through Libby / OverDrive."))
		}
		return out
	}

	var home, elsewhereOn []bc.Copy
	var elsewhereOut int
	var nextDue time.Time
	for _, c := range a.Copies {
		switch {
		case c.BranchCode == m.home:
			home = append(home, c)
		case c.Available:
			elsewhereOn = append(elsewhereOn, c)
		default:
			elsewhereOut++
			if !c.Due.IsZero() && (nextDue.IsZero() || c.Due.Before(nextDue)) {
				nextDue = c.Due
			}
		}
	}

	out = append(out, "", titleStyle.Render(m.homeName)+dimStyle.Render(" (home)"))
	if len(home) == 0 {
		out = append(out, dimStyle.Render("  No copies here."))
	}
	for _, c := range home {
		out = append(out, copyLine(c, false, w))
	}
	if len(elsewhereOn) > 0 {
		out = append(out, "", titleStyle.Render("On the shelf elsewhere"))
		for _, c := range elsewhereOn {
			out = append(out, copyLine(c, true, w))
		}
	}
	if elsewhereOut > 0 {
		line := fmt.Sprintf("%s checked out or unavailable at other branches", plural(elsewhereOut, "more copy"))
		if !nextDue.IsZero() {
			line += " · next due " + dueLabel(nextDue)
		}
		out = append(out, "", lipgloss.NewStyle().Width(w).Render(dimStyle.Render(line)))
	}
	return out
}

func copyLine(c bc.Copy, showBranch bool, w int) string {
	icon := dimStyle.Render("·")
	status := c.LibraryStatus
	switch {
	case c.Available:
		icon = okStyle.Render("✓")
		status = okStyle.Render("on shelf")
	case !c.Due.IsZero():
		status = warnStyle.Render("due " + dueLabel(c.Due))
	}
	where := c.Collection
	if showBranch {
		where = c.Branch + dimStyle.Render(" · ") + c.Collection
	}
	call := c.CallNumber
	if c.Volume != "" {
		call += " " + c.Volume
	}
	if w < 64 {
		// Too narrow for columns: the call number and status go underneath.
		return "  " + icon + " " + ansi.Truncate(where, w-4, "…") + "\n" +
			"    " + pad(call, max(4, w-5-ansi.StringWidth(status))) + " " + status
	}
	whereW := max(10, w-4-16-14)
	return "  " + icon + " " + pad(where, whereW) + " " + pad(call, 14) + " " + status
}

// --- formatting helpers -----------------------------------------------------

var formatLabels = map[string]string{
	"BK":            "Book",
	"EBOOK":         "eBook",
	"EAUDIOBOOK":    "eAudiobook",
	"AB_ONLINE":     "eAudiobook",
	"AB":            "Audiobook CD",
	"BOOK_CD":       "Audiobook CD",
	"LPRINT":        "Large print",
	"GRAPHIC_NOVEL": "Graphic novel",
	"DVD":           "DVD",
	"BLURAY":        "Blu-ray",
	"MUSIC_CD":      "Music CD",
	"LP":            "Vinyl",
	"MN":            "Sheet music",
	"UK":            "Other",
	"PICTURE_BOOK":  "Picture book",
	"MAG":           "Magazine",
	"EMAG":          "eMagazine",
	"VIDEO_ONLINE":  "Streaming video",
	"MUSIC_ONLINE":  "Streaming music",
	"BOARD_BK":      "Board book",
	"PICTURE_BK":    "Picture book",
	"KIT":           "Kit",
	"VIDEO_GAME":    "Video game",
}

func formatLabel(code string) string {
	if l, ok := formatLabels[code]; ok {
		return l
	}
	return titleCase(code)
}

var digitalLabels = map[string]string{
	"KINDLE":           "Kindle",
	"OVERDRIVE_READ":   "OverDrive Read",
	"OVERDRIVE_LISTEN": "OverDrive Listen",
	"EPUB":             "EPUB",
	"PDF":              "PDF",
}

func digitalLabel(code string) string {
	if l, ok := digitalLabels[code]; ok {
		return l
	}
	return titleCase(code)
}

func titleCase(code string) string {
	words := strings.Fields(strings.ToLower(strings.ReplaceAll(code, "_", " ")))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

var yearRe = regexp.MustCompile(`\d{4}`)

// yearOf returns the first four-digit year in a publication date, which can
// look like "2024", "1978-1992" or "20261006".
func yearOf(date string) string {
	if y := yearRe.FindString(date); y != "" {
		return y
	}
	return date
}

func availShort(s bc.Summary) string {
	switch {
	case s.Total == 0 && s.OnOrder > 0:
		return warnStyle.Render("on order")
	case s.Available > 0:
		return okStyle.Render(fmt.Sprintf("%d/%d in", s.Available, s.Total))
	case s.Holds > 0:
		return fmt.Sprintf("0/%d · %s", s.Total, plural(s.Holds, "hold"))
	default:
		return fmt.Sprintf("0/%d in", s.Total)
	}
}

func availLong(s bc.Summary) string {
	var parts []string
	switch {
	case s.Total == 0 && s.OnOrder > 0:
		parts = append(parts, warnStyle.Render(fmt.Sprintf("%s on order", plural(s.OnOrder, "copy"))))
	case s.Available > 0:
		parts = append(parts, okStyle.Render(fmt.Sprintf("%d of %d available", s.Available, s.Total)))
	default:
		parts = append(parts, fmt.Sprintf("0 of %d available", s.Total))
	}
	if s.Holds > 0 {
		parts = append(parts, plural(s.Holds, "hold"))
	}
	if s.LibraryUseOnly {
		parts = append(parts, "library use only")
	}
	return strings.Join(parts, " · ")
}

func ratingShort(r bc.Rating) string {
	if r.Count == 0 {
		return ""
	}
	return starStyle.Render("★") + fmt.Sprintf("%.1f", r.Stars()) + dimStyle.Render(fmt.Sprintf(" (%s)", compact(r.Count)))
}

// compact shortens large counts: 1234 -> "1.2k".
func compact(n int) string {
	if n < 1000 {
		return fmt.Sprint(n)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}

// stars renders a 0-5 rating rounded to the nearest whole star.
func stars(v float64) string {
	n := max(0, min(5, int(math.Round(v))))
	return strings.Repeat("★", n) + strings.Repeat("☆", 5-n)
}

func dueLabel(t time.Time) string {
	today := startOfToday()
	if t.Before(today) {
		return "overdue (" + t.Format("Jan 2") + ")"
	}
	return t.Format("Jan 2")
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	if strings.HasSuffix(word, "y") && !strings.HasSuffix(word, "ay") {
		return fmt.Sprintf("%d %sies", n, word[:len(word)-1])
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// pad truncates or right-pads s (which may contain ANSI styling) to width w.
func pad(s string, w int) string {
	s = ansi.Truncate(s, w, "…")
	return s + strings.Repeat(" ", max(0, w-ansi.StringWidth(s)))
}

func indent(s string, n int) string {
	p := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = p + l
		}
	}
	return strings.Join(lines, "\n")
}

func firstOr(xs []string, def string) string {
	if len(xs) == 0 {
		return def
	}
	return xs[0]
}

func mapStrings(xs []string, f func(string) string) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = f(x)
	}
	return out
}
