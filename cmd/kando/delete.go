package main

import (
	"fmt"

	"github.com/kaiohenricunha/kando/internal/board"
)

// deleteArgs parses kando delete's arguments: a card and an optional board.
func deleteArgs(args []string) (card, name string, err error) {
	vals, name, err := splitBoard("delete", "a card", args, 1)
	if err != nil {
		return "", "", err
	}
	card, err = required(vals[0], "card id or title")
	if err != nil {
		return "", "", err
	}
	return card, name, nil
}

// deleteCard is kando delete's testable core: immediate, no confirmation —
// no surface asks for one.
func deleteCard(root, name, cardArg string) (title string, lane board.Lane, err error) {
	lane, c, err := withCard(root, name, cardArg, func(b *board.Board, lane board.Lane, i int, c *board.Card) (bool, error) {
		b.DeleteCard(lane, i)
		return true, nil
	})
	if err != nil {
		return "", 0, err
	}
	return c.Title, lane, nil
}

func runDelete(args []string) {
	if helpWanted(args) {
		usage()
		return
	}
	cardArg, name, err := deleteArgs(args)
	if err != nil {
		usageErr(err)
	}
	title, lane, err := deleteCard(kandoRoot(), name, cardArg)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("deleted %q from %s\n", title, lane)
}
