# kando CLI

`kando <verb>` does what the terminal and the web page do, one command at a
time, so a script or a shell alias can drive a board. Every verb reads and
writes the same Markdown files ([format](format.md)), so its changes show up
in an open terminal or browser within seconds.

## Synopsis

```text
kando [board]
kando web [board] [--port N]
kando board create <name> | board list [--json]
kando add <title> [board] [--lane L] [--tag T]
kando show <card> [board] [--json]
kando list [board] [--filter "..."] [--json]
kando move <card> <lane> [board]
kando tag <card> <tag> [board]
kando notes <card> [board] (--set TEXT | --file PATH | -)
kando block <card> --reason "..." [board]
kando unblock <card> [board]
kando delete <card> [board]
kando checklist add <card> <text> [board]
kando checklist toggle <card> <n> [board] [--was TEXT]
kando checklist edit <card> <n> <text> [board] [--was TEXT]
kando archive <card> [board]
kando archive list [board] [--filter "..."] [--json]
kando archive restore <card> [board]
```

`kando -h` prints the same list.

## Rules every verb shares

- `[board]` defaults to `life` and may come before or after a verb's flags.
  Only `kando [board]`, `kando web [board]` and `kando board create` create a
  board; every other verb needs one that already exists.
- `<card>` is a card's id or its exact title, matched case-insensitively. A
  title matching more than one card is refused, and the error lists the
  matching ids so a script has an unambiguous way to retry.
- `<lane>` is `Backlog`, `Todo`, `Doing` or `Done`, case-insensitive.
- `--filter` takes the same query as the terminal's `/`: title text, `#tag`,
  `!blocked`, `age>7d`, `age<3d` (`h` works too); tokens are AND-ed.
- A board named like a verb (`add`, `list`, `web`, …) opens in the terminal
  with `kando -- <board>`. `kando archive` reserves `list`, `restore` and the
  help words (`help`, `-h`, `--help`), so a card titled one of them needs its
  id there.

## Exit codes

Every verb uses the same three, so a script can rely on them without checking
which one ran:

| Code | Meaning |
|---|---|
| `0` | The command did what it says. |
| `1` | It could not complete — a store, filesystem or on-disk-conflict error. Nothing further was attempted. |
| `2` | The arguments were wrong (a missing value, a bad lane name). Caught before any file was opened, so nothing changed. |

## Scripting with `--json`

`show`, `list`, `board list` and `archive list` take `--json`. Every list in
every `--json` output is an array, never `null`, so a script can iterate
without a guard.

```sh
# titles of every card in Doing
kando list --json | jq -r '.lanes[] | select(.lane == "Doing") | .cards[].title'

# every card older than a week, with its age
kando list --json | jq -r '.lanes[].cards[] | select(.age_hours > 168) | "\(.age) \(.title)"'

# how busy each board is
kando board list --json | jq -r '.[] | "\(.name): \(.cards) cards, \(.lanes.doing) doing"'
```

## Verbs

### `kando web`

`kando web [board] [--port N]` serves every board as a local web page at
`http://127.0.0.1:4242/`, opening `[board]` first. The port comes from
`--port`, then `KANDO_WEB_PORT`, then 4242. It binds `127.0.0.1` only and has
no login: the loopback bind is the whole security boundary, and requests from
other origins are refused. A port already in use is an error that names it,
never a silent fallback to another port.

### `kando move`

`kando move <card> <lane> [board]` moves one card to a lane — the terminal's
`H`/`L`/`m`+digit and the web's lane picker — without opening either. A board
that does not already exist is an error, not something `move` creates for you:
there is no card to move on a board that was never opened. `move` takes a lane
and no position: placing a card inside a lane is an interactive act (the web
drags it there, the terminal steps it with `J`/`K`), and a headless verb would
have to name the neighbouring card by id.

### `kando board`

`kando board create <name>` makes a new board. It is safe to run more than
once: an existing board is left exactly as is and reported rather than
treated as an error, so a script can call it unconditionally. `kando board
list [--json]` prints every board under `$KANDO_HOME`, one name per line.
`--json` emits an array of `{name, cards, lanes}`, `lanes` keyed by the
lowercase lane name (`todo`, `doing`, …), for a script that wants counts
without opening each board itself.

