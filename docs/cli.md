# CLI reference

Use `shelfcheck` for the interactive terminal UI and `shelfcheck json` for scripts
and agents. See [the TUI guide](tui.md) for keyboard controls and the
[README](../README.md) for installation and account setup.

## Command structure

```text
shelfcheck [flags] [query...]
shelfcheck json <command> [flags] [arguments...]
shelfcheck login [flags]
shelfcheck libby link <code>
shelfcheck libby probe [flags]
shelfcheck export hardcover [flags]
shelfcheck serve [flags]
```

Put the subcommand first, then its flags, then positional arguments. Flag parsing
stops at the first positional argument; `shelfcheck json search dune -brief`
searches for the text `dune -brief` instead of enabling brief output. Use
`-brief=false` or `-reviews=false` to disable a boolean flag. Use `--` before a
query that starts with a hyphen. There is no `--version` command.

Help is available through `shelfcheck -h`, `shelfcheck json -h`, and
`shelfcheck json <command> -h`. Other commands also accept `-h`, except
`libby link`, which expects exactly one code. Help normally goes to stderr.

### Catalog settings

The local TUI, `serve`, `login`, `export hardcover`, and all JSON commands except
`json libby` accept:

| Flag | Default | Meaning |
| --- | --- | --- |
| `-library` | `cincinnatilibrary` | Catalog subdomain, such as the `example` in `example.bibliocommons.com`; do not pass the full URL. |
| `-home` | `1` | Branch code used for shelf availability; `1` is Cincinnati's Main Library. Discover other codes with `json branches`. |

`-home` is a command setting, not a saved preference or the account's assigned
home branch. The default code's meaning depends on the library. Some commands
accept it for consistency without using it, including `branches`, `login`,
`history`, `checkouts`, `holds`, and `export hardcover`.

### Launch the local TUI

```sh
shelfcheck
shelfcheck -home 1 alien clay
shelfcheck -anywhere -theme vellum dune
shelfcheck -covers blocks
```

Without `-anywhere`, local searches start with the home-branch shelf filter.
Additional flags are `-anywhere` (default false), `-theme`, and `-covers`.
Themes resolve from the flag, then `SHELFCHECK_THEME`, then the choice saved with
`t`, otherwise the terminal theme. Built-in names are `terminal`, `umber`,
`vellum`, `orpiment`, and `cinnabar`; custom themes come from `themes.toml` in the
config directory.

Cover modes are `auto`, `kitty`, `blocks`, and `off`. Precedence is `-covers`,
`SHELFCHECK_COVERS`, `covers` in `config.toml`, then `auto`. Auto uses Kitty
graphics in Konsole, kitty, or Ghostty when outside tmux/screen, and otherwise uses
half-block rendering. Cover settings apply to the local TUI, not `serve`.

## JSON commands

JSON commands emit one indented JSON value on stdout. Errors and warnings go to
stderr. Failures exit nonzero; successful commands and most help requests exit
zero. JSON operations have a two-minute overall timeout. Keep stderr separate
from stdout when piping into a parser.

These commands read remote catalog/account data; they do not place or cancel
holds, renew or borrow books, change Libby tags, or edit Hardcover books. Account
reads can refresh the saved login session, and catalog/Hardcover reads can write
local caches. The local TUI has separate Hardcover editing features.

### `json search [flags] <query...>`

