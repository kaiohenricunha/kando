package board

import (
	"slices"
	"strings"
)

// These mutation helpers are the single place tag/notes/blocked/checklist
// edits happen, so the TUI and the future web page always apply the exact
// same rules for a given edit, not just visually agree (BOUND-1,
// docs/specs/kando-web/spec/2-scope.md).

// SetTag sets the card's tag, trimming whitespace and a leading "#" if present.
func (c *Card) SetTag(value string) {
	c.Tag = strings.TrimPrefix(strings.TrimSpace(value), "#")
}

// SetNotes sets the card's notes, trimming trailing blank lines and spaces.
func (c *Card) SetNotes(value string) {
	c.Notes = strings.TrimRight(value, "\n ")
}

// SetBlocked sets the card's blocked reason; a reason that's empty after
// trimming clears the blocked flag entirely.
func (c *Card) SetBlocked(reason string) {
	reason = strings.TrimSpace(reason)
	c.Blocked = reason != ""
	c.BlockedReason = reason
}

// InsertChecklistItem inserts a trimmed, non-empty item right after cursor
// (or at index 0 if the checklist is empty) and returns the new item's
// index. An empty (after trimming) text is a no-op that returns -1.
func (c *Card) InsertChecklistItem(cursor int, text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return -1
	}
	at := 0
	if len(c.Checklist) > 0 {
		at = cursor + 1
	}
	c.Checklist = slices.Insert(c.Checklist, at, Item{Text: text})
	return at
}

// ToggleChecklistItem flips the Done flag of the item at index i; a no-op if
// i is out of range.
func (c *Card) ToggleChecklistItem(i int) {
	if i >= 0 && i < len(c.Checklist) {
		c.Checklist[i].Done = !c.Checklist[i].Done
	}
}

// SetChecklistItemText replaces the text of the item at index i with a
// trimmed value, reporting whether it did. A no-op (false) if the trimmed
// text is empty or i is out of range; Done is left untouched either way.
func (c *Card) SetChecklistItemText(i int, text string) bool {
	text = strings.TrimSpace(text)
	if text == "" || i < 0 || i >= len(c.Checklist) {
		return false
	}
	c.Checklist[i].Text = text
	return true
}

// DeleteCard permanently removes the card at index i of lane l — unlike
// Move, it is not reinserted anywhere. Returns the removed card, or nil if
// the index was invalid.
func (b *Board) DeleteCard(l Lane, i int) *Card {
	return b.Remove(l, i)
}
