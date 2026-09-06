package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

// runArchive dispatches kando archive's subcommands. Only "list" exists so
// far; the bare <card> form and "restore" are a later PR's addition to this
// same switch.
func runArchive(args []string) {
	if helpWanted(args) {
		usage()
		return
	}
	if len(args) > 0 && args[0] == "list" {
		runArchiveList(args[1:])
		return
	}
	usageErr(fmt.Errorf("kando archive needs list"))
}

// archiveListArgs parses kando archive list's arguments: an optional board,
// an optional --filter, and an optional --json.
func archiveListArgs(args []string, errOut io.Writer) (name, query string, jsonOut bool, err error) {
	fs := flag.NewFlagSet("archive list", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() {}
	f := fs.String("filter", "", "filter query, same syntax as the TUI's /")
	j := fs.Bool("json", false, "emit JSON")
	pos, err := parseMixed(fs, args)
	if err != nil {
		return "", "", false, err
	}
	if len(pos) > 1 {
		return "", "", false, fmt.Errorf("unexpected argument %q", pos[1])
	}
	if len(pos) == 1 {
		name = pos[0]
	}
	return name, *f, *j, nil
}

// archiveList is kando archive list's testable core: the package-level
// store.LoadArchive (never the Store method, which may persist ids for a
// hand-written archive.md — a list must not have that side effect), then
// board.ArchiveView over the parsed filter, mirroring the TUI's own archive
// screen exactly. resolvedName is returned since "" defaults to "life".
func archiveList(root, name, query string, now time.Time) (resolvedName string, groups []board.ArchiveGroup, matched, scanned, total int, err error) {
	resolvedName, err = resolveBoard(root, name)
	if err != nil {
		return "", nil, 0, 0, 0, err
	}
	a, err := store.LoadArchive(root, resolvedName)
	if err != nil {
		return "", nil, 0, 0, 0, err
	}
	groups, matched, scanned, total = board.ArchiveView(a, board.Parse(query), now)
	return resolvedName, groups, matched, scanned, total, nil
}

func formatArchiveList(root, name string, groups []board.ArchiveGroup, query string, matched, scanned, total int) string {
	var b strings.Builder
	if len(groups) == 0 {
		fmt.Fprintf(&b, "nothing archived on %q\n", name)
		return strings.TrimRight(b.String(), "\n")
	}
	for _, g := range groups {
		fmt.Fprintf(&b, "%s %d\n", g.Label, len(g.Cards))
		for _, c := range g.Cards {
			line := "✓ " + c.Title
			if c.Tag != "" {
				line += "  #" + c.Tag
			}
			if !c.DoneAt.IsZero() {
				line += "  " + board.DayLabel(c.DoneAt)
			}
			fmt.Fprintf(&b, "%s\n", line)
		}
	}
	if query != "" {
		fmt.Fprintf(&b, "%d of the newest %d match %q\n", matched, scanned, query)
	}
	if total > scanned {
		fmt.Fprintf(&b, "Older entries live in %s\n", store.ArchiveDisplayPath(root, name))
	}
	return strings.TrimRight(b.String(), "\n")
}

func runArchiveList(args []string) {
	if helpWanted(args) {
		usage()
		return
	}
	name, query, jsonOut, err := archiveListArgs(args, os.Stderr)
	if err != nil {
		usageErr(err)
	}
	root := kandoRoot()
	now := time.Now()
	resolvedName, groups, matched, scanned, total, err := archiveList(root, name, query, now)
	if err != nil {
		fatal(err)
	}
	if jsonOut {
		out := archiveListJSON{Board: resolvedName, Filter: query, Matched: matched, Scanned: scanned, Total: total}
		for _, g := range groups {
			cards := make([]cardJSON, len(g.Cards))
			for i, c := range g.Cards {
				cards[i] = cardToJSON(-1, c, now)
			}
			out.Groups = append(out.Groups, archiveGroupJSON{Label: g.Label, Cards: cards})
		}
		if err := writeJSON(os.Stdout, out); err != nil {
			fatal(err)
		}
		return
	}
	fmt.Println(formatArchiveList(root, resolvedName, groups, query, matched, scanned, total))
}
