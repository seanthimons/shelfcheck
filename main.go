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

	"shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/tui"
)

// options are the catalog settings shared by local and serve modes.
type options struct {
	library  string
	home     string
	anywhere bool
}

func (o *options) register(fs *flag.FlagSet) {
	o.registerCatalog(fs)
	fs.BoolVar(&o.anywhere, "anywhere", false, "start searches across all branches instead of only what's on the shelf at -home")
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

	client := bibliocommons.New(o.library)
	m := tui.New(client, o.home, !o.anywhere, strings.Join(flag.Args(), " "))
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "shelfcheck:", err)
		os.Exit(1)
	}
}
