# §6 — Implementation Plan

> Phases, workstreams, prompts, tests, migrations, rollback.

## 6.1 Phased Rollout

1. **Shared foundations** (hard prerequisite for everything else).
   `internal/store` gains `ListBoards`. `internal/board` gains (or the TUI's
   existing inline logic is lifted into) a single set of mutation helpers —
   tag set, notes set, blocked set/clear, checklist add/toggle/edit, and
   card delete (a thin wrapper over the existing `Board.Remove`,
   `internal/board/board.go:133`) — so the TUI and the web page call the
   exact same functions for a given edit, not just visually agree (BOUND-1
   by construction, the same reasoning as KD-1/§4).
2. **TUI: delete (`x`) and the board picker (`B`)** — can build in parallel
   with 3, since they touch disjoint files. Delete and the picker are
   exposed through the help overlay only, not the persistent footer (see
   R-1, §8) so the three spec-provided golden frames
   (`board_120x40.txt`, `board_120x40_done.txt`, `board_80x24.txt`) stay
   byte-identical.
3. **Web: server skeleton + read-only board view** (`kando web [board]`,
   `GET /`, `/boards`, `/b/{board}`) — the thinnest slice that proves the
   read path and board switching before any mutation exists.
4. **Web: card mutations** (create/edit/move/block/checklist/delete) — needs
   3's server and 1's shared helpers.
5. **Web: archive view + restore** — parallel with 4, different files.
6. **Web: live sync (SSE)** — can start as soon as 3's server exists;
   independent of which mutation routes are done yet.
7. **Parity audit + docs** — a manual pass over every TUI key and every §5
   route confirming BOUND-1 holds, plus the README.
8. **Web: drag-and-drop placement** (U11) — needs 4's `/move` route and 6's
   listener, which it has to defer while a drag is in flight. Splits three
   ways: `board.MoveAt` (pure domain, testable alone), the `pos`/`anchor`
   fields and the redirect rule on `/move` (fully testable with `httptest`,
   and where the risk is), then the asset and template wiring, which only a
   browser can check. Ships the first web-only capability, so it lands with
   BOUND-1b (§2) and the parity row, not after them.

## 6.2 Workstream Breakdown

Phase 1 is the one hard gate — it freezes the mutation-helper contract both
the TUI (phase 2) and the web page (phase 4) build on. After phase 1, phases
2 and 3 run in parallel (different packages: `internal/tui/*` vs.
`internal/web/*`; `cmd/kando/main.go` is touched by both but only
additively — a new `x`/`B` key branch and a new `web` subcommand). Phase 4
needs phases 1 and 3. Phase 5 runs alongside phase 4. Phase 6 needs only
phase 3's server to exist, not phases 4/5's specific routes.

## 6.3 Prompt Sequence

**U1 — Store: list boards**
- Read first: `internal/store/store.go`, `internal/store/store_test.go`
- Command: `/think` — small, well-scoped
- Tests first: `TestListBoardsEmpty`, `TestListBoardsFindsExisting`,
  `TestListBoardsIgnoresNonBoardDirs`
- Files: `internal/store/store.go`, `internal/store/store_test.go`

**U2 — Domain: shared mutation helpers**
- Read first: `internal/board/board.go`, `internal/board/board_test.go`,
  `internal/tui/detail.go` (`commitEdit`), `internal/tui/board_update.go`
- Command: `/plan` — touches two packages; must not change existing TUI
  behavior or goldens
- Tests first (package `board`): `TestCardSetTagStripsHash`,
  `TestCardSetBlockedEmptyClears`, `TestChecklistAddAtCursor`,
  `TestChecklistToggle`, `TestChecklistEditText`, `TestBoardDeleteCard`
- Files: `internal/board/ops.go`, `internal/board/ops_test.go`;
  `internal/tui/detail.go` and `internal/tui/board_update.go` updated to
  call the shared helpers — existing TUI tests must stay green unchanged

**U3 — TUI: delete key (`x`)**
- Read first: `internal/tui/board_update.go`, `internal/tui/help.go`,
  `internal/tui/board_update_test.go`, the three golden files
- Command: `/plan`
- Tests first: `TestDeleteKeyRemovesSelectedCard`,
  `TestDeleteKeyClampsSelectionAfterDelete`, `TestDeleteKeyOnEmptyLaneNoop`,
  `TestHelpOverlayListsDeleteAndBoards`; regenerate `help_120x40.txt` via
  `go test ./internal/tui/ -run TestGoldenScreens -update`; add an explicit
  assertion that the three spec-provided goldens are still byte-identical
- Files: `internal/tui/board_update.go`, `internal/tui/help.go`

