//go:build unix

package tui

import (
	"os"

	"golang.org/x/sys/unix"
)

// cellPixels is the size of a terminal cell in pixels, or zeros if the
// terminal doesn't say (or this isn't one, as over SSH).
func cellPixels() (w, h int) {
	for _, f := range []*os.File{os.Stdout, os.Stdin} {
		ws, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
		if err == nil && ws.Col > 0 && ws.Row > 0 && ws.Xpixel > 0 && ws.Ypixel > 0 {
			return int(ws.Xpixel) / int(ws.Col), int(ws.Ypixel) / int(ws.Row)
		}
	}
	return 0, 0
}
