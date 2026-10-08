package recommend

import (
	"context"
	"fmt"

	"shelfcheck/internal/account"
)

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
