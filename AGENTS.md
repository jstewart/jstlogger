# AGENTS.md

Guidance for AI coding agents working on this repository.

## Common commands

Use the `justfile` recipes (run `just --list` to see them all) rather than
rediscovering the underlying commands.

- `just format` — `gofmt -w .`. Run before every commit.
- `just build` — build the binary (optimizations disabled for debugging).
- `just run` — build, then launch the app.
- `just test` — `go test ./... -short`.

## Environment Setup & Tooling

Before making changes, verify that your local environment aligns with these system-level dependencies:

- **Go Toolchain:** Go 1.27.3, as pinned in `go.mod` and `.tool-versions`.

## Architectural Patterns & Directory Structure

jstlogger is a terminal (Bubble Tea) app, not a server. Do not place core business logic inside the UI layer.

- `main.go` (repo root): Application entry point. Only handles flag parsing, configuration loading, and dependency injection.
- `/internal/tui`: Bubble Tea models, keybindings, and Lip Gloss styles. Feature screens are composed into the root `Model`'s body.
- New non-UI code (domain types such as QSOs, storage, ADIF import/export) belongs in its own package under `/internal`, decoupled from `tui`.

## Idiomatic Go Code & Formatting Guidelines

Write idiomatic, modern Go code matching community standards ("Effective Go" and current practices):

- **Error Handling:** Never swallow errors. Always wrap errors with context using `fmt.Errorf("context: %w", err)`.
- **Modern Syntactic Patterns:**
  - Use `slices.Contains` instead of writing custom iteration loops.
  - Use the built-in `min` and `max` functions instead of custom math helper blocks.
  - Use `sync.WaitGroup` idiomatic chaining wherever appropriate.
  - Use `errors.As` or `errors.Is` for type-safe error checking and asserts.
- **Context Propagation:** The first argument of every network, database, or asynchronous function must be `ctx context.Context`. Never pass a `nil` context; use `context.TODO()` or `context.Background()` only at the application boundary.
- **Allocation and Pointers:** Avoid unnecessary pointer allocations (`*string`, `*int`) unless explicitly handling null fields from database schemas.

## Testing & Quality Assurance

- **Mocking:** Implement interface-driven testing. Generate mocks using `mockgen` and store them alongside the packages under test.
- **Concurrency Tests:** Any test verifying channels, goroutines, or sync primitives must run with the `-race` flag enabled.
- **Assertions:** Use `github.com/stretchr/testify/assert` and `github.com/stretchr/testify/require` for consistent, readable assertions. Prefer `require` for fatal initialization failures.

## Operational Boundaries & Constraints

- **No Database Schema Mutations:** Do not write code that directly executes `ALTER TABLE` or structurally alters SQL schemas. All schema mutations must be handled strictly inside localized migrations scripts (`/migrations`).
- **Dependency Audits:** Do not arbitrarily introduce third-party libraries via `go get` without explicit human authorization. Favor the standard library whenever possible.
- **Secrets Management:** Never hardcode secrets, private keys, API configurations, or access tokens into files or test cases. Utilize environment variables parsed exclusively at startup.

## Prefer gopls MCP tools for Go symbol questions

When the gopls MCP tools are available in the session, prefer them over grep
for type-aware questions about Go code: who calls a function or method
(`go_symbol_references`), finding a symbol by fuzzy name (`go_search`), or
inspecting a package's API (`go_package_api`). Method names in this codebase
collide a lot (`draw`, `Show`, `Refresh` exist on several types), and grep
needs manual filtering that gopls doesn't. This includes code under
`vendor/`, which gopls resolves as part of the module build.

Grep remains the right tool for strings, comments, config keys, non-Go
files, and anything textual. Don't adopt the full workflow from
`gopls mcp -instructions` (vulncheck on session start, `go_file_context`
after every file read); that overhead isn't worth it here.

If the tools aren't available in a session, fall back to grep silently —
don't try to install, register, or start the server.

## Git/Commits
Work on feature branches for large pieces of work, the author will merge into main when ready. Make sure commits are granular Use conventional commits. Add claude attribution to each commit Ensure that commits are atomic and encompass a logical unit of work
