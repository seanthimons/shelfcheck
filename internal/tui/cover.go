package tui

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
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

// cover is a jacket drawn for the detail page, by its URL. It's loading
// until the image arrives; then it has half-block lines, or for kitty
// graphics a base64 PNG to fill rows rows (pxH pixels tall), or neither if
// there's no usable picture.
type cover struct {
	loading bool
	lines   []string

	png       string
	rows, pxH int
	id        int  // the terminal's ID for the image
	sent      bool // the terminal has it
}

type coverMsg struct {
	url   string
	lines []string
	png   string
	rows  int
	pxH   int
}

// fetchCover loads the jacket at url, if it hasn't been.
func (m Model) fetchCover(url string) tea.Cmd {
	if url == "" || m.covers[url] != nil || m.coverMode == CoverOff {
		return nil
	}
	m.covers[url] = &cover{loading: true}
	hc, kitty := m.client.HTTP, m.coverMode == CoverKitty
	return func() tea.Msg {
		if kitty {
			// The catalog's large jacket (266×400 to the medium's 133×200),
			// to shrink rather than stretch.
			big := strings.Replace(url, "/MC.GIF", "/LC.GIF", 1)
			img := getImage(hc, big)
			if img == nil && big != url {
				img = getImage(hc, url)
			}
			return kittyCover(url, img)
		}
		img := getImage(hc, url)
		if img == nil {
			return coverMsg{url: url}
		}
		return coverMsg{url: url, lines: halfBlocks(img, coverW)}
	}
}

// getImage downloads and decodes an image, or returns nil.
func getImage(hc *http.Client, url string) image.Image {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	img, _, err := image.Decode(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil
	}
	return img
}

// kittyCover sizes img to the cells it will cover, when the terminal says
// how big they are, so it isn't scaled again there.
func kittyCover(url string, img image.Image) coverMsg {
	if img == nil || halfBlocks(img, coverW) == nil {
		return coverMsg{url: url} // too small, or the flat gray stand-in
	}
	b := img.Bounds()
	cw, ch := cellPixels()
	if cw == 0 || ch == 0 {
		cw, ch = 1, 2 // a guess at the shape, for the rows
	}
	rows := min(coverMaxH, max(2, int(math.Round(float64(coverW*cw*b.Dy())/float64(b.Dx()*ch)))))
	if cw > 1 {
		img = toRGBA(shrink(img, coverW*cw, rows*ch), coverW*cw, rows*ch)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return coverMsg{url: url}
	}
	return coverMsg{url: url, png: base64.StdEncoding.EncodeToString(buf.Bytes()), rows: rows, pxH: img.Bounds().Dy()}
}

func toRGBA(px [][3]uint8, w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i, p := range px {
		o := i * 4
		img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = p[0], p[1], p[2], 255
	}
	return img
}

func (m Model) handleCover(msg coverMsg) (Model, tea.Cmd) {
	c := &cover{lines: msg.lines, png: msg.png, rows: msg.rows, pxH: msg.pxH}
	if c.png != "" {
		m.kitty.nextID++
		c.id = m.kitty.nextID
	}
	m.covers[msg.url] = c
	if m.screen != detailScreen {
		return m, nil
	}
	m.refreshDetail()
	return m, m.fetchCover(m.coverURL()) // Hardcover's, if that was the catalog's stand-in
}

// coverURL is the jacket to show for the selected book: the catalog's, or
// Hardcover's when the catalog has none (for some records it sends a 1×1
// stand-in) or there's no catalog record.
func (m Model) coverURL() string {
	if u := m.selected.CoverURL; u != "" {
		if c := m.covers[u]; c == nil || c.loading || c.lines != nil || c.png != "" {
			return u
		}
	}
	if h := m.hcInfo[m.detailKey()]; h != nil && h.ok {
		return h.info.CoverURL
	}
	return ""
}

// coverLines is the selected book's jacket, or blank cells for one coming
// or drawn by the terminal over them; nil if it has none.
func (m Model) coverLines() []string {
	if m.coverMode == CoverOff {
		return nil
	}
	c := m.covers[m.coverURL()]
	rows := 0
	switch {
	case c == nil:
		if h := m.hcInfo[m.detailKey()]; h == nil || !h.loading {
			return nil
		}
		rows = coverW * 3 / 4 // Hardcover may have one
	case c.loading:
		rows = coverW * 3 / 4
	case c.png != "":
		rows = c.rows
	default:
		return c.lines
	}
	blank := make([]string, rows)
	for i := range blank {
		blank[i] = strings.Repeat(" ", coverW)
	}
	return blank
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
