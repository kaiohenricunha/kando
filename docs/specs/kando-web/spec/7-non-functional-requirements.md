# §7 — Non-Functional Requirements

> Performance, reliability, operational, security constraints.

## Performance

**PERF-1.** An edit made in one surface (the TUI, another browser tab, or
hand-editing the Markdown files) is reflected in an open browser tab within
2 seconds of the underlying file write, driven by the existing
`Store.Watch()` fsnotify channel (`internal/store/watch.go:13`) pushed over
SSE (KD-2, §4) — never by polling. On breach: the SSE test in §6 (U9) fails
before this ships.

**PERF-2 (invariant).** A freshly loaded or reloaded page always reflects
current disk state, because every request re-checks via
`Store.CheckReload` itself (KD-3, §4) rather than trusting a
previously-watched value — no caching layer, no stale data. This is what
makes "lazy refresh, possibly hours later" (§3) safe without a polling
interval to tune, and it means correctness never depends on the file
watcher: a watcher failure (already non-fatal, `cmd/kando/main.go:74-80`)
degrades SSE push latency (PERF-1), never page correctness.

## Reliability

**REL-1 (invariant).** Every request that returns a success status has
already durably saved the change to disk, via the existing atomic
temp-file-plus-rename write (`internal/store/store.go`) — no response can
claim success for an edit that isn't safely on disk.

**REL-2 (invariant).** A crash or kill of the web server process never
corrupts `board.md` or `archive.md`. Same atomicity guarantee the TUI
already relies on today, reused unchanged, not reimplemented (§3 Data
Stores).

**REL-3 (invariant).** The TUI and the web server writing to the same board
concurrently never silently lose an edit outright. The existing
self-write-suppression and watch/reload machinery (already exercised by the
TUI) is reused verbatim by the web server — both surfaces converge on the
same per-mutation behavior the TUI already has today; this spec does not
introduce a new conflict-resolution scheme.

**REL-4 (invariant).** If the SSE connection drops (network blip, server
restart), the browser's native `EventSource` auto-reconnects, and the
reconnected client's next page render is current, per PERF-2 — no missed
event can leave the page permanently stale.

## Operational

**OPS-1 (invariant).** The web server binds to loopback (`127.0.0.1`) only,
never `0.0.0.0`; this is not configurable within this spec's scope, per §2
(remote access is explicitly out of scope).

**OPS-2 (invariant).** `kando web` requires no build step beyond the
existing `go build` / `go run` — no npm, no separate frontend build
pipeline. Direct consequence of KD-1 (§4).

**OPS-3.** Default port 4242, overridable with `--port` or `KANDO_WEB_PORT`
(matching the existing `KANDO_HOME` / `KANDO_THEME` naming convention,
`cmd/kando/main.go`). On breach (port already in use): the process exits
non-zero with an error naming the port and how to override it — it never
silently falls back to a different port.

**OPS-4 (invariant).** Operational errors (failed to bind, failed to save)
are printed to stderr and never crash the process silently — the same
error-surfacing pattern the TUI already uses (`fatal()`, the model's `err`
field, `cmd/kando/main.go`).

## Security

**SEC-1 (invariant).** There is no authentication (§2). OPS-1 (loopback
only) is therefore the entire security boundary: any process on the same
machine that can reach `127.0.0.1:<port>` can read and modify the board.
Accepted risk for a single-user local tool (§2's explicit choice).

**SEC-2 (invariant).** All user-provided text (titles, notes, tags,
checklist items, blocked reasons) is rendered through Go's `html/template`
package, which contextually auto-escapes output — never `text/template`,
never raw string concatenation into HTML. No card's content can inject
markup or script into the page.

**SEC-4 (invariant).** Every user-provided string reaching a rendered
surface passes `board.SafeForDisplay`, which drops category Cc (C0, DEL and
the C1 block) and `unicode.Bidi_Control`, and converts the Zl/Zp line
separators U+2028 and U+2029 to `\n`. The write-time helpers in
`internal/board/ops.go` apply the same predicate, `board.UnsafeRune`, before
a value is stored, and `store.ValidBoardName` applies it to board names.

SEC-2 and SEC-4 answer different threats and neither subsumes the other.
`html/template` prevents markup and script injection; it does nothing about a
bidirectional override, which reorders the text a browser displays without any
markup at all (CVE-2021-42574). Neither does it help the terminal surfaces,
where an escape sequence in stored text drives the terminal itself. The guard
is needed on the read path specifically because `board.md` is hand-editable
and is parsed verbatim: the store assigns fields directly and re-emits them
unchanged, so text already on disk has never met a write-time sanitizer.

The line separators are a third case, and are converted rather than dropped
because they are a line ending the author meant. They matter because the TUI
composes rows of an exact terminal-cell count and measures U+2028 as one cell,
while a terminal that honours the break emits none — every later row then sits
one line out of position. Single-line values (titles, tags, board names) drop
them instead, exactly as those values already drop `\n`, since a newline in a
title would put a second line into `board.md`, which the parser reads as
structure.

Deliberately out of scope: the rest of category Cf. Stripping it would take
U+200C and U+200D, which are load-bearing in Persian and Indic shaping and in
ZWJ emoji sequences, and the tag block that spells the England, Scotland and
Wales flags. `Bidi_Control` is the narrow subset that misrepresents order.

**SEC-3 (invariant).** Every state-changing route only accepts `POST`,
never `GET` (already true of every mutation in §5's route table). In
addition, every request is rejected unless it is same-origin: the `Host`
header must be `127.0.0.1:<port>` or `localhost:<port>`, and an `Origin`
header, when present, must equal the server's own origin. Without this, a
web page the user merely visits could drive a cross-origin form `POST` at
the loopback port — a plain HTML form submission needs no preflight and no
token to fire — and a remote page could read the board via DNS rebinding
(binding a hostname it controls to `127.0.0.1`). A CSRF token is still not
required: the Host/Origin check already blocks the browser-driven path,
and SEC-1 accepts same-machine process access as the remaining risk.

One clarification, found in U11 by driving a real browser rather than
`httptest`: an `Origin` of `null` is an origin *withheld*, not a foreign
one, and the two must not be conflated. Chrome sends `Origin: null` on every
form `POST` from a page whose `Referrer-Policy` is `no-referrer` — which is
the policy this server sets — so before U11 every mutation form in
`kando web` was refused with a 403 in Chrome, while the `httptest` suite
passed because it set the header a real browser never sends. A withheld
origin is accepted only when `Sec-Fetch-Site` says `same-origin`; the
browser sets that header and script cannot forge it, a cross-site value is
already refused, and an `Origin` naming any other host is still refused
outright.
