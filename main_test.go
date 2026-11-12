package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shelfcheck/internal/theme"
)

func TestCatalogConfig(t *testing.T) {
	for _, tt := range []struct {
		name, config, library, home string
		args                        []string
		invalid                     bool
	}{
		{name: "missing", library: "cincinnatilibrary", home: "1"},
		{name: "unrelated settings", config: "theme = 'vellum'\ncovers = 'off'\n", library: "cincinnatilibrary", home: "1"},
		{name: "saved", config: "library = 'testlibrary'\nhome = '007'\n", library: "testlibrary", home: "007"},
		{name: "partial", config: "home = '2'\n", library: "cincinnatilibrary", home: "2"},
		{name: "flags override", config: "library = 'testlibrary'\nhome = '007'\n", args: []string{"-library", "other", "-home", "3"}, library: "other", home: "3"},
		{name: "malformed", config: "library = [", invalid: true},
		{name: "wrong type", config: "home = 7\n", invalid: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			if tt.config != "" {
				if err := os.MkdirAll(configDir(), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(configDir(), "config.toml"), []byte(tt.config), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var o options
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			err := o.registerCatalog(fs)
			if tt.invalid {
				if err == nil || !strings.Contains(err.Error(), "config.toml") {
					t.Fatalf("expected config error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := fs.Parse(tt.args); err != nil {
				t.Fatal(err)
			}
			if o.library != tt.library || o.home != tt.home {
				t.Fatalf("got %q/%q, want %q/%q", o.library, o.home, tt.library, tt.home)
			}
			if tt.config != "" {
				if err := theme.Save(configDir(), "umber"); err != nil {
					t.Fatal(err)
				}
				var reloaded options
				if err := reloaded.registerCatalog(flag.NewFlagSet("reload", flag.ContinueOnError)); err != nil {
					t.Fatal(err)
				}
				if reloaded.library != fs.Lookup("library").DefValue || reloaded.home != fs.Lookup("home").DefValue {
					t.Fatal("saving theme changed catalog settings")
				}
			}
		})
	}
}
