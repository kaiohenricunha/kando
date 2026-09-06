package main

import (
	"fmt"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
)

// moveArgs parses kando move's purely positional arguments (no flags):
// splitBoard's shared grammar with a card and a lane as the two required
// values.
func moveArgs(args []string) (card, lane, name string, err error) {
	vals, name, err := splitBoard("move", "a card and a lane", args, 2)
	if err != nil {
		return "", "", "", err
	}
	return vals[0], vals[1], name, nil
}

// moveCard is kando move's testable core, built on withCard: findCard
// resolves cardArg, and the card moves to lane `to` and saves only if it
// actually changed lane. now is threaded through explicitly, matching
// Board.Move's own signature and every other mutating board function, so
// tests can assert exact stamps deterministically.
//
// A same-lane move skips both the mutation and the save — Board.Move
// already treats it as a no-op, and saving anyway would spuriously bump the
// board's on-disk version for every connected kando web client.
//
// to must already be a value from board.ParseLane (0-3): Board.Move indexes
// a fixed [4][]*Card array with it directly and will panic on an
// out-of-range Lane, exactly as the TUI or the web would if either skipped
// ParseLane.
func moveCard(root, name, cardArg string, to board.Lane, now time.Time) (title string, from board.Lane, err error) {
	lane, c, err := withCard(root, name, cardArg, func(b *board.Board, lane board.Lane, i int, c *board.Card) (bool, error) {
		if lane == to {
			return false, nil
		}
		if b.Move(lane, i, to, now) < 0 {
			return false, fmt.Errorf("card %q could not be moved", c.Title)
		}
		return true, nil
	})
	if err != nil {
		return "", 0, err
	}
	return c.Title, lane, nil
}

// runMove is kando move's process-level wrapper: parse, validate the lane,
// mutate, report one line of confirmation to stdout.
func runMove(args []string) {
	if helpWanted(args) {
		usage()
		return
	}
	cardArg, laneArg, name, err := moveArgs(args)
	if err != nil {
		usageErr(err)
	}
	to, ok := board.ParseLane(laneArg)
	if !ok {
		usageErr(fmt.Errorf("invalid lane %q", laneArg))
	}
	title, from, err := moveCard(kandoRoot(), name, cardArg, to, time.Now())
	if err != nil {
		fatal(err)
	}
	if from == to {
		fmt.Printf("%q is already in %s\n", title, to)
		return
	}
	fmt.Printf("moved %q: %s -> %s\n", title, from, to)
}
