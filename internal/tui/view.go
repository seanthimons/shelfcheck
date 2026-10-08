package tui

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	bc "shelfcheck/internal/bibliocommons"
)

// ANSI palette colors, so the UI follows the terminal's own theme.
var (
	accentStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	titleStyle   = lipgloss.NewStyle().Bold(true)
	headingStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	dimStyle     = lipgloss.NewStyle().Faint(true)
	okStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	errStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	starStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	selStyle     = lipgloss.NewStyle().Reverse(true)
	tabOnStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")).Underline(true)
	barStyle     = lipgloss.NewStyle().Faint(true)
)

func (m Model) bodyHeight() int { return max(1, m.height-2) }
func (m Model) listHeight() int { return max(1, m.bodyHeight()-2) }

func (m Model) View() tea.View {
	var body string
	switch m.screen {
	case searchScreen:
		body = m.viewSearch()
	case resultsScreen:
		body = m.viewResults()
	case detailScreen:
		body = m.vp.View()
	}
	body = lipgloss.NewStyle().Height(m.bodyHeight()).MaxHeight(m.bodyHeight()).Render(body)
	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, m.viewHeader(), body, m.viewFooter()))
	v.AltScreen = true
	v.WindowTitle = "shelfcheck"
	return v
}

func (m Model) viewHeader() string {
	left := accentStyle.Bold(true).Render("shelfcheck") + dimStyle.Render(" · "+m.client.Library)
	right := dimStyle.Render("home: ") + m.homeName
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m Model) viewFooter() string {
	var keys string
	switch m.screen {
	case searchScreen:
		keys = "enter search · tab search type"
		if len(m.results) > 0 {
			keys += " · esc back to results"
		} else {
			keys += " · esc quit"
		}
	case resultsScreen:
		switch {
		case m.panel == panelCategories:
			keys = "↑↓ move · enter choose · x clear one · c clear all · esc apply"
		case m.panel == panelValues:
			keys = "↑↓ move · space/enter toggle · esc back"
		default:
			keys = "enter open · f filters · a on shelf at home · o sort · / search · q quit"
			if m.q.Narrowed() || m.q.Sort != "" {
				keys = "enter open · f filters · F clear · a at home · o sort · / search · q quit"
			}
		}
	case detailScreen:
		keys = "↑↓ scroll · r refresh · esc back · / search · q quit"
	}
	line := barStyle.Render(keys)
	switch {
	case m.err != nil:
		line = errStyle.Render("error: "+m.err.Error()) + "  " + line
	case m.loadingMore:
		line = m.spin.View() + dimStyle.Render(" loading more… ") + line
	}
	return ansi.Truncate(line, max(1, m.width), "…")
}

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
	lines := []string{
		"",
		"  " + titleStyle.Render("Search the catalog"),
		"",
		"  " + strings.Join(tabs, "   "),
		"",
		"  " + m.input.View(),
		"",
	}
	if m.searching {
		lines = append(lines, "  "+m.spin.View()+" Searching…")
	}
	return strings.Join(lines, "\n")
}

// --- results ----------------------------------------------------------------

func (m Model) viewResults() string {
	if m.searching {
		return "\n  " + m.spin.View() + " Searching…"
	}
	if m.panel != panelClosed {
		return m.viewPanel()
	}
	head := fmt.Sprintf("%s for %s", plural(m.total, "result"), titleStyle.Render(fmt.Sprintf("%q", m.q.Text)))
	if m.q.Type != "" && m.q.Type != bc.Keyword {
		head += dimStyle.Render(" (" + string(m.q.Type) + ")")
	}
	filters := m.filterSummary(m.q)
	if len(m.results) == 0 {
		msg := "\n  No results for " + titleStyle.Render(fmt.Sprintf("%q", m.q.Text))
		if filters != "" {
			return msg + " with " + accentStyle.Render(filters) + ".\n  Press f to change filters or F to clear them."
		}
		return msg + ". Press / to search again."
	}

	// Columns: marker, title, author, format, year, rating, availability, home.
	const formatW, yearW, ratingW, availW, homeW = 12, 4, 4, 15, 6
	authorW := 22
	if m.width < 100 {
		authorW = 0
	}
	fixed := 2 + formatW + yearW + ratingW + availW + homeW + 6
	if authorW > 0 {
		fixed += authorW + 1
	}
	titleW := max(12, m.width-fixed)

	rows := []string{"  " + head, "  " + accentStyle.Render(ansi.Truncate(filters, max(1, m.width-4), "…"))}
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
		if i == m.cursor {
			rows = append(rows, accentStyle.Render("▸ ")+selStyle.Render(ansi.Strip(line)))
		} else {
			rows = append(rows, "  "+m.styleRow(b, line))
		}
	}
	return strings.Join(rows, "\n")
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
	if m.availPending[b.ID] {
		return dimStyle.Render("…")
	}
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

