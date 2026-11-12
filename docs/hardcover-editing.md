# Editing a Hardcover record

[TUI guide](tui.md) · [CLI and account setup](cli.md)

Set `HARDCOVER_TOKEN` in the environment or your private shelfcheck
`credentials.env` file. Open a book's detail screen and press `e` once its
Hardcover match has loaded. You can also open Account (`m`), select a Hardcover shelf, focus its books,
and press `e` on a row to open the editor directly. Saving or canceling returns
to the account list. The book must already be in your Hardcover library.

The editor fetches your current record before making changes:

- Use Tab / Shift+Tab to move between status, rating, review, and spoiler flag.
- Use arrow keys to change status; Space toggles the spoiler flag.
- Ratings accept 0.5–5 in half-star steps. A blank rating requests removal.
- Reviews accept multiple lines and Markdown. An empty review requests removal.
- Ctrl+S saves changed fields. Esc discards the draft before saving.

Available statuses are Want to Read, Currently Reading, Read, Paused, Did Not
Finish, and Ignored. On the status field, Left/Up moves backward and
Right/Down/Space/Enter moves forward. On the spoiler field, Space, Enter, Left,
or Right toggles the flag. Enter in the review inserts a new line.

The editor owns letter keys, including `q`, `p`, `w`, `m`, and `t`, so you can
type ordinary review text. Ctrl+C or Ctrl+Q still quits the application. Once you press
Ctrl+S, Esc cannot cancel the request already sent to Hardcover; wait for the
response before making further edits. A successful save closes the editor.

While saving, the editor waits for Hardcover's response. Successful saves update
book details and Hardcover shelf markers; wishlist and recommendation data reload
when you next open those screens. Reviews marked as containing spoilers are
hidden on the detail screen and available in the editor.

Hardcover may reject or ignore an update, including a request to clear a field.
Shelfcheck checks the returned values, reports mismatches, and retains your draft.
Some fields may already have saved when an error appears. If a connection fails
or the response can't confirm the update, reopen the editor to load the current
record before retrying.

## Checking a change and recovering from errors

After saving, reopen the editor to load the current server record. For important
changes, check the same book on Hardcover too. Shelfcheck sends only changed
fields; upstream service behavior can still affect the resulting record.

If you see a validation error, correct the rating and press Ctrl+S again. If
Hardcover rejects or ignores a field, read the error and inspect the saved
values before retrying. If a network error leaves the outcome unclear, press
Esc and reopen the editor rather than assuming nothing saved. You may want to
copy your review draft before leaving after an error.

This editor updates existing records. To add a new book, add it on Hardcover
first, or use `+` from [Book details](tui.md#book-details) or a
[Hardcover collection](tui.md#hardcover-prompts-lists-and-vibes) to add it to
Want to Read.
