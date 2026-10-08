package tui

import (
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Book jackets on the detail page, drawn with half blocks: each cell is two
// pixels, the top one the ▀'s foreground and the bottom one its background.
// That's plain text to the terminal, so it scrolls and sits under boxes like
// anything else, in any terminal with true color (others get the nearest
// 256 colors).

const (
	coverW    = 16 // cells
	coverMaxH = 14 // rows; taller jackets are squeezed
)

// cover is a jacket drawn for the detail page, by its URL. loading until the
// image arrives; then lines is nil if there's no usable picture.
type cover struct {
	loading bool
	lines   []string
}

type coverMsg struct {
	url   string
	lines []string
}

// fetchCover loads the jacket at url, if it hasn't been.
func (m Model) fetchCover(url string) tea.Cmd {
	if url == "" || m.covers[url] != nil {
		return nil
	}
	m.covers[url] = &cover{loading: true}
	hc := m.client.HTTP
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return coverMsg{url: url}
		}
		resp, err := hc.Do(req)
		if err != nil {
			return coverMsg{url: url}
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return coverMsg{url: url}
		}
		img, _, err := image.Decode(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			return coverMsg{url: url}
		}
		return coverMsg{url: url, lines: halfBlocks(img, coverW)}
	}
}

func (m Model) handleCover(msg coverMsg) (Model, tea.Cmd) {
	m.covers[msg.url] = &cover{lines: msg.lines}
	if m.screen == detailScreen && m.selected.CoverURL == msg.url {
		m.refreshDetail()
	}
	return m, nil
}

// coverLines is the selected book's jacket, blank while it loads; nil if it
// has none.
func (m Model) coverLines() []string {
	c := m.covers[m.selected.CoverURL]
	switch {
	case c == nil:
		return nil
	case c.loading:
		blank := make([]string, coverW*3/4)
		for i := range blank {
			blank[i] = strings.Repeat(" ", coverW)
		}
		return blank
	}
	return c.lines
}

// halfBlocks draws img w cells wide. It returns nil for images too small or
// too plain to be a jacket: the catalog's stand-in for a missing one is a
// flat gray square.
func halfBlocks(img image.Image, w int) []string {
	b := img.Bounds()
	if b.Dx() < 8 || b.Dy() < 8 {
		return nil
	}
	h := min(coverMaxH*2, max(2, int(math.Round(float64(w*b.Dy())/float64(b.Dx())))))
	h += h % 2
	px := shrink(img, w, h)
	if flat(px) {
		return nil
	}
	lines := make([]string, h/2)
	for y := 0; y < h; y += 2 {
		var sb strings.Builder
		for x := range w {
			t, u := px[y*w+x], px[(y+1)*w+x]
			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm▀", t[0], t[1], t[2], u[0], u[1], u[2])
		}
		sb.WriteString("\x1b[m")
		lines[y/2] = sb.String()
	}
	return lines
}

// shrink averages img down to w×h pixels, row by row.
func shrink(img image.Image, w, h int) [][3]uint8 {
	b := img.Bounds()
	out := make([][3]uint8, w*h)
	for y := range h {
		y0, y1 := b.Min.Y+y*b.Dy()/h, b.Min.Y+(y+1)*b.Dy()/h
		for x := range w {
			x0, x1 := b.Min.X+x*b.Dx()/w, b.Min.X+(x+1)*b.Dx()/w
			var r, g, bl, n uint64
			for sy := y0; sy < max(y1, y0+1); sy++ {
				for sx := x0; sx < max(x1, x0+1); sx++ {
					cr, cg, cb, _ := img.At(sx, sy).RGBA()
					r, g, bl, n = r+uint64(cr>>8), g+uint64(cg>>8), bl+uint64(cb>>8), n+1
				}
			}
			out[y*w+x] = [3]uint8{uint8(r / n), uint8(g / n), uint8(bl / n)}
		}
	}
	return out
}

// flat reports whether every channel of px barely varies.
func flat(px [][3]uint8) bool {
	for c := range 3 {
		var sum, sq float64
		for _, p := range px {
			v := float64(p[c])
			sum, sq = sum+v, sq+v*v
		}
		n := float64(len(px))
		if mean := sum / n; math.Sqrt(max(0, sq/n-mean*mean)) >= 8 {
			return false
		}
	}
	return true
}
