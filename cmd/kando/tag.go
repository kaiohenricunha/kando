package main

import (
	"fmt"
	"strings"

	"github.com/kaiohenricunha/kando/internal/board"
)

// tagArgs parses kando tag's arguments by hand rather than through
// splitBoard: the tag positional is allowed to be empty (that's how a
// script clears it), which splitBoard's blanket non-empty rule on every
// required value does not support — only the card is ever required to be
// non-blank here.
func tagArgs(args []string) (card, tag, name string, err error) {
	switch len(args) {
	case 0, 1:
		return "", "", "", fmt.Errorf("kando tag needs a card and a tag")
	case 2:
		card, tag = args[0], args[1]
	case 3:
		card, tag, name = args[0], args[1], args[2]
		if strings.HasPrefix(name, "-") {
			return "", "", "", fmt.Errorf("unexpected flag %q", name)
		}
		if strings.TrimSpace(name) == "" {
			return "", "", "", fmt.Errorf("board name required")
		}
	default:
		return "", "", "", fmt.Errorf("unexpected argument %q", args[3])
	}
	card = strings.TrimSpace(card)
	if card == "" {
		return "", "", "", fmt.Errorf("card id or title required")
	}
	return card, tag, name, nil
}

// tagCard is kando tag's testable core: SetTag strips a leading "#" and an
// empty value clears the tag entirely.
func tagCard(root, name, cardArg, tag string) (title, newTag string, err error) {
	_, c, err := withCard(root, name, cardArg, func(b *board.Board, lane board.Lane, i int, c *board.Card) (bool, error) {
		before := c.Tag
		c.SetTag(tag)
		return c.Tag != before, nil
	})
	if err != nil {
		return "", "", err
	}
	return c.Title, c.Tag, nil
}

func runTag(args []string) {
	if helpWanted(args) {
		usage()
		return
	}
	cardArg, tag, name, err := tagArgs(args)
	if err != nil {
		usageErr(err)
	}
	title, newTag, err := tagCard(kandoRoot(), name, cardArg, tag)
	if err != nil {
		fatal(err)
	}
	if newTag == "" {
		fmt.Printf("cleared tag on %q\n", title)
		return
	}
	fmt.Printf("tagged %q #%s\n", title, newTag)
}