```sh
shelfcheck json search -brief -limit 5 alien clay
shelfcheck json search -type author -format book,ebook -home-only "Martha Wells"
shelfcheck json search -type genre -sort top-rated -format book "Science fiction"
shelfcheck json search -facets -filter AUDIENCE=teen -added-within 90 fantasy
shelfcheck json search -page 2 dune
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `-type` | `keyword` | `keyword`, `title`, `author`, `subject`, `genre`, or `series`. |
| `-home-only` | false | Require a copy on the shelf at `-home`. JSON searches otherwise cover all branches. |
| `-format` | none | Repeatable, comma-separated format names or raw codes; values in the field are ORed. |
| `-genre` | none | Repeatable, comma-separated genre headings, exactly as returned by facets; values are ORed. |
| `-filter` | none | Repeatable, comma-separated raw `FIELD=VALUE` filters. Field names are uppercased. |
| `-min-stars` | `0` | No minimum, or a raw average threshold of `1`, `2`, `3`, or `4`. |
| `-fiction` / `-nonfiction` | false | Apply the corresponding content filter. Choose the one you want. |
| `-added-within` | `0` | No restriction, or `30`, `60`, `90`, or `180` days since acquisition. |
| `-sort` | `relevance` | `relevance`, `top-rated`, `newest` (publication), `added`, `title`, `author`, or `raw-rating`. |
| `-page` | `1` | Catalog page, normally 25 records; ignored for `top-rated`. |
| `-limit` | `0` | Trim the returned page to this many results; `0` leaves it intact. This does not fetch more pages. |
| `-brief` | false | Omit descriptions. |
| `-facets` | false | Include refinement fields and their values/counts. |

Format aliases include `book`/`books` → `BK`, `large-print`/`largeprint` →
`LPRINT`, `ebook` → `EBOOK`, `eaudiobook` → `EAUDIOBOOK`,
`audiobook`/`audiobook-cd` → `AB`, `graphic-novel`/`comic` → `GRAPHIC_NOVEL`,
`dvd` → `DVD`, `bluray` → `BLURAY`, `music-cd` → `MUSIC_CD`, `vinyl` → `LP`,
and `sheet-music` → `MN`. Unknown format names are uppercased and sent as raw
catalog codes. Discover available facet values with `-facets`; availability
of filters depends on the catalog and query. Repeatable filters toggle values,
so supplying the exact same value twice removes it.

Output is an object containing `query`, `total`, `page`, `pages`, and `results`.
`query` records the submitted text, type, filters, and requested sort.
`-facets` adds a map of field names to arrays of `{value, count}`.

`top-rated` fetches up to ten catalog pages sorted by raw rating and reorders
them by weighted rating, which pulls small samples toward 3.75 stars. It returns
one ranked page, potentially larger than 25 records. When the returned set is
smaller than the catalog total, `partial: true` and `note` explain the subset.
Do not treat it as exhaustive pagination. A four-star average from one reader
is weaker evidence than the same average from many readers; inspect `rating.count`.

### Common book fields

Search records and other catalog-backed results use the following fields:

| Field | Meaning |
| --- | --- |
| `id`, `title`, `format`, `url` | Catalog bib ID, title, normalized format name, and catalog record link. Preserve the full ID as a string. |
| `subtitle`, `authors`, `year`, `edition`, `series` | Bibliographic information when present. |
| `digital`, `genres`, `subjects`, `audience`, `content_type` | Format/content metadata when available. |
| `rating` | `stars` (raw 0–5 average), `count`, and `weighted` (0–5); omitted for unrated records. |
| `availability` | `available`, `total`, `holds`, and optional `on_order` / `library_use_only`; this summary is not a per-branch copy count. |
| `call_number`, `isbns`, `description` | Optional detail; some commands intentionally omit these. |

Optional zero/empty fields are often omitted rather than emitted as `null`.
Consumers should tolerate absent fields and additional fields. Arrays such as
empty profile collections may be `null`; top-level result arrays are initialized
as `[]`. Catalog descriptions, reviews, and excerpts are converted to plain text
with paragraph breaks.

### `json book [flags] <id...>`

Get IDs from `json search`; do not invent IDs from titles or pass catalog URLs.

```sh
shelfcheck json book -reviews=false '<bib-id-from-search>'
shelfcheck json book -home 1 -excerpt '<bib-id-1>' '<bib-id-2>'
```

`-reviews` defaults true and includes trade reviews and an author bio, using an
extra request per record. `-excerpt` defaults false and may return a long opening
chapter. This command always returns an **array**, even for a single ID.

Each record extends the common fields with `on_shelf_at_home`, optional `copies`,
`digital_formats`, `reviews` (`source`, `text`), `about_author`, and `excerpt`.
Copies include `branch`, `branch_code`, optional `home`, `shelf`, `call_number`,
`volume`, `on_shelf`, `status`, and optional `due` (`YYYY-MM-DD`).

Availability or review lookup failures appear in a record's `errors` array while
the command can still exit zero. Missing IDs produce a stderr warning and a
shorter array, also potentially with exit zero. Check both per-record errors
and the returned IDs before assuming the lookup is complete.

### `json branches [flags]`

```sh
shelfcheck json branches
shelfcheck json branches -library cincinnatilibrary | jq '.[] | {code, name}'
```

Returns an array with `code`, `name`, `address`, `hours`, and optional `phone`
and `url`. Pass a returned code as `-home`; do not assume codes are universal.

### `json history`, `json checkouts`, `json holds`

These commands require a catalog session or credentials to obtain one:

```sh
shelfcheck login
shelfcheck json history
shelfcheck json checkouts -full
shelfcheck json holds
```

Each accepts `-full` (default false), `-credentials <file>`, and
`-session <file>`, as well as catalog settings. Without `-full`, descriptions,
subjects, and ISBNs are omitted. Output is an array of common book fields plus:

| Command | Additional fields when available |
| --- | --- |
| `history` | `checked_out`, `returned`; borrowing history is newest first. |
| `checkouts` | `due`, `status`, `times_renewed`. |
| `holds` | `status`, `hold_position`, `hold_placed`. |

History reflects the history available from your library account; it cannot
recover records the library never retained. Checkouts and holds supplement the
catalog list with linked Libby items. Those supplemental items can have an empty
catalog `id`; their constructed catalog `url` is not a usable record link.
A Libby failure warns on stderr but can still return the catalog list successfully.

### `json libby [flags]`

```sh
shelfcheck json libby
shelfcheck json libby -tag borrowed -subjects=false
shelfcheck json libby -tag 'want to read'
```

Requires `shelfcheck libby link`. Accepts `-tag` (default all tags, matches name
or kind case-insensitively) and `-subjects` (default true). It does not accept
`-library` or `-home`.

Returns `{tags: [...]}`. Tags contain `name`, optional `kind` and `description`,
`count`, and `titles`. Titles contain `title`, `libby_id`, and optional `author`,
`format`, `tagged` (`YYYY-MM-DD`), and `subjects`. No matching tag is an error.

### `json wishlist [flags]`

```sh
shelfcheck json wishlist -home 1
shelfcheck json wishlist -all -lang ''
shelfcheck json wishlist -tag 'Summer reading' -format book,ebook
```

Accepts `-tag` (default `want to read`), repeatable/comma-separated `-format`
(default `book,ebook`), `-all` (default false), and `-lang` (default `eng`; empty
means any language), plus catalog settings.

Reads the linked Libby tag and, for the default want-to-read tag, the Hardcover
Want to Read shelf when a token is configured. Titles are deduplicated across
sources. Libby wishlist input includes book-format items, excluding its magazine
and audiobook items. If no source is configured, the current implementation
returns an empty array rather than an authentication error.

Print matches must be on the shelf at `-home`; digital matches must be available
online now. Matching compares normalized titles and author surnames, so it can
miss a title or choose another edition. Review the returned record before relying
on a match. `-all` includes unavailable source titles; it does not add unavailable
catalog matches.

Returns an array with `title`, optional `author` and `tagged`, `available_now`,
and optional `matches`. Each match extends common book fields with `available_at`
(the home branch name or `online`), omitting descriptions, subjects, and ISBNs.
If one source fails while another provides titles, a stderr warning accompanies
the partial result.

### `json prompt [flags] <url|slug>`

```sh
shelfcheck json prompt -limit 10 '<prompt-slug>'
shelfcheck json prompt -all 'https://hardcover.app/prompts/<slug>'
shelfcheck json prompt -home 1 'https://hardcover.app/@<user>/lists/<slug>'
```

Requires `HARDCOVER_TOKEN`. Accepts `-limit` (default 30 books to check),
`-all` (default false), `-lang` (default `eng`, empty for any), and catalog settings.
Bare slugs refer to prompts. Prompts use most-given answer order; lists use list
order. Availability uses the same home-print/online-ebook matching as wishlist.

Returns an object with `kind` (`prompt` or `list`), `title`, `url`, optional
`readers`, and `books`. Each book has `title`, optional `author`, `votes`, and
`on_your_shelf`, plus `available_now` and optional wishlist-style `matches`.
`-all` retains unavailable books. Failure to read your Hardcover library can warn
while still returning the collection and availability.

### `json recommend [flags]`

```sh
shelfcheck json recommend -limit 10
shelfcheck json recommend -wishlist -v > picks.json
```

Accepts `-limit` (default 20 picks), `-wishlist` (default false; adds available
want-to-read titles and more work), `-v` (progress on stderr), and catalog settings.
It merges configured catalog, Libby, and Hardcover reading records. At least one
populated source is useful; no configured accounts can yield empty picks instead
of an authentication error. Partial source failures are stderr warnings.
Recommendations use English-language matching and favor books available now.

Returns `{profile: {...}, picks: [...]}`. Picks extend common book fields with
`on_shelf_at_home` (boolean), `ebook_now`, `score`, `why` (reason strings), and
optional `hardcover` (source prompt/list). The profile includes `books_read`,
`active_series`, `top_authors`, and `top_genres`, with optional
`recommended_not_on_shelf`, `recommended_to_buy`, `up_next_not_available`, and
`also_picked_from`. Scores express the recommender's ranking, not a probability
that you will like a book.

### `json arrivals [flags]`

```sh
shelfcheck json arrivals
shelfcheck json arrivals -home 1 -scan 150 -limit 10 -v
```

Lists print books sitting on a New Arrivals shelf at the home branch now. It
needs no account. Accepts `-audience` (default `adult`; empty for any), `-scan`
(default 75 recently added titles to check), `-limit` (default 30 returned),
`-v` (progress on stderr), and catalog settings. Only English-language books
added in the last 180 days are considered. A book counts when one copy is both
available and shelved in a collection named "new" at the home branch.

With a configured catalog, Libby, or Hardcover reading record the arrivals are
ranked against it and books you have already read, borrowed, placed on hold, or left out are dropped.
With `HARDCOVER_TOKEN`, Hardcover's own picks for you, ratings, and reader tags
also count. If the record cannot be loaded, a stderr warning is printed and the
arrivals are returned unranked, newest first.

Returns `{checked, failed, already_known, partial, ranked, arrivals}`. `checked`
is the number of recent titles examined, `failed` the availability checks that
failed, and `already_known` the books dropped because of your record. `partial`
is true when more recent titles remained beyond `-scan` or any check failed, so
an empty or short list is not proof that the shelf is empty; raise `-scan` to
look further. Each arrival extends common book fields with `shelf`, `score`,
optional `why` (reason strings), and optional `hardcover_url`. Scores are zero
when `ranked` is false.

## Accounts, linking, and export

### `login [flags]`

```sh
shelfcheck login
shelfcheck login -check
shelfcheck login -library cincinnatilibrary -credentials /private/credentials.env -session /private/session.json
```

Reads `SHELFCHECK_CARD` and `SHELFCHECK_PIN` from the environment or a private
credentials file. Environment values take precedence individually. The file
accepts `KEY=value` lines and must have no group/other permissions (`chmod 600`).
The default is `credentials.env` in the platform config directory's `shelfcheck`
subdirectory (`~/.config/shelfcheck` on typical Linux systems). The default
session file is `session.json` there.

Without `-check`, login creates/saves a session. `-check` only loads and verifies
the existing session; it does not log in again. Output is human-readable and
includes user/account IDs and the account home branch. Other account commands
automatically log in once and retry when the session is absent or expired, using
the saved credentials. The login command has a one-minute timeout.

`HARDCOVER_TOKEN` can be supplied in the environment or the same credentials file;
the environment wins. Custom `-credentials` / `-session` paths are supported only
by `login` and `json history|checkouts|holds`. Other features use the default store.

### `libby link <code>` and `libby probe [flags]`

Get an eight-digit code from Libby's **Menu → Settings → Copy To Another Device**,
then run `shelfcheck libby link <code>`. Successful linking writes `libby.json` in
the default config directory. Linking again replaces the saved identity.

`shelfcheck libby probe` writes raw sync, tags, and the first page of each tag to
`libby-probe` in the config directory. `-out <dir>` selects another directory.
This is a diagnostic tool; the files contain private account/reading data.
It prints a human-readable summary and uses private file permissions. It does
not export the complete contents of every tag.

### `export hardcover [flags]`

```sh
shelfcheck export hardcover -o reading.csv
shelfcheck export hardcover > reading.csv
```

Writes CSV with Hardcover's `Title`, `Author`, and `Status` columns plus
Goodreads-style columns. `-o` selects a file instead of stdout and overwrites
an existing file. Catalog settings are accepted. A count summary and source
warnings go to stderr; the operation has a three-minute timeout.

The merged reading record maps borrowing history and Libby borrows to Read,
current loans to Currently Reading, and holds/want-to-read titles to Want to
Read. Sampled-only titles are excluded. When a Hardcover token is configured,
titles already recognized in your Hardcover library are excluded. A source
failure can still produce a partial export, so inspect warnings. This generates
a file for manual import; it does not upload or change your Hardcover library.

## Serve the TUI over SSH

```sh
shelfcheck serve -addr 127.0.0.1:2222 -authorized-keys /path/to/authorized_keys
ssh -p 2222 localhost
ssh -p 2222 -t localhost dune
```

`serve` accepts catalog settings, `-anywhere`, and `-theme`, plus:

| Flag | Default | Meaning |
| --- | --- | --- |
| `-addr` | `:2222` | Listen address; the default listens on all interfaces. |
| `-host-key` | `.ssh/shelfcheck_ed25519` | Server host key path, relative to the working directory; generated if absent. |
| `-authorized-keys` | `~/.ssh/authorized_keys` | Allowed client public keys, in OpenSSH authorized-keys format. |
| `-idle-timeout` | `1h` | Idle session timeout, using Go durations such as `30m`. |

At least one valid public key is required before startup. Any SSH username is
accepted if its key is authorized; this is a TUI service, not a shell. A terminal
is required, so add `-t` when passing a search command. All sessions share one
catalog client/cache and the startup theme. Account screens are unavailable over
SSH, and sessions cannot switch themes. See [deployment](deployment.md) for
service deployment.

## Scripting and freshness

With optional `jq` installed, a discovery flow can preserve real bib IDs:

```sh
shelfcheck json search -brief -limit 5 -format book dune > search.json
jq -r '.results[] | [.id, .title, .availability.available] | @tsv' search.json
bib_id=$(jq -r '.results[0].id // empty' search.json)
if [ -n "$bib_id" ]; then
  shelfcheck json book -reviews=false "$bib_id" > book.json
  jq '.[0] | {id, on_shelf_at_home, errors}' book.json
fi
```

Check the producer's exit status before parsing a saved file. Shell pipelines
normally report only the last command's status; in Bash, `set -o pipefail` makes
an upstream failure visible. Do not combine stderr with stdout using `2>&1` when
expecting JSON. Empty results are valid, and a zero exit does not mean every
source or detail lookup succeeded.

Successful catalog responses are cached across runs in the platform cache
directory's `shelfcheck/cache.sqlite` (typically
`~/.cache/shelfcheck/cache.sqlite` on Linux). Search/bib metadata expire after
five minutes, copy availability after one minute, and branches/reviews after
24 hours. Hardcover library snapshots expire after five minutes, metadata after
24 hours, successful matches after seven days, and negative matches after ten
minutes. Hardcover private snapshots are scoped by a token fingerprint; private
reading information still exists in the cache, so do not share it.

There is no CLI refresh/cache-bypass flag. Availability is a recent snapshot,
not a reservation: verify with the library or Libby before a trip or borrowing.
Cache failures fall back to network access; a cache is not an offline catalog.
