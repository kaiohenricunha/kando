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
U12 adds the write path in the other direction — a Done card moving *into*
the archive, on all three surfaces — which is a new capability, not a
narrowing of this exception: an already-archived card is still restore-only.

**BOUND-1b — accepted exception: a chosen drop position is web-only.** The
board page lets a card be dragged to an exact place in a lane — in front of a
named card, or last — and the TUI has no equivalent: `H`/`L`, `d` and `u`
still land a card on top of its new lane, and nothing reorders a lane from
the terminal. The gap is accepted, not overlooked: a pointer makes "put it
*there*" a one-gesture operation, and the keyboard equivalent is a second
grammar (a grab key plus movement keys) that the TUI has not needed. Both
surfaces share one rule for what a placement does — `Board.MoveAt` in
`internal/board/board.go`, the same helper the route calls — so re-opening
this is binding two TUI keys (`J`/`K` to reorder inside a lane) to a helper
that already exists, not a second implementation. §5's `/move` row and the
parity checklist record the exception so the audit has something to check
against.

The exception is narrower than "drag-and-drop is web-only": the *capability*
— put a card in a lane — is on both surfaces, and only the *precision* is
not. It is narrower still in practice, because HTML5 drag fires from neither
touch nor the keyboard: a phone or keyboard user of the web page is in
exactly the TUI's position, re-laning through the `<select>` and unable to
reorder. That is acceptable only because the card stays an ordinary link and
that `<select>` stays untouched — which the "no position asked for means the
move it always meant" rule guarantees.

| Touches | Does Not Touch |
| ------- | -------------- |
| `internal/board` (new delete/board-listing operations), `internal/store` (board discovery/creation; already keys boards by name at `Open(root, name)`, `internal/store/store.go:56`), a new local web server + frontend, `internal/tui/board_update.go` (wire up delete) | Authentication/authorization, remote or cloud infrastructure, mobile apps |

## Urgency

Not time-sensitive; no deadline given.
