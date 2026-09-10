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
footnotes), not because it is missing; `—` means it is not built yet⁵.

| Capability | Web | TUI | CLI |
|---|---|---|---|
| Open / switch to a board | V | V | V |
| Create a new board | V | V | V |
| List all boards | V | V | V |
| Quick-add a card | V | V | V |
| Show a card's full detail | V | V | V |
| Move a card between lanes | V | V | V |
| Reorder a card within a lane | V | V | X¹ |
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

¹ The web drags a card to a position; the TUI steps it there with `J`/`K`,
one slot per press. Both call the same `board.MoveAt`. `kando move` takes a
lane and no position, and that is deliberate rather than pending: placing a
card is an interactive act — you put it *there*, relative to what you can see
— and a headless verb would have to name the anchor by id, which is the one
thing neither of the other two surfaces makes you do
(`docs/specs/kando-web/spec/2-scope.md`, BOUND-1b). ² Not in this effort's scope — `Card.SetTitle` already exists, so a
`kando title` verb is a one-file follow-up on the same pattern as `kando tag`.
³ A CLI verb is a one-shot process — there is nothing running for it to
refresh. ⁴ The web page labels its own controls instead of a help key.
⁵ `kando -h` lists every verb that exists; today that is `web`, `move`,
`board create`/`board list`, `add`, `show`, `list`, `tag`, `notes`, `block`,
`unblock`, `delete`, `checklist add`/`toggle`/`edit`, `archive <card>`,
`archive list` and `archive restore`.
Each `—` becomes a `V` in the pull request that adds its verb, so this column
is what the CLI can do at that merge point rather than what it is meant to do
eventually — an audit pre-filled with the answer cannot catch a unit that is
dropped or descoped.
`docs/specs/kando-web/parity.md` is the same audit at TUI-key/route
granularity.

## Run

```sh
go run ./cmd/kando            # opens board "life" in the terminal
go run ./cmd/kando work       # opens board "work"
make build && ./bin/kando
kando web                     # serves the boards at http://127.0.0.1:4242/
kando web work --port 8080    # a specific board, a specific port
kando board create work && kando board list   # every board under $KANDO_HOME
kando add "Renew passport" --tag errand   # new card at the top of Todo
kando list --filter "#errand !blocked"    # the board as text; --json for scripts
kando show "Renew passport" --json        # one card, every field
kando move "Renew passport" Doing         # move a card by title (or id) to a lane
kando tag "Renew passport" urgent
kando notes "Renew passport" --file plan.md   # or --set TEXT, or - for stdin
kando block "Renew passport" --reason "waiting on photos"
kando unblock "Renew passport"
kando checklist add "Renew passport" "Fill in the form"
kando checklist toggle "Renew passport" 1     # 1-based, as "kando show" lists them
kando delete "Renew passport"
kando archive "Cancel gym membership"     # Done → archive.md
kando archive list --filter "#money"      # the archive, grouped by week
kando archive restore "Cancel gym membership"   # back to Doing
```

A board, or a card, named the same as a verb (`add`, `list`, `board`, …) is
still reachable — a card by its id, a board with `kando -- <board>` — see
"A board literally named like a verb" below and `kando archive`'s own note
on `list`/`restore`.

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
below the card you dropped it against. The terminal reaches the same
positions with `J`/`K`, one slot per press — only the gesture differs, and
both call the same helper. Dragging needs a mouse, so on a touch
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

### `kando board`

`kando board create <name>` makes a new board — safe to run more than once;
an existing board is left exactly as is and reported rather than treated as
an error, so a script can call it unconditionally. `kando board list
[--json]` prints every board under `$KANDO_HOME`, one name per line — the
CLI's counterpart to the web's boards page and the TUI's `B`. `--json` emits
an array of `{name, cards, lanes}`, `lanes` keyed by the lowercase lane name
(`todo`, `doing`, …), for a script that wants counts without opening each
board itself.

