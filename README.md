# jstlogger

A terminal-based amateur radio logbook for recording QSOs (contacts), written
in Go with the [Charm](https://charm.land) TUI toolkit.

Created by Jason Stewart - K8JST

> **Status:** early development. The app has a full-screen layout (header,
> body, help footer), global keybindings and rig control through Hamlib's
> rigctld. QSOs can be imported and exported as ADIF from the command
> line. QSO entry and logging in the app are not implemented yet.

## Planned Features

Some of the features that I envision for jstlogger. These are the ones that are immediately useful to me. Feel free to open a pull request with your ideas.

### Offline Features

- A [mutt-like](http://www.mutt.org/) interface for logging QSOs
- Offline mode will store qsos in an outbox until online, then sync with your logging services

### Online Features

- Receive UDP messages from other apps (WSJTX, GridTracker, etc)
- Forward logged QSOs to Wavelog

## Requirements

- **Go 1.27.3** (pinned in `.tool-versions` for [asdf](https://asdf-vm.com)
  / [mise](https://mise.jdx.dev) users; `go.mod` requires the same version)
- **[just](https://github.com/casey/just)** (optional) to run the project
  recipes
- A terminal with ANSI color support

## Building and running

With `just`:

```sh
just build     # build ./jstlogger (with optimizations disabled, for debugging)
just run       # build, then launch the app
just install   # install jstlogger into $GOBIN / $GOPATH/bin
```

With plain Go:

```sh
go build -o jstlogger .
./jstlogger
```

Or run it without building a binary:

```sh
go run .
```

The app opens full-screen in the terminal's alternate screen.

### QSO index

jstlogger opens on the QSO index: every QSO in the logbook, newest first,
filling the window. Each row shows the date and time (UTC), call, mode
(the submode when there is one, so `FT4` rather than `MFSK`), the reports
sent and received, band, country and station callsign. A `*` marks a
marked QSO. Set `[index] format` in the config to choose other columns;
see [Index format](#index-format).

The status line above the help shows where you are and how many QSOs are
marked, e.g. `QSO 12/4,567 · 3 marked`, along with messages such as
"You are on the last QSO."

The index loads rows in blocks of 500 in the background as you move, so
it stays quick on large logbooks: jumping to the oldest QSO or to row N
reads only the rows around it. Marks are kept in memory and are lost on
quit; nothing acts on them yet.

`l` limits the index to the QSOs matching a [pattern](#patterns), and the
status line shows the limit, e.g. `QSO 2/57 · limit: ~b 20m`. The cursor
stays on the selected QSO if it matches. As in mutt, a pattern no QSO
matches leaves the index as it was. To show every QSO again, limit to
`~A` or submit an empty pattern. `ctrl+l` reloads under the limit, and
limiting never changes the marks.

`T` marks, and `ctrl+t` unmarks, the QSOs matching a pattern. Like mutt,
they act only on the QSOs within the limit, so `ctrl+t ~A` unmarks just
the QSOs you can see.

### Showing a QSO

`enter` opens the QSO under the cursor in a pager, in the style of
NeoMutt's split pager. Ten index rows stay above it with the QSO
highlighted; set `[index] pager_index_lines` to keep more or fewer, or
`0` for none. In a window too short for them, the strip shrinks so the
top of the QSO tab stays visible. Below them, the QSO tab lists the date
and time (UTC), call, LoTW status, eQSL AG status, band, frequency, mode and submode, the reports sent and received, grid,
distance (in `[logbook] distance_unit`), state, county, name, comment,
country and continent. An empty field shows `—`, so the layout doesn't
shift between QSOs.

Next to the call, the badges `[Q] [H] [E]` link to the station's page on
QRZ, HamQTH and eQSL. In a terminal that supports OSC 8 hyperlinks
(iTerm2, kitty, WezTerm, Ghostty, VTE…), click or Cmd-click a badge to
open it. From the keyboard, `gq`, `gh` and `ge` open the same pages with
`open` on macOS or `xdg-open` on Linux; `esc` after `g` cancels. The
links use the base callsign, so `VE3/W1AW/P` looks up `W1AW`. The badges
are left out when the pager is too narrow for them.

An `[L]` badge follows them when the station uploads to
[LoTW](https://lotw.arrl.org), colored by how recently it last did:
green within 30 days, amber within a year, dim for longer ago. It is not
a link. The LoTW row under the call gives the detail, such as `last
upload 3 days ago`, `—` when the station is not in the feed, or
`lookup failed` (with the error on the status line). Like the other
badges, it uses the base callsign.

The LoTW data comes from the ARRL's
[user activity feed](https://lotw.arrl.org/lotw-user-activity.csv),
stored in the logbook. When the TUI starts and the stored copy is more
than a week old (or there is none yet), it is refreshed in the
background; the badge appears once the first download finishes. A
failed refresh shows `LoTW sync failed: …` on the status line and keeps
the stored copy. To refresh it now, run:

```sh
jstlogger --sync-lotw   # prints e.g. "LoTW: 237123 users (2 rows skipped)"
```

The download is conditional, so an unchanged feed costs no download and
prints `LoTW: feed unchanged`.

The `[E]` badge also shows whether the station is an eQSL Authenticity
Guaranteed (AG) member: bright and bold for a member, dim for a station
that is not one, and its usual color while that is not yet known or the
lookup failed. It stays a link either way. The eQSL AG row, after the
LoTW row, says `yes` for a member, `—` otherwise, or `lookup failed`
(with the error on the status line). It uses the base callsign too.

The AG data comes from eQSL's
[AG member list](https://www.eqsl.cc/qslcard/DownloadedFiles/AGMemberList.txt),
stored in the logbook and refreshed in the background like the LoTW feed
when it is more than a week old. A failed refresh shows `eQSL sync
failed: …` on the status line and keeps the stored copy. To refresh it
now, run:

```sh
jstlogger --sync-eqsl   # prints e.g. "eQSL: 199457 AG members (850 rows skipped)"
```

An unchanged list prints `eQSL: AG list unchanged`.

`j`/`↓` and `k`/`↑` move to the next or previous QSO in index order
without leaving the pager, and the index rows follow. `q` or `esc` closes
the pager and returns to the index with the cursor where it was. While
the pager is open, `q` closes it instead of quitting; `ctrl+c` still quits.

`tab` and `shift+tab` cycle through the pager's tabs, and the tab bar
shows which one is active. The tab stays the same as you move between
QSOs.

- **QSO** — the details above.
- **Station** — the station callsign, operator, station grid, TX power
  and station name, followed by any `MY_*` ADIF fields such as `MY_RIG`
  or `MY_POTA_REF`. The station name is the one stored on the QSO in its
  `APP_JSTLOGGER_STATION` ADIF field, which records the station it was
  logged from. A QSO without one shows the `[stations.*]` entry whose
  callsign and grid match the QSO's station callsign and grid, ignoring
  case, when exactly one does; otherwise it is blank.
- **QSL** — one row per confirmation (card, LoTW, eQSL…) with its sent
  and received status and dates.
- **ADIF** — every field of the QSO as ADIF name/value pairs, sorted by
  name, which is handy for checking an import.

A tab taller than the pager scrolls: `space` or `pgdown` pages down, `-`
or `pgup` pages up, and `home` and `end` jump to the top and bottom.

### Index format

`[index] format` lays out each row of the QSO index, in the style of
mutt's `index_format`. The default reproduces the columns above:

```toml
[index]
format = "%Z %-10{%Y-%m-%d} %-5{%H:%M} %-10c %-6m %-5s %-5r %-5b %-*C %-10S"
```

Text in the format is shown as is, and `%%` shows a `%`. Each `%` expando
is a column, written `%[-][width]X`:

- `width` is the column width in cells, up to 255. Longer values are cut
  short with `…`. Without a width, a column is as wide as its value, so it
  doesn't line up with the header or other rows.
- `-` aligns the value left; without it, the value is aligned right.
- `*` in place of a width makes the column take the width that remains.
  At most one column can.

| Expando     | Column                                                   |
|-------------|----------------------------------------------------------|
| `%Z`        | `*` when the QSO is marked                               |
| `%{format}` | start date and time (UTC), in strftime `format`          |
| `%c`        | call                                                     |
| `%L`        | `L` when the station uses LoTW, colored by last upload   |
| `%m`        | mode, or the submode when there is one                   |
| `%s`, `%r`  | report sent, received                                    |
| `%b`        | band                                                     |
| `%f`        | frequency in MHz, e.g. `14.074`                          |
| `%C`        | country                                                  |
| `%S`        | station callsign                                         |
| `%o`        | operator                                                 |
| `%n`        | name                                                     |
| `%g`        | grid square                                              |
| `%D`        | distance, in whole `[logbook] distance_unit`s            |
| `%k`        | continent                                                |
| `%x`        | DXCC entity number                                       |

A date and time format can use `%Y` `%y` `%m` `%d` `%e` `%j` `%H` `%M`
`%S` `%b` `%B` `%a` `%A` and `%%`. The header row shows each column's
name in the same layout. A format needs at least one expando; an unknown
expando or conversion is reported as a config error at startup.

`%L` is one cell wide unless given a width, and is not in the default
format. Its `L` is colored like the pager's `[L]` badge (green, amber or
dim by how recently the station last uploaded), except on the cursor's
row. It is blank when the station is not in the LoTW feed, while it is
being looked up, when the lookup fails (with the error on the status line)
and when the logbook has no LoTW data.

### Patterns

Patterns follow mutt's. A term is `~` followed by a case-sensitive letter
and, for most letters, a value:

| Term       | Matches                                                       |
| ---------- | ------------------------------------------------------------- |
| `~A`       | Every QSO                                                     |
| `~c CALL`  | Call contains `CALL`                                          |
| `~b BAND`  | Band is `BAND`, e.g. `~b 20m`                                 |
| `~m MODE`  | Mode or submode is `MODE`, so `~m FT4` matches MFSK/FT4       |
| `~y TAG`   | Carries the tag `TAG`                                         |
| `~C NAME`  | Country is `NAME`, ignoring case, e.g. `~C "United States"`   |
| `~s STATE` | State or province is `STATE`, e.g. `~s MA`                    |
| `~o CONT`  | Continent is `CONT`: `AF`, `AN`, `AS`, `EU`, `NA`, `OC`, `SA` |
| `~d RANGE` | Started within `RANGE` (see below)                            |

`~s` matches the ADIF state code of any country, so `~s WA` also matches
Western Australia; add `~C "United States"` to narrow it.

`RANGE` is in UTC dates, `YYYY-MM-DD`: `D` is that day, `D-` is from `D`
on, `-D` is through `D`, and `D1-D2` is `D1` through `D2`.

Terms side by side must all match; `|` matches either side and `!` negates
the term after it. `!` binds tightest, then side by side, then `|`, and
parentheses group:

```text
~c W1 (~b 20m | ~b 40m) !~m FT8
```

A value ends at a space, `(`, `)` or `|`; quote it with double or single
quotes to include those, as in `~y "field day"`, where a backslash escapes
the next character. The space between a term and its value is optional.

As in mutt's simple search, input with none of `~ ! | ( ) " '` is a bare
value matched against the default fields, as if typed as
`~c V | ~b V | ~m V | ~C V | ~s V | ~o V`. So `W1AW` finds the call, `20m`
the band and `United States` the country. To match a value containing any
of those characters, use its term, as in `~C "Cote d'Ivoire"`.

### Keybindings

The index keys follow NeoMutt's generic menu.

| Key                       | Action                                             |
| ------------------------- | -------------------------------------------------- |
| `j` / `↓`                 | Next QSO                                           |
| `k` / `↑`                 | Previous QSO                                       |
| `z` / `PgDn` / `→`        | Next page                                          |
| `Z` / `PgUp` / `←`        | Previous page                                      |
| `]` / `[`                 | Half a page down / up                              |
| `>` / `<`                 | Scroll the view one line down / up                 |
| `H` / `M` / `L`           | Top / middle / bottom of the page                  |
| `=` / `Home`              | First (newest) QSO                                 |
| `*` / `End`               | Last (oldest) QSO                                  |
| `1`–`9`                   | Jump to a QSO by its row number                    |
| `t`                       | Mark or unmark the QSO, then move to the next one  |
| `T`                       | Mark the QSOs matching a pattern                   |
| `ctrl+t`                  | Unmark the QSOs matching a pattern                 |
| `l`                       | Limit the index to the QSOs matching a pattern     |
| `ctrl+l`                  | Reload the index and redraw the screen             |
| `enter`                   | [Show the QSO](#showing-a-qso) in the pager        |
| `f`                       | Set the rig frequency                              |
| `m`                       | Set the rig mode                                   |
| `?`                       | Show or hide full help                             |
| `q` / `ctrl+c`            | Quit                                               |

Keys are case-sensitive: `M` is the middle of the page and `m` sets the
mode. `ctrl+l` keeps the cursor on the same QSO if it still exists, and
redraws the whole screen, which fixes a display the terminal has drawn
wrong. `f` and `m` are only available when rig control is on. In a prompt,
press `enter` to submit or `esc` to cancel.

### Rig control

jstlogger talks to your radio through Hamlib's `rigctld`. It shows the
rig's frequency, mode and connection status in the header, and keeps
retrying if rigctld goes away. Pick a rig profile in the config file (run
`jstlogger --print-default-config` for a commented example), or choose one
with `--rig <profile>`. Use `--no-rig` to turn rig control off for a session.

If the profile has a `spawn` table, jstlogger starts rigctld itself and
stops it on exit. Otherwise it connects to a rigctld that is already
running.

- **Frequency:** type it in MHz (`14.074`), kHz (`14074`) or Hz
  (`14074000`), or add a unit (`7074 khz`).
- **Mode:** type a Hamlib mode name with an optional passband in Hz, such as
  `USB` or `PKTUSB 3000`. Leave the passband out to use the rig's default.

**Using WSJT-X at the same time:** only one program can own the radio's
serial port. Let rigctld own it, and point both jstlogger and WSJT-X at the
same rigctld. In WSJT-X, choose the "Hamlib NET rigctl" rig with the
rigctld address (for example `localhost:4532`).

### Debug log

Run `jstlogger --debug` to log rig events (state changes, connection
changes and commands) and the background LoTW and eQSL syncs to `$XDG_STATE_HOME/jstlogger/debug.log`
(`~/.local/state/jstlogger/debug.log` by default). Watch it with
`tail -f` in another terminal.

### Logbook

QSOs are stored in a SQLite database at `$XDG_DATA_HOME/jstlogger/logbook.db`
(`~/.local/share/jstlogger/logbook.db` by default). It is created on first
run and its schema is upgraded automatically at startup. Set
`[logbook] path` in the config to keep it somewhere else, and
`distance_unit = "km"` to work in kilometers instead of miles.

### ADIF import and export

jstlogger reads and writes ADIF `.adi` files from the command line, so you
can bring in a log from another program or hand yours to one. Both commands
use the logbook from your config, run without the TUI and exit when done.

```sh
jstlogger --import-adif log.adi                      # import every QSO in log.adi
jstlogger --import-adif log.adi --tag pota,fd-2026   # ...and tag the new ones
jstlogger --export-adif out.adi                      # export the whole log, oldest first
jstlogger --export-adif out.adi --tag pota           # only QSOs tagged pota
jstlogger --export-adif - > out.adi                  # write ADIF to stdout
```

A summary goes to stderr, such as `jstlogger: imported 298 QSOs, skipped 2
duplicates` or `jstlogger: exported 300 QSOs to out.adi`. With `-`, stdout
carries only the ADIF.

**Tags.** Give `--tag` a comma-separated list or repeat it
(`--tag pota --tag fd-2026`). On import, the tags are applied only to newly
imported QSOs. On export, a QSO is written only if it carries **all** of
the tags, so `--tag pota,fd-2026` exports QSOs tagged both `pota` and
`fd-2026`. `--tag` on its own is an error.

**Duplicates.** A record is a duplicate when its `APP_JSTLOGGER_UUID`
matches a QSO already in the logbook, or when its call, start time (to the
second), band, mode and station callsign match a QSO in the logbook or an
earlier record in the same file. Duplicates are skipped and counted in the
summary. They are **not** updated, and **not** tagged, so importing the same
file twice is harmless, but it won't add tags to QSOs you already have.

**Safety.** An import is all or nothing: if any record is malformed, the
error names the record and field and nothing is imported. An export is
written to a temporary file next to the target and renamed into place, so
a failed export never leaves a partial file. An existing file is
overwritten.

**Known differences after a round trip.** Importing a file and exporting it
again gives the same QSOs with the same fields and values, apart from these
normalizations:

- 4-digit times (`HHMM`) are written as `HHMMSS`.
- Trailing zeros in `FREQ`, `TX_PWR` and `DISTANCE`, and leading zeros in
  numeric fields, are dropped.
- Frequencies finer than 1 Hz are rounded to the nearest Hz.
- Text values are trimmed, and callsigns, grids, mode and continent are
  upper-cased.
- The header is replaced with jstlogger's own.
- `QSO_DATE_OFF` is written whenever there is an end time.
- A `QSO_DATE_OFF` without a `TIME_OFF` is ignored.
- Header `USERDEF` definitions are not kept. Their fields still come
  through.
- `BAND` is added when it can be worked out from `FREQ`.
- `DISTANCE` is added, in km to 0.1 km, when a record has none but has a
  `GRIDSQUARE`; it is measured from `MY_GRIDSQUARE`, or from your station's
  grid when that is missing. A `DISTANCE` in the file is always kept.
- `APP_JSTLOGGER_UUID` is added to every record.

## Development

```sh
just format    # gofmt -w . (run before committing)
just test      # go test ./... -short
just vendor    # go mod tidy && go mod vendor
just generate  # go generate ./... (regenerates mocks)
just lint      # golangci-lint run
just vuln      # govulncheck ./... (known vulnerabilities in code we call)
just --list    # list all recipes
```

`just lint` and `just vuln` need golangci-lint and govulncheck. Run
`just tools` once to install the versions CI uses into `$GOBIN`. They are
installed with `go install`, not added to `go.mod`. CI runs both checks on
every pull request and push to main, and fails when either finds a problem.
The linters are configured in `.golangci.yml`; a `//nolint` must name the
linter and say why.

To check that one of your own ADIF files survives an import and export,
point the round-trip test at it:

```sh
JSTLOGGER_ROUNDTRIP_ADIF=/path/to/log.adi go test -run TestRoundTripExternalFile .
```

### Project layout

```
main.go            entry point; parses flags, loads config and starts the app
internal/adif/     ADIF .adi file reading, writing and comparison
internal/config/   config file loading, validation and session overrides
internal/eqsl/     eQSL AG member list download, storage and lookup
internal/feed/     conditional download and sync metadata shared by the lists
internal/indexfmt/ QSO index format parsing and column values
internal/logbook/  QSO storage: SQLite store, filtering, paging and tags
internal/lotw/     LoTW user activity feed download, storage and recency
internal/rig/      rigctld client: polls the rig and sends it commands
internal/tui/      Bubble Tea models, keybindings and styles for the UI
migrations/        SQL schema migrations, embedded and applied at startup
testdata/          synthetic ADIF files for the import/export tests
```

## Dependencies

Direct dependencies (see `go.mod`):

| Module                                                   | Purpose                                                        |
| -------------------------------------------------------- | -------------------------------------------------------------- |
| [`charm.land/bubbletea/v2`](https://charm.land/bubbletea) | TUI framework (Elm-style model / update / view)                |
| [`charm.land/bubbles/v2`](https://charm.land/bubbles)     | Ready-made TUI components (keybindings, help footer, text input) |
| [`charm.land/lipgloss/v2`](https://charm.land/lipgloss)   | Terminal styling and layout                                    |
| [`modernc.org/sqlite`](https://gitlab.com/cznic/sqlite)  | Pure-Go SQLite driver for the logbook (no cgo)                 |
| [`github.com/stretchr/testify`](https://github.com/stretchr/testify) | Test assertions (`assert`, `require`); tests only   |
| [`go.uber.org/mock`](https://github.com/uber-go/mock)     | `mockgen` and the gomock runtime for generated mocks; tests only |

Everything else in `go.mod` is an indirect dependency pulled in by these.

## Contributing

Pull requests are welcome. See [Planned Features](#planned-features) for
ideas.

### AI-assisted pull requests

You're welcome to use AI tools to write code for jstlogger, but every pull
request must have a human behind it. Pull requests with no sign of human
involvement will be rejected.

- **A person must review it first.** Before you open a pull request, read
  and understand every change in it. You're responsible for the code, not
  the tool that wrote it.
- **Summarize it in plain language.** The description should explain what
  changed and why, the way you'd explain it to another ham. Leave out
  needless technical jargon.
- **Let the code speak for itself.** Don't list files, functions or line
  numbers in the description, or paste in code. The diff already shows
  them.
- **Be there for the review.** Answer questions and make changes yourself.
  Pull requests that are opened and left alone, or that get only automated
  replies, will be closed.
