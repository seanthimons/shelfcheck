package tui

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"shelfcheck/internal/account"

	bc "shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/hardcover"
	"shelfcheck/internal/theme"
)

func TestYearOf(t *testing.T) {
	for in, want := range map[string]string{"2024": "2024", "1978-1992": "1978", "20261006": "2026", "": "", "n.d.": "n.d."} {
		if got := yearOf(in); got != want {
			t.Errorf("yearOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPlural(t *testing.T) {
	for _, c := range []struct {
		n    int
		word string
		want string
	}{{1, "hold", "1 hold"}, {2, "hold", "2 holds"}, {3, "copy", "3 copies"}, {1, "copy", "1 copy"}} {
		if got := plural(c.n, c.word); got != c.want {
			t.Errorf("plural(%d, %q) = %q, want %q", c.n, c.word, got, c.want)
		}
	}
}

func TestFilterSummary(t *testing.T) {
	m := Model{home: "1", branches: map[string]string{"1": "Downtown Main Library"}}
	q := bc.Query{Text: "x", Sort: bc.SortTopRated}
	q.Toggle(bc.FacetFormat, "BK")
	q.Toggle(bc.FacetStatus, "1")
	q.Toggle(bc.FacetRating, "FOUR_PLUS_STARS")
	want := "On shelf at Downtown Main Library (home) · Book · ★ 4+ · sorted by top rated"
	if got := m.filterSummary(q); got != want {
		t.Errorf("filterSummary = %q, want %q", got, want)
	}
	if got := m.filterSummary(bc.Query{Text: "x"}); got != "" {
		t.Errorf("empty filterSummary = %q", got)
	}
}

func TestDueIn(t *testing.T) {
	today := time.Now()
	cases := map[string]string{
		today.AddDate(0, 0, 10).Format("2006-01-02"): "· 10d",
		today.AddDate(0, 0, 2).Format("2006-01-02"):  "· 2d",
		today.Format("2006-01-02"):                   "today",
		today.AddDate(0, 0, -3).Format("2006-01-02"): "overdue",
		"garbage": "garbage",
	}
	for in, want := range cases {
		if got := ansi.Strip(dueIn(in)); !strings.Contains(got, want) {
			t.Errorf("dueIn(%q) = %q, want it to contain %q", in, got, want)
		}
	}
}

func TestHoldStatus(t *testing.T) {
	for _, c := range []struct {
		h    bc.Hold
		want string
	}{
		{bc.Hold{Status: "SUSPENDED", Position: 1}, "paused · #1 in line"},
		{bc.Hold{Status: "NOT_YET_AVAILABLE", Position: 4}, "#4 in line"},
		{bc.Hold{Status: "READY_FOR_PICKUP"}, "ready for pickup"},
	} {
		if got := ansi.Strip(holdStatus(c.h)); got != c.want {
			t.Errorf("holdStatus(%+v) = %q, want %q", c.h, got, c.want)
		}
	}
}

func TestBestMatchPrefersPrint(t *testing.T) {
	w := account.WishItem{Matches: []account.WishMatch{
		{Bib: bc.Bib{ID: "e"}, Online: true},
		{Bib: bc.Bib{ID: "p"}},
	}}
	if b, ok := bestMatch(w); !ok || b.ID != "p" {
		t.Errorf("bestMatch = %q, want print copy", b.ID)
	}
	if _, ok := bestMatch(account.WishItem{}); ok {
		t.Error("bestMatch on unavailable item")
	}
}

func TestFooterSpansWidthInEveryTheme(t *testing.T) {
	defer ApplyTheme(theme.Theme{Name: theme.Terminal})
	for _, th := range theme.Builtins() {
		ApplyTheme(th)
		m := Model{width: 80, screen: resultsScreen, client: bc.New("x")}
		for _, err := range []error{nil, errors.New("something broke")} {
			m.err = err
			if w := ansi.StringWidth(m.viewFooter()); w != 80 {
				t.Errorf("%s theme, err=%v: footer is %d wide, want 80", th.Name, err, w)
			}
		}
	}
}

func TestScrollbar(t *testing.T) {
	if scrollbar(10, 10, 0) != nil {
		t.Error("scrollbar shown when everything fits")
	}
	thumb := func(bar []string) (first, n int) {
		first = -1
		for i, c := range bar {
			if ansi.Strip(c) == "┃" {
				if first < 0 {
					first = i
				}
				n++
			}
		}
		return first, n
	}
	if first, n := thumb(scrollbar(10, 100, 0)); first != 0 || n != 1 {
		t.Errorf("at the top: thumb at %d, %d tall", first, n)
	}
	if first, n := thumb(scrollbar(10, 100, 90)); first != 9 || n != 1 {
		t.Errorf("at the bottom: thumb at %d, %d tall", first, n)
	}
	if first, n := thumb(scrollbar(10, 20, 5)); first != 2 || n != 5 {
		t.Errorf("halfway: thumb at %d, %d tall", first, n)
	}
	for _, l := range withScrollbar([]string{"short"}, scrollbar(4, 8, 0), 20) {
		if w := ansi.StringWidth(l); w != 20 {
			t.Errorf("scrollbar row is %d wide, want 20", w)
		}
	}
}

func TestHalfBlocks(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 60))
	for y := range 60 {
		for x := range 40 {
			c := color.RGBA{200, 60, 40, 255}
			if y >= 30 {
				c = color.RGBA{30, 90, 160, 255}
			}
			img.Set(x, y, c)
		}
	}
	lines := halfBlocks(img, 16)
	if len(lines) != 12 {
		t.Fatalf("got %d lines, want 12 (a 2:3 jacket, 16 wide)", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 16 {
			t.Errorf("line %d is %d wide, want 16", i, w)
		}
	}
	if !strings.Contains(lines[0], "38;2;200;60;40;48;2;200;60;40m") || !strings.Contains(lines[11], "38;2;30;90;160") {
		t.Errorf("colors out of place: %q … %q", lines[0], lines[11])
	}

	gray := image.NewUniform(color.RGBA{203, 203, 203, 255})
	if l := halfBlocks(image.NewRGBA(image.Rect(0, 0, 4, 4)), 16); l != nil {
		t.Error("drew a tiny image")
	}
	if l := halfBlocks(&boundedImage{gray, image.Rect(0, 0, 200, 200)}, 16); l != nil {
		t.Error("drew the catalog's flat gray stand-in for a missing jacket")
	}
}

// boundedImage gives a uniform color a size.
type boundedImage struct {
	image.Image
	r image.Rectangle
}

func (b *boundedImage) Bounds() image.Rectangle { return b.r }

func TestPickCovers(t *testing.T) {
	env := func(kv ...string) func(string) string {
		return func(k string) string {
			for i := 0; i < len(kv); i += 2 {
				if kv[i] == k {
					return kv[i+1]
				}
			}
			return ""
		}
	}
	for _, c := range []struct {
		setting string
		env     func(string) string
		want    Covers
	}{
		{"", env("KONSOLE_VERSION", "260801"), CoverKitty},
		{"auto", env("TERM", "xterm-kitty"), CoverKitty},
		{"", env("TERM_PROGRAM", "ghostty"), CoverKitty},
		{"", env("KONSOLE_VERSION", "260801", "TMUX", "/tmp/tmux-1000/default,1,0"), CoverBlocks},
		{"", env("TERM", "xterm-256color"), CoverBlocks}, // over SSH, say
		{"kitty", env(), CoverKitty},
		{"OFF", env("KONSOLE_VERSION", "260801"), CoverOff},
	} {
		if got, err := PickCovers(c.setting, c.env); err != nil || got != c.want {
			t.Errorf("PickCovers(%q) = %v, %v; want %v", c.setting, got, err, c.want)
		}
	}
	if _, err := PickCovers("sixel", env()); err == nil {
		t.Error("PickCovers accepted sixel")
	}
}

// TestKittyPlacement follows the escapes as the detail page opens, scrolls
// the jacket partly off the top, and closes.
func TestKittyPlacement(t *testing.T) {
	m := New(bc.New("x"), "1", false, "", nil).WithCovers(CoverKitty)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = mm.(Model)
	b := bc.Bib{ID: "a", Title: "A Book", CoverURL: "u", Description: strings.Repeat("Words go here. ", 300)}
	m.covers["u"] = &cover{png: "QUJD", rows: 12, pxH: 240}
	m, _ = m.handleCover(coverMsg{url: "u", png: "QUJD", rows: 12, pxH: 240})

	// step delivers the pending placement, if any, and returns its escapes.
	step := func(msg tea.Msg) string {
		t.Helper()
		next, _ := m.Update(msg)
		m = next.(Model)
		var out string
		if m.kitty.pending {
			next, cmd := m.Update(kittyMsg{})
			m = next.(Model)
			if cmd != nil {
				out = fmt.Sprint(cmd().(tea.RawMsg).Msg)
			}
		}
		return out
	}

	next, _ := m.openBib(b, resultsScreen)
	m = next.(Model)
	out := step(spinner.TickMsg{})
	if !strings.Contains(out, "a=t,t=d,f=100,i=1,q=2,m=0;QUJD") {
		t.Errorf("first placement didn't send the image: %q", out)
	}
	if !strings.Contains(out, "\x1b7\x1b[4;3H\x1b_Ga=p,i=1,p=1,y=0,h=240,c=16,r=12,C=1,q=2\x1b\\\x1b8") {
		t.Errorf("jacket not placed at row 4, column 3: %q", out)
	}

	m.vp.SetYOffset(3) // the top two of its rows scroll off
	out = step(spinner.TickMsg{})
	if strings.Contains(out, "a=t") || !strings.Contains(out, "\x1b[3;3H\x1b_Ga=p,i=1,p=1,y=40,h=200,c=16,r=10,") {
		t.Errorf("scrolled jacket not cropped and moved: %q", out)
	}

	if out := step(tea.KeyPressMsg{Code: tea.KeyEscape}); out != "\x1b_Ga=d,d=i,i=1,p=1,q=2\x1b\\" {
		t.Errorf("leaving the page: %q, want the jacket deleted", out)
	}
}

func TestCoverFallsBackToHardcover(t *testing.T) {
	m := New(bc.New("x"), "1", false, "", nil)
	m.selected = bc.Bib{ID: "a", CoverURL: "catalog"}
	m.hcInfo["a"] = &hcInfo{ok: true, info: hardcover.BookInfo{CoverURL: "hardcover"}}
	m.covers["catalog"] = &cover{loading: true}
	if got := m.coverURL(); got != "catalog" {
		t.Errorf("while the catalog's loads: %q, want it", got)
	}
	m.covers["catalog"] = &cover{lines: []string{"▀"}}
	if got := m.coverURL(); got != "catalog" {
		t.Errorf("with the catalog's: %q, want it", got)
	}
	m.covers["catalog"] = &cover{} // its 1×1 stand-in
	if got := m.coverURL(); got != "hardcover" {
		t.Errorf("without the catalog's: %q, want Hardcover's", got)
	}
	m.selected = bc.Bib{Title: "Not here"}
	m.hcInfo[m.detailKey()] = &hcInfo{ok: true, info: hardcover.BookInfo{CoverURL: "hc2"}}
	if got := m.coverURL(); got != "hc2" {
		t.Errorf("with no catalog record: %q, want Hardcover's", got)
	}
}

// TestDueInTheEvening pins the clock to 9 pm Eastern, when it's already
// tomorrow in UTC: due dates once counted from UTC midnight and read a day
// short then.
func TestDueInTheEvening(t *testing.T) {
	defer func(l *time.Location, f func() time.Time) { time.Local, timeNow = l, f }(time.Local, timeNow)
	time.Local = time.FixedZone("EDT", -4*60*60)
	timeNow = func() time.Time { return time.Date(2026, 10, 8, 21, 0, 0, 0, time.Local) }
	for date, want := range map[string]string{
		"2026-10-18": "due Oct 18 · 10d",
		"2026-10-10": "due Oct 10 · 2d",
		"2026-10-08": "due Oct 8 · today",
		"2026-10-07": "overdue · Oct 7",
	} {
		if got := ansi.Strip(dueIn(date)); got != want {
			t.Errorf("dueIn(%q) at 9 pm = %q, want %q", date, got, want)
		}
	}
	if got := dueLabel(time.Date(2026, 10, 8, 12, 0, 0, 0, time.Local)); got != "Oct 8" {
		t.Errorf("dueLabel(today) at 9 pm = %q, want Oct 8 (not overdue)", got)
	}
}
