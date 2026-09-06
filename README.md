# kando

A personal kanban TUI that replaces a notepad todo list. Four lanes (Backlog,
Todo, Doing, Done): the active lane is expanded with full cards, the other three
collapse to title lists. Storage is plain Markdown you can edit by hand.

Design handoff: `docs/design/README.md` (tokens, copy, glyphs, keys) and
`docs/design/Kando TUI.dc.html` (mockups). Built with Bubble Tea, Lip Gloss and
Bubbles.

## What each surface can do

The terminal, the web page and the CLI (`kando <verb>`, e.g. `kando move`)
share one rule: no capability exists on only one of them. `V` means the
surface has it; `X` means it does not, by deliberate design (see the
footnotes), not because it is missing.

| Capability | Web | TUI | CLI |
|---|---|---|---|
| Open / switch to a board | V | V | V |
| Create a new board | V | V | V |
| List all boards | V | V | V |
| Quick-add a card | V | V | V |
| Show a card's full detail | V | V | V |
| Move a card between lanes | V | V | V |
| Reorder a card within a lane (drag position) | V | X¹ | X¹ |
| Delete a card | V | V | V |
| Edit title | V | V | X² |
| Edit tag | V | V | V |
| Edit notes | V | V | V |
| Block / clear block reason | V | V | V |
| Checklist: add / toggle / edit item | V | V | V |
| Filter / search (`title`, `#tag`, `!blocked`, `age>`/`<`) | V | V | V |
| List the archive | V | V | V |
| Restore a card from the archive | V | V | V |
| Archive a card (Done → archived) | V | V | V |
| Live auto-refresh on external changes | V | V | X³ |
| Help | X⁴ | V | V |

¹ Positioning a card within a lane needs a pointer; the TUI and CLI move a
card to the top of a lane instead (`docs/specs/kando-web/spec/2-scope.md`,
BOUND-1b). ² Not in this effort's scope — `Card.SetTitle` already exists, so a
`kando title` verb is a one-file follow-up on the same pattern as `kando tag`.
³ A CLI verb is a one-shot process — there is nothing running for it to
refresh. ⁴ The web page labels its own controls instead of a help key.
`docs/specs/kando-web/parity.md` is the same audit at TUI-key/route
granularity.

## Run

```sh
go run ./cmd/kando            # opens board "life" in the terminal
go run ./cmd/kando work       # opens board "work"
make build && ./bin/kando
kando web                     # serves the boards at http://127.0.0.1:4242/
kando web work --port 8080    # a specific board, a specific port
kando move "Renew passport" Doing         # move a card by title (or id) to a lane
```

### `kando web`

The same board as a local web page: add a card, edit its title, tag and
notes, move it between lanes, block it, tick checklist items, delete it,
archive it from Done, create a board, filter with the same query syntax as
the TUI, and restore from the archive. Every page reflects the files as they
are on disk, and an
open *board* page refreshes itself within a couple of seconds of any change
— whether it came from another tab, from `kando` in a terminal, or from
your text editor. The boards list updates on its next load instead, since
`$KANDO_HOME` itself is not watched. A page with a focused field, or a drag
in flight, waits until you are done before refreshing, so a reload never eats
what you are typing or the card you are still aiming.

Cards drag between lanes, and a drop lands where the line shows: above or
below the card you dropped it against. That position is the one thing the web
page can do that the terminal cannot — everything else `kando web` does, the
TUI does too, and the other way round. Dragging needs a mouse, so on a touch
screen use the lane picker on a card's own page, as the terminal does.
`docs/specs/kando-web/parity.md` is the audit, key by key.

It binds `127.0.0.1` only and has no login: the loopback bind is the whole
security boundary, and requests from other origins are refused.
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

### `kando move`

The scriptable equivalent of the TUI's `H`/`L`/`m`+digit and the web's lane
picker: `kando move <card> <lane> [board]` moves one card to a lane without
opening the terminal or the browser. `<card>` is a card's id or its exact
title, matched case-insensitively; a title matching more than one card is
refused — use the id instead. `<lane>` is `Backlog`, `Todo`, `Doing` or
`Done`, case-insensitive. `[board]` defaults to `life`, like every other
verb. Unlike `kando [board]` and `kando web [board]`, a board that does not
already exist is an error, not something `move` creates for you: there is no
card to move on a board that was never opened.

## Keys

Board: `j/k` select card · `h/l` `tab` `shift+tab` change lane · `H/L` move the
card to the previous/next lane (the lane follows it) · `a` quick add · `enter`
open card · `d` move to Done · `u` undo (Done → Doing) · `x` delete card · `/` filter
· `D` archive view · `A` archive the card (Done only) · `B` boards · `?` help
· `q` quit. Arrow keys work everywhere `j/k/h/l` do.

Card detail: `j/k` checklist item · `x` toggle · `o` new item · `enter` edit item
· `T` title · `e` edit notes (`ctrl+s` saves, `esc` cancels) · `t` tag · `m` move (then `1`–`4`)
· `b` block reason (empty clears) · `A` archive (Done only) · `J/K` previous/next card · `esc` back.

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
load, derived from its contents so every read agrees. `archive.md` groups
cards under `## 2026-W36` ISO-week headings by each card's `done` date. `A`
on a Done card in the TUI, the **archive** button on a Done card's page, and
`kando archive <card>` all move a card there; `u`, **restore**, and
`kando archive restore <card>` bring it back to Doing.

Limits of the hand-editable format: note text after a checklist item is moved
above the checklist on the next save. A note line that would otherwise read as
structure — a `## ` heading or a `- [ ] ` item — is written with a leading
backslash and read back without it, so notes can hold Markdown of their own.
A card written without an `id` is given one derived from its contents, so it
keeps the same id until the file is saved with the id in it.

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
