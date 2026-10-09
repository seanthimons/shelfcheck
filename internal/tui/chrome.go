package tui

import (
	"image/color"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The frame around every screen: the header with its tabs and rule, the
// footer's key help, scrollbars, the cursor row's tint and boxes drawn
// over the body (filters, help).

// --- header -----------------------------------------------------------------

type navTab struct {
	s     screen
	label string
}

// navTabs are the screens to show in the header: ones that are always
// there, plus results and a Hardcover list once there's something in them.
func (m Model) navTabs() []navTab {
	tabs := []navTab{{searchScreen, "Search"}}
	if len(m.results) > 0 {
		tabs = append(tabs, navTab{resultsScreen, "Results"})
	}
	if m.hc.col != nil {
		tabs = append(tabs, navTab{hardcoverScreen, "Hardcover"})
	}
	if m.acct != nil {
		tabs = append(tabs, navTab{forYouScreen, "For you"}, navTab{wishlistScreen, "Wishlist"}, navTab{accountScreen, "Account"})
	}
	return tabs
}

// viewHeader is the app name and screen tabs, with where we are on the
// right, over a rule that's heavier under the current tab.
func (m Model) viewHeader() string {
	current := m.screen
	if current == detailScreen {
		current = m.backScreen
	}
	left := sigilStyle.Render("shelfcheck") + dimStyle.Render(" "+buildStamp) + "   "
	onX, onW := -1, 0
	for i, t := range m.navTabs() {
		if i > 0 {
			left += "  "
		}
		if t.s == current {
			onX, onW = lipgloss.Width(left), lipgloss.Width(t.label)
			left += headingStyle.Render(t.label)
		} else {
			left += dimStyle.Render(t.label)
		}
	}

	// Drop the library, then the home branch, when they don't fit.
	home := dimStyle.Render("home: ") + m.homeName
	var right string
	for _, r := range []string{dimStyle.Render(m.client.Library+"   ") + home, home, ""} {
		if lipgloss.Width(left)+lipgloss.Width(r)+2 <= m.width {
			right = r
			break
		}
	}
	top := spread(ansi.Truncate(left, m.width, "…"), right, m.width)

	w := max(1, m.width)
	rule := ruleStyle.Render(strings.Repeat("─", w))
	if onX >= 0 && onX < w {
		onW = min(onW, w-onX)
		rule = ruleStyle.Render(strings.Repeat("─", onX)) +
			accentStyle.Render(strings.Repeat("━", onW)) +
			ruleStyle.Render(strings.Repeat("─", w-onX-onW))
	}
	return top + "\n" + rule
}

// spread puts left and right at either end of a line w wide; right is
// dropped if there's no room.
func spread(left, right string, w int) string {
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if right == "" || gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

// sectionHead is a heading (and any note after it) followed by a rule to
// width w.
func sectionHead(title, note string, w int) string {
	s := headingStyle.Render(title) + note
	if n := w - lipgloss.Width(s) - 1; n > 0 {
		s += " " + ruleStyle.Render(strings.Repeat("─", n))
	}
	return s
}

// colHead renders a column label padded to w, underlined but not its
// padding.
func colHead(label string, w int) string {
	label = ansi.Truncate(label, w, "")
	if strings.TrimSpace(label) == "" {
		return strings.Repeat(" ", w)
	}
	return dimStyle.Underline(true).Render(label) + strings.Repeat(" ", max(0, w-ansi.StringWidth(label)))
}

// --- rows and scrolling -----------------------------------------------------

// paintBG gives every cell of line, padded to w, the background bg,
// keeping its own colors (a plain Background style would stop at the
// first styled span).
func paintBG(line string, w int, bg color.Color) string {
	c := lipgloss.NewCanvas(w, 1)
	c.Compose(lipgloss.NewLayer(line))
	for x := range w {
		if cell := c.CellAt(x, 0); cell != nil {
			cell.Style.Bg = bg
		}
	}
	return c.Render()
}

// cursorRow marks the selected row: an arrow, and on themes with colors of
// their own, a tinted background.
func cursorRow(selected bool, line string) string {
	if !selected {
		return "  " + line
	}
	line = accentStyle.Render("▸ ") + line
	if selBG != nil {
		return paintBG(line, lipgloss.Width(line), selBG)
	}
	return line
}

// scrollbar is a column h rows tall showing which of total rows, from
// start, are on screen; nil when they all fit.
func scrollbar(h, total, start int) []string {
	if h < 2 || total <= h {
		return nil
	}
	thumb := max(1, h*h/total)
	pos := (h - thumb) * max(0, min(start, total-h)) / (total - h)
	out := make([]string, h)
	for i := range out {
		if i >= pos && i < pos+thumb {
			out[i] = accentStyle.Render("┃")
		} else {
			out[i] = ruleStyle.Render("│")
		}
	}
	return out
}

// withScrollbar puts bar down the right edge of lines, which is w wide.
func withScrollbar(lines, bar []string, w int) []string {
	if bar == nil {
		return lines
	}
	out := make([]string, len(bar))
	for i := range bar {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		out[i] = pad(l, w-2) + " " + bar[i]
	}
	return out
}

// --- footer -----------------------------------------------------------------

// kb is a key binding for the footer and help box.
func kb(keys, desc string) key.Binding {
	return key.NewBinding(key.WithKeys(keys), key.WithHelp(keys, desc))
}

// screenKeys are the current screen's keys, most useful first: the footer
// shows as many as fit, and ? shows them all.
func (m Model) screenKeys() []key.Binding {
	if m.hcEdit.active {
		return []key.Binding{kb("ctrl+s", "save"), kb("tab", "next field"), kb("shift+tab", "previous field"), kb("esc", "cancel")}
	}
	nav := []key.Binding{kb("p", "for you"), kb("w", "wishlist"), kb("m", "account"), kb("/", "search")}
	switch m.screen {
	case searchScreen:
		esc := kb("esc", "quit")
		if len(m.results) > 0 {
			esc = kb("esc", "back to results")
		}
		return []key.Binding{kb("enter", "search"), kb("tab", "type"), esc,
			kb("alt+p", "for you"), kb("alt+w", "wishlist"), kb("alt+m", "account"), kb("alt+t", "theme")}
	case resultsScreen:
		switch m.panel {
		case panelCategories:
			return []key.Binding{kb("↑↓", "move"), kb("enter", "choose"), kb("x", "clear one"), kb("c", "clear all"), kb("esc", "apply")}
		case panelValues:
			return []key.Binding{kb("↑↓", "move"), kb("space", "toggle"), kb("esc", "back")}
		}
		ks := []key.Binding{kb("enter", "open"), kb("f", "filters")}
		if m.q.Narrowed() || m.q.Sort != "" {
			ks = append(ks, kb("F", "clear filters"))
		}
		return append(append(ks, kb("a", "home only"), kb("o", "sort")), nav...)
	case detailScreen:
		ks := []key.Binding{kb("↑↓", "scroll"), kb("esc", "back")}
		if h := m.hcInfo[m.detailKey()]; h != nil && h.ok {
			ks = append(ks, kb("e", "edit Hardcover"))
		}
		if m.selected.ID != "" || m.hcInfo[m.detailKey()] != nil {
			ks = append(ks, kb("r", "refresh"))
		}
		return append(append(ks, kb("c", "search catalog")), nav...)
	case accountScreen:
		if m.accountUI.filtering {
			return []key.Binding{kb("enter", "apply filter"), kb("esc", "cancel")}
		}
		ks := []key.Binding{kb("↑↓", "move"), kb("tab", "switch pane"), kb("enter", "open"), kb("r", "refresh")}
		if m.accountUI.section >= accountAllBooks {
			ks = append(ks, kb("e", "edit"), kb("f", "filter"), kb("o", "sort"))
		}
		return append(ks, kb("pgup/pgdown", "page"), kb("h", "sections"), kb("/", "search"), kb("esc", "search"))
	case forYouScreen:
		return []key.Binding{kb("↑↓", "move"), kb("enter", "open"), kb("h", "Hardcover source"), kb("r", "refresh"),
			kb("w", "wishlist"), kb("m", "account"), kb("/", "search")}
	case hardcoverScreen:
		return append([]key.Binding{kb("↑↓", "move"), kb("enter", "open"), kb("+", "want to read"), kb("A", "show all"),
			kb("r", "recheck"), kb("esc", "back")}, nav...)
	case wishlistScreen:
		return []key.Binding{kb("↑↓", "move"), kb("enter", "open"), kb("A", "show all"), kb("r", "recheck"),
			kb("p", "for you"), kb("m", "account"), kb("/", "search")}
	}
	return nil
}

// globalKeys work on every screen but the search box.
var globalKeys = []key.Binding{kb("t", "theme"), kb("?", "all keys"), kb("q", "quit")}

func (m Model) viewFooter() string {
	// A message (error, flash, progress) leads, in its own style.
	lead, leadStyle := "", barStyle
	switch {
	case m.err != nil:
		lead, leadStyle = "error: "+m.err.Error()+"  ", barWarnStyle
	case m.flash != "":
		lead = m.flash + "  "
	case m.loadingMore:
		lead = ansi.Strip(m.spin.View()) + " loading more…  "
	}
	w := max(1, m.width)
	lead = ansi.Truncate(lead, w, "…")
	rest := max(0, w-ansi.StringWidth(lead))

	h := help.New()
	h.ShortSeparator = " · "
	h.Styles.ShortKey, h.Styles.ShortDesc = footKeyStyle, footDescStyle
	h.Styles.ShortSeparator, h.Styles.Ellipsis = footDescStyle, footDescStyle
	more := ""
	if m.screen != searchScreen && !m.hcEdit.active {
		more = h.ShortHelpView(globalKeys[1:2]) // ? all keys
	}
	h.SetWidth(max(0, rest-lipgloss.Width(more)-3))
	keys := h.ShortHelpView(m.screenKeys())
	if more != "" {
		sep := " · "
		if strings.HasSuffix(ansi.Strip(keys), "…") {
			sep = " "
		}
		keys += footDescStyle.Render(sep) + more
	}
	keys = ansi.Truncate(" "+keys, rest, "…")

	// The bar is painted cell by cell, so a theme's bar color runs edge to
	// edge under the keys' own colors.
	var bar string
	if barBG != nil {
		bar = paintBG(keys, rest, barBG)
	} else {
		bar = pad(keys, rest)
	}
	if lead == "" {
		return bar
	}
	return leadStyle.Render(lead) + bar
}

// --- boxes over the body ----------------------------------------------------

// box frames content with a rounded border, in c if set.
func box(content string, c color.Color) string {
	s := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	if c != nil {
		s = s.BorderForeground(c)
	}
	return s.Render(content)
}

// overlay centers fg over base, which is faded so the box stands out.
func (m Model) overlay(base, fg string) string {
	h := m.bodyHeight()
	lines := strings.Split(base, "\n")
	for i, l := range lines {
		lines[i] = dimStyle.Render(ansi.Strip(l))
	}
	base = lipgloss.NewStyle().Width(m.width).Height(h).MaxHeight(h).Render(strings.Join(lines, "\n"))
	x := max(0, (m.width-lipgloss.Width(fg))/2)
	y := max(0, (h-lipgloss.Height(fg))/3)
	return lipgloss.NewCompositor(lipgloss.NewLayer(base), lipgloss.NewLayer(fg).X(x).Y(y).Z(1)).Render()
}

// viewHelp is the box of every key on the current screen (?).
func (m Model) viewHelp() string {
	h := help.New()
	h.Styles.FullKey, h.Styles.FullDesc, h.Styles.FullSeparator = accentStyle.Bold(true), lipgloss.NewStyle(), lipgloss.NewStyle()
	h.FullSeparator = "    "
	var cols [][]key.Binding
	ks := m.screenKeys()
	for len(ks) > 0 {
		n := min(6, len(ks))
		cols = append(cols, ks[:n])
		ks = ks[n:]
	}
	cols = append(cols, globalKeys)
	body := titleStyle.Render("Keys") + "\n\n" + h.FullHelpView(cols) + "\n\n" + dimStyle.Render("any key closes")
	return box(body, accentColor)
}

// notice centers a short message (an empty list, an error, a loading
// spinner) in the body. Leading indents are dropped.
func (m Model) notice(s string) string {
	lines := strings.Split(strings.Trim(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimLeft(l, " ")
	}
	return lipgloss.Place(m.width, m.bodyHeight(), lipgloss.Center, lipgloss.Center, strings.Join(lines, "\n"))
}
