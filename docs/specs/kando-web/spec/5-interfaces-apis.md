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
| POST | `/b/{board}/cards/{id}` | Update card fields (title, notes, tag) | `t`, `e` edits |
| POST | `/b/{board}/cards/{id}/move` | Move to a lane (`lane` param) | `H`/`L`, `d`, `m` picker |
| POST | `/b/{board}/cards/{id}/block` | Set or clear the blocked reason | `b` |
| POST | `/b/{board}/cards/{id}/checklist` | Add a checklist item (`text` param) | `o` |
| POST | `/b/{board}/cards/{id}/checklist/{index}/toggle` | Toggle one item done | `x` |
| POST | `/b/{board}/cards/{id}/checklist/{index}` | Edit one item's text | `enter` on item |
| POST | `/b/{board}/cards/{id}/delete` | Delete the card | new capability, §2 |
| GET | `/b/{board}/archive` | Archive view, grouped by week; optional `?q=` | `D` |
| POST | `/b/{board}/archive/{id}/restore` | Undo, back to Doing | `u` |
| GET | `/b/{board}/events` | Server-Sent Events stream: one `board-changed` event per `Store.Watch()` firing (KD-2, §4) | — |

Every `POST` route redirects back to a `GET` on success (the standard
post/redirect/get pattern), so a page refresh never resubmits a form.
`/b/{board}/events` is the only non-form route; a small vanilla-JS listener
(KD-1, §4) reacts to it by reloading the current page.

## Database Schema

N/A — no database. Persistence is the existing Markdown format, unchanged;
see §3 Data Stores and `internal/store/markdown.go`.
