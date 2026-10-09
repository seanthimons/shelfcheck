package account

import (
	"context"
	"strings"
	"time"

	bc "shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/libby"
)

// libbyFormats maps Libby formats to catalog format codes.
var libbyFormats = map[string]string{"ebook": "EBOOK", "audiobook": "EAUDIOBOOK", "magazine": "EMAG"}

// WithLibby adds Libby loans and holds missing from the catalog's lists.
// The catalog normally shows them, but its OverDrive link can drop them
// (it then reports a loan quota of -1), and Libby is the record for digital
// loans. Added items have no catalog ID. Without a Libby link the lists come
// back unchanged.
func (s *Store) WithLibby(ctx context.Context, cos []bc.Checkout, holds []bc.Hold) ([]bc.Checkout, []bc.Hold, error) {
	if !s.HasLibby() {
		return cos, holds, nil
	}
	lc, err := s.LibbyClient()
	if err != nil {
		return cos, holds, err
	}
	loans, lholds, err := lc.Account(ctx)
	if err != nil {
		return cos, holds, err
	}
	cos, holds = mergeLibby(cos, holds, loans, lholds)
	return cos, holds, nil
}

// mergeLibby appends the Libby loans and holds the catalog's lists lack.
func mergeLibby(cos []bc.Checkout, holds []bc.Hold, loans []libby.Loan, lholds []libby.Hold) ([]bc.Checkout, []bc.Hold) {
	have := map[string]bool{}
	for _, c := range cos {
		have[WorkKey(c.Bib.Title, firstAuthor(c.Bib.Authors))] = true
	}
	for _, h := range holds {
		have[WorkKey(h.Bib.Title, firstAuthor(h.Bib.Authors))] = true
	}
	for _, l := range loans {
		if have[WorkKey(l.Title, l.Author)] {
			continue
		}
		cos = append(cos, bc.Checkout{Bib: libbyBib(l.Title, l.Author, l.Format), Due: date(l.Due),
			Status: "OUT", Digital: true})
	}
	for _, h := range lholds {
		// Catalog digital holds often duplicate Libby, but omit its wait estimate.
		for i := range holds {
			existing := &holds[i]
			if existing.Digital && WorkKey(existing.Bib.Title, firstAuthor(existing.Bib.Authors)) == WorkKey(h.Title, h.Author) {
				existing.WaitDays = h.WaitDays
			}
		}
		if have[WorkKey(h.Title, h.Author)] {
			continue
		}
		status := "NOT_YET_AVAILABLE"
		switch {
		case h.Ready:
			status = "READY"
		case h.Suspended:
			status = "SUSPENDED"
		}
		holds = append(holds, bc.Hold{Bib: libbyBib(h.Title, h.Author, h.Format), Status: status, Position: h.Position,
			Placed: date(h.Placed), WaitDays: h.WaitDays, Digital: true})
	}
	return cos, holds
}

func libbyBib(title, author, format string) bc.Bib {
	b := bc.Bib{Title: title, Format: libbyFormats[strings.ToLower(format)]}
	if author != "" {
		b.Authors = []string{author}
	}
	return b
}

// date is t as a local YYYY-MM-DD, or "" if unknown.
func date(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02")
}

// libbyCurrent adds Libby loans (currently reading) and holds to the record.
func libbyCurrent(ctx context.Context, lc *libby.Client, add func(Book)) error {
	loans, holds, err := lc.Account(ctx)
	if err != nil {
		return err
	}
	for _, l := range loans {
		add(Book{Title: l.Title, Authors: splitAuthors(l.Author), Format: l.Format, Shelf: ShelfCurrent,
			Date: date(l.Borrowed), LibbyID: l.TitleID})
	}
	for _, h := range holds {
		add(Book{Title: h.Title, Authors: splitAuthors(h.Author), Format: h.Format, Shelf: ShelfHold,
			Date: date(h.Placed), LibbyID: h.TitleID})
	}
	return nil
}
