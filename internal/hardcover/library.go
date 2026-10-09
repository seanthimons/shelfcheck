package hardcover

import (
	"context"
	"fmt"
	"math"
	"strings"
)

// Status is a book's shelf in a Hardcover library.
type Status int

const (
	WantToRead       Status = 1
	CurrentlyReading Status = 2
	Read             Status = 3
	Paused           Status = 4
	DidNotFinish     Status = 5
	Ignored          Status = 6
)

var statusNames = map[Status]string{
	WantToRead: "want to read", CurrentlyReading: "currently reading", Read: "read",
	Paused: "paused", DidNotFinish: "did not finish", Ignored: "ignored",
}

func (s Status) String() string { return statusNames[s] }

// Shelves maps book IDs in the token owner's library to their entries.
func Shelves(lib []UserBook) map[int]UserBook {
	m := make(map[int]UserBook, len(lib))
	for _, ub := range lib {
		m[ub.ID] = ub
	}
	return m
}

// UserBook is a book in the token owner's Hardcover library.
type UserBook struct {
	Book
	UserBookID        int // the library entry, for changing or removing it
	Status            Status
	Rating            float64 // 0.5-5 stars; 0 if unrated
	Added             string  // YYYY-MM-DD
	LastRead          string  // YYYY-MM-DD finished, if recorded
	Review            string  // Markdown, when fetched for editing
	ReviewHasSpoilers bool
}

// Library returns every book in the token owner's Hardcover library.
func (c *Client) loadLibrary(ctx context.Context) ([]UserBook, error) {
	const pageSize = 100 // Hardcover's cap per field
	var out []UserBook
	for offset := 0; ; offset += pageSize {
		var raw struct {
			Me []struct {
				UserBooks []rawUserBook `json:"user_books"`
			} `json:"me"`
		}
		q := `query($n: Int!, $o: Int!) {
  me { user_books(order_by: {id: asc}, limit: $n, offset: $o) {
    id status_id rating date_added last_read_date book { ` + bookFields + ` }
  } }
}`
		if err := c.Query(ctx, q, map[string]any{"n": pageSize, "o": offset}, &raw); err != nil {
			return nil, err
		}
		if len(raw.Me) == 0 {
			return nil, fmt.Errorf("hardcover: no user for this token")
		}
		rows := raw.Me[0].UserBooks
		for _, r := range rows {
			out = append(out, r.userBook())
		}
		if len(rows) < pageSize {
			return out, nil
		}
	}
}

