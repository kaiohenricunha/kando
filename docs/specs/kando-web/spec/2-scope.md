# §2 — Scope

> What's in, what's out, and where are the boundaries?

## In Scope

- A local web page, served from localhost, that shows the full board at a
  glance: all four lanes, every card, its status and progress (not just the
  active lane the way the TUI defaults to).
- Edit an existing card: title, notes, tag, checklist, blocked reason, lane.
- Create a new card.
- Delete a card. **New capability — does not exist in the TUI today**
  (`internal/tui/board_update.go:49`'s `d` only moves a card to Done;
  `board.Remove` exists at the domain layer, `internal/board/board.go:133`,
  but nothing calls it to discard a card). Ships on the TUI first, then the
  web page, so the two never diverge (BOUND-1, below).
- List existing boards and switch between them, from both surfaces:
  the web page gets a boards page (§5); the TUI gets a new board-picker
  screen (key `B`), since BOUND-1 applies in both directions and the TUI
  today only opens one board via its CLI argument (`kando [board]`).
- Create a new board, from both surfaces.

## Out of Scope

Explicitly excluded for now — keep it simple and local:

- User accounts / authentication.
- Multi-device or remote access; anything that isn't localhost.
- A hosted or cloud deployment.

## Boundaries

**BOUND-1 — feature parity.** Any capability added to the web dashboard must
also exist in the TUI, and vice versa. The two are two views onto the same
board data and must never present different capabilities. This shapes §4
(shared domain/store package, not a reimplementation) and §6 (delete ships to
the TUI before or alongside the web page).

**BOUND-1a — accepted exception: archived cards are restore-only on the web.**
The TUI opens an archived card with `enter` and can edit or re-lane it from
there; §5 defines no archived-card route, so the web offers restore alone. The
gap is accepted, not overlooked: an archived card is finished work, and the
one action worth having on it is undo. Re-open the decision, rather than the
audit, if a `GET /b/{board}/archive/{id}` is ever wanted. U10's parity
checklist records this exception so the audit has something to check against.

| Touches | Does Not Touch |
| ------- | -------------- |
| `internal/board` (new delete/board-listing operations), `internal/store` (board discovery/creation; already keys boards by name at `Open(root, name)`, `internal/store/store.go:56`), a new local web server + frontend, `internal/tui/board_update.go` (wire up delete) | Authentication/authorization, remote or cloud infrastructure, mobile apps |

## Urgency

Not time-sensitive; no deadline given.
