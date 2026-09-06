package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kaiohenricunha/kando/internal/board"
)

// blockArgs parses kando block's arguments: a card, an optional board, and
// a required --reason. A blank --reason is rejected here rather than
// silently unblocking the card: Card.SetBlocked("") clears the flag, so
// letting a blank reason through would make `kando block x --reason ""`
// secretly do the opposite of what it says.
func blockArgs(args []string, errOut io.Writer) (card, reason, name string, err error) {
	fs := flag.NewFlagSet("block", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() {}
	r := fs.String("reason", "", "why the card is blocked")
	pos, err := parseMixed(fs, args)
	if err != nil {
		return "", "", "", err
	}
	vals, name, err := splitBoard("block", "a card", pos, 1)
	if err != nil {
		return "", "", "", err
	}
	if strings.TrimSpace(*r) == "" {
		return "", "", "", fmt.Errorf("kando block needs --reason")
	}
	return vals[0], *r, name, nil
}

func blockCard(root, name, cardArg, reason string) (title string, err error) {
	_, c, err := withCard(root, name, cardArg, func(b *board.Board, lane board.Lane, i int, c *board.Card) (bool, error) {
		c.SetBlocked(reason)
		return true, nil
	})
	if err != nil {
		return "", err
	}
	return c.Title, nil
}

func runBlock(args []string) {
	if helpWanted(args) {
		usage()
		return
	}
	cardArg, reason, name, err := blockArgs(args, os.Stderr)
	if err != nil {
		usageErr(err)
	}
	title, err := blockCard(kandoRoot(), name, cardArg, reason)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("blocked %q: %s\n", title, reason)
}

// unblockArgs parses kando unblock's arguments: a card and an optional
// board — the plain splitBoard shape, unlike block, since there is no flag.
func unblockArgs(args []string) (card, name string, err error) {
	vals, name, err := splitBoard("unblock", "a card", args, 1)
	if err != nil {
		return "", "", err
	}
	return vals[0], name, nil
}

// unblockCard is SetBlocked(""); wasBlocked lets the caller say whether
// anything actually changed.
func unblockCard(root, name, cardArg string) (title string, wasBlocked bool, err error) {
	_, c, err := withCard(root, name, cardArg, func(b *board.Board, lane board.Lane, i int, c *board.Card) (bool, error) {
		wasBlocked = c.Blocked
		c.SetBlocked("")
		return wasBlocked, nil
	})
	if err != nil {
		return "", false, err
	}
	return c.Title, wasBlocked, nil
}

func runUnblock(args []string) {
	if helpWanted(args) {
		usage()
		return
	}
	cardArg, name, err := unblockArgs(args)
	if err != nil {
		usageErr(err)
	}
	title, wasBlocked, err := unblockCard(kandoRoot(), name, cardArg)
	if err != nil {
		fatal(err)
	}
	if !wasBlocked {
		fmt.Printf("%q was not blocked\n", title)
		return
	}
	fmt.Printf("unblocked %q\n", title)
}
