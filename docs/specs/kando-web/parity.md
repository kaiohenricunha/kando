# Parity audit — TUI keys against web routes (U10)

BOUND-1 (§2) says the two surfaces must never present different
capabilities. This is the audit: every TUI key on the left, the route that
does the same thing on the right. Verified against `internal/tui/help.go`,
`internal/tui/board_update.go`, `internal/tui/detail.go`,
`internal/tui/archive.go`, `internal/tui/boards.go` and
`internal/web/server.go` at U9/U10.

## Board screen

| TUI | Web | Notes |
| --- | --- | --- |
| `j/k` `h/l` `tab` `shift+tab`, arrows | — | Selection and lane focus. The page shows all four lanes at once, so there is nothing to select. Layout, not a capability. |
| `a` quick add | `GET /b/{board}/cards/new` → `POST /b/{board}/cards` | |
| `enter` open card | `GET /b/{board}/cards/{id}` | |
| `H`/`L` move card ±lane | `POST /b/{board}/cards/{id}/move` | The web picks the lane from a select; the TUI steps one lane at a time. Same `board.Move`. |
| `d` move to Done | `POST …/move` (`lane=done`) | |
| `u` undo (Done → Doing) | `POST …/move` (`lane=doing`) | |
| `x` delete card | `POST /b/{board}/cards/{id}/delete` | Immediate on both, no confirmation (§2). |
| `/` filter | `?q=` on the board and the archive | Same `board.Parse`, so the operators are identical by construction. |
| `D` archive view | `GET /b/{board}/archive` | |
| `B` boards | `GET /boards` | |
| `?` help | — | Bound on every TUI screen (board, detail, archive, boards). The web page labels its own controls, so there is no hidden key to explain. |
| `q` quit | — | Closing a tab is not a server action. |

## Card detail

| TUI | Web | Notes |
| --- | --- | --- |
| `T` title | `POST /b/{board}/cards/{id}` (`title`) | The TUI gained `T` in U6 to close this gap. |
| `t` tag | same route (`tag`) | |
| `e` notes | same route (`notes`) | |
| `b` block reason (empty clears) | `POST …/block` | |
| `o` new checklist item | `POST …/checklist` | The TUI inserts at the checklist cursor; the page has no cursor, so a new item is appended. Same `Card.InsertChecklistItem`. |
| `x` toggle item | `POST …/checklist/{i}/toggle` | The web form carries the item text it was rendered with, so a stale index is a 409 rather than an edit of the wrong item. |
| `enter` edit item | `POST …/checklist/{i}` | |
| `m` move (lane picker) | `POST …/move` | |
| `j/k`, `J/K`, `esc` | — | Navigation between items and cards. |

## Archive

| TUI | Web | Notes |
| --- | --- | --- |
| `u` restore | `POST /b/{board}/archive/{id}/restore` | Both go through `board.Restore` and `store.SaveRestore`, so the dates and the write order match. Both return to the archive with the filter intact. |
| `/` filter | `?q=` | |
| `enter` open an archived card | — | **BOUND-1a (§2): accepted exception.** An archived card is finished work; undo is the action worth having on it. Reopening this means changing the decision and §5, not the audit. |

## Boards

| TUI | Web | Notes |
| --- | --- | --- |
| `enter` switch board | `GET /b/{board}` | |
| `n` new board | `POST /boards` | Same `store.Open` and the same `ValidBoardName` gate. |

## Live updates

| TUI | Web | Notes |
| --- | --- | --- |
| fsnotify watcher → reload | `GET /b/{board}/events` (SSE) → `location.reload()` | One `store.WatchBoard` watcher per watched board, fanned out to every open tab (KD-2). Both treat a signal as "re-read", never as a countable event. Board pages only: `$KANDO_HOME` is not watched, so `/boards` updates on its next load (KD-2, §4). The page defers a reload while a field is focused and flushes it on blur. |

## Web-only

No capability the TUI lacks. Every route above exists to serve something the
TUI already does, and the two write paths share `internal/board` and
`internal/store` rather than reimplementing the rules.

One route has no TUI counterpart because it is plumbing rather than a
capability: `GET /static/live.js` serves the listener that turns an SSE
event into a reload. It is in §5 for completeness.

## Where the surfaces deliberately differ

- **Layout.** Lipgloss frames against HTML, four lanes at once against one
  active lane, a help overlay against labelled controls. KD-1 expects this.
- **Selection.** The TUI has a cursor; the page does not need one.
- **Concurrency.** Two browser tabs can post at once, so the web serialises
  writers per board and refuses a save whose file changed underneath (409).
  The TUI is one process with one goroutine and needs neither.
