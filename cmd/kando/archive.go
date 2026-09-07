package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

// runArchive dispatches kando archive's subcommands: "list" and "restore"
// are reserved subcommand words, checked before the bare <card> form, which
// gets everything else — so a card literally titled "list" or "restore" is
// still reachable by its id, the same accepted collision class as
// `kando -- web` for a board literally named like a verb.
//
// That answers the question the previous unit left open here: rather than
// teaching this switch the "--" escape the top-level dispatch uses, the
// collision is accepted and the id is the way out. Addressing by id is
// something a script can always do, whereas `kando archive -- list` would be
// a second escape convention to learn for one card in a thousand.
// TestCLIArchiveDispatch and TestCLIArchiveReservedWordsNeedAnID pin this
// through the real binary — the unit tests below call the cores directly and
// would not notice this switch being reverted.
func runArchive(args []string) {
	if helpWanted(args) {
		usage()
		return
	}
	if len(args) > 0 {
		switch args[0] {
		case "list":
			runArchiveList(args[1:])
			return
		case "restore":
			runArchiveRestore(args[1:])
			return
		}
	}
	runArchiveCard(args)
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
	// ArchiveView drops empty buckets, so no groups has two very different
	// causes and only one of them is "the archive is empty". Reporting
	// "nothing archived" for a filter that simply matched nothing would
	// assert something false about the user's data, and would also swallow
	// the match-count line below — leaving no hint that a filter was applied
	// at all. list has the same situation and keeps its summary line.
	if len(groups) == 0 {
		if query != "" {
			fmt.Fprintf(&b, "0 of the newest %d match %q\n", scanned, query)
		} else {
			fmt.Fprintf(&b, "nothing archived on %q\n", name)
		}
		// Card fields come off a board.md that is parsed verbatim, so they may
		// never have met a write-time sanitizer, and this string goes straight
		// to a terminal. Guarding the whole assembled block covers every field
		// at once, including one added later.
		return board.SafeForDisplay(strings.TrimRight(b.String(), "\n"))
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
	return board.SafeForDisplay(strings.TrimRight(b.String(), "\n"))
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
		// Groups is built with make, not left nil: an empty archive is the
		// default state of a fresh board, and a nil slice encodes as null,
		// which breaks `jq '.groups[]'` on exactly the case a script hits
		// first. json.go states this guarantee for every container.
		out := archiveListJSON{
			Board: resolvedName, Filter: query,
			Matched: matched, Scanned: scanned, Total: total,
			Groups: make([]archiveGroupJSON, 0, len(groups)),
		}
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

// archiveArgs parses kando archive <card>'s arguments — "list" and "restore"
// are reserved subcommand words, checked by runArchive before this ever runs.
func archiveArgs(args []string) (card, name string, err error) {
	return cardOnlyArgs("archive", args)
}

// findArchived resolves arg against a: an id first (Archive.Find), then —
// on a miss — a case-insensitive exact title match, mirroring findCard's
// own id-then-title shape and ambiguity handling (ids listed on a tie).
func findArchived(a *board.Archive, arg string) (int, *board.Card, error) {
	if i, c := a.Find(arg); c != nil {
		return i, c, nil
	}
	i, n := -1, 0
	var c *board.Card
	var ids []string
	for idx, card := range a.Cards {
		if strings.EqualFold(card.Title, arg) {
			i, c = idx, card
			ids = append(ids, card.ID)
			n++
		}
	}
	switch n {
	case 0:
		return -1, nil, fmt.Errorf("no archived card matches id or title %q", arg)
	case 1:
		return i, c, nil
	default:
		return -1, nil, fmt.Errorf("%d archived cards match title %q; use one of these ids instead: %s", n, arg, strings.Join(ids, ", "))
	}
}

// archiveCard is kando archive <card>'s testable core: the card must be in
// Done and not already archived — the same two guards the web's archive
// route and the TUI's A key enforce — then Board.ArchiveDone and the
// checked, two-file save. doneAt is returned (not just the input now)
// because ArchiveDone stamps a zero DoneAt to now, and the caller reports
// whichever one actually landed.
func archiveCard(root, name, cardArg string, now time.Time) (title string, doneAt time.Time, err error) {
	st, b, err := openBoard(root, name)
	if err != nil {
		return "", time.Time{}, err
	}
	lane, i, c, err := findCard(b, cardArg)
	if err != nil {
		return "", time.Time{}, err
	}
	if lane != board.Done {
		return "", time.Time{}, fmt.Errorf("%q is in %s, not Done", c.Title, lane)
	}
	a, err := st.LoadArchive()
	if err != nil {
		return "", time.Time{}, err
	}
	// The archive already holding this id means a previous archive half
	// failed after archive.md was written: archiving again would file the
	// card twice. One copy has to be deleted first.
	if _, dup := a.Find(c.ID); dup != nil {
		return "", time.Time{}, fmt.Errorf("%q is already archived", c.Title)
	}
	title = c.Title
	b.ArchiveDone(a, i, now)
	if err := st.SaveArchivalIfUnchanged(b, a); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return "", time.Time{}, errConflict
		}
		return "", time.Time{}, err
	}
	return title, c.DoneAt, nil
}

