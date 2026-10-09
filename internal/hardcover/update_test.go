package hardcover

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestUserBookLoadsEditableRecord(t *testing.T) {
	c := fakeHardcover(t, func(q string, vars map[string]any) string {
		if vars["b"] != float64(42) || !strings.Contains(q, "me { user_books(where:") || !strings.Contains(q, "review_markdown") {
			t.Errorf("unexpected lookup: %s %v", q, vars)
		}
		return `{"data":{"me":[{"user_books":[{"id":7,"status_id":3,"rating":4.5,"date_added":"2026-10-01","last_read_date":"2026-10-02","review_markdown":"A **great** book.\nSecond line.","review_has_spoilers":true,"book":{"id":42,"title":"Book"}}]}]}}`
	})
	ub, ok, err := c.UserBook(context.Background(), 42)
	if err != nil || !ok || ub.UserBookID != 7 || ub.ID != 42 || ub.Rating != 4.5 || ub.Status != Read || ub.LastRead != "2026-10-02" || ub.Review != "A **great** book.\nSecond line." || !ub.ReviewHasSpoilers {
		t.Fatalf("UserBook = %+v, %v, %v", ub, ok, err)
	}
}

func TestUserBookMissingAndUnauthorized(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		wantError      bool
	}{
		{"missing", `{"data":{"me":[{"user_books":[]}]}}`, false},
		{"no user", `{"data":{"me":[]}}`, true},
		{"token", `{"errors":[{"message":"invalid JWT"}]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fakeHardcover(t, func(string, map[string]any) string { return tc.response })
			_, ok, err := c.UserBook(context.Background(), 42)
			if ok || (err != nil) != tc.wantError {
				t.Fatalf("ok=%v, err=%v", ok, err)
			}
		})
	}
}

func TestUpdateUserBookPatch(t *testing.T) {
	rating, status, review, spoilers := 4.5, Read, "Review\nwith **Markdown**", false
	cases := []struct {
		name  string
		patch UserBookChanges
		want  map[string]any
		after map[string]any
	}{
		{"rating only", UserBookChanges{Rating: &rating}, map[string]any{"rating": rating}, map[string]any{"rating": rating}},
		{"clear rating", UserBookChanges{ClearRating: true}, map[string]any{"rating": nil}, map[string]any{"rating": nil}},
		{"review only", UserBookChanges{Review: &review}, map[string]any{"review_markdown": review}, map[string]any{"review_markdown": review}},
		{"status and spoilers", UserBookChanges{Status: &status, ReviewHasSpoilers: &spoilers}, map[string]any{"status_id": float64(Read), "review_has_spoilers": false}, map[string]any{"status_id": Read, "review_has_spoilers": false}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := fakeHardcover(t, func(q string, vars map[string]any) string {
				if !strings.Contains(q, "update_user_book(id: $id, object: $object)") || vars["id"] != float64(7) {
					t.Errorf("unexpected update: %s %v", q, vars)
				}
				if !reflect.DeepEqual(vars["object"], tc.want) {
					t.Errorf("patch=%v, want %v", vars["object"], tc.want)
				}
				entry := map[string]any{"id": 7, "status_id": Read, "rating": 3, "review_markdown": "Old review", "review_has_spoilers": true, "book": map[string]any{"id": 42}}
				for k, v := range tc.after {
					entry[k] = v
				}
				b, _ := json.Marshal(map[string]any{"data": map[string]any{"update_user_book": map[string]any{"id": 7, "user_book": entry}}})
				return string(b)
			})
			ub, err := c.UpdateUserBook(context.Background(), 7, tc.patch)
			if err != nil || ub.UserBookID != 7 || ub.ID != 42 {
				t.Fatalf("update=%+v, %v", ub, err)
			}
		})
	}
}

func TestUpdateUserBookRejectsInvalidPatchBeforeRequest(t *testing.T) {
	for _, rating := range []float64{0, 0.25, 3.7, 5.5, math.NaN(), math.Inf(1)} {
		c := fakeHardcover(t, func(string, map[string]any) string { t.Fatal("invalid patch sent request"); return "" })
		if _, err := c.UpdateUserBook(context.Background(), 7, UserBookChanges{Rating: &rating}); err == nil {
			t.Errorf("accepted rating %v", rating)
		}
	}
	c := fakeHardcover(t, func(string, map[string]any) string { t.Fatal("invalid patch sent request"); return "" })
	status, rating := Status(100), 4.0
	for _, patch := range []UserBookChanges{{}, {Status: &status}, {Rating: &rating, ClearRating: true}} {
		if _, err := c.UpdateUserBook(context.Background(), 7, patch); err == nil {
			t.Errorf("accepted %+v", patch)
		}
	}
}

func TestUpdateUserBookReportsRejectedOrUnconfirmedWrites(t *testing.T) {
	rating := 4.5
	for _, response := range []string{
		`{"errors":[{"message":"Permission denied"}]}`,
		`{"data":{"update_user_book":{"error":"Not your book"}}}`,
		`{"data":{"update_user_book":null}}`,
		`{"data":{"update_user_book":{"id":7,"user_book":null}}}`,
		`{"data":{"update_user_book":{"id":8,"user_book":{"id":8}}}}`,
	} {
		c := fakeHardcover(t, func(string, map[string]any) string { return response })
		if _, err := c.UpdateUserBook(context.Background(), 7, UserBookChanges{Rating: &rating}); err == nil {
			t.Errorf("accepted %s", response)
		}
	}
}

func TestUpdateUserBookReportsIgnoredReviewClear(t *testing.T) {
	empty := ""
	c := fakeHardcover(t, func(string, map[string]any) string {
		return `{"data":{"update_user_book":{"id":7,"user_book":{"id":7,"status_id":3,"review_markdown":"Still here","book":{"id":42}}}}}`
	})
	ub, err := c.UpdateUserBook(context.Background(), 7, UserBookChanges{Review: &empty})
	if err == nil || !strings.Contains(err.Error(), "didn't apply review") || ub.Review != "Still here" || ub.ID != 42 {
		t.Fatalf("ignored update=%+v,%v", ub, err)
	}
}
