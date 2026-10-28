# jstlogger

A terminal-based amateur radio logbook for recording QSOs (contacts), written
in Go with the [Charm](https://charm.land) TUI toolkit.

Created by Jason Stewart - K8JST

> **Status:** early development. The app has a full-screen layout (header,
> body, help footer), global keybindings and rig control through Hamlib's
> rigctld. QSO entry and logging are not implemented yet.

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

### Keybindings

| Key            | Action                  |
| -------------- | ----------------------- |
| `f`            | Set the rig frequency   |
| `m`            | Set the rig mode        |
| `?`            | Show or hide full help  |
| `q` / `ctrl+c` | Quit                    |

`f` and `m` are only available when rig control is on. In the prompt, press
`enter` to submit or `esc` to cancel.

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
changes and commands) to `$XDG_STATE_HOME/jstlogger/debug.log`
(`~/.local/state/jstlogger/debug.log` by default). Watch it with
`tail -f` in another terminal.

### Logbook

QSOs are stored in a SQLite database at `$XDG_DATA_HOME/jstlogger/logbook.db`
(`~/.local/share/jstlogger/logbook.db` by default). It is created on first
run and its schema is upgraded automatically at startup. Set
`[logbook] path` in the config to keep it somewhere else, and
`distance_unit = "mi"` to work in miles instead of kilometres.

## Development

```sh
just format    # gofmt -w . (run before committing)
just test      # go test ./... -short
just vendor    # go mod tidy && go mod vendor
just generate  # go generate ./... (regenerates mocks)
just --list    # list all recipes
```

### Project layout

```
main.go            entry point; parses flags, loads config and starts the app
internal/config/   config file loading, validation and session overrides
internal/logbook/  QSO storage: SQLite store, filtering, paging and tags
internal/rig/      rigctld client: polls the rig and sends it commands
internal/tui/      Bubble Tea models, keybindings and styles for the UI
migrations/        SQL schema migrations, embedded and applied at startup
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
