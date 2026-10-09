package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Sharp jackets for terminals that speak kitty's graphics protocol
// (Konsole, kitty, Ghostty). The image is sent once and then placed over
// the blank cells the detail page leaves for it. Placements float above the
// text, so redraws don't touch them, but they don't follow the page either:
// after anything that could move the jacket (scrolling, resizing, leaving
// the page, a box over it), it's placed again or taken away. That waits a
// moment, until the redrawn frame is out, since the renderer's scrolling
// (which moves placements too) and its full clears (which delete them)
// would otherwise undo it.
//
// Konsole doesn't do kitty's Unicode placeholders, which would avoid all
// this, and a terminal without the protocol prints its escapes as text, so
// this is only used where the environment says the terminal has it, or on
// request.

// Covers is how the detail page draws book jackets.
type Covers int

const (
	CoverBlocks Covers = iota // half blocks, in any true-color terminal
	CoverKitty                // kitty graphics
	CoverOff
)

// PickCovers reads a covers setting: "kitty", "blocks", "off", or "" or
// "auto" to go by what getenv says the terminal is.
func PickCovers(setting string, getenv func(string) string) (Covers, error) {
	switch strings.ToLower(setting) {
	case "kitty":
		return CoverKitty, nil
	case "blocks":
		return CoverBlocks, nil
	case "off":
		return CoverOff, nil
	case "", "auto":
	default:
		return CoverBlocks, fmt.Errorf("covers %q: want kitty, blocks, off or auto", setting)
	}
	// tmux and screen don't pass the escapes through by default.
	if getenv("TMUX") != "" || getenv("STY") != "" {
		return CoverBlocks, nil
	}
	if getenv("KONSOLE_VERSION") != "" || getenv("KITTY_WINDOW_ID") != "" ||
		getenv("TERM") == "xterm-kitty" || strings.EqualFold(getenv("TERM_PROGRAM"), "ghostty") {
		return CoverKitty, nil
	}
	return CoverBlocks, nil
}

// WithCovers sets how jackets are drawn; New starts with half blocks.
func (m Model) WithCovers(c Covers) Model {
	m.coverMode = c
	return m
}

// The jacket's place on the detail page: its row (after a blank line) and
// column (the page's indent). The page starts below the header's two lines.
const coverRow, coverLeft, headerRows = 1, 2, 2

// placement is where a jacket is drawn: rows vis of it, from row skip,
// with its top at screen row row. The zero value is none.
type placement struct {
	id             int
	row, skip, vis int
}

// kittyState is shared by every copy of a Model, like its caches.
type kittyState struct {
	nextID  int
	placed  placement
	pending bool // a kittyMsg is on its way
}

// kittyMsg asks for the jacket to be put where it should be by now.
// afterResize means the renderer has cleared the screen since it was
// placed, taking it away.
type kittyMsg struct{ afterResize bool }

// wantPlacement is where the selected book's jacket should be now.
func (m Model) wantPlacement() placement {
	if m.coverMode != CoverKitty || m.screen != detailScreen || m.showHelp || m.hcEdit.active {
		return placement{}
	}
	c := m.covers[m.coverURL()]
	if c == nil || c.png == "" {
		return placement{}
	}
	top := coverRow - m.vp.YOffset() // within the body
	skip := max(0, -top)
	vis := min(c.rows, m.bodyHeight()-top) - skip
	if vis <= 0 {
		return placement{}
	}
	return placement{id: c.id, row: headerRows + top + skip, skip: skip, vis: vis}
}

// syncCover schedules putting the jacket where it should be, after msg.
// One request at a time: each goes by the state when it arrives, so later
// changes ride along, and putting it off for every new message would never
// get there while a spinner ticks.
func (m Model) syncCover(msg tea.Msg) tea.Cmd {
	if m.coverMode != CoverKitty {
		return nil
	}
	k := m.kitty
	if _, ok := msg.(tea.WindowSizeMsg); ok {
		// The resize clears the screen, placements and all, in the next
		// frame: place it again once that's out.
		return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return kittyMsg{afterResize: true} })
	}
	if k.pending || m.wantPlacement() == k.placed {
		return nil
	}
	k.pending = true
	return tea.Tick(30*time.Millisecond, func(time.Time) tea.Msg { return kittyMsg{} })
}

// placeCover draws, moves or takes away the jacket.
func (m Model) placeCover(msg kittyMsg) tea.Cmd {
	k := m.kitty
	if msg.afterResize {
		k.placed = placement{}
	} else {
		k.pending = false
	}
	want := m.wantPlacement()
	if want == k.placed {
		return nil
	}
	var sb strings.Builder
	if k.placed.id != 0 && k.placed.id != want.id {
		fmt.Fprintf(&sb, "\x1b_Ga=d,d=i,i=%d,p=1,q=2\x1b\\", k.placed.id)
	}
	if want.id != 0 {
		c := m.covers[m.coverURL()]
		if !c.sent {
			writeKittyImage(&sb, want.id, c.png)
			c.sent = true
		}
		// Crop to the rows on screen and draw them at the cursor, leaving
		// the cursor where the renderer had it.
		y, h := want.skip*c.pxH/c.rows, want.vis*c.pxH/c.rows
		fmt.Fprintf(&sb, "\x1b7\x1b[%d;%dH\x1b_Ga=p,i=%d,p=1,y=%d,h=%d,c=%d,r=%d,C=1,q=2\x1b\\\x1b8",
			want.row+1, coverLeft+1, want.id, y, h, coverW, want.vis)
	}
	k.placed = want
	return tea.Raw(sb.String())
}

// writeKittyImage sends a base64 PNG to keep as image id, in the protocol's
// 4096-byte chunks.
func writeKittyImage(sb *strings.Builder, id int, b64 string) {
	const chunk = 4096
	for i := 0; i < len(b64); i += chunk {
		more := 0
		if i+chunk < len(b64) {
			more = 1
		}
		if i == 0 {
			fmt.Fprintf(sb, "\x1b_Ga=t,t=d,f=100,i=%d,q=2,m=%d;", id, more)
		} else {
			fmt.Fprintf(sb, "\x1b_Gm=%d;", more)
		}
		sb.WriteString(b64[i:min(len(b64), i+chunk)])
		sb.WriteString("\x1b\\")
	}
}
