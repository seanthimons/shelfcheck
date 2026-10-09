# Terminal interface guide

[Getting started](../README.md) · [CLI reference](cli.md) · [Configuration](configuration.md) · [Hardcover editing](hardcover-editing.md)

Run `shelfcheck` in an interactive terminal to open Search. If you built in the
project directory, use `./shelfcheck` instead. Catalog browsing needs no account.
The default catalog is Cincinnati & Hamilton County Public Library
(`cincinnatilibrary`) and the default home branch is Main Library (`1`).
Choose another BiblioCommons catalog and branch with
`-library` and `-home`; find branch codes with `shelfcheck json branches`.

```sh
shelfcheck
shelfcheck dune
shelfcheck -anywhere "Octavia Butler"
shelfcheck -home 1 -theme vellum -covers blocks
```

Put flags before search words. Startup words immediately run a keyword search.
By default, searches show titles on the shelf at your home branch; `-anywhere`
starts without that restriction. Home branch and filters affect what you see,
so broaden the search if a familiar book is missing.

## A first search

1. Type a title, author, or subject. Use Tab to choose Keyword, Title, Author,
   Subject, Genre, or Series; Shift+Tab moves backward.
2. Press Enter. Move through results with arrow keys or `j`/`k`.
3. Press Enter to open a book. Its details include available copies, branch,
   collection, call number, and due dates where provided by the catalog.
4. Press Esc to return to results. Press `a` to include every branch, or `f`
   to refine the search by format, language, audience, and other facets.
5. Press `/` to edit the search, or `q` to quit from a browsing screen.

The header shows your current view, library, and home branch when space allows.
The footer shows useful keys; `?` opens help on browsing screens and any key
closes it. Wider terminals show more columns and put availability beside book
details; narrow terminals stack information and hide some columns.

## Search and results

Result ratings come from the library catalog. `AVAILABLE` summarizes copies
across the catalog, while `HOME` can show `⌂ here` for a physical copy on your
home shelf. That marker appears only after you have opened the book and fetched
its copies. A blank marker does not establish that no home copy is available.

More results load as you move near the end of the current page. End/`G` jumps
to the last loaded row, not necessarily the final match in the catalog. A
"showing the top" note means the catalog returned a ranked subset.

| Search key | Action |
| --- | --- |
| Enter | Search the entered text, or open a supported Hardcover collection URL |
| Tab / Shift+Tab | Next / previous search type |
| Esc | Return to existing results; quit if there are none |
| Alt+P / Alt+W / Alt+M | Open For you / Wishlist / Account |
| Alt+T | Cycle theme |

Ordinary letters, including `q`, are text while Search is focused. Use Ctrl+C
to quit directly.

| Results key | Action |
| --- | --- |
| Up/`k`, Down/`j` | Previous / next row |
| PageUp / Ctrl+U | Move up one visible page |
| PageDown / Ctrl+D / Space | Move down one visible page |
| Home/`g`, End/`G` | First / last loaded row |
| Enter / Right / `l` | Open selected book |
| `f` | Open filters |
| `F` | Clear all filters and custom sort, including home-only |
| `a` | Toggle on-shelf-at-home filtering |
| `o` | Cycle Relevance, Top rated, Newest published, and Recently added |
| `/` / `s` / Esc | Return to Search |

### Filters

The filter panel edits a draft query. Closing its category view with Esc or `f`
**applies** the draft and reruns the search. Esc from a value list first returns
to categories; it does not discard the changes.

Categories depend on available catalog facets: Available, Format, Genre, Rating,
Fiction, Audience, Recently added, Year, Language, Author, and Sort. Available
includes your home branch, any physical shelf, online, and branches returned by
the catalog. Sort choices are Relevance, Top rated, Newest published, Recently
added, Title, and Author.

| Panel | Keys | Action |
| --- | --- | --- |
| Categories | Up/`k`, Down/`j` | Move between categories |
| Categories | Enter / Right / `l` / Space | Open values |
| Categories | `x` / Backspace | Clear selected category |
| Categories | `c` | Clear every filter and custom sort |
| Categories | Esc / `f` | Apply draft and close |
| Values | Up/`k`, Down/`j`, PageUp, PageDown | Move through values |
| Values | Enter / Space | Toggle a value; choosing sort returns to categories |
| Values | Esc / Left / `h` / Backspace | Return to categories |

