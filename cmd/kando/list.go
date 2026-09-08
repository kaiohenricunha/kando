package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
)

// listArgs parses kando list's arguments: an optional board, an optional
// --filter (same syntax as the TUI's /), and an optional --json.
func listArgs(args []string, errOut io.Writer) (name, query string, jsonOut bool, err error) {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
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
	// De-fanged at the entry point, so board.Parse, the %q echo and the JSON
	// Filter field all see one string. Guarding only the echoed copy would
	// report a filter that is not the one that ran.
	return name, board.SafeForDisplay(*f), *j, nil
}

// listCards is kando list's testable core: a read-only load (list must
// never rewrite board.md), then Board.Matching over the parsed filter. b is
// returned so the caller can read its resolved Name (readBoard/store.Load
// already default "" to "life").
func listCards(root, name, query string, now time.Time) (b *board.Board, lanes [4][]*board.Card, matched, total int, err error) {
	b, err = readBoard(root, name)
	if err != nil {
		return nil, lanes, 0, 0, err
	}
	lanes, matched, total = b.Matching(board.Parse(query), now)
	return b, lanes, matched, total, nil
}

// listRow is one card's line in kando list's plain-text output: id, title,
// and only the fields that have something to show.
func listRow(c *board.Card, now time.Time) string {
	parts := []string{c.ID, c.Title}
	if c.Tag != "" {
		parts = append(parts, "#"+c.Tag)
	}
	if len(c.Checklist) > 0 {
		parts = append(parts, c.ProgressLabel())
	}
	if label := c.BlockedLabel(); label != "" {
		parts = append(parts, "blocked: "+label)
	}
	// A hand-written card with no created: (and no done:) has a zero
	// AgeSince, which would otherwise report an age of several thousand
	// years — omit it rather than show that.
	if age := c.AgeSince(); !age.IsZero() {
		parts = append(parts, board.Age(now, age))
	}
	return strings.Join(parts, "  ")
}

func formatList(lanes [4][]*board.Card, query string, matched, total int, now time.Time) string {
	var b strings.Builder
	for _, l := range board.Lanes {
		fmt.Fprintf(&b, "%s (%d)\n", l, len(lanes[l]))
		for _, c := range lanes[l] {
			fmt.Fprintf(&b, "  %s\n", listRow(c, now))
		}
	}
	if query != "" {
		fmt.Fprintf(&b, "%d of %d cards match %q\n", matched, total, query)
	}
	// Card fields come off a board.md that is parsed verbatim, so they may
	// never have met a write-time sanitizer, and this string goes straight
	// to a terminal. Guarding the whole assembled block covers every field
	// at once, including one added later.
	return board.SafeForDisplay(strings.TrimRight(b.String(), "\n"))
}

func runList(args []string) {
	if helpWanted(args) {
		usage()
		return
	}
	name, query, jsonOut, err := listArgs(args, os.Stderr)
	if err != nil {
		usageErr(err)
	}
	now := time.Now()
	b, lanes, matched, total, err := listCards(kandoRoot(), name, query, now)
	if err != nil {
		fatal(err)
	}
	if jsonOut {
		out := listJSON{Board: b.Name, Filter: query, Matched: matched, Total: total}
		for _, l := range board.Lanes {
			cj := make([]cardJSON, len(lanes[l]))
			for i, c := range lanes[l] {
				cj[i] = cardToJSON(l, c, now)
			}
			out.Lanes = append(out.Lanes, laneJSON{Lane: l.String(), Cards: cj})
		}
		if err := writeJSON(os.Stdout, out); err != nil {
			fatal(err)
		}
		return
	}
	fmt.Println(formatList(lanes, query, matched, total, now))
}
