# kando

A personal kanban TUI: four lanes (Backlog, Todo, Doing, Done), the active lane
expanded, the others collapsed to title lists. Plain Markdown storage under
`~/.kando/<board>/`.

Design handoff: `docs/design/README.md` (tokens, copy, glyphs, keys) and
`docs/design/Kando TUI.dc.html` (mockups).

## Run

```sh
go run ./cmd/kando            # opens board "life"
go run ./cmd/kando work       # opens board "work"
```

Environment: `KANDO_HOME` (root directory, default `~/.kando`), `KANDO_THEME`
(`paper` or `ember`, otherwise detected from the terminal background), `NO_COLOR`.
