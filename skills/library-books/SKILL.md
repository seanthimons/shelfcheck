---
name: library-books
description: Recommend books from the user's BiblioCommons library catalog with the shelfcheck CLI, favoring copies on the shelf at their home branch right now. Use when the user asks for book recommendations, what to read next, what's new at the library, or whether a specific book is available.
---

# Library book recommendations

Recommend books the user can pick up at their **home branch** today, using the
`shelfcheck` CLI for live catalog data and the user's taste notes for fit.

## Setup

- `shelfcheck` must be on `PATH`. If it isn't, say so and stop.
- The library and home branch come from `config.toml` in the shelfcheck config
  directory (`~/.config/shelfcheck` on Linux). Without them, shelfcheck falls
  back to Cincinnati's Main Library: if `config.toml` has no `library` and `home`,
  ask the user which library and branch to use (see `shelfcheck json branches
  -library <slug>`) and suggest saving them there. Don't pass `-library` or `-home` unless the user
  names another library or branch. To name the home branch, read the copy with
  `"home": true` in `shelfcheck json book` output; to find a code for another
  branch, use `shelfcheck json branches`.
- Never read or print `credentials.env`, `session.json`, `libby.json`, or
  `libby-probe/` in that directory. Don't run `login`, `libby link`, or
  `libby probe` yourself; tell the user which one to run.

## Sources of truth

Every account source is optional. Use whichever ones work and say once which
are missing; don't stop because one is absent.

- **Catalog reading record:** `shelfcheck json history`, `json checkouts`,
  `json holds` return borrowing history (newest first), current loans and holds,
  including linked Libby loans and holds. They log in with saved credentials.
  On a login error, suggest `shelfcheck login` and continue without them.
- **Libby:** `shelfcheck json libby` returns tags with titles: `kind: "borrowed"`
  (everything borrowed in Libby, with dates), `"sampled"`, `"notify-me"`, plus
  the user's own tags, such as a want-to-read wishlist or a tag for finished
  books. Use `-tag <name>` to fetch one. If it says Libby isn't linked, suggest
  `shelfcheck libby link <code>` with a fresh **Copy To Another Device** code.
- **Taste notes:** `taste.md` in the shelfcheck config directory: what the user
  loved or disliked and why, and what was already recommended. History says what
  they borrowed, not whether they liked it; taste notes say that. If the file
  doesn't exist, create it (owner-only permissions) with "Loved", "Disliked" and
  "Already recommended" sections, and ask one short question about what they
  loved or disliked most recently.
- **Catalog:** `shelfcheck json …`, which returns live JSON. **Never state
  availability, shelf, call number, or due dates from memory**, only from
  `shelfcheck json book` output in this conversation.
- **Your own knowledge** is for brainstorming candidates and explaining why a
  book fits. Confirm every book you name exists in the catalog first.

## Commands

Flags go before the query. Run `shelfcheck json <command> -h` for the full list.

```bash
# Find candidates. -home-only = a copy is on the shelf at the home branch now.
shelfcheck json search -home-only -format book -sort top-rated -brief -limit 15 -type genre "space operas"
shelfcheck json search -home-only -format book -brief -type author "Martha Wells"
shelfcheck json search -home-only -format book -brief -type title "Piranesi"
shelfcheck json search -facets -brief -limit 1 -type subject "artificial intelligence"   # discover genre names

# Want-to-read titles (Libby tag, plus Hardcover if configured) available now:
# print on the shelf at the home branch, or ebooks available online.
shelfcheck json wishlist

# New Arrivals shelf at the home branch, ranked against the reading record.
shelfcheck json arrivals -limit 15

# Confirm finalists (several IDs per call): copies, shelf, call number, reviews, author bio.
shelfcheck json book '<id-1>' '<id-2>'
```

- `-type`: keyword (default), title, author, subject, genre, series.
- `-sort top-rated` ranks by a **weighted** rating that discounts titles with few
  ratings; prefer it over `rating.stars`. `rating.count` under ~10 means the
  rating barely matters. When `partial` is true, the ranking covers a subset.