**U4 — TUI: board-picker screen (`B`)**
- Read first: `internal/tui/archive.go` (list + cursor + action pattern),
  `internal/tui/board_update.go` (quick-add textinput pattern),
  `internal/store/store.go` (U1's `ListBoards`), `cmd/kando/main.go`
- Command: `/plan` — new screen, and switching boards means swapping the
  model's backing `Store`/`Board`, not just navigating
- Tests first: `TestBoardPickerListsBoards`, `TestBoardPickerEnterSwitches`
  (lane/selection reset, new store wired in), `TestBoardPickerCreateNewBoard`,
  `TestBoardPickerEscCancels`, width invariants for the new screen; new
  authored golden `boards_120x40.txt`
- Files: `internal/tui/boards.go`, `internal/tui/model.go`

**U5 — Web: server skeleton + read-only board view**
- Read first: `cmd/kando/main.go`, `internal/tui/model.go` (the
  "load store, resolve options" pattern), `internal/board/board.go`
- Command: `/plan` — stdlib `net/http.ServeMux` pattern routing (Go 1.22+;
  `go.mod` already pins 1.25.3), no router dependency needed
- Tests first (`httptest`): `TestBoardViewRendersAllLanes`,
  `TestBoardViewFilterQueryParam` (reuses `board.Parse`,
  `internal/board/filter.go:37`), `TestBoardsListShowsExistingBoards`,
  `TestRootRedirectsToDefaultBoardOrBoardsList`, `TestCardDetailPageRenders`,
  `TestNewCardFormRenders` — the two pure-`GET` rendering routes belong
  here, not U6, which covers mutations only
- Files: `internal/web/server.go`, `internal/web/board.go`,
  `internal/web/templates/*.html` (embedded via `go:embed`),
  `cmd/kando/main.go` (new `web` subcommand, `--port` / `KANDO_WEB_PORT`,
  default port 4242)

**U6 — Web: card mutations**
- Read first: §5 (this spec), `internal/board/ops.go` (U2)
- Command: `/plan`
- Tests first: one `httptest` case per §5 route (create, update, move,
  block, checklist add/toggle/edit, delete), each asserting the on-disk
  `board.md` changed via `store.Parse` — the same round-trip assertion
  style `internal/tui/detail_test.go` already uses
- Files: `internal/web/cards.go`

**U7 — Web: create-board route**
- Tests first: `TestCreateBoardRoute` (`POST /boards` → new directory and
  canonical `board.md` exist, redirect to `/b/{name}`)
- Files: `internal/web/boards.go`

**U8 — Web: archive view + restore**
- Read first: `internal/tui/archive.go` (grouping/format to mirror),
  `Store.LoadArchive`/`SaveArchive`
- Tests first: `TestArchiveViewGroupsByWeek`, `TestArchiveRestoreRoute`
- Files: `internal/web/archive.go`

**U9 — Web: SSE live updates**
- Read first: `internal/store/watch.go`, KD-2 (§4)
- Command: `/plan` — one `Store.Watch()` channel must fan out to N open
  browser tabs; needs a small broadcaster
- Tests first: an integration test opening two `httptest` SSE connections,
  writing a change via `Store.SaveBoard`, asserting both receive an event
  within a bound (e.g. 2s) — see 6.4, this is the one genuinely integration
  test in the plan
- Files: `internal/web/events.go`, `internal/web/static/live.js` (embedded,
  served at `GET /static/live.js`), `internal/web/templates/layout.html`.
  The listener is served rather than inlined as first sketched: the CSP
  added in U5 is `default-src 'none'`, so an inline `<script>` would have
  required `script-src 'unsafe-inline'`. Still no JS dependency (KD-1) and
  still no build step (OPS-2).

**U10 — Parity audit + docs**
- Check BOUND-1a (§2): archived cards are restore-only on the web, by
  decision — the audit records it rather than reporting it as a gap
- Check the `title` field of `POST /b/{board}/cards/{id}` against the TUI's
  `T` key, which U6 added to keep BOUND-1
- Read first: §2 (BOUND-1), `internal/tui/help.go`, §5 route table
- Command: `/think`
- No new tests: a manual checklist pass, every TUI key against every §5
  route; add a "kando web" section to `README.md`

**U11 — Drag-and-drop placement on the board page**
- `board.MoveAt`, beside `Move` and sharing `stamp`: a chosen index rather
  than the top, no restamping on a same-lane reorder. `Move` is untouched —
  its same-lane no-op is a deliberate guard against a stray lane-picker
  click, and routing an unpositioned move through `MoveAt` would undo it
- `/move` grows `pos` (`before`/`after`/`start`) with an `anchor` card id,
  and redirects to the board (`?q=` intact) when a position was asked for,
  to the card when one was not. Positions are named against a card the user
  can see, never an index: the page filters, so the nth card on screen is
  not the nth in the lane. A stale `anchor` is a 409, matching the
  checklist's `was` witness
- `static/dnd.js` beside `live.js`, each pinned by its own route so the
  embed is never widened to a pattern; the drop builds its action from the
  card's own `href` rather than re-implementing `url.PathEscape` in JS
- `live.js` gains a general `busy()`: it already refuses to reload over a
  focused field, and a drag in flight is the same thing — a native drag
  cannot be restarted, so a reload would cancel it with no explanation
- This is the first web-only capability, so BOUND-1b (§2) and the parity row
  land with it, not after it
- Read first: §2 (BOUND-1), KD-1 (§4), `internal/web/cards.go`, `parity.md`
- Tests: `internal/board` for the index arithmetic and the stamping rules;
  `httptest` for the vocabulary, the 409 and the no-position guard; a manual
  browser pass for the gesture

## 6.4 Testing Strategy

| Unit | Kinds applied | N/A + reason |
| ---- | -------------- | ------------ |
| `internal/board` mutation helpers (U2) | unit | — |
| `internal/store` `ListBoards` (U1) | unit | — |
| `internal/web` route handlers (U5–U8) | unit (`httptest`), contract (§5's route table is the contract: request shape in, redirect/status out), `-race` (holds KD-3, §4: no shared model across requests) | — |
| SSE fan-out (U9) | integration (real `Store.Watch()`, real file writes, two live connections) | — |
| Drag placement (U11) | unit (`board.MoveAt`'s index arithmetic, including the drop into the gap just below the dragged card — the off-by-one no cross-lane test reaches), unit (`httptest` over the `pos`/`anchor` vocabulary, the stale-anchor 409 and the no-position guard) | The gesture itself: `dragstart`/`dragover`/`drop` never fire without a browser, so `dnd.js` is covered only by "the asset is served and the attributes render", plus the manual pass below |
| TUI board/goldens (U3, U4) | golden/fixture (three spec-provided goldens must stay byte-identical; `help_120x40.txt` and a new `boards_120x40.txt` regenerate normally) | — |
| Web board view rendering | golden/fixture (recommended: HTML snapshot tests for the board template, same idea as the TUI's golden frames) | — |
| End-to-end (`kando web` + real HTTP requests against a temp `KANDO_HOME`) | integration | mirrors the pty smoke test already done for the TUI |
| Filter/age math | — | N/A — existing, already covered in `internal/board`; no new algorithmic surface here |
| — | property | N/A — no new algorithm with a wide input space to generalize over |
| — | mutation testing | N/A — not used elsewhere in this codebase; adopting it is a separate decision |
| — | statistical | N/A — no probabilistic, ranked, or scored output |
| — | load/torture | N/A — single-user, loopback-only server (§2, §3); explicitly out of scope |
| — | post-deploy | N/A — no deploy pipeline; it's a local process (§3) |

## 6.5 Migration Sequence

1. Ship U1 (`ListBoards`) — purely additive, no on-disk format change.
2. Ship U2 (shared mutation helpers) — internal refactor; existing TUI
   tests must stay green, unchanged, with no user-visible or on-disk change.
3. Ship U3 (delete) and U4 (board picker) — additive keys and screen; the
   three reference goldens are unchanged (verified in U3/U4).
4. Ship U5–U9 (the whole web surface) — a new binary subcommand; a user who
   never runs `kando web` sees no change at all.
5. No data migration at any step — the Markdown format is unchanged
   throughout (see "Preserved Behaviors" in `current-state/analysis.md`).

## 6.6 Rollback Plan

| Scenario | Action | Notes |
| -------- | ------ | ----- |
| TUI delete/picker has a bug | Revert the U3/U4 commits; U1/U2 can stay (additive, backward compatible) | Only the TUI-facing commits need reverting |
| Web server crashes or misbehaves | Don't run `kando web`; the TUI is a separate entry point and keeps working | Failure in one surface never affects the other |
| SSE fan-out leaks goroutines/fds under many tabs | Revert the U9 commit: the route, the `static/` embed, the `<script>` in `layout.html`'s `foot` and the `script-src`/`connect-src` CSP widening travel together (the CSP string is pinned by a test, so a partial revert fails the suite). The pages keep working unchanged | Degrades gracefully — SSE is additive, not required for basic function; there is also a per-board stream cap |
| Drag-and-drop places cards wrongly | Revert the U11 commit: `board.MoveAt`, `/move`'s `pos`/`anchor` handling and redirect rule, the `dnd.js` embed and route, `board.html`'s card and lane attributes, the `<script>` and CSS in `layout.html`, and `live.js`'s `busy()` travel together | Degrades to the lane picker, which is untouched: a move with no position is the same code path it always was. Drops out for touch and keyboard users already |
| A board created via the picker or the web page is malformed | `store.Open` already creates a canonical empty `board.md` on first open (`internal/store/store.go`) | This class of bug is unlikely at the storage layer |
