package hardcover

import (
	"context"
	"encoding/json"
	"fmt"
)

// Vibe is one of Hardcover's generated book lists for a reader:
// "recommendations" (picked from their reading history) or "top-picks"
// (popular books they'd like), best first.
type Vibe struct {
	ID          int
	Slug        string
	Title       string
	Description string
	Owner       string
	Generated   string // when Hardcover last picked the books
	Books       []Book
}

func (v Vibe) URL() string { return "https://hardcover.app/@" + v.Owner + "/vibes/" + v.Slug }

// Username is the token owner's Hardcover username.
func (c *Client) Username(ctx context.Context) (string, error) {
	var raw struct {
		Me []struct {
			Username string `json:"username"`
		} `json:"me"`
	}
	if err := c.Query(ctx, `{ me { username } }`, nil, &raw); err != nil {
		return "", err
	}
	if len(raw.Me) == 0 {
		return "", fmt.Errorf("hardcover: no user for this token")
	}
	return raw.Me[0].Username, nil
}

// VibeBySlug fetches a reader's vibe and up to limit of its books, in
// Hardcover's order. It makes two requests.
func (c *Client) VibeBySlug(ctx context.Context, owner, slug string, limit int) (Vibe, bool, error) {
	var raw struct {
		Vibes []struct {
			ID          int             `json:"id"`
			Slug        string          `json:"slug"`
			Title       string          `json:"title"`
			Description string          `json:"description"`
			Generated   string          `json:"books_generated_at"`
			BookIDs     json.RawMessage `json:"cached_book_ids"`
		} `json:"vibes"`
	}
	const q = `query($s: String!, $u: citext!) {
  vibes(where: {slug: {_eq: $s}, user: {username: {_eq: $u}}}, limit: 1) {
    id slug title description books_generated_at cached_book_ids
  }
}`
	if err := c.Query(ctx, q, map[string]any{"s": slug, "u": owner}, &raw); err != nil {
		return Vibe{}, false, err
	}
	if len(raw.Vibes) == 0 {
		return Vibe{}, false, nil
	}
	rv := raw.Vibes[0]
	v := Vibe{ID: rv.ID, Slug: rv.Slug, Title: rv.Title, Description: rv.Description, Owner: owner, Generated: day(rv.Generated)}
	var ids []int
	if len(rv.BookIDs) > 0 {
		if err := json.Unmarshal(rv.BookIDs, &ids); err != nil {
			return v, true, fmt.Errorf("hardcover: vibe book ids: %w", err)
		}
	}
	ids = ids[:min(limit, len(ids), 100)] // 100 rows per field
	if len(ids) == 0 {
		return v, true, nil
	}
	var books struct {
		Books []rawBook `json:"books"`
	}
	if err := c.Query(ctx, `query($ids: [Int!]) { books(where: {id: {_in: $ids}}) { `+bookFields+` } }`,
		map[string]any{"ids": ids}, &books); err != nil {
		return v, true, err
	}
	byID := map[int]Book{}
	for _, b := range books.Books {
		if !b.Compilation {
			byID[b.ID] = b.book()
		}
	}
	for _, id := range ids {
		if b, ok := byID[id]; ok {
			v.Books = append(v.Books, b)
		}
	}
	return v, true, nil
}
