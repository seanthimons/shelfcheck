package hardcover

import (
	"context"
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

// Shelves maps book IDs in the token owner's library to their status.
func Shelves(lib []UserBook) map[int]Status {
	m := make(map[int]Status, len(lib))
	for _, ub := range lib {
		m[ub.ID] = ub.Status
	}
	return m
}

// UserBook is a book in the token owner's Hardcover library.
type UserBook struct {
	Book
	Status   Status
	Rating   float64 // 0.5-5 stars; 0 if unrated
	Added    string  // YYYY-MM-DD
	LastRead string  // YYYY-MM-DD finished, if recorded
}

// Library returns every book in the token owner's Hardcover library.
func (c *Client) Library(ctx context.Context) ([]UserBook, error) {
	const pageSize = 100 // Hardcover's cap per field
	var out []UserBook
	for offset := 0; ; offset += pageSize {
		var raw struct {
			Me []struct {
				UserBooks []struct {
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
    status_id rating date_added last_read_date book { ` + bookFields + ` }
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
			ub := UserBook{Book: r.Book.book(), Status: Status(r.StatusID), Added: day(r.DateAdded)}
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
