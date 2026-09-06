package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

// verbs maps each headless CLI verb to its process-level entry point.
// "help", "-h", "--help", "-v", "--version" and "version" are deliberately
// not table keys — main's own switch handles those, unchanged.
var verbs = map[string]func([]string){
	"web":  runWeb,
	"move": runMove,
}

// dispatch names the verb args select ("" when none matched) and the args
// that verb gets. "--" always means "not a verb": `kando -- move` opens the
// TUI on a board literally named "move", exactly as `kando -- web` already
// does for "web" — dispatch never looks at what follows "--".
func dispatch(args []string) (verb string, rest []string) {
	if len(args) == 0 {
		return "", args
	}
	if args[0] == "--" {
		return "", args[1:]
	}
	if _, ok := verbs[args[0]]; ok {
		return args[0], args[1:]
	}
	return "", args
}

// helpWanted is true when args is exactly one of -h, --help or help — the
// same three tokens main's own switch recognises for the bare `kando` form.
func helpWanted(args []string) bool {
	return len(args) == 1 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help")
}

// usageErr reports an argument-shape error the way every verb does: the
// message, the full usage text, and exit 2 — for errors discoverable by
// pure string comparison, before any filesystem access is attempted.
func usageErr(err error) {
	fmt.Fprintln(os.Stderr, "kando:", err)
	usage()
	os.Exit(2)
}

// errConflict is what a mutating verb reports when saveBoard refuses a
// stale write.
var errConflict = errors.New("the board changed on disk — try again")

// resolveBoard applies every verb's shared board-name rule: "" defaults to
// "life", the name must be valid, and the board must already exist — only
// `board create`, `kando [board]` and `kando web [board]` are allowed to
// create one, so a typo'd name fails clearly instead of silently starting
// an empty board.
func resolveBoard(root, name string) (string, error) {
	if name == "" {
		name = "life"
	}
	if !store.ValidBoardName(name) {
		return "", fmt.Errorf("invalid board name %q", name)
	}
	if !store.Exists(root, name) {
		return "", fmt.Errorf("no such board %q (create it first with \"kando board create %s\")", name, name)
	}
	return name, nil
}

// openBoard resolves name and opens it for writing.
func openBoard(root, name string) (*store.Store, *board.Board, error) {
	name, err := resolveBoard(root, name)
	if err != nil {
		return nil, nil, err
	}
	return store.Open(root, name)
}

// readBoard resolves name and loads it read-only, for a verb that must
// never rewrite board.md just to display it.
func readBoard(root, name string) (*board.Board, error) {
	name, err := resolveBoard(root, name)
	if err != nil {
		return nil, err
	}
	return store.Load(root, name)
}

// saveBoard is every mutating verb's save step: this is a short-lived
// process opening its own Store, same as a kando web request, so nothing
// else stops a concurrent edit (the TUI, kando web, a text editor) landing
// between the open and the save — refuse a stale write rather than
// silently overwrite it.
func saveBoard(st *store.Store, b *board.Board) error {
	if err := st.SaveBoardIfUnchanged(b); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return errConflict
		}
		return err
	}
	return nil
}

// findCard resolves arg against b: a card id first (Board.Find), then — on a
// miss — a case-insensitive exact title match across all four lanes. On any
// error the Lane, index and Card results are zero values (Backlog, -1, nil)
// and must not be used; callers must check err first, exactly as every
// caller of Board.Find's own -1 index already must.
func findCard(b *board.Board, arg string) (board.Lane, int, *board.Card, error) {
	if l, i, c := b.Find(arg); c != nil {
		return l, i, c, nil
	}
	var l board.Lane
	i, n := -1, 0
	var c *board.Card
	var ids []string
	for _, lane := range board.Lanes {
		for idx, card := range b.Lanes[lane] {
			if strings.EqualFold(card.Title, arg) {
				l, i, c = lane, idx, card
				ids = append(ids, card.ID)
				n++
			}
		}
	}
	switch n {
	case 0:
		return 0, -1, nil, fmt.Errorf("no card matches id or title %q", arg)
	case 1:
		return l, i, c, nil
	default:
		return 0, -1, nil, fmt.Errorf("%d cards match title %q; use one of these ids instead: %s", n, arg, strings.Join(ids, ", "))
	}
}

// cardOp is one mutation applied to the card findCard resolved. changed
// false skips the save entirely, so a no-op never bumps the board's on-disk
// version; a non-nil err aborts before any save.
type cardOp func(b *board.Board, lane board.Lane, i int, c *board.Card) (changed bool, err error)

// withCard is the shared core of every card-mutating verb: open the board,
// resolve the card, run op, and save only when op reports a change. c is
// still valid to read afterward even if op deleted it — Go does not free it
// out from under the caller.
func withCard(root, name, cardArg string, op cardOp) (board.Lane, *board.Card, error) {
	st, b, err := openBoard(root, name)
	if err != nil {
		return 0, nil, err
	}
	lane, i, c, err := findCard(b, cardArg)
	if err != nil {
		return 0, nil, err
	}
	changed, err := op(b, lane, i, c)
	if err != nil {
		return 0, nil, err
	}
	if changed {
		if err := saveBoard(st, b); err != nil {
			return 0, nil, err
		}
	}
	return lane, c, nil
}

// parseMixed collects fs's positional arguments interleaved with its flags:
// flag.FlagSet.Parse stops at the first non-flag, so this resumes parsing
// after each one, generalising webArgs's own two-pass loop to any number of
// positionals in any order. Everything after a literal "--" is positional
// even if it looks like a flag, so a card titled "-1 point bug" stays
// reachable through a verb that does take flags.
func parseMixed(fs *flag.FlagSet, args []string) (pos []string, err error) {
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return pos, nil
		}
		if rest[0] == "--" {
			return append(pos, rest[1:]...), nil
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
}

// splitBoard applies every verb's shared positional grammar: exactly n
// required values, plus at most one trailing optional board name. need
// names the required values for the "too few" error, e.g. "a card and a
// lane". Every required value is trimmed and must be non-empty; a present
// board name must not be blank and must not look like a flag — a required
// value may, since a card's title is free text, so only the trailing board
// slot gets that guard.
func splitBoard(verb, need string, pos []string, n int) (vals []string, name string, err error) {
	switch {
	case len(pos) < n:
		return nil, "", fmt.Errorf("kando %s needs %s", verb, need)
	case len(pos) == n:
		vals = append([]string(nil), pos[:n]...)
	case len(pos) == n+1:
		vals = append([]string(nil), pos[:n]...)
		name = pos[n]
		if strings.HasPrefix(name, "-") {
			return nil, "", fmt.Errorf("unexpected flag %q", name)
		}
		if strings.TrimSpace(name) == "" {
			return nil, "", fmt.Errorf("board name required")
		}
	default:
		return nil, "", fmt.Errorf("unexpected argument %q", pos[n+1])
	}
	for i, v := range vals {
		v = strings.TrimSpace(v)
		if v == "" {
			return nil, "", fmt.Errorf("kando %s needs %s", verb, need)
		}
		vals[i] = v
	}
	return vals, name, nil
}
