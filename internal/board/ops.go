package board

import (
	"slices"
	"strings"
	"time"
	"unicode"
)

// These mutation helpers are the single place title/tag/notes/blocked/
// checklist edits and card creation happen, so the TUI and the future web
// page always apply the exact same rules for a given edit, not just
// visually agree (BOUND-1, docs/specs/kando-web/spec/2-scope.md).
//
// The store is line-oriented Markdown, so every single-line field passes
// through sanitizeLine: a stray "\n" would otherwise turn a value into a
// second line the parser reads as structure.

// sanitizeLine trims s and drops control runes (including CR/LF) so a value
// can never become a second line in the Markdown store.
func sanitizeLine(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s))
}

// NewCard builds a card for lane l with a sanitized title, stamped created
// and moved at now (and done at now when l is Done, matching Move). It
// returns nil for an empty title.
func NewCard(title string, l Lane, now time.Time) *Card {
	title = sanitizeLine(title)
	if title == "" {
		return nil
	}
	c := &Card{ID: NewID(), Title: title, CreatedAt: now, MovedAt: now}
	if l == Done {
		c.DoneAt = now
	}
	return c
}

// SetTitle replaces the title with a sanitized value, reporting whether it
// did; an empty (after sanitizing) title is a no-op.
func (c *Card) SetTitle(value string) bool {
	value = sanitizeLine(value)
	if value == "" {
		return false
	}
	c.Title = value
	return true
}

// SetTag sets the card's tag, sanitized and with a leading "#" stripped.
func (c *Card) SetTag(value string) {
	c.Tag = strings.TrimPrefix(sanitizeLine(value), "#")
}

// SetNotes sets the card's notes, normalizing line endings to "\n" and
// trimming trailing blank lines and spaces. Notes keep their newlines.
func (c *Card) SetNotes(value string) {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	c.Notes = strings.TrimRight(value, "\n \t")
}

// SetBlocked sets the card's blocked reason (sanitized); a reason that's
// empty afterwards clears the blocked flag entirely.
func (c *Card) SetBlocked(reason string) {
	reason = sanitizeLine(reason)
	c.Blocked = reason != ""
	c.BlockedReason = reason
}

// InsertChecklistItem inserts a sanitized, non-empty item right after cursor
// (or at index 0 if the checklist is empty) and returns the new item's
// index. The position is clamped into range, matching Board.Insert. An
// empty (after sanitizing) text is a no-op that returns -1.
func (c *Card) InsertChecklistItem(cursor int, text string) int {
	text = sanitizeLine(text)
	if text == "" {
		return -1
	}
	at := 0
	if len(c.Checklist) > 0 {
		at = min(max(cursor+1, 0), len(c.Checklist))
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
// sanitized value, reporting whether it did. A no-op (false) if the text is
// empty afterwards or i is out of range; Done is left untouched either way.
func (c *Card) SetChecklistItemText(i int, text string) bool {
	text = sanitizeLine(text)
	if text == "" || i < 0 || i >= len(c.Checklist) {
		return false
	}
	c.Checklist[i].Text = text
	return true
}

// DeleteCard permanently removes the card at index i of lane l — unlike
// Move, it is not reinserted anywhere. It is the only exported deletion
// entry point. Returns the removed card, or nil if the index was invalid.
func (b *Board) DeleteCard(l Lane, i int) *Card {
	return b.remove(l, i)
}
