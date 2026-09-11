# §8 — Risks and Alternatives

> Known risks with mitigations, rejected approaches with reasoning.

## Risks

| ID | Risk | Likelihood | Impact | Mitigation |
| --- | ---- | ---------- | ------ | ---------- |
| R-1 | Adding "x delete" / "B boards" text to the board screen's persistent footer would change its width and break the three spec-provided golden frames (`board_120x40.txt`, `board_120x40_done.txt`, `board_80x24.txt`). They began as the literal reference frames from the original TUI spec, and they change only when a design change replaces those frames on purpose and is reviewed line by line — which the Ember redesign did once (see the finalization log in the spec README). | Certain, if not mitigated | Breaks a hard, already-established product invariant | Expose `x` and `B` through the help overlay only (`?`), not the footer; the footer's keys change only with a deliberate redesign. Already the plan in §6 (U3). |
| R-2 | SSE connections held open per browser tab could accumulate (leaked goroutines/file descriptors) if a tab disappears without a clean disconnect (sleep, network drop). | Medium, over long-running sessions | Low for one local user; could still degrade the process over days | Use request-context cancellation to detect client disconnect and unsubscribe from the watcher; §6.6's rollback lever (stop serving the endpoint) covers the worst case. |
| R-3 | Lifting existing TUI mutation logic (tag-strip, checklist ops, notes/blocked set) out of `internal/tui/detail.go` into shared `internal/board/ops.go` (U2, §6) could subtly change behavior the TUI's existing tests don't catch, if the refactor isn't purely mechanical. | Medium | Medium — silent behavior drift in an already-shipped, tested tool | U2 requires existing TUI tests to stay green, unchanged, as a hard gate; new ops-level tests are written first (TDD), mirroring exactly what `detail.go` does today, before `detail.go` is changed to call them. |

## Rejected Alternatives

**A-1 — JSON API + JavaScript single-page app**, instead of server-rendered
HTML (KD-1, §4). Rejected: a client-side model of the board is a second
source of truth alongside the Go `*board.Board`, exactly what BOUND-1 (§2)
exists to prevent; it also adds a JS framework, a build step, and a bundler,
against §2's "keep everything simple and local" decision.

**A-2 — WebSocket or client polling**, instead of Server-Sent Events (KD-2,
§4). Rejected: the update direction is one-way (server tells the browser
the board changed), which is exactly what SSE is for; WebSocket would add
protocol complexity with no benefit since nothing flows browser-to-server
over it, and polling means either wasted requests when nothing changed or a
tuned interval that trades latency against load for no reason, when the
existing file watcher already knows the instant something changes.

**A-3 — a separate `kando-web` binary**, instead of a `web` subcommand on
the existing `cmd/kando` binary (§3). Rejected: two binaries could drift to
importing different versions of `internal/board` / `internal/store` over
time (e.g. one gets rebuilt, the other doesn't), which is the same kind of
split BOUND-1 (§2) is meant to prevent, just at the build/dependency level
instead of the feature level. One binary with two subcommands guarantees
the TUI and the web server always run identical domain and storage code.
