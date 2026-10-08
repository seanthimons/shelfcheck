package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltins(t *testing.T) {
	ts := Builtins()
	if got := Names(ts); got != "terminal, umber, vellum, orpiment, cinnabar" {
		t.Fatalf("builtins = %s", got)
	}
	u := ts[Index(ts, "Umber")]
	if u.Label != "Umber" || u.BG != "#16120e" || u.Active != "#d49a3a" || len(u.ANSI) != 16 || u.Color(2) != "#8f9a5a" {
		t.Errorf("umber = %+v", u)
	}
	if ts[0].BG != "" || ts[0].Color(1) != "" {
		t.Errorf("terminal theme should have no colors: %+v", ts[0])
	}
}

func TestParseRejectsBadColors(t *testing.T) {
	for _, data := range []string{
		"[x]\nbg = \"red\"\n",
		"[x]\nansi = [\"#000000\"]\n",
	} {
		if _, err := Parse([]byte(data)); err == nil {
			t.Errorf("Parse(%q) accepted it", data)
		}
	}
}

func TestAllMergesUserThemes(t *testing.T) {
	dir := t.TempDir()
	mine := "[umber]\nlabel = \"My Umber\"\nbg = \"#000000\"\n\n[Sea]\nfg = \"#aaccee\"\n"
	if err := os.WriteFile(filepath.Join(dir, "themes.toml"), []byte(mine), 0o600); err != nil {
		t.Fatal(err)
	}
	ts, err := All(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := Names(ts); got != "terminal, umber, vellum, orpiment, cinnabar, sea" {
		t.Errorf("names = %s", got)
	}
	if u := ts[Index(ts, "umber")]; u.Label != "My Umber" || u.BG != "#000000" {
		t.Errorf("user umber didn't replace the built-in: %+v", u)
	}
	if s := ts[Index(ts, "sea")]; s.Label != "Sea" || s.FG != "#aaccee" {
		t.Errorf("sea = %+v", s)
	}
}

func TestSaveKeepsOtherSettings(t *testing.T) {
	dir := t.TempDir()
	if Saved(dir) != "" {
		t.Fatal("nothing saved yet")
	}
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("other = 3\ntheme = \"vellum\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, "cinnabar"); err != nil {
		t.Fatal(err)
	}
	if got := Saved(dir); got != "cinnabar" {
		t.Errorf("Saved = %q", got)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "other = 3") {
		t.Errorf("other setting lost:\n%s", b)
	}
}