func (m Model) renderDetail() string {
	b := m.selected
	w := max(20, min(m.width-4, 100))
	wrap := lipgloss.NewStyle().Width(w)
	var out []string
	add := func(lines ...string) { out = append(out, lines...) }

	title := b.Title
	if b.Subtitle != "" {
		title += ": " + b.Subtitle
	}
	add("", wrap.Render(titleStyle.Render(title)))
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
	add(wrap.Render(strings.Join(meta, " · ")))
	if b.Rating.Count > 0 {
		add(starStyle.Render(stars(b.Rating.Stars())) + fmt.Sprintf(" %.1f", b.Rating.Stars()) +
			dimStyle.Render(fmt.Sprintf(" (%s)", plural(b.Rating.Count, "rating"))))
	}
	if len(b.Series) > 0 {
		add(dimStyle.Render("Series: ") + strings.Join(b.Series, "; "))
	}

	add("")
	add(m.renderAvailability(w)...)

	if b.Description != "" || len(b.Genres) > 0 {
		add("", headingStyle.Render("ABOUT"))
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

	d, dErr := m.details[b.ID], m.detailsErr[b.ID]
	switch {
	case dErr != nil:
		add("", errStyle.Render("Couldn't load reviews: "+dErr.Error()))
	case d == nil:
		add("", dimStyle.Render(m.spin.View()+" loading reviews…"))
	default:
		for _, r := range d.Reviews {
			add("", headingStyle.Render("REVIEW")+dimStyle.Render(" · "+r.Source), wrap.Render(r.Text))
		}
		if d.AuthorNote != "" {
			add("", headingStyle.Render("ABOUT THE AUTHOR"), wrap.Render(d.AuthorNote))
		}
		if d.Excerpt != "" {
			add("", headingStyle.Render("EXCERPT"), wrap.Render(d.Excerpt))
		}
	}
	add("")
	return indent(strings.Join(out, "\n"), 2)
}

func (m Model) renderAvailability(w int) []string {
	b := m.selected
	a := m.avail[b.ID]
	if a == nil {
		if err := m.availErr[b.ID]; err != nil {
			return []string{headingStyle.Render("AVAILABILITY"), errStyle.Render("Couldn't load: " + err.Error())}
		}
		return []string{headingStyle.Render("AVAILABILITY"), dimStyle.Render(m.spin.View() + " checking shelves…")}
	}
	s := a.Summary
	head := headingStyle.Render("AVAILABILITY") + "  " + availLong(s)
	if m.availPending[b.ID] {
		head += " " + m.spin.View()
	}
	out := []string{head}

	if b.Digital || len(a.Copies) == 0 {
		if len(a.DigitalFormats) > 0 {
			out = append(out, dimStyle.Render("Formats: ")+strings.Join(mapStrings(a.DigitalFormats, digitalLabel), ", "))
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
		out = append(out, "", dimStyle.Render(line))
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
	return starStyle.Render("★") + fmt.Sprintf("%.1f", r.Stars())
}

// stars renders a 0-5 rating rounded to the nearest whole star.
func stars(v float64) string {
	n := max(0, min(5, int(math.Round(v))))
	return strings.Repeat("★", n) + strings.Repeat("☆", 5-n)
}

func dueLabel(t time.Time) string {
	today := time.Now().Truncate(24 * time.Hour)
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
