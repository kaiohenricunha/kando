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

## Research Sources

See [research/sources.md](research/sources.md) for indexed source documents.
