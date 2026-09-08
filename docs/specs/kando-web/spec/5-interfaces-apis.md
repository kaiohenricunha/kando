# §5 — Interfaces and APIs

> External APIs, internal endpoints, database schemas.

## External APIs

None — see §3.

## Internal APIs

All routes are server-rendered HTML (KD-1, §4); every mutation is a plain
HTML form `POST`, no JSON body, no client-side routing. `{board}` and
`{id}` are the existing board name and card id. The board view's `?q=`
query param is handed straight to `board.Parse` (`internal/board/filter.go:37`),
the same parser the TUI's `/` filter already uses — the operators, and
their meaning, are identical by construction.

| Method | Path | Purpose | Mirrors (TUI) |
| ------ | ---- | ------- | -------------- |
| GET | `/` | Redirect to `/b/{board}` if one was given on the CLI (`kando web [board]`), else `/boards` | `kando [board]` arg |
| GET | `/boards` | List all boards; form to create one | new capability, §2 |
| POST | `/boards` | Create a board (name) → redirect to `/b/{board}` | new capability, §2 |
| GET | `/b/{board}` | Board view: all four lanes, every card, status/progress; optional `?q=` filter | board screen |
| GET | `/b/{board}/cards/new?lane={lane}` | New-card form | `a` quick add |
| POST | `/b/{board}/cards` | Create a card (lane, title) | `a` quick add |
| GET | `/b/{board}/cards/{id}` | Card detail: notes, tag, checklist, blocked, meta | `enter` detail screen |
| POST | `/b/{board}/cards/{id}` | Update card fields (title, notes, tag) | `T`, `t`, `e` edits |
| POST | `/b/{board}/cards/{id}/move` | Move to a lane (`lane` param), optionally to a position in it: `pos=before` or `pos=after` with `anchor=<card id>`, or `pos=start` for the top of the lane (any lane accepts it; it is what a lane rendering no cards resolves to). No `pos` is the lane picker's move unchanged: the top of a different lane, and a no-op when the lane is the one the card is already in. A position is named against a card the user can see, never an index — the page filters, so the last card on screen is not the last in the lane — and an `anchor` that has left the destination lane is a 409; an unknown `pos`, or one with no `anchor`, is a 400 | `H`/`L`, `d`, `m` picker; `J`/`K` for the position, one slot per press (BOUND-1b, §2) |
| POST | `/b/{board}/cards/{id}/block` | Set or clear the blocked reason | `b` |
| POST | `/b/{board}/cards/{id}/checklist` | Add a checklist item (`text` param) | `o` |
| POST | `/b/{board}/cards/{id}/checklist/{index}/toggle` | Toggle one item done | `x` |
| POST | `/b/{board}/cards/{id}/checklist/{index}` | Edit one item's text | `enter` on item |
| POST | `/b/{board}/cards/{id}/delete` | Delete the card | new capability, §2 |
| POST | `/b/{board}/cards/{id}/archive` | Move a Done card into archive.md (409 if not in Done or already archived, or if the board changed underneath) → redirect to the board | `A` |
| GET | `/b/{board}/archive` | Archive view, grouped by week; optional `?q=` | `D` |
| POST | `/b/{board}/archive/{id}/restore` | Undo, back to Doing | `u` |
| GET | `/b/{board}/events` | Server-Sent Events stream: one `board-changed` event per `Store.Watch()` firing (KD-2, §4); each frame carries the board's version as its event id, so a reconnecting client replaying `Last-Event-ID` is told at once if it missed a change | — |
| GET | `/static/live.js` | The SSE listener, served from this origin so the Content-Security-Policy can stay at `script-src 'self'` | — (plumbing) |
| GET | `/static/dnd.js` | The board page's drag-and-drop, served from this origin for the same reason. Loaded on every page from the shared template footer, like `live.js`, and equally a no-op off the board (no `.lane` elements to bind). A drop builds and posts a form at the `/move` route above, so there is no second way to write a card | — (plumbing) |

Every `POST` route redirects back to a `GET` on success (the standard
post/redirect/get pattern), so a page refresh never resubmits a form. A route
redirects to where the action came from: `/move` returns to the card when no
position was asked for (the detail page's lane picker), and to the board,
`?q=` intact, when one was (a drag on the board page) — the same rule the
archive's restore already follows for its filter.
`/b/{board}/events`, `/static/live.js` and `/static/dnd.js` are the three
non-form routes. The listener (KD-1, §4) is a small vanilla-JS file that
reacts to the stream by reloading the current page; the drag script is the
other case KD-1 allows, and it ends in a form `POST` to a route in this table
rather than a write of its own. Both are served rather than inlined so the
Content-Security-Policy need not allow inline script, and each is pinned to
its own path so the embed is never widened to a pattern.

## Database Schema

N/A — no database. Persistence is the existing Markdown format, unchanged;
see §3 Data Stores and `internal/store/markdown.go`.
