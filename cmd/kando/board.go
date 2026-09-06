package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

// runBoard dispatches kando board's subcommands.
func runBoard(args []string) {
	if helpWanted(args) {
		usage()
		return
	}
	if len(args) > 0 {
		switch args[0] {
		case "list":
			runBoardList(args[1:])
			return
		case "create":
			runBoardCreate(args[1:])
			return
		}
	}
	usageErr(fmt.Errorf("kando board needs create or list"))
}

// boardCreateArgs parses kando board create's arguments: exactly one name,
// no flags and — unlike every other verb's trailing [board] — no optional
// second positional either, since there is no board to scope the creation
// to; splitBoard's n+1-means-a-trailing-board shape does not fit here, so
// this is a small dedicated parser rather than a misuse of it.
func boardCreateArgs(args []string) (name string, err error) {
	switch len(args) {
	case 0:
		return "", fmt.Errorf("kando board create needs a board name")
	case 1:
		name = args[0]
	default:
		return "", fmt.Errorf("unexpected argument %q", args[1])
	}
	if strings.HasPrefix(name, "-") {
		return "", fmt.Errorf("unexpected flag %q", name)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("board name required")
	}
	return name, nil
}

// createBoard mirrors the web's createBoard handler (internal/web/boards.go)
// and the TUI picker's n: a valid name becomes a directory with a canonical
// board.md, exactly as store.Open already does for a bare `kando <board>`.
// An existing board is left as is — created reports which happened, so the
// caller can tell "made a new one" from "it was already there" — both
// exit 0, since board create is meant to be safe to run unconditionally in
// a script.
func createBoard(root, name string) (created bool, err error) {
	if !store.ValidBoardName(name) {
		return false, fmt.Errorf(`invalid board name %q: 1-64 characters, no / \ # ? %% & + ; and no leading dot`, name)
	}
	if store.Exists(root, name) {
		return false, nil
	}
	if _, _, err := store.Open(root, name); err != nil {
		return false, err
	}
	return true, nil
}

func runBoardCreate(args []string) {
	if helpWanted(args) {
		usage()
		return
	}
	name, err := boardCreateArgs(args)
	if err != nil {
		usageErr(err)
	}
	created, err := createBoard(kandoRoot(), name)
	if err != nil {
		fatal(err)
	}
	if created {
		fmt.Printf("created board %q\n", name)
		return
	}
	fmt.Printf("board %q already exists\n", name)
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
//
// A board that fails to load aborts the whole listing rather than being
// skipped or zero-filled. That is deliberate: board.md is hand-editable and
// store.Parse rejects a whole file on one bad line, so the realistic cause
// is a typo the user wants to hear about. A silent 0 would be indexed by a
// script as a real count, and a skipped board would vanish from a listing
// whose entire job is to be complete — both are worse than exiting non-zero
// with the parse error naming the file. It does mean plain `board list`
// succeeds where `--json` fails, since the plain path never opens a board.
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
		// A bare array, deliberately, where the other --json verbs emit an
		// object: this output is a list and nothing else, and the shell
		// one-liner it exists for reads `jq '.[].name'` rather than
		// `jq '.boards[].name'`. The cost is that a sibling field cannot be
		// added later without breaking consumers — acceptable here because
		// per-board detail belongs on the board's own row, and anything
		// global belongs to a verb that does not yet exist.
		if err := writeJSON(os.Stdout, boards); err != nil {
			fatal(err)
		}
		return
	}
	for _, name := range names {
		fmt.Println(name)
	}
}
