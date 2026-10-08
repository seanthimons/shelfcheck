package hardcover

import (
	"context"
	"fmt"
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
	UserBookID int // the library entry, for changing or removing it
	Status     Status
	Rating     float64 // 0.5-5 stars; 0 if unrated
	Added      string  // YYYY-MM-DD
	LastRead   string  // YYYY-MM-DD finished, if recorded
}

// Library returns every book in the token owner's Hardcover library.
func (c *Client) Library(ctx context.Context) ([]UserBook, error) {
	const pageSize = 100 // Hardcover's cap per field
	var out []UserBook
	for offset := 0; ; offset += pageSize {
		var raw struct {
			Me []struct {
				UserBooks []struct {
					ID           int      `json:"id"`
					StatusID     int      `json:"status_id"`
					Rating       *float64 `json:"rating"`
					DateAdded    string   `json:"date_added"`
					LastReadDate *string  `json:"last_read_date"`
					Book         rawBook  `json:"book"`
				} `json:"user_books"`
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
			return out, nil
		}
		rows := raw.Me[0].UserBooks
		for _, r := range rows {
			ub := UserBook{Book: r.Book.book(), UserBookID: r.ID, Status: Status(r.StatusID), Added: day(r.DateAdded)}
			if r.Rating != nil {
				ub.Rating = *r.Rating
			}
			if r.LastReadDate != nil {
				ub.LastRead = day(*r.LastReadDate)
			}
			out = append(out, ub)
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
