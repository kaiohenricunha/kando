package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

// runBoard dispatches kando board's subcommands. Only "list" exists so far;
// "create" is a later PR's addition to this same switch.
func runBoard(args []string) {
	if helpWanted(args) {
		usage()
		return
	}
	if len(args) > 0 && args[0] == "list" {
		runBoardList(args[1:])
		return
	}
	usageErr(fmt.Errorf("kando board needs list"))
}

// boardListArgs parses kando board list's arguments: no positionals, just
// an optional --json.
func boardListArgs(args []string, errOut io.Writer) (jsonOut bool, err error) {
	fs := flag.NewFlagSet("board list", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() {}
	j := fs.Bool("json", false, "emit JSON")
	pos, err := parseMixed(fs, args)
	if err != nil {
		return false, err
	}
	if len(pos) > 0 {
		return false, fmt.Errorf("unexpected argument %q", pos[0])
	}
	return *j, nil
}

// summarizeBoards loads each of names read-only and reports its card
// counts. store.ListBoards' own doc warns that Open would create or
// rewrite board.md for every listed board; store.Load never does either.
func summarizeBoards(root string, names []string) ([]boardJSON, error) {
	out := make([]boardJSON, 0, len(names))
	for _, name := range names {
		b, err := store.Load(root, name)
		if err != nil {
			return nil, fmt.Errorf("board %q: %w", name, err)
		}
		lanes := make(map[string]int, len(board.Lanes))
		for _, l := range board.Lanes {
			lanes[l.Key()] = len(b.Lanes[l])
		}
		out = append(out, boardJSON{Name: name, Cards: b.Count(), Lanes: lanes})
	}
	return out, nil
}

func runBoardList(args []string) {
	if helpWanted(args) {
		usage()
		return
	}
	jsonOut, err := boardListArgs(args, os.Stderr)
	if err != nil {
		usageErr(err)
	}
	root := kandoRoot()
	names, err := store.ListBoards(root)
	if err != nil {
		fatal(err)
	}
	if jsonOut {
		boards, err := summarizeBoards(root, names)
		if err != nil {
			fatal(err)
		}
		if err := writeJSON(os.Stdout, boards); err != nil {
			fatal(err)
		}
		return
	}
	for _, name := range names {
		fmt.Println(name)
	}
}
