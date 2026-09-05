# Current State Analysis

> Analysis of the existing system being redesigned.

## System Overview

kando is a Go CLI (`cmd/kando`) that opens a terminal UI (`internal/tui`,
Bubble Tea) over a plain-Markdown board (`internal/board` domain,
`internal/store` persistence). One board is opened per invocation
(`kando [board]`), read from and written to `$KANDO_HOME/<board>/`. See §4
for the full current-state data flow.

## Pain Points

None reported — kando is not broken and this is not a rewrite. See §1:
the motivation is a second surface (a browser), not a fix. The TUI's
behavior and keybindings carry forward unchanged, aside from the new
delete capability that ships to both surfaces together (§2 BOUND-1).

## Preserved Behaviors

- The Markdown file format (`internal/store/markdown.go`) does not change.
- `internal/board` and `internal/store` are reused unchanged (beyond the
  additive board-listing/creation and delete operations §2 calls for);
  the web page is a second renderer, not a second model.
- Every existing TUI keybinding and behavior keeps working as it does
  today.
- BOUND-1 (§2): any capability the web page gains, the TUI gains too, and
  vice versa.