Your current home filter becomes the default for subsequent searches within
this session. Other filters and sort do not carry into a fresh search. These
search preferences are not saved between runs.

## Book details

Details show catalog metadata, descriptions, reviews, excerpts, and copy
availability when provided. Digital records list formats and availability for
borrowing through Libby/OverDrive. With `HARDCOVER_TOKEN` configured, shelfcheck
also looks up Hardcover ratings, genres, moods, series, your shelf, and your
rating/review. Matching across services may select a different edition; inspect
the title and author before editing.

Follow the displayed **Open in library catalog** link (or copy its URL) to
place a hold in your browser and choose pickup there. The TUI does not place
holds or renew loans.

| Detail key | Action |
| --- | --- |
| Up/`k`, Down/`j` | Scroll one line |
| PageUp / `b`, PageDown / Space / `f` | Scroll one page |
| Ctrl+U / `u`, Ctrl+D / `d` | Scroll half a page |
| Esc / Backspace / Left / `h` | Return to the view that opened the book |
| `c` | Search the catalog for this title across branches |
| `r` | Refresh availability and Hardcover information |
| `e` | Open your existing Hardcover record for editing |
| `/` / `s` | Open Search |

Availability is cached briefly, so repeated refreshes may reuse a recent
response. Books from Libby or Hardcover without a catalog record can still
open a detail view; `c` searches for alternate catalog matches and formats.

## Account, Wishlist, and For you

These views use the local account setup described in
[Configuration](configuration.md). You can combine a catalog login (`shelfcheck login`),
Libby device linking (`shelfcheck libby link <code>`), and `HARDCOVER_TOKEN`.
Restart the TUI after changing credentials or linking an account.

- **Account** (`m`) opens a workspace with a section sidebar and book list.
  Overview combines current loans, holds, and Hardcover's Currently Reading
  shelf. Library & Libby sections show loans and holds with due dates, queue
  positions, and Libby wait estimates where supplied. Borrowing history shows
  the 10 most recent library entries; it is separate from Hardcover's Read shelf.
  Hardcover sections include All books and all six reading statuses, with your
  ratings and dates. Each integration works independently, including Hardcover
  without a catalog login and Libby-only loans and holds. A source failure leaves
  other sources visible with an error note; `r` retries the selected section's
  sources. Hardcover shelves show all your books without catalog availability
  filtering. Enter opens details; `c` there searches the catalog.
- **Wishlist** (`w`) combines Libby's "want to read" tag and Hardcover's Want
  to Read shelf. It checks each title and initially shows physical copies on
  your home shelf or ebooks available now. `A` switches between available and
  all titles; `r` rechecks. Opening an unavailable title searches the catalog
  across branches.
- **For you** (`p`) uses reading history from any configured catalog, Libby, or
  Hardcover source. Recommendations arrive in stages and include reasons. It
  prioritizes home-shelf print copies and available ebooks, with additional
  sections for Hardcover recommendations at other branches, checked out, or
  absent from this catalog. The TUI requests up to 30 picks in English.
  `r` rebuilds recommendations; `h` opens the selected pick's Hardcover source
  when one exists. Books without catalog records open Hardcover details.

The first recommendation run can take a while; the progress message shows
which source is being checked. Lists fill as checks finish. Source failures may
leave partial results with a visible error or count of failed checks.

Account uses Tab / Shift+Tab to switch focus between sections and books.
Up/`k` and Down/`j` move within the focused pane; Enter/Right/`l` enters the book
list or opens the selected book. Left/`h` focuses sections. Each section remembers
its book selection. Narrow terminals show a section picker instead of the sidebar.
On a Hardcover shelf, `f` edits a title/author filter (Enter applies, Esc cancels),
`o` cycles Recently added, Title, Author, Rating, and Last read, and `e` opens the
selected record's editor directly. Filters and sort are remembered per shelf
within the session; ordinary shortcut letters are text while editing a filter.
Editing from the account list returns to that list after saving or canceling.

