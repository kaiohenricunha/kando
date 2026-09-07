package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/kaiohenricunha/kando/internal/board"
)

// runChecklist dispatches kando checklist's subcommands.
func runChecklist(args []string) {
	if helpWanted(args) {
		usage()
		return
	}
	if len(args) > 0 {
		switch args[0] {
		case "add":
			runChecklistAdd(args[1:])
			return
		case "toggle":
			runChecklistToggle(args[1:])
			return
		case "edit":
			runChecklistEdit(args[1:])
			return
		}
	}
	usageErr(fmt.Errorf("kando checklist needs add, toggle or edit"))
}

// checklistAddArgs parses kando checklist add's arguments: a card and text,
// both required. splitBoard returns them verbatim, so the two required()
// calls below are what reject a blank one — unlike tag, where an empty
// second positional is a clear-it signal rather than a mistake.
func checklistAddArgs(args []string) (card, text, name string, err error) {
	vals, name, err := splitBoard("checklist add", "a card and text", args, 2)
	if err != nil {
		return "", "", "", err
	}
	if card, err = required(vals[0], "card id or title"); err != nil {
		return "", "", "", err
	}
	if text, err = required(vals[1], "text"); err != nil {
		return "", "", "", err
	}
	return card, text, name, nil
}

// addChecklistItem appends text (InsertChecklistItem at the end, mirroring
// the web's own len(c.Checklist)-1 cursor) and reports its 1-based position.
func addChecklistItem(root, name, cardArg, text string) (title string, n int, err error) {
	_, c, err := withCard(root, name, cardArg, func(b *board.Board, lane board.Lane, i int, c *board.Card) (bool, error) {
		idx := c.InsertChecklistItem(len(c.Checklist)-1, text)
		if idx < 0 {
			return false, fmt.Errorf("text required")
		}
		n = idx + 1
		return true, nil
	})
	if err != nil {
		return "", 0, err
	}
	return c.Title, n, nil
}

func runChecklistAdd(args []string) {
	if helpWanted(args) {
		usage()
		return
	}
	cardArg, text, name, err := checklistAddArgs(args)
	if err != nil {
		usageErr(err)
	}
	title, n, err := addChecklistItem(kandoRoot(), name, cardArg, text)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("added item %d to %q: %s\n", n, title, text)
}

// parseItemNumber validates the CLI's 1-based checklist position.
func parseItemNumber(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("item number must be 1 or more")
	}
	return n, nil
}

// wasGiven reports whether --was was actually passed, as opposed to sitting
// at its empty-string default — an unset --was means "don't check", not
// "check for an empty item".
func wasGiven(fs *flag.FlagSet) bool {
	given := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "was" {
			given = true
		}
	})
	return given
}

// checklistIndex converts a 1-based item number to Board's 0-based index,
// checking range and the optional --was witness — the CLI's version of the
// web's stale-checklist-form guard (internal/web/cards.go, the "was"
// field): items have positions, not ids, so a script that read the card
// earlier can assert what it expects to still be there.
func checklistIndex(c *board.Card, n int, was *string) (int, error) {
	if n < 1 || n > len(c.Checklist) {
		return 0, fmt.Errorf("no checklist item %d on %q (it has %d)", n, c.Title, len(c.Checklist))
	}
	i := n - 1
	if was != nil && *was != c.Checklist[i].Text {
		return 0, fmt.Errorf("item %d is now %q, not %q — the card changed; check and try again", n, c.Checklist[i].Text, *was)
	}
	return i, nil
}

func checklistToggleArgs(args []string, errOut io.Writer) (card string, n int, was *string, name string, err error) {
	fs := flag.NewFlagSet("checklist toggle", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() {}
	wasFlag := fs.String("was", "", "refuse unless the item currently reads this text")
	pos, err := parseMixed(fs, args)
	if err != nil {
		return "", 0, nil, "", err
	}
	vals, name, err := splitBoard("checklist toggle", "a card and an item number", pos, 2)
	if err != nil {
		return "", 0, nil, "", err
	}
	if card, err = required(vals[0], "card id or title"); err != nil {
		return "", 0, nil, "", err
	}
	n, err = parseItemNumber(vals[1])
	if err != nil {
		return "", 0, nil, "", err
	}
	if wasGiven(fs) {
		was = wasFlag
	}
	return card, n, was, name, nil
}

func toggleChecklistItem(root, name, cardArg string, n int, was *string) (title string, item board.Item, err error) {
	_, c, err := withCard(root, name, cardArg, func(b *board.Board, lane board.Lane, i int, c *board.Card) (bool, error) {
		idx, err := checklistIndex(c, n, was)
		if err != nil {
			return false, err
		}
		c.ToggleChecklistItem(idx)
		item = c.Checklist[idx]
		return true, nil
	})
	if err != nil {
		return "", board.Item{}, err
	}
	return c.Title, item, nil
}

func runChecklistToggle(args []string) {
	if helpWanted(args) {
		usage()
		return
	}
	cardArg, n, was, name, err := checklistToggleArgs(args, os.Stderr)
	if err != nil {
		usageErr(err)
	}
	title, item, err := toggleChecklistItem(kandoRoot(), name, cardArg, n, was)
	if err != nil {
		fatal(err)
	}
	state := "ticked"
	if !item.Done {
		state = "unticked"
	}
	fmt.Printf("%s item %d on %q: %s\n", state, n, title, item.Text)
}

func checklistEditArgs(args []string, errOut io.Writer) (card string, n int, text string, was *string, name string, err error) {
	fs := flag.NewFlagSet("checklist edit", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() {}
	wasFlag := fs.String("was", "", "refuse unless the item currently reads this text")
	pos, err := parseMixed(fs, args)
	if err != nil {
		return "", 0, "", nil, "", err
	}
	vals, name, err := splitBoard("checklist edit", "a card, an item number and text", pos, 3)
	if err != nil {
		return "", 0, "", nil, "", err
	}
	if card, err = required(vals[0], "card id or title"); err != nil {
		return "", 0, "", nil, "", err
	}
	n, err = parseItemNumber(vals[1])
	if err != nil {
		return "", 0, "", nil, "", err
	}
	if text, err = required(vals[2], "text"); err != nil {
		return "", 0, "", nil, "", err
	}
	if wasGiven(fs) {
		was = wasFlag
	}
	return card, n, text, was, name, nil
}

func editChecklistItem(root, name, cardArg string, n int, text string, was *string) (title string, item board.Item, err error) {
	_, c, err := withCard(root, name, cardArg, func(b *board.Board, lane board.Lane, i int, c *board.Card) (bool, error) {
		idx, err := checklistIndex(c, n, was)
		if err != nil {
			return false, err
		}
		// The comparison has to happen after the set, not before:
		// SetChecklistItemText sanitizes, so the only honest way to know
		// whether anything moved is to look at what actually landed.
		before := c.Checklist[idx].Text
		if !c.SetChecklistItemText(idx, text) {
			return false, fmt.Errorf("text required")
		}
		item = c.Checklist[idx]
		return item.Text != before, nil
	})
	if err != nil {
		return "", board.Item{}, err
	}
	return c.Title, item, nil
}

func runChecklistEdit(args []string) {
	if helpWanted(args) {
		usage()
		return
	}
	cardArg, n, text, was, name, err := checklistEditArgs(args, os.Stderr)
	if err != nil {
		usageErr(err)
	}
	title, item, err := editChecklistItem(kandoRoot(), name, cardArg, n, text, was)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("edited item %d on %q: %s\n", n, title, item.Text)
}