### `kando add`

`kando add <title> [board] [--lane L] [--tag T]` is the TUI's `a`: a new
card at the top of a lane, `Todo` unless `--lane` says otherwise (`--lane
Done` stamps it done, like a move into Done would). The title is sanitized
and capped at 512 bytes the same way every other title-setting verb is; the
tag loses any leading `#`. The new card's id is printed, so a script can
address it in a later command without relying on its title staying unique.

### `kando show`

`kando show <card> [board] [--json]` prints one card's full detail — title,
tag, id, dates, age, blocked reason, notes and checklist — the read-only
equivalent of opening a card in the TUI or on the web page, without the
editing session. `<card>` follows the same id-or-title rule as `move`.
`--json` emits every field kando tracks, with `checklist` always an array and
never `null` — as is every list in every `--json` output, so a script can
iterate without a guard. Timestamps come out exactly as `board.md` holds
them: date-only for a stamp written date-only, RFC 3339 for one carrying a
time. That means the same board gives the same JSON on any machine, which
normalising to UTC would not — a date-only `2026-09-01` read west of
Greenwich would shift to the previous day. `age` is the display label the
TUI shows (`3h`, `12d`); `age_hours` is the same quantity as a number, so
`.age_hours > 168` reproduces `--filter "age>7d"`. A section with nothing in
it (no tag, no notes, an empty checklist) is simply omitted from the
plain-text output.

### `kando list`

`kando list [board] [--filter "..."] [--json]` prints every lane and its
cards — id, title, tag, checklist progress, blocked reason and age — using
the same `#tag` / `!blocked` / `age>7d` query syntax as the TUI's `/`. All
four lanes are always shown, even when empty. `--json` emits
`{board, filter, matched, total, lanes: [{lane, cards}]}` with the same
per-card shape as `kando show --json`.

### `kando tag`

`kando tag <card> <tag> [board]` sets a card's tag, the TUI's `t`. `<tag>` is
sanitized and loses a leading `#` just as it would if typed there; an empty
tag (`kando tag "Renew passport" ""`) clears it, mirroring how `block`'s
empty reason clears a block.

### `kando notes`

`kando notes <card> [board] --set TEXT`, `--file PATH`, or `-` (stdin)
replaces a card's notes — there is no `$EDITOR` integration. Line endings
are normalized to `\n`, trailing blank lines are dropped, and anything past
16 KiB is clipped, the same rules the TUI's notes editor and the web's
textarea already apply. An empty value clears the notes. Exactly one of the
three sources must be given.

### `kando block` / `kando unblock`

`kando block <card> --reason "..." [board]` sets or updates a card's blocked
reason; `--reason` is required and may not be blank (an empty reason is
`unblock`'s job, not a shorthand for it — `Card.SetBlocked("")` would
otherwise silently clear the flag instead of setting it). `kando unblock
<card> [board]` clears it, reporting `"X" was not blocked` rather than an
error when there was nothing to clear.

### `kando delete`

`kando delete <card> [board]` removes a card immediately — no confirmation,
matching every other surface: neither the TUI's `x` nor the web's delete
button asks first.

### `kando checklist`

`kando checklist add <card> <text> [board]` appends an item, mirroring how
the web's own add-item form always appends rather than inserting at a
cursor. `checklist toggle <card> <n> [board]` and `checklist edit <card> <n>
<text> [board]` address items by their 1-based position, exactly as `kando
show` numbers them — items have positions, not ids. A script that already
read the card can pass `--was "current text"`, and the command refuses if
item `<n>` no longer reads that way in the meantime: the CLI's equivalent of
the web page's stale-checklist-form guard.

### `kando archive`