All three views support Up/`k`, Down/`j`, Home/`g`, End/`G`, and
Enter/Right/`l` to open a row. They also support PageUp and PageDown/Space while
browsing books. Esc, `/`, or `s` returns to
Search. For you's "up next in your series" notes are informational; search the
title to see holds and availability.

## Hardcover prompts, lists, and vibes

With a Hardcover token configured, paste a collection URL into the **Search
box** and press Enter. Supported forms include:

```text
https://hardcover.app/prompts/<prompt-slug>
https://hardcover.app/@<user>/lists/<list-slug>
https://hardcover.app/@<user>/vibes/<vibe-slug>
```

Replace placeholders with an actual collection URL. Startup command-line
search words run a catalog keyword search; use the Search box for this feature.
Individual Hardcover book URLs are not collection URLs.

Shelfcheck checks up to 30 collection books against your catalog and initially
shows available home-shelf print copies and ebooks. `A` shows all checked
books, including unavailable ones. Rows can show your Hardcover shelf or the
number of prompt picks. Enter opens the best available match, preferring print;
without a match it opens Hardcover details instead.

**`+` immediately changes your Hardcover library.** It adds a book to Want to
Read if absent, or removes the existing Want to Read record if already there.
Books on another shelf (Read, Currently reading, and so on) are left alone.
There is no separate confirmation step. The Wishlist reloads after a successful
shelf change when next opened.

| Hardcover collection key | Action |
| --- | --- |
| Up/`k`, Down/`j`, PageUp, PageDown/Space | Move through rows |
| Home/`g`, End/`G` | First / last visible row |
| Enter / Right / `l` | Open selected book |
| `+` | Add/remove selected book on Hardcover Want to Read |
| `A` | Toggle all books / available books |
| `r` | Reload collection and recheck availability |
| Esc | Return to the originating view |
| `/` / `s` | Open Search |

For changing rating, review, or status on a book already in your Hardcover
library, open its details and press `e`. See [Hardcover editing](hardcover-editing.md)
for save/cancel behavior and recovery from failed writes.

## Themes and covers

The bundled themes are `terminal`, `umber`, `vellum`, `orpiment`, and `cinnabar`.
`terminal` follows your terminal's palette. Cycle with `t` while browsing or
Alt+T in Search. The choice is saved in the config directory's `config.toml`.
Startup precedence is `-theme`, then `SHELFCHECK_THEME`, then the saved choice,
then `terminal`.

Add themes in `themes.toml` in the [configuration directory](configuration.md). Each TOML
table names a theme; a matching name replaces a built-in. Colors must use
`#rrggbb`; an optional `ansi` array must contain exactly 16 colors. For example:

```toml
[ocean]
label = "Ocean"
active = "#70b8d0"
dim = "#81929c"
border = "#243c48"
bar = "#182a34"
barfg = "#dce7eb"
```

Book jackets appear on details. Choose `-covers auto`, `kitty`, `blocks`, or
`off`. Precedence is the flag, `SHELFCHECK_COVERS`, then `covers` in
`config.toml`, then `auto`. Auto uses Kitty graphics in detected Konsole, kitty,
and Ghostty terminals outside tmux/screen; otherwise it uses text half blocks.
Blocks work best with true color. Use `-covers blocks` or `-covers off` if images
render poorly. Missing or failed jacket downloads simply omit the cover.

```toml
# config.toml
theme = "vellum"
covers = "blocks"
```

## Shared shortcuts and SSH sessions

On browsing screens, `p`, `w`, and `m` switch to For you, Wishlist, and Account;
`t` cycles theme, `?` opens help, and `q` quits. The plain-letter navigation and
theme shortcuts do not operate inside filters; Alt+P/W/M/T remain available.
An active Hardcover editor owns input, so review text does not trigger those
shortcuts. Ctrl+C exits everywhere, including the editor.

When connecting to `shelfcheck serve` over SSH, catalog search and details work
with the same keys. Personal accounts, Hardcover features, and recommendations
are unavailable in those sessions. The server selects one shared theme;
theme cycling is disabled and covers use text blocks. See the
[CLI reference](cli.md) for serving and connection commands.