- `-genre` values must match the catalog's headings exactly; get them from
  `-facets`.
- Print and ebook records share one rating; use `-format book` unless the user
  wants ebooks or audiobooks.
- Each call is a live request to the library's site. Keep it to a handful of
  searches plus one `book` call per recommendation round, and don't loop over pages.
- Take IDs only from search output. A Libby `libby_id` is not a catalog ID, and
  a checkout or hold with an empty `id` has no usable catalog link.

## Method

1. **Read their record and taste notes.** Run history, checkouts, holds and
   `json libby`. Note repeat authors, series they're partway through, recurring
   genres and subjects, and recency (recent borrowing says more than old
   borrowing), plus the taste notes' loves, dislikes and "Already recommended".
   **Never recommend anything in their history, loans, holds, Libby borrowed or
   finished tags, or Already recommended.** Match on title and author across
   formats: an ebook they read counts for the print book too. Borrowing a whole
   series, or several books by one author, is a strong signal they liked it.
2. **Check the wishlist first.** `shelfcheck json wishlist` (one call, can take
   ~20 seconds) returns only want-to-read titles they can get now. Each match
   has `available_at`: the home branch name, or `online` for ebooks. Prefer home
   branch matches for "what should I pick up"; ebook matches suit "something to
   read tonight". These are books they already chose, so lead with up to two of
   them, labeled "from your want-to-read list". Matches are by title and author,
   so confirm the edition with `json book`. It reads the Libby tag named
   "want to read"; pass `-tag '<name>'` if theirs is named differently. An empty
   array can also mean no wishlist source is configured. Sampled-but-never-borrowed titles
   are weak signals; don't recommend them unless asked.
3. **Generate candidates two ways** (aim for 10–20):
   - *From knowledge:* read-alikes, the next book in series they've started,
     other books by authors they return to. Look each up with `-type title` or
     `-type author` and `-home-only`.
   - *From the catalog:* 2–4 genre or subject searches drawn from their taste,
     with `-home-only -format book -sort top-rated`, to surface books you
     wouldn't have thought of. For "what's new", use `json arrivals` instead;
     if it returns `partial: true`, say the shelf scan was incomplete.
4. **Filter:** drop anything already read or recommended, anything matching a
   dislike, series entries they can't start yet (check `series`; recommend book 1
   unless they've read the earlier ones), and titles with fewer than ~5 ratings
   unless there's a strong taste reason. New arrivals rarely have ratings yet;
   judge those by author track record, reviews and fit.
5. **Confirm 3–5 finalists** with one `shelfcheck json book` call. Keep only
   books with `on_shelf_at_home > 0` (unless the user asked for anywhere, holds,
   or ebooks). If a record has `errors`, its availability is unconfirmed; don't
   recommend it as on the shelf. Skim `reviews` to make sure the book is what
   you think it is.
6. **Recommend 3** (fewer is fine; never pad).
7. **Update the taste notes:** append the picks under "Already recommended" with
   today's date.

## Output

For each pick, one block:

**Title** by Author (year) · ★4.4 from 872 ratings
Why it fits *them*, in one or two sentences tied to something specific in the
taste notes or reading record, not a generic blurb.
On the shelf at *Home Branch*: *shelf*, call number *call number* · [catalog](url)

Then at most one line of follow-up (e.g. "Want more like X, or something different?").

Style: plain and specific. No hype words (must-read, masterpiece, incredible,
perfect), no filler, no "not just X but Y". Say the one true reason it fits. If
a book they'd love is only at another branch or checked out, you may mention it
in one line after the picks with its branch or due date, from tool output.

## Specific-book questions

"Is X at the library?" or "Where is X?": search `-type title` without
`-home-only`, then `shelfcheck json book <id>`. Answer with the home branch's
status first (shelf and call number, or due date), then other branches with it
on the shelf, then holds. Mention ebook or audiobook availability if relevant.

## Updating taste

When the user says they read, loved, disliked, or abandoned something, update
the taste notes right away under the right heading, with a few words on why if
they gave a reason. Keep the file short and scannable.
