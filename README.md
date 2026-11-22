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

- **Go 1.27.4** (pinned in `.tool-versions` for [asdf](https://asdf-vm.com)
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

jstlogger opens on the QSO index: every QSO in the logbook, newest first
unless you [sort](#sorting-the-index) it otherwise, filling the window.
Each row shows the date and time (UTC), call, mode (the submode when
there is one, so `FT4` rather than `MFSK`), the reports sent and
received, band, country and station callsign. A `*` marks a marked QSO.
Set `[index] format` in the config to choose other columns; see
[Index format](#index-format).

The status line above the help shows where you are and how many QSOs are
marked, e.g. `QSO 12/4,567 · 3 marked`, along with messages such as
"You are on the last QSO."

The index loads rows in blocks of 500 in the background as you move, so
it stays quick on large logbooks: jumping to the last QSO or to row N
reads only the rows around it. Marks are kept in memory and are lost on
quit; nothing acts on them yet.

`l` limits the index to the QSOs matching a [pattern](#patterns), and the
status line shows the limit, e.g. `QSO 2/57 · limit: ~b 20m`. The cursor
stays on the selected QSO if it matches. As in mutt, a pattern no QSO
matches leaves the index as it was. To show every QSO again, limit to
`~A` or submit an empty pattern. `ctrl+l` reloads under the limit, and
limiting never changes the marks.

#### Sorting the index

`o` sorts the index, and `O` sorts it in reverse, as in mutt. The prompt
lists the fields, each with the key of its [pattern](#patterns) term:
`d`ate, `c`all, `b`and, `m`ode, `C`ountry, `s`tate and c`o`ntinent;
`esc` cancels. Every field sorts ascending, and in reverse descending:
dates oldest first, so `o d` shows the oldest QSO first and `O d` goes
back to newest first. Bands sort by frequency (160m before 20m before
70cm), and a mode by the submode when there is one, so `FT4` sorts
between `FM` and `FT8`. QSOs with the same value are newest first, and
QSOs with no value (no state, say) come last either way. Unless the index
is in its default order, the status line shows the sort, e.g.
`QSO 2/57 · sort: reverse-band`. The sort applies within the limit, keeps
the cursor on its QSO and the marks, and lasts until you quit; `j` and
`k` in the pager follow it. Set `[index] sort` in the config to start
with another sort: `date`, `call`, `band`, `mode`, `country`, `state` or
`continent`, optionally prefixed with `reverse-`. The default is
`reverse-date`, newest first; an unknown value is reported as a warning
and ignored.

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
country, continent and tags. An empty field shows `—`, so the layout doesn't
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
shows which one is active; clicking a tab in the tab bar also opens it.
The tab stays the same as you move between QSOs.

`e` edits the QSO shown in the pager, as it does in the index.

As in NeoMutt's pager, `d` deletes the QSO shown, after the same y/N
question as the index, and `t` marks or unmarks it; both then move on to
the next QSO, so you can work through the log while reading it. On the
last QSO, `t` stays put and `d` shows the new last QSO; deleting the only
QSO closes the pager. The mark-prefix works too: `;d` deletes, `;s`
saves, `;y` tags and `;Y` untags the marked QSOs, with the same prompts
as in the index, and the pager shows the result. Marking by pattern
(`T`, `ctrl+t`), limiting (`l`), searching (`/`) and jumping (`1`–`9`)
stay in the index, since they change which QSOs are listed or where the
cursor is.

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
or `pgup` pages up, and `home` and `end` jump to the top and bottom. The
mouse wheel scrolls it three lines at a time.

To resize the pager without editing the config, `+` makes it one row
taller, showing one fewer index row above it, and `_` makes it one row
shorter, showing one more, as vim's `ctrl-w +` and `ctrl-w _` do. The
strip goes from no rows up to half the body. `=` goes back to
`[index] pager_index_lines`. The size lasts until you quit and isn't
written to the config.

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
| `=` / `Home`              | First QSO                                          |
| `*` / `End`               | Last QSO                                           |
| `1`–`9`                   | Jump to a QSO by its row number                    |
| `t`                       | Mark or unmark the QSO, then move to the next one  |
| `T`                       | Mark the QSOs matching a pattern                   |
| `ctrl+t`                  | Unmark the QSOs matching a pattern                 |
| `l`                       | Limit the index to the QSOs matching a pattern     |
| `o` / `O`                 | Sort the index / sort it in reverse                |
| `ctrl+l`                  | Reload the index and redraw the screen             |
| `enter`                   | [Show the QSO](#showing-a-qso) in the pager        |
| `e`                       | [Edit the QSO](#editing-a-qso)                     |
| `r`                       | [Set the rig](#rig-control)                        |
| `?` / `F1`                | [Show every key](#help)                            |
| `q` / `ctrl+c`            | Quit                                               |

Keys are case-sensitive. `ctrl+l` keeps the cursor on the same QSO if it
still exists, and redraws the whole screen, which fixes a display the
terminal has drawn wrong. `r` is only available when rig control is on. In
a prompt, press `enter` to submit or `esc` to cancel.

### Help

The footer shows the most used keys of the screen you're on. `?` opens
a keybinds window over the screen that lists every key in effect there,
in groups such as navigation, marks and rig. In the New QSO screen, its
pick lists and prompts, where `?` is typed as text, `F1` opens it
instead; `F1` works everywhere.

`/` starts a filter: typing narrows the list by key or description, so
`mark` finds the mark keys and `ctrl+t` finds what it does. `enter`
keeps the filter and `esc` clears it; `↑`/`↓` and `ctrl+p`/`ctrl+n`
scroll while you type. Otherwise `j`/`k`, `↑`/`↓`, `ctrl+p`/`ctrl+n` and
the mouse wheel scroll, and `PgUp`/`PgDn` move a page. `esc`, `enter`,
`?` or `F1` closes the window, returning to the screen as it was. The
window only lists keys: none of them runs from it, and no key reaches
the screen behind it, except `ctrl+c`, which quits.

### New QSO tabs

`a` opens the New QSO screen, whose fields are split across the QSO,
Station, General, Satellite, Notes and QSL tabs. The Satellite tab is
shown only while the propagation mode is `SAT`.

| Key                   | Action                                      |
| --------------------- | ------------------------------------------- |
| `alt+.` / `ctrl+PgDn` | Next tab, wrapping to the first             |
| `alt+,` / `ctrl+PgUp` | Previous tab, wrapping to the last          |
| `alt+1`–`alt+6`       | Go to a tab by its position in the tab bar  |

`alt+.` and `alt+,` sit on the `>` and `<` keys and work on keyboards
without PgUp and PgDn. They work in any field, including Notes, without
typing a character. On macOS, the `alt` keys need the terminal to send
Option as Meta (Esc+): in Terminal, tick *Use Option as Meta key*; in
iTerm2, set the Option key to *Esc+*.

Clicking a tab in the tab bar also opens it. A form taller than the
window scrolls to keep the field being typed in on screen; the mouse
wheel scrolls it three lines at a time, and the next key press scrolls
back to that field.

### Pickers

Mode, submode, propagation mode, antenna path and the QSL fields are
pickers: they hold one of the values ADIF defines. A picker shows its
ADIF code followed by what it means in grey, such as `‹ G › grayline` or
`TEP trans-equatorial`, so a tab reads at a glance. In a narrow window
the meaning is cut short before the code.

| Key               | Action                                         |
| ----------------- | ---------------------------------------------- |
| `←` / `→`         | Previous or next value, in ADIF's order        |
| typing            | Jump to the first value starting with the text |
| `enter` / `space` | Open a list of every value and what it means   |

A space typed after other text is part of the code, as in `VARA HF`,
rather than opening the list.

In the list, typing filters by code or meaning, so `meteor` finds `MS`
and `bureau` finds `B`. `↑`/`↓` move, `enter` picks, `esc` closes
without a change and `tab` moves on to the next field. The Mode list
starts with the most used modes (SSB, CW, FT8, FT4, FM, AM, RTTY, PSK31
and JS8); picking a submode there, such as FT4, also picks its mode,
MFSK.

### Dates and times

Dates and times are UTC. In post mode, the New QSO screen opens with the
date set to today; the time on is left blank, so a QSO is never saved at
a time nobody entered. After a `ctrl+s` save, the date and time on stay
for the next QSO, as when typing in a paper log, and the cursor moves to
Call.

Besides `YYYY-MM-DD` and `YYYYMMDD`, dates take short forms:

| Typed            | Date                                                      |
| ---------------- | --------------------------------------------------------- |
| `t`              | Today                                                     |
| `y`              | Yesterday                                                 |
| `-N`             | N days ago, e.g. `-3`                                     |
| `MMDD` / `MM-DD` | That day this year, or last year if it's later than today |

Besides `HH:MM`, `HH:MM:SS`, `HHMM` and `HHMMSS`, times take `n` for now
and an hour on its own, such as `14` for 14:00. On leaving a date or time
field, its value is rewritten in full (`1430` becomes `14:30`, `t`
today's date), so a mistake shows before the save.

Date off and time off copy the date and time on, shown in grey, until
you type in them; clearing one starts the copying again. When the time
off is earlier than the time on, the date off moves to the next day. A
copied value is saved as shown, and isn't an unsaved change. In live
mode the clock fills in all four fields as before, and an edited QSO
keeps its own date and time off.

### Country and zones

As the call is typed, the New QSO screen looks it up in the
[country file](#logbook) and fills in DXCC, continent, CQ zone and ITU
zone on the General tab. The entity also shows after the call on the
QSO tab, such as `Canada · NA · CQ 4 · ITU 4`, before its LoTW status.
A value typed by hand is never overwritten; changing the call updates
the fields that were filled in, and a call with no entity, such as an
unknown prefix or `/MM`, clears them.

The New QSO screen also fills in the name, QTH, grid, state, county and
email from the [callbook](#callbook-lookup), in the same way: typed values
are kept, and a changed call replaces what was filled in.

### Mouse

jstlogger turns on the terminal's mouse reporting while a logbook is
open, so the wheel and clicks work the same in every terminal.

In the index, the wheel moves the cursor three QSOs per step, loading
rows as it goes, and clicking a QSO opens it in the pager, as `enter`
does. In the pager, clicking a row in the index strip above it shows
that QSO, as `j` and `k` would. In the pager and the New QSO screen,
clicking a tab switches to it, and the wheel scrolls the tab. While the
[help window](#help) is open, the wheel scrolls it instead. Clicks do
nothing while a prompt or the help window is open or a prefix key, `;`
or `g`, is waiting for its second key.

With mouse reporting on, click-drag no longer selects text in most
terminals, the index included. Hold a modifier to select text anyway:
Option-drag in iTerm2, Shift-drag in many others, such as xterm, GNOME
Terminal, kitty, WezTerm and Alacritty.

### Editing a QSO

`e` in the index or the pager opens the selected QSO in the New QSO
form, in post mode, with every tab filled from the logbook. Saving
updates the QSO in place and keeps it selected; the distance and ITU
region are recalculated from the saved values. ADIF fields the form
doesn't show, and QSL details such as received status and dates, are
kept as they were. `esc` asks before discarding unsaved changes.

The QSO's station values are shown as stored, even when they differ
from the current profile; choosing a profile on the Station tab replaces
them. The rig never changes an edited QSO, and live mode is off while
editing. The worked-before panel leaves out the QSO being edited, and
never marks its call as new.

### Rig control

jstlogger talks to your radio through Hamlib's `rigctld`. It shows the
rig's frequency, mode, name and connection status in the header, and keeps
retrying if rigctld goes away. Pick a rig profile in the config file (run
`jstlogger --print-default-config` for a commented example), or choose one
with `--rig <profile>`. Use `--no-rig` to turn rig control off for a session.

A dot in front of the rig's name shows the connection: a green `●` when
connected, a grey `◌` while connecting and a red `○` when disconnected.
Unless the rig is connected, the frequency and mode are greyed out, as
they may be out of date. The rig window (`r`) shows why the rig is
disconnected.

The header names the rig by its profile, such as `ic7300`. Set `name` in
the profile to show something else:

```toml
[rig.profiles.ic7300]
name    = "IC-7300"
backend = "rigctld"
```

If the profile has a `spawn` table, jstlogger starts rigctld itself and
stops it on exit. Otherwise it connects to a rigctld that is already
running.

`r` opens the rig window, focused on the frequency. It starts with the
rig's current settings filled in and shows the connection
status; when the rig is disconnected, it also shows why. Press `tab` or
`shift+tab` to move between fields, `enter` to apply, or `esc` to close the
window without changing anything. Only the fields you changed are sent to
the rig. If the rig reports an error, the window stays open and shows it.

- **Frequency:** type it in MHz (`14.074` or `14.074.000`), kHz (`14074`)
  or Hz (`14074000`), or add a unit (`7074 khz`). `↑` and `↓` recall
  frequencies you set earlier in the session; the history isn't saved.
- **Mode:** type to filter the Hamlib modes, then pick one with `↑` and `↓`.
  Modes you used earlier in the session are listed first.
- **Passband:** optional, in Hz. Leave it blank to use the rig's normal
  passband. When you change the mode without touching the passband, the
  new mode gets its normal passband.

**Using WSJT-X at the same time:** only one program can own the radio's
serial port. Let rigctld own it, and point both jstlogger and WSJT-X at the
same rigctld. In WSJT-X, choose the "Hamlib NET rigctl" rig with the
rigctld address (for example `localhost:4532`).

### Debug log

Run `jstlogger --debug` to log rig events (state changes, connection
changes and commands) and the background LoTW, eQSL and DXCC syncs to `$XDG_STATE_HOME/jstlogger/debug.log`
(`~/.local/state/jstlogger/debug.log` by default). Watch it with
`tail -f` in another terminal.

### Logbook

QSOs are stored in a SQLite database at `$XDG_DATA_HOME/jstlogger/logbook.db`
(`~/.local/share/jstlogger/logbook.db` by default). It is created on first
run and its schema is upgraded automatically at startup. Set
`[logbook] path` in the config to keep it somewhere else, and
`distance_unit = "km"` to work in kilometers instead of miles.

The logbook also keeps a copy of AD1C's
[Big CTY](https://www.country-files.com/big-cty/) country file, which
maps callsigns to DXCC entities, continents and CQ and ITU zones. The
program has a copy built in for the first run; when the TUI starts and
the stored copy is more than a week old, it downloads the latest release
in the background. AD1C updates it every few weeks for new DXpeditions
and special-event calls. The copy is read once at startup, so a newly
downloaded one is used from the next launch. To refresh it now, run:

```sh
jstlogger --sync-dxcc   # prints e.g. "DXCC: 6331 prefixes, 21430 exception calls"
```

The download is conditional, so an unchanged file costs no download and
prints `DXCC: country file unchanged`.

### Callbook lookup

jstlogger can look up a callsign in an online callbook,
[QRZ.com](https://www.qrz.com/) or [HamQTH](https://www.hamqth.com/), to
find the operator's name, QTH, grid, state, county and email. Configure
the services in the `[callbook]` section of the config:

```toml
[callbook]
# Services to try, in order, until one finds the call. Empty disables lookups.
services = ["qrz", "hamqth"]
cache_ttl = "168h"     # how long to keep a found call; "0" keeps it for the session only

[callbook.qrz]
username = "W1AW"
password = "..."
full_name = false      # true adds the last name to the first name

[callbook.hamqth]
username = "w1aw"
password = "..."
```

Any QRZ.com account can log in, but without an XML data subscription QRZ
returns limited data, mainly the name and basic address, and allows only a
few lookups a day. HamQTH is free with an account. For a portable call
such as `VE3/W1AW` or `W1AW/P`, jstlogger looks up the base call and keeps
the name, but not the grid or location, which may not apply where the
station is operating.

On the New QSO screen, once you pause typing the call (or leave the field),
jstlogger looks it up in the background and fills in Name, QTH, Grid,
State, County (as `ST,County`, the ADIF form, when the callbook gives both)
and Email. Typing never waits for the lookup. A value you typed is never
overwritten; changing the call replaces the values that were filled in,
and a call the callbook doesn't list clears them. The lookup is canceled
when the call changes, the QSO is saved or the screen closes, and an edited
QSO is never looked up.

Next to the call, the screen shows where the details came from: `QRZ.com`,
`QRZ.com (limited)` for a QRZ.com account without an XML subscription,
`HamQTH` or `logbook`. When no callbook is configured, or the lookup fails,
the details come from your latest earlier QSO with the same base call: the
name and email always, and the QTH, grid, state and county only when that
QSO was with exactly the same call. A failure also shows why: `callbook
login failed`, `lookup limit reached`, `callbook unreachable` or `lookup
failed`. A failed lookup never stops you saving the QSO.

The passwords live in the config file, so keep it private with
`chmod 600`; jstlogger warns at startup when the file holds a password and
can be read by group or others. To keep the passwords out of the file, set
`JSTLOGGER_QRZ_PASSWORD` and `JSTLOGGER_HAMQTH_PASSWORD` instead; they
override the file. `--print-config` masks the passwords.

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
internal/callbook/ QRZ.com and HamQTH callbook lookups, with caching
internal/config/   config file loading, validation and session overrides
internal/dxcc/     DXCC entity lookup from callsign prefixes (AD1C Big CTY)
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

## License

jstlogger is released under the MIT License. See [LICENSE](LICENSE) for
the full text.

The DXCC country data is AD1C's
[Big CTY](https://www.country-files.com/big-cty/) file, distributed under
the terms in
[`internal/dxcc/cty-copyright.txt`](internal/dxcc/cty-copyright.txt).
