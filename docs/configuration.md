# Configuration and account setup

[Back to README](../README.md) · [CLI reference](cli.md) · [TUI guide](tui.md)

## Catalog and home branch

`-library` is the BiblioCommons subdomain, without `.bibliocommons.com`.
`-home` is a branch code, not a branch name. Defaults are `cincinnatilibrary`
and `1` (Cincinnati's Main Library). Discover codes with:

```sh
./shelfcheck json branches -library cincinnatilibrary
```

Supply these flags on each invocation that needs another catalog or branch;
`config.toml` does not save them. The local TUI starts with home-shelf filtering;
`-anywhere` changes that starting state. JSON search starts across the catalog;
use `-home-only` to restrict it. Other JSON discovery commands have their own
availability rules; see the [CLI reference](cli.md).

## File locations

Shelfcheck uses Go's platform configuration and cache directories. On Linux,
these normally resolve to `~/.config/shelfcheck` and `~/.cache/shelfcheck`;
`XDG_CONFIG_HOME` and `XDG_CACHE_HOME` override those base directories. On macOS,
the bases are normally `~/Library/Application Support` and `~/Library/Caches`;
on Windows they follow `%AppData%` and `%LocalAppData%`.

| Location relative to its directory | Purpose |
| --- | --- |
| Config: `credentials.env` | Optional library card/PIN and Hardcover token |
| Config: `session.json` | Saved library login session |
| Config: `libby.json` | Linked Libby identity |
| Config: `config.toml` | Saved theme and optional cover mode |
| Config: `themes.toml` | Custom palettes |
| Config: `libby-probe/` | Raw account responses if you run `libby probe` |
| Cache: `cache.sqlite` (and SQLite sidecars) | Cached catalog and Hardcover responses, including private Hardcover snapshots |
| Cache: `hardcover/` | Cached Hardcover AlsoPicked recommendation data |

Keep credentials, sessions, Libby links, account exports, and raw probe files
private. The credentials reader rejects files with any group/other permissions.
Session and Libby files are written with mode `0600`; created private directories
use `0700`. Do not attach these files or account responses to issues.

## Library login

On Linux, create a private credentials file, then edit it with your editor:

```sh
mkdir -p ~/.config/shelfcheck
chmod 700 ~/.config/shelfcheck
touch ~/.config/shelfcheck/credentials.env
chmod 600 ~/.config/shelfcheck/credentials.env
```

Adapt the path for your platform or XDG setting. Put these lines in the file,
replacing the placeholders:

```dotenv
SHELFCHECK_CARD=your-library-card-number
SHELFCHECK_PIN=your-library-pin
```

This file supports simple `KEY=value` lines, optional `export ` prefixes, quoted
values, blank lines, and full-line `#` comments. It is not a shell script; avoid
inline comments or shell substitutions. Nonempty environment variables override
individual values in the file.

```sh
./shelfcheck login
./shelfcheck login -check
./shelfcheck json checkouts
./shelfcheck json holds
./shelfcheck json history
```

Use the same `-library` for login and subsequent commands when changing catalogs.
There is one default session file; `login` and JSON patron commands also expose
`-credentials` and `-session` overrides. The TUI uses the default files.
Account reads can log in automatically and save a refreshed session when the
session is missing or expired. History depends on the library account's history
being enabled and accessible.

## Libby

In Libby, go to **Menu → Settings → Copy To Another Device** to obtain an
eight-digit code, then run:

```sh
./shelfcheck libby link YOUR_8_DIGIT_CODE
./shelfcheck json libby
```

Linking saves `libby.json`. The code and saved identity are account credentials.
Libby tags provide borrowed, sampled, and wishlist information; results depend
on the linked account's tags and cards. `libby probe` is a diagnostic command
that writes raw private responses, not a required setup step.

## Hardcover

Get your API token from your Hardcover account, then add it to the same private
`credentials.env` file. For development from a checkout, you can instead copy
`.env.example` to the ignored local `.env`, restrict it to `0600`, and load it
into your shell (`set -a && . ./.env && set +a`).

`HARDCOVER_USERNAME` in `.env` is an optional human-readable operator label;
Shelfcheck uses the authenticated token's `me` result for the account identity.
Do not treat it as authorization or an API target selector.

The required token setting is:

```dotenv
HARDCOVER_TOKEN=your-hardcover-api-token
```

Alternatively, provide `HARDCOVER_TOKEN` through your process environment. The
environment takes precedence. Restart the TUI after changing account setup.
Hardcover matching uses title/author data; inspect the match before editing.
The editor changes only a book already present in your Hardcover library;
see [editing a record](hardcover-editing.md).

## Themes and covers

Theme selection is `-theme`, then `SHELFCHECK_THEME`, then the saved `theme` in
`config.toml`, then `terminal`. Built-ins are `terminal`, `umber`, `vellum`,
`orpiment`, and `cinnabar`. Press `t` in the TUI to cycle and save the choice.

Cover selection is `-covers`, then `SHELFCHECK_COVERS`, then `covers` in
`config.toml`, then `auto`. Modes are `auto`, `kitty`, `blocks`, and `off`.
`auto` selects Kitty graphics in detected Konsole, kitty, or Ghostty terminals
outside tmux/screen, and block rendering otherwise.

```toml
# config.toml
theme = "terminal"
covers = "blocks"
```

```sh
./shelfcheck -theme vellum -covers blocks dune
./shelfcheck -covers off dune
```

Custom themes live in `themes.toml`. A table name becomes the theme name;
using a built-in name replaces that palette. Fields are optional, and color
values must be `#rrggbb`. If supplied, `ansi` must contain exactly 16 colors
(normal black through white, then bright black through white). For example:

```toml
[night]
label = "Night"
bg = "#182028"
fg = "#eeeeee"
dim = "#a0a8b0"
active = "#80c0ff"
bar = "#304050"
barfg = "#ffffff"
bar_warn = "#ffcc66"
sigil = "#80c0ff"
border = "#607080"
```

See the bundled [palettes](../internal/theme/themes.toml) for full examples.
Invalid custom themes produce a warning and leave built-ins available; an unknown
explicit theme or cover mode is an error.

## Troubleshooting

| Symptom | What to check |
| --- | --- |
| No results for a known book | Start with `-anywhere` in the TUI or omit `-home-only` in JSON; check library and branch codes, formats, and filters |
| Credentials file rejected | Set file permissions to `0600`; check the platform config path and any `-credentials` override |
| Login/session failure | Run `login` again with the intended catalog; verify credentials and account access without posting them |
| Missing history or recommendations | Check enabled library history and linked source accounts; inspect JSON notes for source failures |
| Missing Hardcover features | Check token placement, credentials file permissions, and whether the book has a Hardcover match |
| Broken or missing jackets | Try `-covers blocks` or `-covers off`; image downloads and terminal support can fail independently of catalog search |
| Stale availability or metadata | Catalog availability can be cached for one minute, search for five minutes, details/branches for a day; Hardcover library snapshots for five minutes and matches for a week |

The cache is disposable, but contains account-derived data. To clear it manually,
stop running shelfcheck processes and remove only the shelfcheck cache directory
at your platform's cache path. Preserve the configuration directory if you want
to keep logins, Libby links, and preferences. Cache failures fall back to network
requests; shelfcheck is not an offline catalog mirror.
