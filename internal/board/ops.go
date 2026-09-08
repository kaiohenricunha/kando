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

// UnsafeRune reports whether r must never reach a stored field or a rendered
// surface. It is the single definition of "unsafe" for this program: the
// write-time sanitizers below use it, and so do the renderers, which is what
// keeps the two from drifting apart as they did before.
//
// Two categories, for two different reasons:
//
// Cc (unicode.IsControl, so C0 plus DEL plus the C1 block U+0080-U+009F)
// drives the terminal. ESC opens OSC 52 and writes the reader's clipboard;
// U+009B is CSI, the single-byte form of "ESC [", so the C1 half is not
// theoretical padding.
//
// Bidi_Control is the Trojan Source set (CVE-2021-42574) — the embeddings and
// overrides U+202A-U+202E, the isolates U+2066-U+2069, and the direction
// marks. These are category Cf, so unicode.IsControl is blind to them, and
// they reorder how text renders in a terminal and in a browser alike: a note
// can display as text it does not contain.
//
// Zl and Zp — U+2028 LINE SEPARATOR and U+2029 PARAGRAPH SEPARATOR — end a
// line. They neither drive the terminal nor misrepresent order, so they are
// here for a third reason: they break a row that is supposed to be one row.
// The TUI composes rows of an exact terminal-cell count, and ansi.StringWidth
// measures these as zero cells, exactly like the bidi controls — so the row's
// own arithmetic is right and the break itself is the damage. A terminal that
// honours it splits the row anyway, and every later row lands one line out of
// position. The same break forges a line in the CLI's key: value output.
//
// Deliberately NOT the whole of Cf. That would take U+200C and U+200D, the
// zero-width non-joiner and joiner, which are load-bearing in Persian and
// Indic shaping and in ZWJ emoji sequences, and the tag block U+E0020-E007F,
// which spells out the England, Scotland and Wales flags. (Variation
// selectors are safe from a Cf filter either way — they are category Mn, not
// Cf.) Bidi_Control is the narrow set that misrepresents order; the rest of
// Cf is invisible but honest.
//
// Combining marks (category Mn) are deliberately absent, and this is the place
// that question gets settled so nobody re-derives it. "Zalgo" text looks like
// it should break the TUI's exact-cell rows, and it does not: ansi.StringWidth
// clusters by UAX #29, so a base character with five hundred marks measures
// one cell, and fit() pads from the same number the frame tests check with.
// The arithmetic closes. A cap would also have to be a run-length rule, which
// this predicate cannot express — it is one rune at a time — and U+0301 and
// U+FE0F are both on the must-survive list, so a naive Mn filter would break
// Persian, Indic, Hebrew and every emoji presentation sequence. What remains
// is vertical bleed, which the terminal owns and no cell-width model
// describes. The byte cost of such a value is bounded instead, at the point it
// is rendered: see maxRenderBytes in internal/tui/cells.go.
func UnsafeRune(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) ||
		r == '\u2028' || r == '\u2029'
}

// Why none of this happens at parse time, since it keeps coming up: the store
// re-emits what it parsed. cardBlock writes Title, Tag, BlockedReason and
// Notes straight back from memory (internal/store/markdown.go), and store.Open
// rewrites the whole board through Marshal whenever any card lacks an id. So a
// value clipped or sanitized during parsing would be written into the user's
// hand-edited file on the next open, before they touched anything. That is why
// every guard in this package is either a write-time helper the surfaces call
// deliberately, or a read-time one applied on the way to a screen.

// SafeForDisplay drops every unsafe rune from s, keeping newlines and tabs so
// multi-line text still lays out. It is the read-side counterpart to the
// write-time sanitizers.
//
// It exists because board.md is the user's own file and is parsed verbatim:
// store.parseSections assigns fields directly and Marshal re-emits them, so
// nothing on the read path has ever been through sanitizeLine or SetNotes. A
// note a pre-fix build stored, or one typed in by hand, reaches a renderer
// with its escape sequences intact. Rather than rewrite the user's file, each
// surface puts values through this on the way out.
//
// It drops the line separators rather than converting them to "\n", and that
// is deliberate: most callers render a single line — a key: value row in
// kando show, a lane row in kando list, an HTML attribute — and a helper that
// manufactures a newline lets a card field forge one of those rows. Turning a
// separator into the author's line break is a normalization that only makes
// sense where a line break is legal, so SetNotes does it on the way in,
// alongside the CRLF normalization it already performs.
func SafeForDisplay(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if UnsafeRune(r) {
			return -1
		}
		return r
	}, s)
}

// sanitizeLine trims s and drops unsafe runes (including CR/LF) so a value
// can never become a second line in the Markdown store.
func sanitizeLine(s string) string {
	return clip(strings.TrimSpace(strings.Map(func(r rune) rune {
		if UnsafeRune(r) {
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
// trimming trailing blank lines and spaces. Notes keep their newlines and
// tabs; every other control rune is dropped, as sanitizeLine does for the
// single-line fields.
//
// Notes were the one field that kept them, which mattered once a note could
// arrive from somewhere other than a keyboard: `kando notes --file` and its
// stdin form make piping in an issue body or a web page the normal way to
// use the verb. An escape sequence in that text is stored verbatim and
// replayed on every later read — by kando show, by the TUI's detail pane,
// and by kando web — so a single paste keeps rewriting the terminal, moving
// the cursor or driving OSC 52, long after the paste is forgotten.
//
// This is a guard on what the surfaces write, not on what the file holds.
// kando does rewrite the file — every mutation re-marshals the whole board,
// and store.Open rewrites it outright when a card has no id — but the read
// path assigns Notes directly (store.parseSections) and Marshal re-emits
// whatever it parsed, so a rewrite preserves unsanitized bytes rather than
// cleaning them. A hand-edited board.md therefore keeps whatever the user put
// in it, which is the intended boundary: their file, their content.
//
// The bytes that boundary leaves in place — a note a pre-fix build stored, or
// one typed in by hand — are handled on the way out instead, by
// SafeForDisplay at each renderer, so nothing is rewritten behind the user's
// back. See UnsafeRune for which runes both halves drop and why.
//
// The line separators are the one rune class this function converts rather
// than drops, and it converts them here because notes are the one multi-line
// field. SafeForDisplay drops them, so a hand-edited note keeps its own bytes
// until it is next edited through a surface.
func (c *Card) SetNotes(value string) {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	// The Unicode line separators are line endings too, so they normalize
	// here with the others rather than being dropped by SafeForDisplay below.
	// Notes are the one multi-line field, so this is the one place where
	// turning a separator into a real break is both safe and what the author
	// meant.
	value = strings.ReplaceAll(value, "\u2028", "\n")
	value = strings.ReplaceAll(value, "\u2029", "\n")
	value = SafeForDisplay(value)
	value = trimLeadingBlankLines(value)
	c.Notes = strings.TrimRight(clip(value, maxNotesBytes), "\n \t")
}

// trimLeadingBlankLines drops whole blank lines from the front of s, matching
// what store.trimBlank already does on both the read and the write path.
//
// Whole lines, not characters: a TrimLeft over "\n \t" would also eat the
// indentation of a first line that has content, which trimBlank never does.
// The trailing end is handled by SetNotes' own TrimRight and is deliberately
// left alone.
//
// Without this, SetNotes kept a leading blank line that the store then dropped
// on save, so the TUI detail pane showed a row that disappeared on reload.
func trimLeadingBlankLines(s string) string {
	for {
		i := strings.IndexByte(s, '\n')
		if i < 0 || strings.TrimSpace(s[:i]) != "" {
			return s
		}
		s = s[i+1:]
	}
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
