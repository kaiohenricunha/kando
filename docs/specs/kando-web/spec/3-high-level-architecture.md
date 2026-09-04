# §3 — High-Level Architecture

> System view: components, data stores, external dependencies, deployment.

## System Overview

`kando web [board]` starts a local HTTP server inside the same `cmd/kando`
binary that already runs the TUI. It reuses `internal/board` and
`internal/store` unchanged — the same domain model and persistence code the
TUI uses today — so there is one source of truth, not a parallel
reimplementation (this is how BOUND-1, §2, holds by construction).

On each request the server renders the current board as HTML straight from
the `*board.Board`/`*board.Archive` in memory. It also watches the board
directory with the store's existing `Store.Watch()`
(`internal/store/watch.go:11`) and pushes a "board changed" event over
Server-Sent Events (SSE) to any open browser tab whenever that fires —
whether the change came from the TUI, from hand-editing the Markdown files,
or from another browser tab. A tab that isn't open does nothing; when it's
next opened it simply reads current state (the "lazy refresh" the user
asked for). Tabs left open, or a tab and the TUI open together, both see
each other's edits within moments of the watcher firing (the "sync as soon
as possible" the user asked for).

## Data Stores

| Store | Role | Access Pattern |
| ----- | ---- | -------------- |
| `~/.kando/<board>/board.md`, `archive.md` (plain Markdown; unchanged format) | Board and archive persistence, same as the TUI today | Read on each HTTP request / SSE reconnect; written on mutation via the existing `Store.SaveBoard`/`SaveArchive`; watched via `Store.Watch()` for cross-process change detection — all three reused as-is, not reimplemented for the web path |

## External APIs / Dependencies

None. Everything is local — no external services, no third-party APIs — per
the "keep everything simple and local" decision in §2.

## Deployment

Single Go binary (`cmd/kando`), a new `web` subcommand alongside the
existing (implicit) TUI mode. Binds to loopback (`127.0.0.1`) only, never
`0.0.0.0`, since remote access is explicitly out of scope (§2). Port is
configurable (flag / env var, default TBD — settled in §6). Runs until
interrupted (Ctrl+C). No containerization, no build or deploy pipeline —
it's a local-only process, same as the TUI.
