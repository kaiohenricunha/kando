# Parity audit — TUI keys against web routes (U10)

BOUND-1 (§2) says the two surfaces must never present different
capabilities. This is the audit: every TUI key on the left, the route that
does the same thing on the right. Verified against `internal/tui/help.go`,
`internal/tui/board_update.go`, `internal/tui/detail.go`,
`internal/tui/archive.go`, `internal/tui/boards.go`, `internal/web/server.go`
and `internal/web/archive.go` at U9/U10/U12. The CLI (`kando <verb>`) is a
third surface with the same no-divergence rule; `README.md`'s "What each
surface can do" table is that audit at capability granularity.

## Board screen

| TUI | Web | Notes |
| --- | --- | --- |
| `j/k` `h/l` `tab` `shift+tab`, arrows | — | Selection and lane focus. The page shows all four lanes at once, so there is nothing to select. Layout, not a capability. |
| `a` quick add | `GET /b/{board}/cards/new` → `POST /b/{board}/cards` | |
| `enter` open card | `GET /b/{board}/cards/{id}` | |
| `H`/`L` move card ±lane | `POST /b/{board}/cards/{id}/move` | The web picks the lane from a select; the TUI steps one lane at a time. Same `board.Move`. |
| `J`/`K` reorder in lane | drag a card onto a lane | **BOUND-1b (§2) is closed for the TUI.** Dragging also picks the *position* in the lane; `J`/`K` step the card one slot at a time to reach the same placements. Same `board.MoveAt`, by way of `MoveBefore`/`MoveAfter`, and both name the position against a card the user can see rather than an index, so a filtered lane behaves the same on both. The CLI still has no position argument, which is what remains of the exception. |
| `d` move to Done | `POST …/move` (`lane=done`) | |
| `u` undo (Done → Doing) | `POST …/move` (`lane=doing`) | |
| `x` delete card | `POST /b/{board}/cards/{id}/delete` | Immediate on both, no confirmation (§2). |
| `A` archive (Done only) | `POST /b/{board}/cards/{id}/archive` | Both go through `board.ArchiveDone` and `store.SaveArchival`/`SaveArchivalIfUnchanged`; the button only renders on a Done card, the key only fires in Done. `DoneAt` is kept — it is the week bucket archive.md files the card under. Both refuse an id that is already archived: a 409 on the web, `"title" is already archived` in the TUI footer. |
| `/` filter | `?q=` on the board and the archive | Same `board.Parse`, so the operators are identical by construction. |
| `D` archive view | `GET /b/{board}/archive` | |
| `B` boards | `GET /boards` | |
| `?` help | — | Bound on every TUI screen (board, detail, archive, boards), and each screen lists its own keys. The web page labels its own controls, so there is no hidden key to explain. |
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
| `d` move to Done | `POST …/move` (`lane=done`) | The card page's `✓ done` button, which `d` on that page also submits (`edit.js`). Both follow the card into Done, as `m` then `4` does in the TUI; the button does not render on a card already in Done. |
| `A` archive (Done only) | `POST /b/{board}/cards/{id}/archive` | The same route and the same two guards as the board screen's `A` (`detail.go`'s `archiveDetailCard`); it returns to the board only when the archive happened, since only then is the card's own page gone — which is what the button does too. A refusal (already archived, not in Done, or an unreadable archive) stays on the card with the reason in the footer. |
| `J/K` next/previous card | the lane list's links on the card page | `edit.js` binds `J`/`K` to the same links, wrapping. Navigation, not a capability. |
| `esc` back | the `← lane` link and the breadcrumb | `edit.js` binds `esc` to the back link. |
| `j/k` checklist cursor | — | The page has no checklist cursor, so there is nothing to move. |

## Archive

| TUI | Web | Notes |
| --- | --- | --- |
| `u` restore | `POST /b/{board}/archive/{id}/restore` | Both go through `board.Restore`, so the dates and the write order match; the TUI saves with the unchecked `store.SaveRestore`, the web with the checked `SaveRestoreIfUnchanged`, since each web request opens its own Store. Both return to the archive with the filter intact. Both refuse a card whose id is already on the board — a 409 on the web, the CLI's `"title" is already on the board` in the TUI footer — and `m` on an open archived card is the same restore, so it takes the same guard. |
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
| fsnotify watcher → reload | `GET /b/{board}/events` (SSE) → `location.reload()` | One `store.WatchBoard` watcher per watched board, fanned out to every open tab (KD-2). Both treat a signal as "re-read", never as a countable event. Board pages only: `$KANDO_HOME` is not watched, so `/boards` updates on its next load (KD-2, §4). The page defers a reload while a field is focused or a drag is in flight, and flushes it on blur or `dragend`. |

## Web-only

No capability is web-only. **Where in a lane a card lands** was the last one,
and `J`/`K` closed it — see BOUND-1b (§2) and the board row above. What differs
now is only the gesture: a drag names a position in one motion, `J`/`K` step
toward it one slot at a time. Both go through `board.MoveAt`, by way of
`MoveBefore`/`MoveAfter`, so the rules for what a placement does are shared
rather than reimplemented, and every other route exists to serve something
the TUI already does.

The CLI is the remaining gap: `kando move` takes a lane and no position. It is
tracked in `README.md`'s capability table, which audits all three surfaces.

Four routes have no TUI counterpart because they are plumbing rather than
capabilities: `GET /static/live.js` serves the listener that turns an SSE
event into a reload; `GET /static/dnd.js` serves the drag handler, which ends
in a form `POST` to `/move` rather than a write of its own; `GET
/static/lanes.js` keeps the phone layout's lane tabs in step with the lane in
view; and `GET /static/edit.js` submits the card page's existing forms when a
field is left or a detail-screen key is pressed. None of them writes anything
of its own. All four are in §5 for completeness.

## Where the surfaces deliberately differ

- **Layout.** Lipgloss frames against HTML, four lanes at once against one
  active lane, a help overlay against labelled controls. KD-1 expects this.
- **Selection.** The TUI has a cursor; the page does not need one.
- **Concurrency.** Two browser tabs can post at once, so the web serialises
  writers per board and refuses a save whose file changed underneath at all
  (409). The TUI is one process with one goroutine and needs no serialising,
  and its unchecked writers still win over a concurrent change that parses,
  on purpose; they refuse only a changed file that does not parse, wrapping
  `ErrUnparsable` instead of overwriting a hand edit that broke it.
- **How unsafe runes are removed on screen (SEC-4).** All three surfaces drop
  the same set, `board.UnsafeRune`, but the TUI substitutes a space for a C0
  rune where the other two delete it. The TUI lays out in exact terminal
  cells, and a C0 rune measures zero while some terminals still draw it, so
  the space is what keeps the lane grid from shifting. The bidi controls need
  no substitute — they genuinely occupy no cells — so all three delete those.
  The Zl/Zp line separators need no divergence at all: all three surfaces drop
  them, and the conversion to a newline that preserves an author's line break
  happens once on the write path, in `SetNotes`.
