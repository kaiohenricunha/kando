// Package board is the kando domain model: lanes, cards, ages and filters.
// It has no I/O and no UI dependencies.
package board

import (
	"fmt"
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

// Key is the lane's lower-case name as used in URLs and forms; ParseLane
// accepts it back.
func (l Lane) Key() string { return strings.ToLower(l.String()) }

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

// NotePreview returns the first line of the notes, clipped, or "".
//
// The name is the contract: callers get a preview, not the field. This is the
// only read accessor in the package that bounds what it returns, so a consumer
// that wanted the stored value — a JSON projection, an export — would be
// getting a quietly shortened one. Nothing needs that today; both callers are
// presentation, and if one ever does need the full value it should read
// c.Notes rather than this.
//
// It is a preview — the TUI puts it in one card row and the web in one
// clipped div — so it is bounded to the single-line budget every other
// one-line value already uses. Without that bound it would return the whole
// body — and the real ceiling is not maxNotesBytes but nothing at all, since
// store.parseSections assigns Notes verbatim with no cap, so a hand-edited
// board.md can hold any size. That went into a row that renders a few dozen
// cells, on every frame and in every board page.
func (c *Card) NotePreview() string {
	if i := strings.IndexByte(c.Notes, '\n'); i >= 0 {
		return clip(c.Notes[:i], maxFieldBytes)
	}
	return clip(c.Notes, maxFieldBytes)
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

// ProgressLabel is "done/total" for the checklist, or "" when there is none.
// Both renderers show exactly this.
func (c *Card) ProgressLabel() string {
	done, total := c.ChecklistProgress()
	if total == 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d", done, total)
}

// BlockedLabel is the reason a blocked card shows ("blocked" when none was
// given), or "" when the card is not blocked. Both renderers show exactly this.
func (c *Card) BlockedLabel() string {
	if !c.Blocked {
		return ""
	}
	if c.BlockedReason == "" {
		return "blocked"
	}
	return c.BlockedReason
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

// remove takes the card at index i out of lane l and returns it (nil if out of
// range). Move and DeleteCard are the callers; DeleteCard is the exported entry.
func (b *Board) remove(l Lane, i int) *Card {
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
	if from == to {
		// Moving a card to the lane it is already in is a no-op, not a
		// reshuffle: it must not jump the card to the top or restamp its
		// dates. The web's lane form defaults to the current lane, so this
		// is one stray click away.
		if i < 0 || i >= len(b.Lanes[from]) {
			return -1
		}
		return i
	}
	c := b.remove(from, i)
	if c == nil {
		return -1
	}
	stamp(c, to, now)
	b.Insert(to, 0, c)
	return 0
}

// MoveAt places the card at from[i] into lane `to` at position `at`, where
// Move always lands a card on top. `at` is insert-before against the
// destination lane as it stood before the move: the card ends up in front of
// whatever sat at `at`, and at == len(lane) appends. A cross-lane move stamps
// the card exactly as Move does; a move inside one lane is a reorder, not a
// lane change, so MovedAt and DoneAt are left alone. Returns the card's new
// index, or -1 if the source index was invalid.
//
// Move is the top-insert the TUI's H/L/d and the web's lane picker use, and
// its same-lane case is a deliberate no-op; this is the positional form the
// web page's drag-and-drop needs.
func (b *Board) MoveAt(from Lane, i int, to Lane, at int, now time.Time) int {
	if i < 0 || i >= len(b.Lanes[from]) {
		return -1
	}
	// Removing from the same lane shifts everything below i up one, so a
	// position measured before the removal is one too far. The same
	// correction makes both ways of dropping a card on itself (at == i and
	// at == i+1) fall out as no-ops, with no special case for either.
	dst := at
	if from == to && at > i {
		dst--
	}
	c := b.remove(from, i)
	if from != to {
		stamp(c, to, now)
	}
	if dst < 0 {
		dst = 0
	}
	if n := len(b.Lanes[to]); dst > n {
		dst = n
	}
	b.Insert(to, dst, c)
	return dst
}

// MoveBefore places the card at from[i] immediately above the card at index
// anchor of lane to, where anchor indexes that lane as it stood before the
// move. It is MoveAt named for what it does: the TUI's K and the web's
// pos=before both mean it.
func (b *Board) MoveBefore(from Lane, i int, to Lane, anchor int, now time.Time) int {
	return b.MoveAt(from, i, to, anchor, now)
}

// MoveAfter places the card at from[i] immediately below the card at index
// anchor of lane to. Below a card is in front of whatever follows it, so this is
// MoveAt at anchor+1 — the step both surfaces used to re-derive, and the one
// that reads wrong at a glance: moving a card one slot down is anchor+1, not
// i+1, because i+1 names the position the card already holds.
func (b *Board) MoveAfter(from Lane, i int, to Lane, anchor int, now time.Time) int {
	return b.MoveAt(from, i, to, anchor+1, now)
}

// stamp records a card's arrival in lane to at now: the one rule for what a
// lane change does to the dates, shared by Move and Unarchive.
func stamp(c *Card, to Lane, now time.Time) {
	c.MovedAt = now
	if to == Done {
		c.DoneAt = now
	} else {
		c.DoneAt = time.Time{}
	}
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

// Insert adds c keeping the archive newest-DoneAt-first: before the first
// card whose DoneAt is not after c's, so a card done today lands at index 0
// and a tie goes ahead of what was already there. Returns c's index. The
// order is functional, not cosmetic: ArchiveView takes a.Cards[:ArchiveMax]
// without sorting, so an appended card would vanish from both archive
// screens once the archive held ArchiveMax entries.
func (a *Archive) Insert(c *Card) int {
	i := 0
	for i < len(a.Cards) && a.Cards[i].DoneAt.After(c.DoneAt) {
		i++
	}
	a.Cards = append(a.Cards, nil)
	copy(a.Cards[i+1:], a.Cards[i:])
	a.Cards[i] = c
	return i
}

// Find locates an archived card by id, as Board.Find does on the board:
// (index, card), or (-1, nil) when no archived card has that id.
func (a *Archive) Find(id string) (int, *Card) {
	for i, c := range a.Cards {
		if c.ID == id {
			return i, c
		}
	}
	return -1, nil
}
