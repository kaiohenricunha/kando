# Board files

Every board is a directory under `$KANDO_HOME` (default `~/.kando`) holding
two plain Markdown files: `board.md` for the four lanes and `archive.md` for
archived cards. kando rewrites a file canonically after every change and
reloads it when it is edited elsewhere, so a text editor is one more way in.

## board.md

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
date-only at midnight, otherwise as RFC 3339.

A card without an `id` gets one on load, derived from its contents so every
read agrees — and so does every card sharing an id but one, since a
copy-pasted card block keeps its `id:` line. The card that keeps it is the one
kando already reached, so anything pointing at it still does.

## archive.md

`archive.md` groups cards under `## 2026-W36` ISO-week headings by each card's
`done` date. `A` on a Done card in the terminal, the **archive** button on a
Done card's web page and `kando archive <card>` move a card there; `u`,
**restore** and `kando archive restore <card>` bring it back to Doing.

## A hand edit that does not parse

A hand edit that stops `board.md` or `archive.md` from parsing is not loaded,
and none of kando's writers will save over it. The terminal's footer names
the file and the parse error and refuses a change made meanwhile rather than
write it; a CLI command such as `kando move` or `kando archive` prints the
same error to stderr and exits 1; and `kando web` answers 500 for the board
it belongs to. The footer holds the message to half the row and truncates a
long one with `…`; the CLI's stderr line is never truncated. Fix the file and
the next reload (or command) picks it up; then make the change again.

## Limits of hand editing

- Note text after a checklist item is moved above the checklist on the next
  save.
- A line shaped like a key kando does not know — a lowercase letter, then
  lowercase letters, digits, `_` or `-`, a colon, and a space, a tab or
  nothing, such as `priority: high` — is kept as a note without ending the
  keys, and the next save moves it below them. A key line after it still
  counts if its value is valid and the card does not have that key yet;
  otherwise it stays a note too.
- A note line that would otherwise read as structure — a `## ` heading, a
  `- [ ] ` item or a line that starts with one of kando's own keys — is
  written with a leading backslash and read back without it, so notes can
  hold Markdown of their own.
- A card written without an `id` keeps the id derived from its contents
  until the file is saved with the id in it. When cards share an `id`, every
  one but the card kando looks up first — lanes in their fixed order,
  archived cards newest first — is given a new one the same way. Opening the
  board in the terminal, or any command or web action that changes it,
  writes the repaired ids; a read such as `kando list` leaves the file as it
  is until then.
- Text outside any card — anything before the first `## ` heading, or between
  a heading and its first `### ` card — belongs to no card and is dropped the
  next time kando writes the file: any edit does that, and so does opening a
  board whose ids need repairing.

## Control characters

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
