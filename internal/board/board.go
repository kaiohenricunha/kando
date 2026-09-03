// Package board is the kando domain model: lanes, cards, ages and filters.
// It has no I/O and no UI dependencies.
package board

import (
	"strings"
	"time"
)

// Lane is one of the four fixed board lanes.
type Lane int

// The lanes in their fixed on-screen and on-disk order.
const (
	Backlog Lane = iota
	Todo
	Doing
	Done
)

// Lanes lists every lane in order.
var Lanes = [4]Lane{Backlog, Todo, Doing, Done}

var laneNames = [4]string{"Backlog", "Todo", "Doing", "Done"}

// String returns the lane's display name ("Backlog", "Todo", "Doing", "Done").
func (l Lane) String() string {
	if l < 0 || int(l) >= len(laneNames) {
		return "?"
	}
	return laneNames[l]
}

// ParseLane maps a display name (case-insensitive) back to a Lane.
func ParseLane(s string) (Lane, bool) {
	for i, n := range laneNames {
		if strings.EqualFold(strings.TrimSpace(s), n) {
			return Lane(i), true
		}
	}
	return 0, false
}

// Item is one checklist entry.
type Item struct {
	Text string
	Done bool
}

// Card is a single kanban card. IDs are short random strings never shown to the user.
type Card struct {
	ID            string
	Title         string
	Notes         string
	Tag           string
	Checklist     []Item
	Blocked       bool
	BlockedReason string
	CreatedAt     time.Time
	MovedAt       time.Time
	DoneAt        time.Time
}

// FirstNoteLine returns the first line of the notes, or "".
func (c *Card) FirstNoteLine() string {
	if i := strings.IndexByte(c.Notes, '\n'); i >= 0 {
		return c.Notes[:i]
	}
	return c.Notes
}

// ChecklistProgress returns (done, total) checklist counts.
func (c *Card) ChecklistProgress() (done, total int) {
	for _, it := range c.Checklist {
		if it.Done {
			done++
		}
	}
	return done, len(c.Checklist)
}

// AgeSince is the instant ages are measured from: DoneAt for done cards, else CreatedAt.
func (c *Card) AgeSince() time.Time {
	if !c.DoneAt.IsZero() {
		return c.DoneAt
	}
	return c.CreatedAt
}

// Board holds the four lanes of one named board.
type Board struct {
	Name  string
	Lanes [4][]*Card
}

// Count is the number of cards across all four lanes.
func (b *Board) Count() int {
	n := 0
	for _, l := range b.Lanes {
		n += len(l)
	}
	return n
}

// Find locates a card by id.
func (b *Board) Find(id string) (Lane, int, *Card) {
	for _, l := range Lanes {
		for i, c := range b.Lanes[l] {
			if c.ID == id {
				return l, i, c
			}
		}
	}
	return 0, -1, nil
}

// Insert places c at index i of lane l (clamped).
func (b *Board) Insert(l Lane, i int, c *Card) {
	cards := b.Lanes[l]
	if i < 0 {
		i = 0
	}
	if i > len(cards) {
		i = len(cards)
	}
	cards = append(cards, nil)
	copy(cards[i+1:], cards[i:])
	cards[i] = c
	b.Lanes[l] = cards
}

// Remove takes the card at index i out of lane l and returns it (nil if out of range).
func (b *Board) Remove(l Lane, i int) *Card {
	cards := b.Lanes[l]
	if i < 0 || i >= len(cards) {
		return nil
	}
	c := cards[i]
	b.Lanes[l] = append(cards[:i], cards[i+1:]...)
	return c
}

// Move takes the card at from[i], stamps MovedAt (and DoneAt when entering Done,
// clearing it when leaving), inserts it at the top of the destination lane and
// returns its new index (0), or -1 if the source index was invalid.
func (b *Board) Move(from Lane, i int, to Lane, now time.Time) int {
	c := b.Remove(from, i)
	if c == nil {
		return -1
	}
	c.MovedAt = now
	if to == Done {
		c.DoneAt = now
	} else {
		c.DoneAt = time.Time{}
	}
	b.Insert(to, 0, c)
	return 0
}

// Archive is the list of cards moved out of Done, newest DoneAt first.
type Archive struct {
	Cards []*Card
}

// Remove deletes the card at index i and returns it.
func (a *Archive) Remove(i int) *Card {
	if i < 0 || i >= len(a.Cards) {
		return nil
	}
	c := a.Cards[i]
	a.Cards = append(a.Cards[:i], a.Cards[i+1:]...)
	return c
}
