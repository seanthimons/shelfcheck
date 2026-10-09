# Using shelfcheck from scripts and AI agents

Use `shelfcheck json` for structured data. The [CLI reference](cli.md) lists all
flags and setup requirements; [configuration](configuration.md) covers account
files. The root [AGENTS.md](../AGENTS.md) is for agents modifying this repository.

## Invocation and output contract

```sh
./shelfcheck json help
./shelfcheck json search -h
./shelfcheck json search -home 1 -home-only -format book -brief "alien clay"
```

Pass flags before positional arguments. Use an argument array when invoking the
binary from a program; do not concatenate user queries into a shell command.
Library and branch defaults are `cincinnatilibrary` and `1` (its Main Library).
Pass `-library` and `-home` explicitly when using another catalog or branch.
These flags belong to individual commands, not before `json`.

Successful data commands write one indented JSON value to stdout. Diagnostics,
warnings and help go to stderr. Errors produce a nonzero exit; help exits
successfully without JSON. Each JSON invocation has an overall two-minute
timeout. Read the exit status before parsing output, but also inspect diagnostics
and result fields: a successful exit can still describe incomplete data.

There is no published versioned JSON schema. The current shapes are defined by
the output structs in `json.go`. Ignore unfamiliar fields, tolerate omitted
optional fields, and treat absent data as unknown rather than zero evidence.
Some slice fields can be `null`; normalize those to empty collections where
appropriate for your consumer. Retain catalog IDs as strings.

| Command | Top-level JSON | Notes |
| --- | --- | --- |
| `search` | Object with `query`, `total`, `page`, `pages`, `results` | Optional `facets`, `partial` and `note` |
| `book` | Array of detailed records | Array even for one ID; inspect each record's `errors` |
| `branches` | Array of branches | `code` is the branch identifier; includes human-readable hours |
| `history`, `checkouts`, `holds` | Array of records with account fields | Dates, loan status or hold position where supplied |
| `libby` | Object with `tags` | Each tag contains `titles` with `libby_id` |
| `wishlist` | Array of requested titles | `available_now`, optional catalog `matches` and `available_at` |
| `prompt` | Object with collection metadata and `books` | `kind` is `prompt` or `list`; each book can have `matches` |
| `recommend` | Object with `profile` and `picks` | Each pick includes `score`, `why` and availability indicators |

## A catalog discovery workflow

1. Run `json branches` to discover branch codes rather than assuming `1` works
   for every library.
2. Search by keyword, title or author. `-brief` trims descriptions; `-facets`
   returns available refinements and their exact values.
3. Take `results[].id` into `json book` for copies and detailed availability.
   Use `-reviews=false` when reviews and author biographies are unnecessary.
4. Present the returned title, author, format, branch, call number and `url` so
   the user can confirm the edition and act through their catalog.

For example, with `jq` installed:

```sh
./shelfcheck json search -type title -format book -brief "alien clay" > search.json
jq '.results[] | {id, title, authors, format, url}' search.json

# Replace this with an actual ID from that search.
./shelfcheck json book -home 1 -reviews=false '<bib-id>' > book.json
jq '.[] | {id, title, on_shelf_at_home, copies, errors}' book.json
```

Store complete outputs when auditing results, and check the command's exit status
before running the next step. These files can contain private data when using
account commands; choose a private destination for those outputs.

## Interpreting completeness and availability

Ordinary searches return a page of 25 results. `-limit` only truncates that page;
it does not fetch additional pages. For pagination, use the returned `page` and
`pages`. `-sort top-rated` instead ranks a bounded candidate set into one page;
when `partial` is true, its `note` explains the subset. Do not report that subset
as the entire catalog.

`json book` may omit IDs it could not find and print a count warning to stderr.
Compare returned IDs against the requested set. Availability and detail failures
appear in each record's `errors` array while other fields are returned. If an
availability error exists, do not treat `on_shelf_at_home: 0` as confirmed absence.

The common `availability.available` count is a catalog summary across branches.
Use `on_shelf_at_home` or inspect `copies` with `home` and `on_shelf` to describe
the selected branch. Digital formats and physical copies have different
availability semantics. Wishlist/prompt matches are title/author matches, so
confirm the edition and format before recommending a specific copy.

Availability is a snapshot, not a reservation. Public catalog availability is
cached for one minute, search/record responses for five minutes, and locations
and enriched details for 24 hours. A second command can read the same cached
snapshot. There is no CLI flag to bypass all caches. Hardcover library snapshots
are cached for five minutes; metadata and successful matches can remain cached
longer. Avoid repeatedly polling merely to obtain an identical response.

## Account access and action boundaries

Public search, record and branch lookup need no login. Account commands use the
owner's existing setup: catalog history/loans/holds require catalog access, Libby
tags require a linked Libby device, and Hardcover collections require a token.
Recommendations combine available reading sources, and wishlists can combine
Libby and Hardcover. See the [CLI reference](cli.md) for the source requirements.

JSON account reads can automatically log in again and save a renewed local
session. Queries can also populate local caches, including private Hardcover
snapshots. Treat credentials, session cookies, Libby identity, history, holds,
reviews and generated exports as private. Do not include them in logs or chat
unless the owner requests that disclosure.

Checkouts and holds can include Libby-only titles without a catalog ID. A Libby
`libby_id` is not a BiblioCommons bib ID; search the catalog before requesting
details for that title. Do not use the catalog `url` of an item with an empty
`id` as a valid record link.

The JSON interface has no commands for placing holds, borrowing, renewing,
returning, or editing Hardcover. Exporting CSV does not import it into Hardcover.
The interactive TUI does have Hardcover writes: toggling want-to-read can add or
remove a library entry, and its editor updates an existing record. Use those only
within the user's requested account action, and consult
[Hardcover editing](hardcover-editing.md) for partial-save and retry behavior.

Set up or repair account access deliberately. `login` saves a catalog session;
`libby link` links another device identity; `libby probe` writes raw account data.
They are not necessary for a public catalog query.
