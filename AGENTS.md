# Working on shelfcheck

Shelfcheck is a Go terminal catalog browser with a JSON CLI, a Bubble Tea TUI,
and an SSH server that runs the TUI. Start with [README.md](README.md) for setup,
[docs/cli.md](docs/cli.md) for commands, and [docs/tui.md](docs/tui.md) for the
interactive interface. [docs/agents.md](docs/agents.md) describes using the
application from scripts and AI agents.

## Build and verification

Use the Go version declared in `go.mod` (currently Go 1.26.8). From the repository
root:

```sh
go test ./...
go vet ./...
go build -o shelfcheck .
```

The root `shelfcheck` executable is a local build artifact ignored by Git. Rebuild
it after changes when requested; do not add it to a commit. Format changed Go
files with `gofmt`. Add focused tests for behavior changes, using the existing
fixtures and local HTTP servers rather than requiring user credentials.

Normal tests use fixtures, temporary directories, and fake HTTP/GraphQL servers.
The public catalog smoke test is opt-in:

```sh
SHELFCHECK_LIVE=1 go test ./internal/bibliocommons -run '^TestLive$' -v
```

That command contacts the real catalog. It is not required for ordinary local
verification. Hardcover write tests use a fake server; a successful test suite
does not establish that a live Hardcover account accepts every update. See
[docs/hardcover-editing.md](docs/hardcover-editing.md) for the manual checks.

## Source map

| Area | Files / packages | Responsibility |
| --- | --- | --- |
| Entry points | `main.go`, `json.go`, `login.go`, `libbycmd.go`, `export.go`, `serve.go` | Dispatch, flag parsing, output, local TUI and SSH setup |
| Catalog | `internal/bibliocommons` | Undocumented BiblioCommons gateway API, queries, records, availability, account sessions, fixtures |
| Account integration | `internal/account` | Private file locations, credentials, session renewal, combined reading record, Libby loan/hold merging, wishlist matching |
| Libby | `internal/libby` | Device linking, sync, tags and tag pagination |
| Hardcover | `internal/hardcover` | GraphQL metadata, library, prompts/lists, shelf mutations and verified record edits |
| Recommendations | `internal/recommend` | Reading profile, catalog candidates, series, community picks and scoring |
| TUI | `internal/tui` | Bubble Tea model/update/view, account and discovery screens, Hardcover editor, cover rendering |
| Themes | `internal/theme` | Embedded built-in TOML themes, user themes, persisted choice |
| Cache | `internal/cache` | Shared SQLite response cache, request coalescing and invalidation |
| Deployment | `deploy` | Example SSH service and installer |

Keep upstream API details in their client packages, combined account behavior in
`internal/account`, and presentation in the CLI/TUI. The CLI and TUI share clients
and account logic; changes to those layers can affect both interfaces.

## Boundaries to preserve

- BiblioCommons catalog/account access is read-only apart from authentication.
  There are no commands for placing holds, renewing loans, or returning books.
- JSON commands query services and emit JSON; they do not edit Hardcover shelves
  or records. TUI Hardcover actions can add/remove a library entry and edit an
  existing record. The export command writes CSV for a separate manual import.
- Preserve JSON stdout for data. Put progress, warnings and errors on stderr.
  Flags precede positional arguments because the CLI uses Go's `flag` package.
- Do not silently turn partial results into complete results. Search can have
  `partial: true`; book details can have per-record `errors`, and missing IDs are
  reported on stderr without necessarily causing a failed exit.
- Hardcover updates can partially succeed. Preserve draft retention, changed-field
  patches, response verification and cache invalidation; do not claim an error
  means no upstream field changed.
- Preserve context cancellation, client throttling and cache scope separation.
  The SQLite cache holds private Hardcover snapshots as well as public catalog
  data, with scopes keyed by an endpoint/token fingerprint.

## Private data and live operations

The platform config directory's `shelfcheck` folder contains `credentials.env`,
`session.json` and `libby.json`; the cache directory can contain private snapshots.
Do not print, commit, or use existing private files as test fixtures. Use temporary
config/cache directories for account tests and demonstrations. Credential files
must have permissions restricted to the owner (`0600`).

Read the user's task for authorization before live account operations. Routine
development does not require running `login`, `libby link`, `libby probe`, or
writing to Hardcover. `libby probe` saves raw private account responses and is a
diagnostic command, not a routine health check. Starting `serve` exposes a network
service; review its listen address and authorized-key configuration first. SSH
sessions use no account store and cannot access personal account screens.

When behavior or flags change, update the corresponding user documentation,
`docs/agents.md` if machine output changes, and built-in usage/help where needed.
