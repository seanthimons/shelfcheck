//go:build !unix

package tui

func cellPixels() (w, h int) { return 0, 0 }