func runArchiveCard(args []string) {
	// No helpWanted check: runArchive is the only caller and already ran it on
	// this same slice. The two subcommand entry points do re-check, because
	// they receive args[1:] and so can still be handed -h.
	cardArg, name, err := archiveArgs(args)
	if err != nil {
		usageErr(err)
	}
	title, doneAt, err := archiveCard(kandoRoot(), name, cardArg, time.Now())
	if err != nil {
		fatal(err)
	}
	fmt.Printf("archived %q (done %s)\n", title, board.DayLabel(doneAt))
}

// archiveRestoreArgs parses kando archive restore's arguments: the same
// cardOnlyArgs grammar archiveArgs uses.
func archiveRestoreArgs(args []string) (card, name string, err error) {
	return cardOnlyArgs("archive restore", args)
}

// archiveRestore is kando archive restore's testable core: the same
// Board.Restore the TUI's u and the web's restore
// button use, with the same duplicate-id guard the web route enforces
// (internal/web/archive.go): if the card's id is already on the board, a
// previous restore or archive half failed, and one copy has to be deleted
// by hand before this can proceed.
//
// Unlike archiveList this needs the Store method, not the package-level
// LoadArchive: it holds the Store that will do the writing. That method
// persists ids for a hand-written archive.md, and the card has to be resolved
// before any guard can run, so a refused restore may still have canonicalized
// archive.md. Idempotent, but worth knowing — archiveCard avoids it only
// because its lane guard can run before the archive is loaded at all.
func archiveRestore(root, name, cardArg string, now time.Time) (title string, err error) {
	st, b, err := openBoard(root, name)
	if err != nil {
		return "", err
	}
	a, err := st.LoadArchive()
	if err != nil {
		return "", err
	}
	if len(a.Cards) == 0 {
		return "", fmt.Errorf("nothing archived on %q", b.Name)
	}
	i, c, err := findArchived(a, cardArg)
	if err != nil {
		return "", err
	}
	if _, _, dup := b.Find(c.ID); dup != nil {
		return "", fmt.Errorf("%q is already on the board", c.Title)
	}
	title = c.Title
	b.Restore(a, i, now)
	// The checked writer, not SaveRestore: this is a short-lived process that
	// opened its own Store, so a TUI or kando web edit landing since then must
	// be refused rather than silently overwritten — the same rule saveBoard
	// applies to every other mutating verb, and the same one archiveCard
	// applies on the way in.
	if err := st.SaveRestoreIfUnchanged(b, a); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return "", errConflict
		}
		return "", err
	}
	return title, nil
}

func runArchiveRestore(args []string) {
	if helpWanted(args) {
		usage()
		return
	}
	cardArg, name, err := archiveRestoreArgs(args)
	if err != nil {
		usageErr(err)
	}
	title, err := archiveRestore(kandoRoot(), name, cardArg, time.Now())
	if err != nil {
		fatal(err)
	}
	fmt.Printf("restored %q to Doing\n", title)
}
