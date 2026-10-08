package account

import (
	"testing"
	"time"

	bc "shelfcheck/internal/bibliocommons"
	"shelfcheck/internal/libby"
)

func TestMergeLibby(t *testing.T) {
	cos := []bc.Checkout{{Bib: bc.Bib{ID: "S1", Title: "Alien Clay", Authors: []string{"Tchaikovsky, Adrian"}}, Status: "OUT"}}
	due := time.Date(2026, 10, 12, 4, 14, 0, 0, time.UTC)
	loans := []libby.Loan{
		{Title: "Platform Decay", Author: "Wells, Martha", Format: "ebook", Due: due},
		{Title: "Alien Clay", Author: "Tchaikovsky, Adrian", Format: "ebook"}, // the catalog has it
	}
	holds := []libby.Hold{
		{Title: "The Vorrh", Author: "Catling, Brian", Format: "ebook", Suspended: true, Position: 1},
		{Title: "Hope and Help for Your Nerves", Author: "Weekes, Claire", Format: "audiobook", Ready: true},
	}
	gotCos, gotHolds := mergeLibby(cos, nil, loans, holds)
	if len(gotCos) != 2 || gotCos[1].Bib.Title != "Platform Decay" || !gotCos[1].Digital ||
		gotCos[1].Bib.Format != "EBOOK" || gotCos[1].Bib.ID != "" || gotCos[1].Due != date(due) {
		t.Errorf("checkouts = %+v", gotCos)
	}
	if len(gotHolds) != 2 || gotHolds[0].Status != "SUSPENDED" || gotHolds[0].Position != 1 ||
		gotHolds[1].Status != "READY" || gotHolds[1].Bib.Format != "EAUDIOBOOK" {
		t.Errorf("holds = %+v", gotHolds)
	}
}
