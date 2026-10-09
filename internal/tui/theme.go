package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"shelfcheck/internal/theme"
)

// ApplyTheme sets the styles from t. Empty theme colors fall back to the
// terminal's: ANSI palette indexes, faint text, and its own background.
// Styles are shared by every Model in the process, so serve mode applies
// one theme at startup and doesn't switch.
func ApplyTheme(t theme.Theme) {
	color := func(hex, ansiIndex string) lipgloss.Style {
		if hex != "" {
			return lipgloss.NewStyle().Foreground(lipgloss.Color(hex))
		}
		return lipgloss.NewStyle().Foreground(lipgloss.Color(ansiIndex))
	}
	accentStyle = color(t.Active, "6")
	titleStyle = accentStyle.Bold(true)
	headingStyle = accentStyle.Bold(true)
	tabOnStyle = accentStyle.Bold(true).Underline(true)
	dimStyle = lipgloss.NewStyle().Faint(true)
	if t.Dim != "" {
		dimStyle = color(t.Dim, "")
	}
	okStyle = color(t.Color(2), "2")
	warnStyle = color(t.Color(3), "3")
	errStyle = color(t.Color(1), "1")
	starStyle = color(t.Color(3), "3")
	selStyle = lipgloss.NewStyle().Reverse(true)
	sigilStyle = accentStyle.Bold(true)
	if t.Sigil != "" {
		sigilStyle = color(t.Sigil, "").Bold(true)
	}

	barStyle = lipgloss.NewStyle().Faint(true)
	barWarnStyle = errStyle
	if t.Bar != "" || t.BarFG != "" {
		bar := lipgloss.NewStyle()
		if t.Bar != "" {
			bar = bar.Background(lipgloss.Color(t.Bar))
		}
		barStyle = bar
		if t.BarFG != "" {
			barStyle = bar.Foreground(lipgloss.Color(t.BarFG))
		}
		barWarnStyle = bar.Foreground(lipgloss.Color(firstOf(t.BarWarn, t.Color(9), t.Color(1), t.BarFG)))
	}
}

func firstOf(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// WithThemes offers themes to switch between with t (alt+t from the search
// box), starting at current, which the caller has applied with ApplyTheme.
// save, if set, remembers a choice. With one theme there's nothing to
// switch to (serve mode, where sessions share the styles).
func (m Model) WithThemes(themes []theme.Theme, current int, save func(name string) error) Model {
	m.themes, m.themeIdx, m.saveTheme = themes, current, save
	m.spin.Style = accentStyle
	return m
}

// theme is the current theme; the terminal's if none was set.
func (m Model) theme() theme.Theme {
	if m.themeIdx >= 0 && m.themeIdx < len(m.themes) {
		return m.themes[m.themeIdx]
	}
	return theme.Theme{Name: theme.Terminal}
}

// cycleTheme switches to the next theme and remembers it.
func (m Model) cycleTheme() (Model, tea.Cmd) {
	if len(m.themes) < 2 {
		return m, nil
	}
	m.themeIdx = (m.themeIdx + 1) % len(m.themes)
	t := m.themes[m.themeIdx]
	ApplyTheme(t)
	m.spin.Style = accentStyle
	if m.screen == detailScreen {
		m.refreshDetail()
	}
	m.flash = "theme: " + t.Label
	if m.saveTheme != nil {
		if err := m.saveTheme(t.Name); err != nil {
			m.flash += " (couldn't save: " + err.Error() + ")"
		}
	}
	return m, nil
}
