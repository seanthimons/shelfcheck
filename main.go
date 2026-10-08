// Command shelfcheck browses a BiblioCommons library catalog in the terminal.
//
//	shelfcheck                  open the search screen
//	shelfcheck alien clay       search right away
//	shelfcheck serve            serve the same UI over SSH (see serve.go)
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
	fs.StringVar(&o.library, "library", "cincinnatilibrary", "BiblioCommons library (subdomain of its bibliocommons.com catalog)")
	fs.StringVar(&o.home, "home", "1", "home branch code, listed first (1 = Main Library)")
	fs.BoolVar(&o.anywhere, "anywhere", false, "start searches across all branches instead of only what's on the shelf at -home")
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		if err := serve(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "shelfcheck serve:", err)
			os.Exit(1)
		}
		return
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
