// Command shelfcheck browses a BiblioCommons library catalog in the terminal.
//
//	shelfcheck                  open the search screen
//	shelfcheck alien clay       search right away
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

func main() {
	library := flag.String("library", "cincinnatilibrary", "BiblioCommons library (subdomain of its bibliocommons.com catalog)")
	home := flag.String("home", "1", "home branch code, listed first (1 = Main Library)")
	anywhere := flag.Bool("anywhere", false, "start searches across all branches instead of only what's on the shelf at -home")
	flag.Parse()

	client := bibliocommons.New(*library)
	m := tui.New(client, *home, !*anywhere, strings.Join(flag.Args(), " "))
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "shelfcheck:", err)
		os.Exit(1)
	}
}
