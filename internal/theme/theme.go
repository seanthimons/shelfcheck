// Package theme reads color themes in athanor's palette.toml format (one
// table per theme: bg, fg, dim, active, bar, barfg, bar_warn, sigil and a
// 16-color ansi list) and remembers which one the patron chose.
package theme

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// Terminal is the default theme: no colors of its own, so the UI follows the
// terminal's palette.
const Terminal = "terminal"

// Theme is a named palette. Empty colors fall back to the terminal's.
type Theme struct {
	Name    string   `toml:"-"`
	Label   string   `toml:"label"`
	BG      string   `toml:"bg"`
	FG      string   `toml:"fg"`
	Dim     string   `toml:"dim"`
	Border  string   `toml:"border"`
	Active  string   `toml:"active"`
	Bar     string   `toml:"bar"`
	BarFG   string   `toml:"barfg"`
	BarWarn string   `toml:"bar_warn"`
	Sigil   string   `toml:"sigil"`
	ANSI    []string `toml:"ansi"` // 16 colors: black, red, green, yellow, blue, magenta, cyan, white, then bright
}

// Color returns ANSI palette color i, or "" if the theme has none.
func (t Theme) Color(i int) string {
	if i < len(t.ANSI) {
		return t.ANSI[i]
	}
	return ""
}

//go:embed themes.toml
var builtinTOML []byte

// Builtins are the terminal theme and the bundled palettes, in order.
func Builtins() []Theme {
	ts, err := Parse(builtinTOML)
	if err != nil {
		panic("theme: bundled themes.toml: " + err.Error())
	}
	return append([]Theme{{Name: Terminal, Label: "Terminal"}}, ts...)
}

var hexRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// Parse reads palette.toml data: a table per theme, in file order.
func Parse(data []byte) ([]Theme, error) {
	var raw map[string]Theme
	md, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&raw)
	if err != nil {
		return nil, err
	}
	var out []Theme
	for _, k := range md.Keys() {
		if len(k) != 1 {
			continue // a theme's own fields
		}
		t := raw[k[0]]
		t.Name = strings.ToLower(k[0])
		if t.Label == "" {
			t.Label = k[0]
		}
		if err := t.check(); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (t Theme) check() error {
	fields := map[string]string{"bg": t.BG, "fg": t.FG, "dim": t.Dim, "border": t.Border, "active": t.Active,
		"bar": t.Bar, "barfg": t.BarFG, "bar_warn": t.BarWarn, "sigil": t.Sigil}
	for name, v := range fields {
		if v != "" && !hexRe.MatchString(v) {
			return fmt.Errorf("theme %q: %s = %q isn't a #rrggbb color", t.Name, name, v)
		}
	}
	if len(t.ANSI) != 0 && len(t.ANSI) != 16 {
		return fmt.Errorf("theme %q: ansi has %d colors, want 16", t.Name, len(t.ANSI))
	}
	for i, v := range t.ANSI {
		if !hexRe.MatchString(v) {
			return fmt.Errorf("theme %q: ansi[%d] = %q isn't a #rrggbb color", t.Name, i, v)
		}
	}
	return nil
}

// All returns the built-in themes followed by the patron's own from
// <dir>/themes.toml, if it exists; a theme there replaces a built-in of the
// same name.
func All(dir string) ([]Theme, error) {
	ts := Builtins()
	data, err := os.ReadFile(filepath.Join(dir, "themes.toml"))
	if errors.Is(err, os.ErrNotExist) {
		return ts, nil
	}
	if err != nil {
		return ts, err
	}
	mine, err := Parse(data)
	if err != nil {
		return ts, fmt.Errorf("%s: %w", filepath.Join(dir, "themes.toml"), err)
	}
	for _, t := range mine {
		if i := Index(ts, t.Name); i >= 0 {
			ts[i] = t
		} else {
			ts = append(ts, t)
		}
	}
	return ts, nil
}

// Index finds a theme by name (case-insensitive), or -1.
func Index(ts []Theme, name string) int {
	for i, t := range ts {
		if strings.EqualFold(t.Name, name) {
			return i
		}
	}
	return -1
}

// Names lists theme names, for messages.
func Names(ts []Theme) string {
	var ns []string
	for _, t := range ts {
		ns = append(ns, t.Name)
	}
	return strings.Join(ns, ", ")
}

// config is <dir>/config.toml.
type config struct {
	Theme string `toml:"theme"`
}

// Saved returns the theme name saved in <dir>/config.toml, or "".
func Saved(dir string) string {
	var c config
	if _, err := toml.DecodeFile(filepath.Join(dir, "config.toml"), &c); err != nil {
		return ""
	}
	return c.Theme
}

// Save records the chosen theme in <dir>/config.toml, keeping any other
// settings there.
func Save(dir, name string) error {
	path := filepath.Join(dir, "config.toml")
	settings := map[string]any{}
	if _, err := toml.DecodeFile(path, &settings); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s: %w", path, err)
	}
	settings["theme"] = name
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(settings); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o600)
}
