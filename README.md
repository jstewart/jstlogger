# jstlogger

A terminal-based amateur radio logbook for recording QSOs (contacts), written
in Go with the [Charm](https://charm.land) TUI toolkit.

Created by Jason Stewart - K8JST

> **Status:** early development. The app currently has a full-screen layout
> (header, body, help footer) and global keybindings. QSO entry and logging
> are not implemented yet.

## Planned Features

Some of the features that I envision for jstlogger. These are the ones that are immediately useful to me. Feel free to open a pull request with your ideas.

### Offline Features

- A [mutt-like](http://www.mutt.org/) interface for logging QSOs
- hamlib, rigctl integration
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
| `?`            | Show or hide full help  |
| `q` / `ctrl+c` | Quit                    |

## Development

```sh
just format    # gofmt -w . (run before committing)
just test      # go test ./... -short
just vendor    # go mod tidy && go mod vendor
just --list    # list all recipes
```

### Project layout

```
main.go            entry point; starts the Bubble Tea program
internal/tui/      Bubble Tea models, keybindings and styles for the UI
```

## Dependencies

Direct dependencies (see `go.mod`):

| Module                                                   | Purpose                                                        |
| -------------------------------------------------------- | -------------------------------------------------------------- |
| [`charm.land/bubbletea/v2`](https://charm.land/bubbletea) | TUI framework (Elm-style model / update / view)                |
| [`charm.land/bubbles/v2`](https://charm.land/bubbles)     | Ready-made TUI components (keybindings, help footer)           |
| [`charm.land/lipgloss/v2`](https://charm.land/lipgloss)   | Terminal styling and layout                                    |
| [`github.com/stretchr/testify`](https://github.com/stretchr/testify) | Test assertions (`assert`, `require`); tests only   |

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
