# §4 — Data Flow / Components

> Current state analysis + target architecture.

## Current State

`internal/board` is the pure domain (Card, Lane, Board, Archive, Filter,
Age, ID) with no I/O. `internal/store` parses and marshals the Markdown
format and persists it with atomic writes, hash-based self-write
suppression, and an fsnotify watcher (`internal/store/store.go`,
`internal/store/watch.go`). `internal/tui` is the only consumer today: it
loads one `*board.Board` at startup, mutates it via `board.Move`/`Insert`/
`Remove`, saves via `Store.SaveBoard`/`SaveArchive` after every mutation,
and reloads via `Store.CheckReload` when `Watch()` fires
(`internal/tui/model.go`). A board is selected by name at
`store.Open(root, name)` (`internal/store/store.go:56`) — the storage layer
already supports many boards, one per directory — but nothing anywhere
lists the boards under `KANDO_HOME` (confirmed by grep: no `ReadDir` /
`ListBoards` in product code).

## Component Boundaries

Reused as-is:
- `internal/board` — domain model.
- `internal/store` — persistence; gains board listing (`ListBoards`, U1).
  Board creation needs no new operation: `store.Open` already creates a
  canonical empty `board.md` on first open (§6.6).

New:
- An HTTP handler / template package (name TBD, e.g. `internal/web`) that
  turns the same `*board.Board` / `*board.Archive` into HTML — a second
  *renderer* over the same model, the same way `internal/tui` turns them
  into terminal frames. Not a second model.
- `cmd/kando/main.go` gains a `web` subcommand that wires `store.Open`, the
  new web package, and — like the TUI already does — `store.Watch()`.

## Shared State

The Markdown files on disk are the only state shared between the TUI
process and the web process; they are separate OS processes (even though
they're the same binary), so nothing is shared in memory *between them*.
Synchronization is entirely file-based, through the store package's
existing atomic-write + hash-suppression + watch machinery — unchanged
from what the TUI already does today.

**Within** the web process, that guarantee does not extend: `net/http`
runs every request in its own goroutine, and `internal/board` is
deliberately lock-free (it declares itself I/O- and UI-free,
`internal/board/board.go:1-3`; `Board`/`Card` carry no synchronization).
The TUI never needed locking because Bubble Tea serialises `Update` in one
goroutine — a web server removes that property, so KD-3 below settles it.

## Target Architecture

A local HTTP server, in the existing binary, rendering server-side HTML
from the same domain model the TUI already uses, kept in sync across
processes via the store's existing file watcher and pushed to the browser
over SSE.

### Key Decisions

**KD-1 — Server-rendered HTML, not a JSON API + single-page app.**
Recommended and adopted (the user asked for a recommendation). Reasoning:
(a) matches §2's "keep everything simple and local" — no JS build step, no
bundler, no frontend framework dependency; (b) matches the TUI's own
minimalism (its design spec calls for "nothing else for the UI" beyond
three Charm libraries); (c) a client-side JS model of the board is exactly
the kind of second source of truth BOUND-1 exists to prevent — with
server-rendered HTML there is one state (the Go `*board.Board`) and one
renderer per surface (TUI frames, HTML pages). Edits, creates and deletes
are plain HTTP requests (forms/links), one real request per action, the
same "every mutation saves" discipline the TUI already follows. A small
amount of vanilla JS is used only where a full page reload would be
jarring (subscribing to the SSE stream, drag interactions) — no SPA
framework, no client-side router, no build step.

**KD-2 — Server-Sent Events for cross-tab/cross-process live updates, not
WebSocket or polling.** The update direction is one-way (server → browser:
"the board changed, re-fetch"), which is exactly what SSE is for. It needs
no new dependency beyond the standard library (`net/http`'s
`http.Flusher`) and reuses the same *mechanism* the TUI already uses —
`Store.Watch()` (`internal/store/watch.go:13`) — not the same channel: a
call to `Watch()` builds a fresh fsnotify watcher and channel, scoped to
one board directory, and the channel is capacity-1 (`internal/store/watch.go:22`) and drops a
signal when full (`internal/store/watch.go:33-36`). That is correct coalescing for
one consumer that reloads after every signal — treat a signal as "something
changed, re-read," never as a countable event — but it means the web
server needs one watcher per open board (or re-registration on switch),
and `KANDO_HOME` itself is never watched, so a board created by another
process never updates the `/boards` page (§5) until it is next loaded.

**KD-3 — Each request loads, mutates, and saves; nothing survives between
requests.** No goroutine holds a `*board.Board` across requests, and no
mutex guards one. A handler that renders reads the board fresh (via
`Store.CheckReload`, which is stat-gated and returns early when nothing
changed, `internal/store/store.go:197`); a handler that mutates reads,
applies the change, and calls `Store.SaveBoard`/`SaveArchive`, all within
one request. This sidesteps the alternative — one shared in-memory model
behind a `sync.RWMutex` — at the cost of a parse per request, trivial at
personal-board sizes, and keeps `internal/board` exactly as lock-free as
it is today. It also closes the freshness gap KD-2 leaves: correctness
never depends on the watcher firing, which becomes a pure latency
optimisation for SSE, not a requirement for a page to be correct (PERF-2,
§7). `internal/web`'s handler tests run with `-race` to hold this
invariant (§6.4).
