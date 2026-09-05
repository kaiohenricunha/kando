# kando

A personal kanban TUI that replaces a notepad todo list. Four lanes (Backlog,
Todo, Doing, Done): the active lane is expanded with full cards, the other three
collapse to title lists. Storage is plain Markdown you can edit by hand.

Design handoff: `docs/design/README.md` (tokens, copy, glyphs, keys) and
`docs/design/Kando TUI.dc.html` (mockups). Built with Bubble Tea, Lip Gloss and
Bubbles.

## Run

```sh
go run ./cmd/kando            # opens board "life" in the terminal
go run ./cmd/kando work       # opens board "work"
make build && ./bin/kando
kando web                     # serves the boards at http://127.0.0.1:4242/
kando web work --port 8080    # a specific board, a specific port
```

The web page is the same board as a local website: create, edit, move,
block, check off and delete cards, create boards, restore from the archive.
Loopback only, no login — the loopback bind is the whole security boundary.
A board literally named `web` opens in the terminal with `kando -- web`.

Minimum terminal size is 60×16; the design target is 120×40. Below 100 columns
the collapsed lanes become a tab strip above the active lane.

Environment:

| Variable | Effect |
|---|---|
| `KANDO_HOME` | Root directory for boards (default `~/.kando`). |
| `KANDO_THEME` | `paper` (light) or `ember` (dark). Otherwise the terminal background is detected at startup. |
| `NO_COLOR` | Drop all colours; bold, strikethrough and borders stay. |
| `KANDO_WEB_PORT` | Port for `kando web` (default 4242); `--port` overrides it. |

## Keys

Board: `j/k` select card · `h/l` `tab` `shift+tab` change lane · `H/L` move the
card to the previous/next lane (the lane follows it) · `a` quick add · `enter`
open card · `d` move to Done · `u` undo (Done → Doing) · `x` delete card · `/` filter
· `D` archive · `B` boards · `?` help · `q` quit. Arrow keys work everywhere `j/k/h/l` do.

Card detail: `j/k` checklist item · `x` toggle · `o` new item · `enter` edit item
· `e` edit notes (`ctrl+s` saves, `esc` cancels) · `t` tag · `m` move (then `1`–`4`)
· `b` block reason (empty clears) · `J/K` previous/next card · `esc` back.

Filter: type to match titles; `#tag` matches tags; `!blocked`, `age>7d`, `age<3d`
(`h` also works) are operators; tokens are AND-ed. `enter` keeps the filter,
`esc` clears it.

Archive: `j/k` move · `u` back to Doing · `enter` open · `/` filter · `esc` board.

## Files

`~/.kando/<board>/board.md` and `archive.md`, rewritten canonically after every
change and reloaded when edited elsewhere:

```markdown
## Todo

### Renew passport
tag: errand
created: 2026-08-31
moved: 2026-09-01
id: k7q2m9ab
Expires 14 Nov. Two photos, old passport, printed form.
- [x] Photos from the pharmacy
- [ ] Fill in the form
```

`## Lane` headings come in fixed order; each `### Title` is a card; `key: value`
lines (`tag`, `created`, `moved`, `done`, `blocked`, `id`) follow the heading;
then free-text notes; then `- [ ]` / `- [x]` checklist items. Dates are written
date-only at midnight, otherwise as RFC 3339. A card without an `id` gets one on
load. `archive.md` groups cards under `## 2026-W36` ISO-week headings. Nothing in
the TUI archives cards; move them there by hand or with a future CLI verb.

Limits of the hand-editable format: a note line that starts with `## ` or `### `
is read as a heading, and note text after a checklist item is moved above the
checklist on the next save.

## Development

```sh
make test       # go test -race -count=1 ./...
make vet
make fmt        # rewrite
make fmt-check  # verify only
```

`.local-attest.config.mjs` runs the same targets (plus an advisory
`govulncheck`) as the local CI matrix; `dotbabel local-attest --pr <N>` posts
a SHA-pinned attestation to the open PR. The repo has no GitHub Actions
workflows, so that attestation is the review gate.

Golden frames live in `internal/tui/testdata/`. The three `board_*.txt` files are
the reference frames from the spec and are never regenerated; the other goldens
are refreshed with `go test ./internal/tui/ -run TestGoldenScreens -update`.

Glyphs such as `…`, `✓`, `⊘`, `╌` and the arrows are East-Asian-ambiguous width;
kando counts them as one cell, which matches most terminals. A terminal set to
render ambiguous characters double-width will misalign the frame.
