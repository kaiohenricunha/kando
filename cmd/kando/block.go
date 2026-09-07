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
	if card, err = required(vals[0], "card id or title"); err != nil {
		return "", "", "", err
	}
	if strings.TrimSpace(*r) == "" {
		return "", "", "", fmt.Errorf("kando block needs --reason")
	}
	return card, *r, name, nil
}

// blockCard reports changed only when the flag or the reason actually moved.
// SetBlocked is a plain assignment, so re-running `kando block X --reason R`
// with R already set produces byte-identical output — and saving it anyway
// would rewrite board.md and bump store.Version, waking every connected
// kando web client for nothing. This is the verb most likely to be re-run
// unconditionally by a script, so the guard earns its place.
func blockCard(root, name, cardArg, reason string) (title string, err error) {
	_, c, err := withCard(root, name, cardArg, func(b *board.Board, lane board.Lane, i int, c *board.Card) (bool, error) {
		wasBlocked, before := c.Blocked, c.BlockedReason
		c.SetBlocked(reason)
		return c.Blocked != wasBlocked || c.BlockedReason != before, nil
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
	fmt.Printf("blocked %q: %s\n", title, board.SafeForDisplay(reason))
}

// unblockArgs parses kando unblock's arguments: a card and an optional
// board — the plain splitBoard shape, unlike block, since there is no flag.
// splitBoard validates nothing about the card itself, so required() is what
// makes a blank one an argument error rather than a lookup that reaches the
// board and, against a hand-edited untitled card, matches it.
func unblockArgs(args []string) (card, name string, err error) {
	vals, name, err := splitBoard("unblock", "a card", args, 1)
	if err != nil {
		return "", "", err
	}
	if card, err = required(vals[0], "card id or title"); err != nil {
		return "", "", err
	}
	return card, name, nil
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
