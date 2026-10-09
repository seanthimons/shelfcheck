# Editing a Hardcover record

Set `HARDCOVER_TOKEN` in the environment or your private shelfcheck
`credentials.env` file. Open a book's detail screen and press `e` once its
Hardcover match has loaded. The book must already be in your Hardcover library.

The editor fetches your current record before making changes:

- Use Tab / Shift+Tab to move between status, rating, review, and spoiler flag.
- Use arrow keys to change status; Space toggles the spoiler flag.
- Ratings accept 0.5–5 in half-star steps. A blank rating requests removal.
- Reviews accept multiple lines and Markdown. An empty review requests removal.
- Ctrl+S saves changed fields. Esc discards the draft before saving.

While saving, the editor waits for Hardcover's response. Successful saves update
book details and Hardcover shelf markers; wishlist and recommendation data reload
when you next open those screens. Reviews marked as containing spoilers are
hidden on the detail screen and available in the editor.

Hardcover may reject or ignore an update, including a request to clear a field.
Shelfcheck checks the returned values, reports mismatches, and retains your draft.
Some fields may already have saved when an error appears. If a connection fails
or the response can't confirm the update, reopen the editor to load the current
record before retrying.

## Verification before closing issue #3

Automated coverage uses a local fake GraphQL server and tests editing, validation,
partial requests, missing records, rejected/ignored writes, cancellation, and
cache invalidation. Run `go test ./...` and `go vet ./...`.

A live check remains necessary with a designated test book/account:

1. Save a rating, review, spoiler flag, and status; reopen the editor and verify
   the values on Hardcover too.
2. Edit only the review and confirm rating, status, dates, and privacy remain
   unchanged. Check whether rating changes cause any upstream status changes.
3. Clear the rating and review; confirm whether Hardcover applies those requests
   or shelfcheck displays an ignored-field error.
4. Cancel a draft and confirm nothing was written. Try an expired token and
   confirm the draft remains available after a failed save.

No live account records are changed by the automated tests.
