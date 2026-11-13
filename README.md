# shelfcheck

Browse a BiblioCommons library catalog from your terminal, or query it as JSON
from scripts and agents. Shelfcheck includes a local terminal UI (TUI), an SSH
catalog browser, and optional library, Libby, and Hardcover account features.

Public search and availability need no account. Choose your BiblioCommons
catalog and home branch using the setup below; account features may vary by
library.

## Try it

Install Git and Go **1.26.8 or later**, as required by `go.mod`. Build for your
machine from source:

```sh
git clone https://github.com/seanthimons/shelfcheck.git
cd shelfcheck
go build -o shelfcheck .
```

Use an interactive terminal for the TUI. Building downloads Go dependencies;
catalog queries need internet access. On Windows, build with
`go build -o shelfcheck.exe .` and run `./shelfcheck.exe`.

## Choose your library

1. Open your library's website and follow its **Catalog** or **Search the
   catalog** link. Search for a book if needed to reach the catalog itself.
2. Look at the address bar. In `https://yourlibrary.bibliocommons.com/...`,
   the short name is `yourlibrary`: the part immediately before
   `.bibliocommons.com`. Use that exact value, without `https://`, the domain
   suffix, or a page path. `yourlibrary` here is a placeholder, not a working
   catalog. Use the catalog address rather than the library's main website;
   do not guess the short name from the library's name or initials.
3. Replace `YOUR_LIBRARY` below with that short name and list its branches:

   ```sh
   ./shelfcheck json branches -library YOUR_LIBRARY
   ```

   Find your preferred branch in the output and copy its `code`. Use that code
   for `home`, not the branch's display name. Codes vary between catalogs.
4. Create or edit `config.toml` in your Shelfcheck configuration directory:

| Platform | Configuration directory |
| --- | --- |
| Linux | `~/.config/shelfcheck` or `$XDG_CONFIG_HOME/shelfcheck` |
| macOS | `~/Library/Application Support/shelfcheck` |
| Windows | `%AppData%\shelfcheck` |

Replace both placeholders with the values you found, keeping the quotes even
when a branch code contains only digits. Add these top-level settings alongside
any existing settings:

```toml
library = "YOUR_LIBRARY"
home = "YOUR_BRANCH_CODE"
theme = "terminal"
covers = "auto"
```

Launch `./shelfcheck` to use your saved catalog and branch. Command flags
override saved settings, so you can try another selection without editing
your file:

```sh
./shelfcheck -library YOUR_LIBRARY -home YOUR_BRANCH_CODE
```

If you cannot find a `.bibliocommons.com` catalog address, confirm that your
library uses BiblioCommons before proceeding. Other catalog platforms are not
supported. See [configuration](docs/configuration.md) for all settings and file
locations.

## Start browsing

```sh
# Search immediately; the TUI initially limits results to items on the home shelf.
./shelfcheck alien clay

# Include all branches and availability states.
./shelfcheck -anywhere dune

# JSON search includes all branches unless -home-only is supplied.
./shelfcheck json search -brief -limit 5 dune
./shelfcheck json search -home-only -format book dune
```

Flags go **before** search text or other positional arguments. The TUI shows
contextual key hints; start with Enter to search, arrow keys to select a result,
Enter to open it, and Esc to go back. Ctrl+C exits. See the [TUI guide](docs/tui.md)
for filters, paging, account screens, and the full key reference.

## Filter what you see

Press **`f`** in search results, Wishlist, For you, Hardcover collections, or
New arrivals. Choose a category, toggle values with Space or Enter, then press
Esc from the values and categories to apply. **`F`** clears that screen's filters.

For an ebooks-only wishlist, open Wishlist with `w` or Alt+W, then select
**Format → eBook** in the filter panel. Enter on a result opens the matching
ebook edition, even when a print copy is also available.

Filters include format, genre, catalog rating, fiction/nonfiction, audience,
publication year, language, and author where metadata is present. Search also
supports branch/online availability, recently added, and sorting. `A` in
Wishlist or Hardcover collections toggles available-only versus all titles.

Discovery-list filters narrow loaded results within each screen's existing
scope; they do not expand the catalog scan. Selections stay separate per screen
and last for the current run. See the [TUI guide](docs/tui.md#filters) for details.

## Optional account features

Public catalog browsing needs no credentials. Set up whichever integrations you
want; they can be used independently.

### Credentials

Create `credentials.env` in the same configuration directory as `config.toml`.
On Linux:

```sh
mkdir -p ~/.config/shelfcheck
chmod 700 ~/.config/shelfcheck
touch ~/.config/shelfcheck/credentials.env
chmod 600 ~/.config/shelfcheck/credentials.env
```

Edit that file and add only the credentials you use:

```dotenv
SHELFCHECK_CARD=your-library-card-number
SHELFCHECK_PIN=your-library-pin
HARDCOVER_TOKEN=your-hardcover-api-token
```

Shelfcheck reads this file automatically. Environment variables override its
values. Keep credentials in `credentials.env` or the environment; authentication
tokens are not read from `config.toml`. Restart the TUI after changing account
setup.

### Library account

After adding your library card and PIN, run:

```sh
./shelfcheck login
```

Login uses your configured library. It enables loans, holds, and the borrowing
history retained by that library. The application does not place holds, renew,
or return library books.

### Hardcover

Get an API token from your Hardcover account and add it as `HARDCOVER_TOKEN`
in `credentials.env`. No separate Hardcover login command is needed.

Hardcover enables Want to Read in Wishlist, reading history for For you, your
Hardcover shelves, community collections, and record editing. Inspect matched
editions before editing; see the [Hardcover editor guide](docs/hardcover-editing.md)
for writes, partial failures, and recovery.

### Libby

In Libby, open **Menu → Settings → Copy To Another Device** to get an eight-digit
code, then run:

```sh
./shelfcheck libby link YOUR_8_DIGIT_CODE
```

Linking saves your Libby identity locally. It adds Libby loans, holds, reading
record, and the "want to read" tag used by Wishlist. Open Account with `m`,
Wishlist with `w`, and For you with `p`; use Alt+letter while typing in Search.

### Local environment setup

For local development, copy `.env.example` to `.env`, make it owner-readable
only, and replace the placeholders. The file is ignored by Git:

```sh
cp .env.example .env
chmod 600 .env
# edit .env, then load it for this shell
set -a && . ./.env && set +a
```

`HARDCOVER_USERNAME` is an optional operator label; Shelfcheck uses the token
for account identity. A checkout `.env` is not loaded automatically. For normal
use, prefer `credentials.env` as described above.

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
| [Book recommendation skill](skills/library-books/SKILL.md) | Agent skill that recommends books on the shelf at your home branch |
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
