package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
)

// addArgs parses kando add's arguments: a title, an optional board, and
// optional --lane/--tag flags.
func addArgs(args []string, errOut io.Writer) (title, name string, lane board.Lane, tag string, err error) {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() {}
	laneFlag := fs.String("lane", "Todo", "lane to add the card to")
	tagFlag := fs.String("tag", "", "tag for the new card")
	pos, err := parseMixed(fs, args)
	if err != nil {
		return "", "", 0, "", err
	}
	vals, name, err := splitBoard("add", "a title", pos, 1)
	if err != nil {
		return "", "", 0, "", err
	}
	title, err = required(vals[0], "title")
	if err != nil {
		return "", "", 0, "", err
	}
	l, ok := board.ParseLane(*laneFlag)
	if !ok {
		return "", "", 0, "", fmt.Errorf("invalid lane %q", *laneFlag)
	}
	return title, name, l, *tagFlag, nil
}

// addCard is kando add's testable core: a new card lands at the top of
// lane, exactly as the TUI's a and the web's create-card form do.
// --lane done stamps DoneAt through NewCard, the same as a move into Done.
func addCard(root, name, title string, lane board.Lane, tag string, now time.Time) (*board.Card, error) {
	st, b, err := openBoard(root, name)
	if err != nil {
		return nil, err
	}
	c := board.NewCard(title, lane, now)
	if c == nil {
		return nil, fmt.Errorf("title required")
	}
	if tag != "" {
		c.SetTag(tag)
	}
	b.Insert(lane, 0, c)
	if err := saveBoard(st, b); err != nil {
		return nil, err
	}
	return c, nil
}

func runAdd(args []string) {
	if helpWanted(args) {
		usage()
		return
	}
	title, name, lane, tag, err := addArgs(args, os.Stderr)
	if err != nil {
		usageErr(err)
	}
	c, err := addCard(kandoRoot(), name, title, lane, tag, time.Now())
	if err != nil {
		fatal(err)
	}
	fmt.Printf("added %q to %s (%s)\n", c.Title, lane, c.ID)
}
