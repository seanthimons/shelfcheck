package recommend

import (
	"context"
	"fmt"

	"shelfcheck/internal/account"
	"shelfcheck/internal/hardcover"
)

// maxElsewhere caps each of the not-on-the-shelf and to-buy lists.
const maxElsewhere = 12

// elsewhere files a pick that isn't available now: under not on the shelf
// if the library has it, else under to buy.
func (e *engine) elsewhere(ctx context.Context, b hardcover.Book, reason, from string) error {
	if len(e.notOnShelf) >= maxElsewhere && len(e.toBuy) >= maxElsewhere {
		return nil
	}
	k := account.WorkKey(b.Title, b.Author)
	for _, u := range append(append([]Elsewhere{}, e.notOnShelf...), e.toBuy...) {
		if account.WorkKey(u.Title, u.Author) == k {
			return nil // already listed, from the other vibe
		}
	}
	bib, ok, err := e.owned(ctx, b)
	if err != nil {
		return err
	}
	u := Elsewhere{Title: b.Title, Author: b.Author, Reason: reason, From: from, URL: b.URL(), Bib: bib}
	switch {
	case ok && len(e.notOnShelf) < maxElsewhere:
		e.notOnShelf = append(e.notOnShelf, u)
	case !ok && len(e.toBuy) < maxElsewhere:
		e.toBuy = append(e.toBuy, u)
	}
	e.opts.Debug("    not available now; the library has it: %v", ok)
	return nil
}

// vibes checks the catalog for Hardcover's own picks for the patron: its
// "recommendations" vibe (from their reading history) and "top-picks"
// (popular books they'd like), best first. Books an earlier stage already
// found just gain the reason, without another catalog search.
func (e *engine) vibes(ctx context.Context) error {
	hc := e.opts.Hardcover
	if hc == nil {
		return nil
	}
	user, err := hc.Username(ctx)
	if err != nil {
		return err
	}
	lists := []struct {
		slug   string
		share  float64 // of MaxVibes lookups
		base   float64
		reason string
	}{
		{"recommendations", 0.7, 2.5, "Hardcover recommends it from your reading (#%d)"},
		{"top-picks", 0.3, 1.5, "one of Hardcover's top picks for you (#%d)"},
	}
	for _, l := range lists {
		budget := max(1, int(float64(e.opts.MaxVibes)*l.share))
		v, ok, err := hc.VibeBySlug(ctx, user, l.slug, 100)
		if err != nil {
			return err
		}
		if !ok {
			e.opts.Debug("vibe %q: none for @%s", l.slug, user)
			continue
		}
		e.opts.Debug("vibe %q: %d books, picked %s", l.slug, len(v.Books), v.Generated)
		looked := 0
		for rank, b := range v.Books {
			if looked >= budget {
				break
			}
			if !e.candidate(b) {
				continue
			}
			// Best first: #1 gets the base plus 1.5, fading over the list.
			score := l.base + 1.5*(1-float64(rank)/float64(len(v.Books)))
			reason := fmt.Sprintf(l.reason, rank+1)
			if p := e.picks[account.WorkKey(b.Title, b.Author)]; p != nil {
				p.note("vibe", score, reason)
				if p.From == "" {
					p.From = v.URL()
				}
				continue
			}
			looked++
			found, err := e.findTitle(ctx, b.Title, b.Author, "vibe", score, reason)
			if err != nil {
				return err
			}
			if len(found) == 0 {
				if err := e.elsewhere(ctx, b, reason, v.URL()); err != nil {
					return err
				}
			}
			for _, p := range found {
				if p.From == "" {
					p.From = v.URL()
				}
			}
			e.picked += len(found)
			e.opts.Debug("  #%d %q by %s: %d available", rank+1, b.Title, b.Author, len(found))
		}
	}
	return nil
}
