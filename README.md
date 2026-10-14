# shelfcheck

Browse a BiblioCommons library catalog from your terminal, or query it as JSON
from scripts and agents. Shelfcheck includes a local terminal UI (TUI), an SSH
catalog browser, and optional library, Libby, and Hardcover account features.

The default catalog is Cincinnati & Hamilton County Public Library
(`cincinnatilibrary`), with Main Library (`1`) as the home branch. Public search
needs no account. Other BiblioCommons catalogs can be selected with `-library`;
compatibility and account features may vary by library.

## Try it

Install Git and Go **1.26.8 or later**, as required by `go.mod`. Build for your
machine from source:

```sh
git clone https://github.com/seanthimons/shelfcheck.git
cd shelfcheck
go build -o shelfcheck .
./shelfcheck
```

Use an interactive terminal for the TUI. Building downloads Go dependencies;
catalog queries need internet access. On Windows, build with
`go build -o shelfcheck.exe .` and run `./shelfcheck.exe`.

```sh
# Search immediately; the TUI initially limits results to items on the home shelf.
./shelfcheck alien clay

# Include all branches and availability states.
./shelfcheck -anywhere dune

# Find branch codes before selecting another home branch or catalog.
./shelfcheck json branches -library cincinnatilibrary
./shelfcheck -library cincinnatilibrary -home 1 -anywhere dune

# JSON search includes all branches unless -home-only is supplied.
./shelfcheck json search -brief -limit 5 dune
./shelfcheck json search -home-only -format book dune
```

Flags go **before** search text or other positional arguments. The TUI shows
contextual key hints; start with Enter to search, arrow keys to select a result,
Enter to open it, and Esc to go back. Ctrl+C exits. See the [TUI guide](docs/tui.md)
for filters, paging, account screens, and the full key reference.

## Optional account features

Configure a library card and PIN for loans, holds, and borrowing history. Link
Libby for its tags and reading record. Add a Hardcover API token for Hardcover
library information, wishlists, community lists, and editing existing records.
These integrations can be used independently; some combined views report notes
when a source is unavailable.

### Local environment setup

For local development, copy `.env.example` to `.env`, make it owner-readable
only, and replace the placeholders. The file is ignored by Git:

```sh
cp .env.example .env
chmod 600 .env
# edit .env, then load it for this shell
set -a && . ./.env && set +a
```

`HARDCOVER_TOKEN` is used by Shelfcheck. `HARDCOVER_USERNAME` is an optional
operator label only; Shelfcheck derives the account identity from the token and
does not trust a configured username for API operations. For normal installed
use, you may instead keep the same variables in the private platform config
file described in the [configuration guide](docs/configuration.md).

Follow the [configuration guide](docs/configuration.md) to store credentials
privately and choose themes or book jackets. The [Hardcover editor guide](docs/hardcover-editing.md)
explains saves, clearing fields, and recovery after a failed update.

Shelfcheck can produce a CSV for import into Hardcover with
`./shelfcheck export hardcover -o reading.csv`. You review and import the file
yourself. This command does not upload books to Hardcover.

## Documentation

| Guide | What it covers |
| --- | --- |
| [CLI reference](docs/cli.md) | Commands, flags, JSON output, CSV export, and examples |
| [TUI guide](docs/tui.md) | Search, filters, book details, account screens, and keyboard controls |
| [Configuration](docs/configuration.md) | Account setup, file locations, themes, covers, and troubleshooting |
| [Agent usage](docs/agents.md) | JSON workflows, identifiers, partial results, and account action boundaries |
| [SSH deployment](docs/deployment.md) | Running the catalog browser remotely and the systemd installer |
| [AGENTS.md](AGENTS.md) | Repository map and development instructions for coding agents |

## Development

```sh
go test ./...
go vet ./...
go build -o shelfcheck .
```

The root `shelfcheck` binary is ignored by Git; rebuild it after changing code.
Tests use local fixtures and fake services. Live integration checks require your
own accounts and should use a designated test record for any writes. The project
integrates with external catalog and account APIs, so upstream changes can affect
results and availability. Check current availability before making a trip.
