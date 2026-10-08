package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"shelfcheck/internal/account"
	bc "shelfcheck/internal/bibliocommons"
)

const exportUsage = `usage: shelfcheck export hardcover [-o file.csv]

Writes your reading record as a CSV that Hardcover's importer accepts (its
Title/Author/Status columns plus Goodreads' columns, so Goodreads-style
importers work too): library history and Libby borrows as "read",
current loans as "currently-reading", and holds plus your Libby "want to
read" tag as "to-read". Sampled-only titles are left out.
`

func exportCmd(args []string) error {
	if len(args) == 0 || args[0] != "hardcover" {
		fmt.Fprint(os.Stderr, exportUsage)
		if len(args) == 0 || args[0] == "-h" || args[0] == "help" {
			return nil
		}
		return fmt.Errorf("unknown export %q", args[0])
	}
	fs := flag.NewFlagSet("export hardcover", flag.ContinueOnError)
	var o options
	o.registerCatalog(fs)
	outPath := fs.String("o", "", "write to this file instead of stdout")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	st := account.DefaultStore(configDir())
	rec, err := st.LoadRecord(ctx, bc.New(o.library), o.library)
	if err != nil {
		return err
	}
	for _, n := range rec.Notes {
		fmt.Fprintln(os.Stderr, "shelfcheck export: warning:", n)
	}

	out := os.Stdout
	if *outPath != "" {
		f, err := os.Create(*outPath)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}
	w := csv.NewWriter(out)
	// Hardcover's own importer requires Title, Author and Status; the rest
	// are Goodreads' export columns, which Goodreads-style importers use.
	w.Write([]string{"Book Id", "Title", "Author", "Status", "Author l-f", "Additional Authors", "ISBN", "ISBN13",
		"My Rating", "Average Rating", "Publisher", "Binding", "Number of Pages", "Year Published",
		"Original Publication Year", "Date Read", "Date Added", "Bookshelves", "Bookshelves with positions",
		"Exclusive Shelf", "My Review", "Spoiler", "Private Notes", "Read Count", "Owned Copies"})
	counts := map[account.Shelf]int{}
	for _, b := range rec.Books {
		shelf := b.Shelf
		switch shelf {
		case account.ShelfSampled:
			continue
		case account.ShelfHold:
			shelf = account.ShelfWant
		}
		counts[shelf]++
		var lf, fl, more string
		if len(b.Authors) > 0 {
			lf, fl = b.Authors[0], firstLast(b.Authors[0])
			var rest []string
			for _, a := range b.Authors[1:] {
				rest = append(rest, firstLast(a))
			}
			more = strings.Join(rest, ", ")
		}
		isbn10, isbn13 := "", ""
		switch len(b.ISBN) {
		case 10:
			isbn10 = b.ISBN
		case 13:
			isbn13 = b.ISBN
		}
		binding := map[string]string{"book": "Hardcover", "ebook": "Kindle Edition", "audiobook": "Audiobook"}[b.Format]
		readCount, dateRead := "0", ""
		if shelf == account.ShelfRead {
			readCount = "1"
		}
		w.Write([]string{"", b.Title, fl, hardcoverStatus[shelf], lf, more, isbn10, isbn13,
			"0", "", b.Publisher, binding, "", b.Year,
			"", dateRead, b.Date, "", "",
			string(shelf), "", "", "", readCount, "0"})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "exported %d read, %d currently reading, %d to read\n",
		counts[account.ShelfRead], counts[account.ShelfCurrent], counts[account.ShelfWant])
	return nil
}

// hardcoverStatus maps shelves to Hardcover's status names.
var hardcoverStatus = map[account.Shelf]string{
	account.ShelfRead:    "Read",
	account.ShelfCurrent: "Currently Reading",
	account.ShelfWant:    "Want to Read",
}

// firstLast turns "Wells, Martha" into "Martha Wells".
func firstLast(a string) string {
	last, first, ok := strings.Cut(a, ",")
	if !ok {
		return strings.TrimSpace(a)
	}
	return strings.TrimSpace(strings.TrimSpace(first) + " " + strings.TrimSpace(last))
}
