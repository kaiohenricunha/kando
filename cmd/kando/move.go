package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

// moveArgs parses kando move's purely positional arguments (no flags),
// separated from execution so argument shape is testable without touching
// the filesystem — mirroring webArgs's split of parsing from serving.
func moveArgs(args []string) (card, lane, name string, err error) {
	switch len(args) {
	case 0, 1:
		return "", "", "", fmt.Errorf("kando move needs a card and a lane")
	case 2:
		card, lane = args[0], args[1]
	case 3:
		card, lane, name = args[0], args[1], args[2]
		if strings.HasPrefix(name, "-") {
			return "", "", "", fmt.Errorf("unexpected flag %q", name)
		}
	default:
		return "", "", "", fmt.Errorf("unexpected argument %q", args[3])
	}
	card = strings.TrimSpace(card)
	if card == "" {
		return "", "", "", fmt.Errorf("card id or title required")
	}
	return card, lane, name, nil
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
	for _, lane := range board.Lanes {
		for idx, card := range b.Lanes[lane] {
			if strings.EqualFold(card.Title, arg) {
				l, i, c = lane, idx, card
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
		return 0, -1, nil, fmt.Errorf("%d cards match title %q; use the card's id instead", n, arg)
	}
}

// moveCard is kando move's testable core: root/name identify the board
// (name defaults to "life"), cardArg resolves to a card via findCard, and it
// is moved to lane `to` and saved. now is threaded through explicitly,
// matching Board.Move's own signature and every other mutating board
// function, so tests can assert exact stamps deterministically.
//
// Unlike store.Open (used by the bare `kando <board>` and `kando web
// <board>` forms), a nonexistent board is a hard error, not something this
// creates: store.Exists is checked first deliberately, because move
// requires an existing card, so auto-creating an empty board would only
// turn one clear error into a confusing two-step one.
//
// to must already be a value from board.ParseLane (0-3): Board.Move/Insert
// index b.Lanes (a fixed [4][]*Card array) with it directly and will panic
// on an out-of-range Lane, exactly as they would for the TUI or the web if
// either skipped ParseLane.
func moveCard(root, name, cardArg string, to board.Lane, now time.Time) (title string, from board.Lane, err error) {
	if name == "" {
		name = "life"
	}
	if !store.ValidBoardName(name) {
		return "", 0, fmt.Errorf("invalid board name %q", name)
	}
	if !store.Exists(root, name) {
		return "", 0, fmt.Errorf("no such board %q (create it first with \"kando %s\" or \"kando web %s\")", name, name, name)
	}
	st, b, err := store.Open(root, name)
	if err != nil {
		return "", 0, err
	}
	from, i, c, err := findCard(b, cardArg)
	if err != nil {
		return "", 0, err
	}
	title = c.Title
	b.Move(from, i, to, now)
	if err := st.SaveBoard(b); err != nil {
		return "", 0, err
	}
	return title, from, nil
}

// runMove is kando move's process-level wrapper: parse, validate the lane,
// mutate, report one line of confirmation to stdout.
func runMove(args []string) {
	cardArg, laneArg, name, err := moveArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "kando:", err)
		usage()
		os.Exit(2)
	}
	to, ok := board.ParseLane(laneArg)
	if !ok {
		fmt.Fprintf(os.Stderr, "kando: invalid lane %q\n", laneArg)
		usage()
		os.Exit(2)
	}
	title, from, err := moveCard(kandoRoot(), name, cardArg, to, time.Now())
	if err != nil {
		fatal(err)
	}
	fmt.Printf("moved %q: %s -> %s\n", title, from, to)
}
