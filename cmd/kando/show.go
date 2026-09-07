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

// showArgs parses kando show's arguments: a card, an optional board, and an
// optional --json.
func showArgs(args []string, errOut io.Writer) (card, name string, jsonOut bool, err error) {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() {}
	j := fs.Bool("json", false, "emit JSON")
	pos, err := parseMixed(fs, args)
	if err != nil {
		return "", "", false, err
	}
	vals, name, err := splitBoard("show", "a card", pos, 1)
	if err != nil {
		return "", "", false, err
	}
	// splitBoard hands required values back verbatim; judging them is the
	// verb's job, so that the message names what is wrong rather than
	// repeating the too-few-arguments text. Same rule as moveArgs.
	card = strings.TrimSpace(vals[0])
	if card == "" {
		return "", "", false, fmt.Errorf("card id or title required")
	}
	return card, name, *j, nil
}

// showCard resolves cardArg against a read-only load of the board: show
// must never rewrite board.md just to display it.
func showCard(root, name, cardArg string) (board.Lane, *board.Card, error) {
	b, err := readBoard(root, name)
	if err != nil {
		return 0, nil, err
	}
	lane, _, c, err := findCard(b, cardArg)
	if err != nil {
		return 0, nil, err
	}
	return lane, c, nil
}

// formatCard renders c as kando show's plain-text output: every field kando
// tracks, but only the sections that actually hold something.
func formatCard(lane board.Lane, c *board.Card, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", c.Title)
	fmt.Fprintf(&b, "lane: %s\n", lane)
	if c.Tag != "" {
		fmt.Fprintf(&b, "tag: #%s\n", c.Tag)
	}
	fmt.Fprintf(&b, "id: %s\n", c.ID)
	if !c.CreatedAt.IsZero() {
		fmt.Fprintf(&b, "created: %s\n", board.DayLabel(c.CreatedAt))
	}
	if !c.MovedAt.IsZero() {
		fmt.Fprintf(&b, "moved: %s\n", board.DayLabel(c.MovedAt))
	}
	if !c.DoneAt.IsZero() {
		fmt.Fprintf(&b, "done: %s\n", board.DayLabel(c.DoneAt))
	}
	if age := c.AgeSince(); !age.IsZero() {
		fmt.Fprintf(&b, "age: %s\n", board.Age(now, age))
	}
	if label := c.BlockedLabel(); label != "" {
		fmt.Fprintf(&b, "blocked: %s\n", label)
	}
	if c.Notes != "" {
		fmt.Fprintf(&b, "notes:\n")
		for _, line := range strings.Split(c.Notes, "\n") {
			fmt.Fprintf(&b, "  %s\n", line)
		}
	}
	if len(c.Checklist) > 0 {
		fmt.Fprintf(&b, "checklist %s:\n", c.ProgressLabel())
		for i, it := range c.Checklist {
			mark := " "
			if it.Done {
				mark = "x"
			}
			fmt.Fprintf(&b, "  %d. [%s] %s\n", i+1, mark, it.Text)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func runShow(args []string) {
	if helpWanted(args) {
		usage()
		return
	}
	cardArg, name, jsonOut, err := showArgs(args, os.Stderr)
	if err != nil {
		usageErr(err)
	}
	now := time.Now()
	lane, c, err := showCard(kandoRoot(), name, cardArg)
	if err != nil {
		fatal(err)
	}
	if jsonOut {
		if err := writeJSON(os.Stdout, cardToJSON(lane, c, now)); err != nil {
			fatal(err)
		}
		return
	}
	fmt.Println(formatCard(lane, c, now))
}