`kando archive <card> [board]` moves a Done card into the archive — the
TUI's `A` and the web page's archive button. The card must be in Done
(anywhere else is an error naming its actual lane) and must not already be
archived. Its `done:` date is kept — that is the week `archive.md` files it
under — except when it has none (a hand-edited board), which is stamped to
now rather than filed under an undated heading. `list`, `restore` and the
help words (`help`, `-h`, `--help`) are reserved subcommand words below; a
card literally titled one of them is still reachable by its id, the same
trade-off `kando -- web` already makes for a board literally named like a
verb.

`kando archive list [board] [--filter "..."] [--json]` prints the archive
exactly as the TUI's `D` screen and the web's archive page do: the newest 50
entries, grouped by week, oldest week last. A board with nothing archived
yet prints `nothing archived on "life"` rather than an empty table. `--json`
mirrors the plain output's groups, plus `matched`/`scanned`/`total` so a
script can tell "12 of the newest 50 match" from "12 of 200, the rest
untouched by this filter."

`kando archive restore <card> [board]` brings an archived card back to the
top of Doing — the TUI's `u` and the web's restore button, going through the
exact same `board.Restore` call. Refused if a card with that id is already on
the board, as the TUI's `u` and the web's button are too (a previous restore or
archive half failed; one copy has to be deleted by hand first), or if the board
has nothing archived at all.

## Exit codes

Every verb uses the same three, so a script can rely on them without
checking which one ran: `0` the command did what it says; `1` it could not
— a store, filesystem, or on-disk-conflict error, nothing was necessarily
touched but nothing further was attempted either; `2` the arguments
themselves were wrong (a missing value, a bad lane name) —
caught before any file was opened, so a `2` always means nothing changed.

## Keys

Board: `j/k` select card · `h/l` `tab` `shift+tab` change lane · `H/L` move the
card to the previous/next lane (the lane follows it) · `J/K` move the card up or
down inside its lane · `a` quick add · `enter`
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
load, derived from its contents so every read agrees — and so does every card
sharing an id but one, since a copy-pasted card block keeps its `id:` line.
The card that keeps it is the one kando already reached, so anything pointing
at it still does. `archive.md` groups
cards under `## 2026-W36` ISO-week headings by each card's `done` date. `A`
on a Done card in the TUI and the **archive** button on a Done card's page
move a card there; `u` and **restore** bring it back to Doing. All three CLI archive verbs
exist too: `kando archive <card>` files it, `kando archive list` reads the
archive, and `kando archive restore <card>` brings it back.

Limits of the hand-editable format: note text after a checklist item is moved
above the checklist on the next save. A note line that would otherwise read as
structure — a `## ` heading or a `- [ ] ` item — is written with a leading
backslash and read back without it, so notes can hold Markdown of their own.
A card written without an `id` is given one derived from its contents, so it
keeps the same id until the file is saved with the id in it. When cards share
an `id`, every one but the card kando looks up first — lanes in their fixed
order, archived cards newest first — is given a new one the same way. Opening
the board in the TUI, or any command or web action that changes it, writes the
repaired ids; a read such as `kando list` leaves the file as it is until then. Text
outside any card — anything before the first `## ` heading, or between a
heading and its first `### ` card — belongs to no card and is dropped the next
time kando writes the file: any edit does that, and so does opening a board
whose ids need repairing.

Text you type into a card through any surface has its control characters and
bidirectional overrides removed before it is stored, so a note pasted from an
issue or a web page cannot carry an escape sequence into the file. A Unicode
line separator in a note becomes an ordinary line break, the same way a
Windows line ending does; every other field drops it, since those hold one
line.

Text already in the file keeps its own bytes. A save re-emits what it parsed,
character for character, so kando neither cleans those characters out of your
hand-edited text nor adds any — the reshaping described above is all it does.
It removes them on the way to the screen instead: an escape sequence in a
hand-edited note is displayed with the control characters stripped, so it
reads as ordinary text rather than driving the terminal. Editing that card
through a surface does store the stripped version, since everything you commit
goes through the same filter as anything else you type.

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