### `kando add`

`kando add <title> [board] [--lane L] [--tag T]` is the terminal's `a`: a new
card at the top of a lane, `Todo` unless `--lane` says otherwise (`--lane
Done` stamps it done, like a move into Done would). The title is sanitized and
capped at 512 bytes the same way every other title-setting verb is; the tag
loses any leading `#`. The new card's id is printed, so a script can address
it in a later command without relying on its title staying unique.

### `kando show`

`kando show <card> [board] [--json]` prints one card's full detail — title,
tag, id, dates, age, blocked reason, notes and checklist — the read-only
equivalent of opening a card. A section with nothing in it (no tag, no notes,
an empty checklist) is omitted from the plain-text output.

`--json` emits every field kando tracks, with `checklist` always an array.
Timestamps come out exactly as `board.md` holds them: date-only for a stamp
written date-only, RFC 3339 for one carrying a time. That means the same board
gives the same JSON on any machine, which normalising to UTC would not — a
date-only `2026-09-01` read west of Greenwich would shift to the previous day.
`age` is the display label (`3h`, `12d`); `age_hours` is the same quantity as
a number, so `.age_hours > 168` reproduces `--filter "age>7d"`.

### `kando list`

`kando list [board] [--filter "..."] [--json]` prints every lane and its
cards — id, title, tag, checklist progress, blocked reason and age. All four
lanes are always shown, even when empty. `--json` emits
`{board, filter, matched, total, lanes: [{lane, cards}]}`, with `filter` only
when one was given and each card in the same shape as `kando show --json`.

### `kando tag`

`kando tag <card> <tag> [board]` sets a card's tag, the terminal's `t`. The tag
is sanitized and loses a leading `#`; an empty tag (`kando tag "Renew
passport" ""`) clears it.

### `kando notes`

`kando notes <card> [board] --set TEXT`, `--file PATH`, or `-` (stdin) replaces
a card's notes; exactly one of the three sources must be given. There is no
`$EDITOR` integration. Line endings are normalized to `\n`, trailing blank
lines are dropped, and anything past 16 KiB is clipped — the same rules the
terminal's notes editor and the web page apply. An empty value clears the
notes.

### `kando block` / `kando unblock`

`kando block <card> --reason "..." [board]` sets or updates a card's blocked
reason. `--reason` is required and may not be blank: clearing a block is
`unblock`'s job. `kando unblock <card> [board]` clears it, reporting `"X" was
not blocked` rather than an error when there was nothing to clear.

### `kando delete`

`kando delete <card> [board]` removes a card immediately, with no
confirmation — the same as the terminal's `x` and the web's delete button.

### `kando checklist`

`kando checklist add <card> <text> [board]` appends an item. `checklist toggle
<card> <n> [board]` and `checklist edit <card> <n> <text> [board]` address
items by their 1-based position, exactly as `kando show` numbers them — items
have positions, not ids. A script that already read the card can pass `--was
"current text"`, and the command refuses if item `<n>` no longer reads that
way.

### `kando archive`

`kando archive <card> [board]` moves a Done card into the archive — the
terminal's `A` and the web page's archive button. The card must be in Done
(anywhere else is an error naming its actual lane) and must not already be
archived. Its `done:` date is kept, since that is the week `archive.md` files
it under; a card with none (a hand-edited board) is stamped with the current
time rather than filed under an undated heading.

`kando archive list [board] [--filter "..."] [--json]` prints the archive as
the terminal's `D` screen and the web's archive page do: the newest 50
entries, grouped by week, oldest week last. A board with nothing archived
prints `nothing archived on "life"` rather than an empty table. `--json`
mirrors the plain output's groups, plus `matched`/`scanned`/`total` so a
script can tell "12 of the newest 50 match" from "12 of 200".

`kando archive restore <card> [board]` brings an archived card back to the top
of Doing — the terminal's `u` and the web's restore button. It is refused if a
card with that id is already on the board (a previous restore or archive half
failed, and one copy has to be deleted by hand first), or if the board has
nothing archived.
