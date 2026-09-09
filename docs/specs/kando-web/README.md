# kando-web — Engineering Spec

> A local web UI for kando: manage the same personal kanban board from a browser at localhost.
>
> Created: 2026-09-03

## Status

| #   | Section                     | Status    |
| --- | ---------------------------- | --------- |
| 1   | Problem / Motivation        | [x] done  |
| 2   | Scope                       | [x] done  |
| 3   | High-Level Architecture     | [x] done  |
| 4   | Data Flow / Components      | [x] done  |
| 5   | Interfaces and APIs         | [x] done  |
| 6   | Implementation Plan         | [x] done  |
| 7   | Non-Functional Requirements | [x] done  |
| 8   | Risks and Alternatives      | [x] done  |

## Quick Start

Start with §1 for the problem, then §2 for scope and the standing rule that
drives everything else: **BOUND-1**, the TUI and the web page must never
diverge in what they can do. §4's two key decisions (server-rendered HTML,
Server-Sent Events) explain *how* that rule is kept, not just stated.
§6 is the build order: shared foundations first, then the TUI's new delete
key and board picker, then the web server. §5 is the concrete route table
implementers will work from directly.

Finalization pass (2026-09-03): structural check clean — every `§`
reference resolves, all 22 constraint IDs are defined exactly once with no
orphans. Content checks: §1/§2's two quality claims (fast cross-surface
sync, no staleness on reopen) map to §7's PERF-1/PERF-2; DOC-1 (research/)
is a design reference, not a benchmark, so nothing needed promoting into
§7; §6.4's testing table was found to be missing explicit tests for two
`GET`-only routes in §5 (the card detail page, the new-card form) — fixed
in U5.

Finalization pass (2026-09-05, U9/U10): one deviation from §6.3 recorded —
U9's listener is served from an embedded `/static/live.js` rather than
inlined in the board template, because U5's review added a
`default-src 'none'` CSP that an inline `<script>` would have forced open to
`script-src 'unsafe-inline'`. §5 gains the `/static/live.js` row, §6.3's
Files line is corrected, and §6.6's SSE rollback row now lists all four
coupled edits. KD-1 ("no JS dependency") and OPS-2 ("no build step") are
unaffected: it is one embedded file in the same binary.

Finalization pass (2026-09-05, U11): drag-and-drop placement makes the web
page the first surface with a capability the other lacks, so §2 gains
BOUND-1b and the parity audit's "Web-only" section is no longer empty. The
exception is *precision*, not capability — both surfaces put a card in a
lane; only the browser picks where in it — and it shrinks further on touch
and keyboard, where HTML5 drag never fires at all. §5's `/move` row gains
the `pos`/`anchor` vocabulary and, with it, the first redirect in this
server that depends on what the form asked for; that rule is now stated
under the route table. §6.1, §6.3, §6.4 and §6.6 gain U11 entries.

Finalization pass (2026-09-06, U12): archiving a card (Done → `archive.md`)
existed on no surface before this unit — only `Unarchive`/`Restore`
(archive → board) did. §2's BOUND-1a note is extended (archiving *into* the
archive is a new capability, not a narrowing of the restore-only exception,
which still stands for an *already-archived* card), §5 gains the
`/cards/{id}/archive` row, and the parity audit's Board-screen table gains
the matching `A` row. §6.1, §6.4 and §6.6 gain U12 entries; the write order
(`archive.md` then `board.md`, the inverse of `SaveRestore`) is the one
deviation worth flagging — chosen so a half failure duplicates a card
instead of losing it. This spec's scope stays the web frontend: the CLI
verb built on the same U12 primitives (`kando archive`) is tracked outside
`docs/specs/kando-web` entirely, alongside the dozen other headless verbs
added in the same effort — see the "What each surface can do" table in the
top-level `README.md`.

Finalization pass (2026-09-08, TUI reorder): BOUND-1b is **closed for the
TUI**. It was the last exception §2 listed — BOUND-1a still stands — and it
named its own fix, so this is
the change it anticipated: `J`/`K` bind two board-screen keys to
`Board.MoveAt`, the helper the `/move` route already called, rather than
adding a second placement rule. §2's BOUND-1b paragraph is rewritten from
"web-only" to "narrowed to the CLI", §5's `/move` row swaps "the position is
drag-and-drop only" for the key pair, and the parity audit's Board-screen row
becomes a real pairing instead of a `—`. The audit's "Web-only" section now
opens by saying no capability is web-only.

Two properties transferred with the helper rather than being restated: a
same-lane move does not restamp `MovedAt`/`DoneAt`, and a position is named
against a card the user can *see* rather than an index — which is what makes
a filtered lane behave the same on both surfaces. Only the gesture differs.

No unit id: `U12` remains the last, and the follow-on work on another surface
is tracked the way U12's CLI verbs were — outside this spec, in the top-level
`README.md` capability table. That table is also where the **remaining** gap
now lives, as a footnoted `X` rather than a `—`: `kando move` takes a lane and
no position, and that is deliberate — a headless verb would have to name the
anchor by id, which neither interactive surface makes you do.

The persistent board footer was deliberately **not** touched, so `J`/`K` are
documented in the `?` overlay only. R-1 (§8) freezes the three reference
frames, which contain that footer line and have no regeneration path; `x`,
`B` and `A` are help-only for the same reason.

## Parity audit

[parity.md](parity.md) — every TUI key against the §5 route that does the
same thing (U10). BOUND-1b, one of the two accepted exceptions it records, is
closed for the TUI; BOUND-1a (archived cards are restore-only on the web) is
unchanged. What is left of BOUND-1b is the CLI's missing position argument,
audited in the top-level `README.md` table.

## Research Sources

See [research/sources.md](research/sources.md) for indexed source documents.
