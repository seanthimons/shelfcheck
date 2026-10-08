// Command shelfcheck browses a BiblioCommons library catalog in the terminal.
//
//	shelfcheck                  open the search screen
//	shelfcheck alien clay       search right away
//	shelfcheck serve            serve the same UI over SSH (see serve.go)
//	shelfcheck json ...         JSON output for scripts and agents (see json.go)
//	shelfcheck login            sign in with a library card for history, loans and holds (see login.go)
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"shelfcheck/internal/account"
	"shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/theme"
	"shelfcheck/internal/tui"
)

// options are the catalog settings shared by local and serve modes.
type options struct {
	library  string
	home     string
	anywhere bool
	theme    string
}

func (o *options) register(fs *flag.FlagSet) {
	o.registerCatalog(fs)
	fs.BoolVar(&o.anywhere, "anywhere", false, "start searches across all branches instead of only what's on the shelf at -home")
	fs.StringVar(&o.theme, "theme", "", `color theme: terminal (follows your terminal), umber, vellum, orpiment, cinnabar, or one from themes.toml in the config directory (default $SHELFCHECK_THEME, else the last one picked with t)`)
}

// pickTheme resolves the theme from -theme, then $SHELFCHECK_THEME, then the
// choice saved with t. A name from the flag or environment must exist; a
// saved one that's gone falls back to the terminal theme.
func pickTheme(flagValue string) ([]theme.Theme, int, error) {
	themes, err := theme.All(configDir())
	if err != nil {
		fmt.Fprintln(os.Stderr, "shelfcheck: warning: skipping your themes:", err) // the built-ins still work
	}
	for _, name := range []string{flagValue, os.Getenv("SHELFCHECK_THEME")} {
		if name == "" {
			continue
		}
		i := theme.Index(themes, name)
		if i < 0 {
			return themes, 0, fmt.Errorf("no theme %q; have %s", name, theme.Names(themes))
		}
		return themes, i, nil
	}
	return themes, max(0, theme.Index(themes, theme.Saved(configDir()))), nil
}

func (o *options) registerCatalog(fs *flag.FlagSet) {
	fs.StringVar(&o.library, "library", "cincinnatilibrary", "BiblioCommons library (subdomain of its bibliocommons.com catalog)")
	fs.StringVar(&o.home, "home", "1", "home branch code (1 = Main Library)")
}

func main() {
	if len(os.Args) > 1 {
		var run func([]string) error
		switch os.Args[1] {
		case "serve":
			run = serve
		case "json":
			run = jsonCmd
		case "login":
			run = loginCmd
		case "libby":
			run = libbyCmd
		case "export":
			run = exportCmd
		}
		if run != nil {
			if err := run(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "shelfcheck %s: %v\n", os.Args[1], err)
				os.Exit(1)
			}
			return
		}
	}

	var o options
	o.register(flag.CommandLine)
	flag.Parse()

	themes, cur, err := pickTheme(o.theme)
	if err != nil {
		fmt.Fprintln(os.Stderr, "shelfcheck:", err)
		os.Exit(1)
	}
	tui.ApplyTheme(themes[cur])

	client := bibliocommons.New(o.library)
	m := tui.New(client, o.home, !o.anywhere, strings.Join(flag.Args(), " "), account.DefaultStore(configDir()))
	m = m.WithThemes(themes, cur, func(name string) error { return theme.Save(configDir(), name) })
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "shelfcheck:", err)
		os.Exit(1)
	}
}