func day(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

// AddToShelf puts a book in the token owner's library with the given
// status (e.g. WantToRead), returning the new library entry's ID.
func (c *Client) AddToShelf(ctx context.Context, bookID int, status Status) (int, error) {
	var out struct {
		Insert struct {
			ID    int    `json:"id"`
			Error string `json:"error"`
		} `json:"insert_user_book"`
	}
	const q = `mutation($b: Int!, $s: Int!) {
  insert_user_book(object: {book_id: $b, status_id: $s}) { id error }
}`
	if err := c.Query(ctx, q, map[string]any{"b": bookID, "s": int(status)}, &out); err != nil {
		return 0, err
	}
	if out.Insert.Error != "" {
		return 0, fmt.Errorf("hardcover: %s", out.Insert.Error)
	}
	return out.Insert.ID, nil
}

// RemoveFromLibrary deletes a library entry (a UserBookID).
func (c *Client) RemoveFromLibrary(ctx context.Context, userBookID int) error {
	var out struct {
		Delete struct {
			ID *int `json:"id"`
		} `json:"delete_user_book"`
	}
	const q = `mutation($id: Int!) { delete_user_book(id: $id) { id } }`
	if err := c.Query(ctx, q, map[string]any{"id": userBookID}, &out); err != nil {
		return err
	}
	if out.Delete.ID == nil {
		return fmt.Errorf("hardcover: library entry %d wasn't removed", userBookID)
	}
	return nil
}

// rawUserBook is shared by the library reader and the record editor.
type rawUserBook struct {
	ID                int      `json:"id"`
	StatusID          int      `json:"status_id"`
	Rating            *float64 `json:"rating"`
	DateAdded         string   `json:"date_added"`
	LastReadDate      *string  `json:"last_read_date"`
	Review            string   `json:"review_markdown"`
	ReviewHasSpoilers bool     `json:"review_has_spoilers"`
	Book              rawBook  `json:"book"`
}

func (r rawUserBook) userBook() UserBook {
	ub := UserBook{Book: r.Book.book(), UserBookID: r.ID, Status: Status(r.StatusID), Added: day(r.DateAdded), Review: r.Review, ReviewHasSpoilers: r.ReviewHasSpoilers}
	if r.Rating != nil {
		ub.Rating = *r.Rating
	}
	if r.LastReadDate != nil {
		ub.LastRead = day(*r.LastReadDate)
	}
	return ub
}

const editableUserBookFields = `id status_id rating date_added last_read_date review_markdown review_has_spoilers book { ` + bookFields + ` }`

// UserBook fetches the token owner's entry for a book, including its review.
// ok is false when the book isn't in their library.
func (c *Client) UserBook(ctx context.Context, bookID int) (ub UserBook, ok bool, err error) {
	var raw struct {
		Me []struct {
			Books []rawUserBook `json:"user_books"`
		} `json:"me"`
	}
	q := `query($b: Int!) { me { user_books(where: {book_id: {_eq: $b}}, limit: 1) { ` + editableUserBookFields + ` } } }`
	if err := c.Query(ctx, q, map[string]any{"b": bookID}, &raw); err != nil {
		return ub, false, err
	}
	if len(raw.Me) == 0 {
		return ub, false, fmt.Errorf("hardcover: no user for this token")
	}
	if len(raw.Me[0].Books) == 0 {
		return ub, false, nil
	}
	return raw.Me[0].Books[0].userBook(), true, nil
}

// UserBookChanges is a patch: nil fields leave existing values untouched.
// ClearRating explicitly sends null; it can't be combined with Rating.
type UserBookChanges struct {
	Status            *Status
	Rating            *float64
	ClearRating       bool
	Review            *string
	ReviewHasSpoilers *bool
}

func (p UserBookChanges) object() (map[string]any, error) {
	obj := map[string]any{}
	if p.Status != nil {
		if _, ok := statusNames[*p.Status]; !ok {
			return nil, fmt.Errorf("hardcover: invalid reading status")
		}
		obj["status_id"] = int(*p.Status)
	}
	if p.Rating != nil {
		r := *p.Rating
		if p.ClearRating || math.IsNaN(r) || math.IsInf(r, 0) || r < 0.5 || r > 5 || r*2 != math.Trunc(r*2) {
			return nil, fmt.Errorf("hardcover: rating must be 0.5–5 in half-star steps")
		}
		obj["rating"] = r
	} else if p.ClearRating {
		obj["rating"] = nil
	}
	if p.Review != nil {
		obj["review_markdown"] = *p.Review
	}
	if p.ReviewHasSpoilers != nil {
		obj["review_has_spoilers"] = *p.ReviewHasSpoilers
	}
	if len(obj) == 0 {
		return nil, fmt.Errorf("hardcover: no changes to save")
	}
	return obj, nil
}

// UpdateUserBook changes an existing library entry, returning Hardcover's
// resulting values. A rejected/ignored field is reported even on HTTP 200;
// on such an error, a nonzero entry contains what Hardcover actually saved.
func (c *Client) UpdateUserBook(ctx context.Context, userBookID int, p UserBookChanges) (UserBook, error) {
	var ub UserBook
	if userBookID <= 0 {
		return ub, fmt.Errorf("hardcover: invalid library entry ID")
	}
	obj, err := p.object()
	if err != nil {
		return ub, err
	}
	var raw struct {
		Update struct {
			ID    *int         `json:"id"`
			Error string       `json:"error"`
			Book  *rawUserBook `json:"user_book"`
		} `json:"update_user_book"`
	}
	q := `mutation($id: Int!, $object: UserBookUpdateInput!) { update_user_book(id: $id, object: $object) { id error user_book { ` + editableUserBookFields + ` } } }`
	if err := c.Query(ctx, q, map[string]any{"id": userBookID, "object": obj}, &raw); err != nil {
		return ub, err
	}
	if raw.Update.Error != "" {
		return ub, fmt.Errorf("hardcover: %s", raw.Update.Error)
	}
	if raw.Update.ID == nil || *raw.Update.ID != userBookID || raw.Update.Book == nil || raw.Update.Book.ID != userBookID {
		return ub, fmt.Errorf("hardcover: update wasn't confirmed; reload the entry before retrying")
	}
	ub = raw.Update.Book.userBook()
	if p.Review != nil && !sameReview(ub.Review, *p.Review) {
		// The mutation's review can lag behind its derived Markdown. Check
		// the persisted record before reporting that the write was ignored.
		current, ok, err := c.UserBook(ctx, ub.ID)
		if err != nil {
			return ub, fmt.Errorf("hardcover: update accepted, but couldn't reload to verify the review: %w", err)
		}
		if !ok || current.UserBookID != userBookID {
			return ub, fmt.Errorf("hardcover: update accepted, but the library entry wasn't found when verifying the review")
		}
		ub = current
	}
	var ignored []string
	if p.Status != nil && ub.Status != *p.Status {
		ignored = append(ignored, "status")
	}
	if p.Rating != nil && ub.Rating != *p.Rating || p.ClearRating && ub.Rating != 0 {
		ignored = append(ignored, "rating")
	}
	if p.Review != nil && !sameReview(ub.Review, *p.Review) {
		ignored = append(ignored, "review")
	}
	if p.ReviewHasSpoilers != nil && ub.ReviewHasSpoilers != *p.ReviewHasSpoilers {
		ignored = append(ignored, "spoiler setting")
	}
	if len(ignored) > 0 {
		return ub, fmt.Errorf("hardcover: didn't apply %s; other changes may have saved", strings.Join(ignored, ", "))
	}
	return ub, nil
}

// Hardcover may normalize line endings and whitespace around a review.
// Keep internal formatting significant so different content still fails
// verification, including an ignored request to clear an existing review.
func sameReview(saved, requested string) bool {
	normalize := func(s string) string {
		return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	}
	return normalize(saved) == normalize(requested)
}
