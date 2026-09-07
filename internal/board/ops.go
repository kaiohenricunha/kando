package board

import (
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Field limits. The store is a text file the user also edits by hand and the
// TUI re-measures every visible string on every frame, so one pasted
// megabyte would degrade all three surfaces. Values are trimmed to these
// lengths on a rune boundary rather than rejected.
const (
	maxFieldBytes = 512
	maxNotesBytes = 16 << 10
)

// clip truncates s to at most n bytes without splitting a rune.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

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
	return clip(strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)), maxFieldBytes)
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
	c.Notes = strings.TrimRight(clip(value, maxNotesBytes), "\n \t")
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

// Unarchive takes the archived card at index i out of a and puts it at the
// top of lane `to` on b, stamped exactly as Move stamps a lane change: the
// TUI's `u` and `m`-on-an-archived-card and the web's restore button, one
// rule. Returns the card, or nil if i was out of range.
func (b *Board) Unarchive(a *Archive, i int, to Lane, now time.Time) *Card {
	if i < 0 || i >= len(a.Cards) {
		return nil
	}
	c := a.Remove(i)
	stamp(c, to, now)
	b.Insert(to, 0, c)
	return c
}

// Restore is Unarchive back to Doing, the undo both surfaces offer.
func (b *Board) Restore(a *Archive, i int, now time.Time) *Card {
	return b.Unarchive(a, i, Doing, now)
}

// ArchiveDone takes the card at Done[i] off b and into a: the inverse of
// Restore, and the one board→archive path the TUI's A, the web's archive
// button and `kando archive` all share. Done-only is in the signature — there
// is no Lane parameter, so nothing can archive from Backlog, Todo or Doing.
// DoneAt is the week archive.md files the card under, so it is kept; a zero
// DoneAt (a hand-edited board) is set to now so the card lands in a real
// week rather than under "## undated". MovedAt is left alone and stamp() is
// deliberately not called: archiving is not a lane change. Returns the card,
// or nil if i was out of range.
func (b *Board) ArchiveDone(a *Archive, i int, now time.Time) *Card {
	c := b.remove(Done, i)
	if c == nil {
		return nil
	}
	if c.DoneAt.IsZero() {
		c.DoneAt = now
	}
	a.Insert(c)
	return c
}

// ArchiveMax is how many of the most recent archived cards a surface shows;
// the rest stay in archive.md.
const ArchiveMax = 50

// ArchiveGroup is one week bucket of an archive view.
type ArchiveGroup struct {
	Label string
	Cards []*Card
}

// ArchiveView is the archive as both surfaces present it: the newest
// ArchiveMax cards, filtered, bucketed by week with empty buckets dropped.
// Scanned is how many cards the filter actually looked at and Total how many
// the archive holds, so a caller can say "3 matches in the newest 50 of 200"
// without implying it searched all of them.
func ArchiveView(a *Archive, f Filter, now time.Time) (groups []ArchiveGroup, matched, scanned, total int) {
	if a == nil {
		return nil, 0, 0, 0
	}
	cards := a.Cards
	total = len(cards)
	if len(cards) > ArchiveMax {
		cards = cards[:ArchiveMax]
	}
	scanned = len(cards)
	var buckets [3]ArchiveGroup
	for g := ThisWeek; g <= Earlier; g++ {
		buckets[g].Label = g.Label()
	}
	for _, c := range cards {
		if !f.Empty() && !f.Match(c, now) {
			continue
		}
		buckets[GroupOf(now, c.DoneAt)].Cards = append(buckets[GroupOf(now, c.DoneAt)].Cards, c)
		matched++
	}
	for _, g := range buckets {
		if len(g.Cards) > 0 {
			groups = append(groups, g)
		}
	}
	return groups, matched, scanned, total
}
