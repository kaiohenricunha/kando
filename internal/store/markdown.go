// Package store persists boards as plain, hand-editable Markdown files.
package store

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
)

// cardKeys is the alternation of every key line cardBlock writes under a
// card heading. keyRe and structuralNote are both built from it because they
// have to agree: keyRe decides what the parser eats as a key, structuralNote
// decides what the writer escapes so it is not eaten. They drifted once —
// structuralNote knew about headings and checklist items but not keys, so a
// note whose first line read "done: soon" was written unescaped, parsed back
// as the done: key, and failed parseTime, which made the whole board
// unopenable on every surface.
const cardKeys = `tag|created|moved|done|blocked|id`

var (
	keyRe  = regexp.MustCompile(`^(` + cardKeys + `):[ \t]?(.*)$`)
	itemRe = regexp.MustCompile(`^- \[( |x|X)\] ?(.*)$`)
)

type section struct {
	heading string
	cards   []*board.Card
}

// parseSections reads "## heading" sections containing "### title" card blocks.
// Once the whole file is read, assignIDs gives every card an id no other card
// holds; needsRewrite reports whether it had to change any.
func parseSections(data []byte) (secs []section, needsRewrite bool, err error) {
	var (
		cur      *section
		card     *board.Card
		inKeys   bool
		notes    []string
		lineNo   int
		checkist []board.Item
	)
	flush := func() {
		if card == nil {
			return
		}
		card.Notes = strings.Join(trimBlank(notes), "\n")
		card.Checklist = checkist
		cur.cards = append(cur.cards, card)
		card, notes, checkist = nil, nil, nil
	}
	for _, raw := range strings.Split(string(data), "\n") {
		lineNo++
		line := strings.TrimRight(raw, "\r")
		switch {
		case strings.HasPrefix(line, "## "):
			flush()
			secs = append(secs, section{heading: strings.TrimSpace(line[3:])})
			cur = &secs[len(secs)-1]
		case strings.HasPrefix(line, "### "):
			flush()
			if cur == nil {
				return nil, false, fmt.Errorf("line %d: card before any \"## \" section", lineNo)
			}
			card = &board.Card{Title: strings.TrimSpace(line[4:])}
			inKeys = true
		case card == nil:
			// Text outside a card (preamble, stray lines): ignored.
		case inKeys && strings.TrimSpace(line) == "":
			// Blank lines between the heading and the keys are tolerated.
		case inKeys && keyRe.MatchString(line):
			m := keyRe.FindStringSubmatch(line)
			if err := setKey(card, m[1], strings.TrimSpace(m[2])); err != nil {
				return nil, false, fmt.Errorf("line %d: %w", lineNo, err)
			}
		default:
			inKeys = false
			if m := itemRe.FindStringSubmatch(line); m != nil {
				checkist = append(checkist, board.Item{Text: m[2], Done: m[1] != " "})
			} else {
				notes = append(notes, unescapeNote(line))
			}
		}
	}
	flush()
	return secs, assignIDs(secs), nil
}

// assignIDs gives every card an id no other card in the file holds, and reports
// whether it changed any: a card with no id, or a later duplicate of one.
//
// It runs after the whole file is read, in two passes, because order matters.
// Written ids claim their values first; only then are ids derived for the cards
// that need one. Deciding card by card in file order would let an id-less card
// near the top derive the very value a card further down wrote by hand, and the
// written one would then be reassigned — changing an id the user typed, which
// is what keeping the first occurrence exists to avoid.
//
// A duplicate is reassigned rather than refused. This is the one value parsing
// changes instead of preserving (see the policy note in internal/board/ops.go):
// an id naming two cards identifies neither, and Board.Find only ever reaches
// the first, so the later twin was already unreachable. Keeping the first
// occurrence means anything that resolved before still resolves to the same card.
//
// Derived, not random: the same file parsed twice yields the same ids, so a link
// or form rendered from a read-only load still resolves on the request it
// produces. The ordinal keeps two identical card blocks apart; the salt matters
// only when a derived id is already taken.
func assignIDs(secs []section) (changed bool) {
	type pending struct {
		card *board.Card
		seed string
	}
	seen := make(map[string]bool)
	var need []pending
	ordinal := 0
	for _, sec := range secs {
		for _, c := range sec.cards {
			seed := fmt.Sprintf("%d\x00%s\x00%s\x00%s\x00%s",
				ordinal, sec.heading, c.Title, formatTime(c.CreatedAt), c.Notes)
			ordinal++
			if c.ID != "" && !seen[c.ID] {
				seen[c.ID] = true
				continue
			}
			need = append(need, pending{c, seed})
		}
	}
	for _, p := range need {
		id := board.DeriveID(p.seed)
		for salt := 1; seen[id]; salt++ {
			id = board.DeriveID(fmt.Sprintf("%s\x00%d", p.seed, salt))
		}
		p.card.ID = id
		seen[id] = true
	}
	return len(need) > 0
}

func setKey(c *board.Card, key, val string) error {
	switch key {
	case "tag":
		c.Tag = strings.TrimPrefix(val, "#")
	case "id":
		c.ID = val
	case "blocked":
		c.Blocked = true
		c.BlockedReason = val
	case "created", "moved", "done":
		t, err := parseTime(val)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		switch key {
		case "created":
			c.CreatedAt = t
		case "moved":
			c.MovedAt = t
		default:
			c.DoneAt = t
		}
	}
	return nil
}

var timeLayouts = []string{"2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"}

func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	for _, l := range timeLayouts {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse time %q", s)
}

func formatTime(t time.Time) string {
	if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0 {
		return t.Format("2006-01-02")
	}
	return t.Format(time.RFC3339)
}

// trimBlank drops leading and trailing all-whitespace lines.
func trimBlank(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// Parse reads a board.md. Cards without an id get one; needsRewrite reports that.
func Parse(data []byte) (b *board.Board, needsRewrite bool, err error) {
	secs, needsRewrite, err := parseSections(data)
	if err != nil {
		return nil, false, err
	}
	b = &board.Board{}
	for _, s := range secs {
		l, ok := board.ParseLane(s.heading)
		if !ok {
			return nil, false, fmt.Errorf("unknown lane %q", s.heading)
		}
		b.Lanes[l] = append(b.Lanes[l], s.cards...)
	}
	return b, needsRewrite, nil
}

// ParseArchive reads an archive.md; cards are ordered newest DoneAt first.
func ParseArchive(data []byte) (a *board.Archive, needsRewrite bool, err error) {
	secs, needsRewrite, err := parseSections(data)
	if err != nil {
		return nil, false, err
	}
	a = &board.Archive{}
	for _, s := range secs {
		a.Cards = append(a.Cards, s.cards...)
	}
	sort.SliceStable(a.Cards, func(i, j int) bool { return a.Cards[i].DoneAt.After(a.Cards[j].DoneAt) })
	return a, needsRewrite, nil
}

// structuralNote matches a note line that would otherwise be read back as
// structure: a heading, a checklist item, a card key, or an already-escaped
// line.
//
// The key alternation matters only for a note's *first* line, since that is
// where the parser is still in its inKeys state — but escaping every one of
// them is both simpler and harmless, because unescapeNote strips the
// backslash back off wherever it appears.
var structuralNote = regexp.MustCompile(`^\\*(#{1,6} |- \[[ xX]\] |(` + cardKeys + `):)`)

// escapeNote prefixes a structural note line with a backslash so the parser
// reads it back as prose. Without it a notes line of "## Nope" makes the
// whole board unparsable, "### Ghost" silently splits the card in two, and
// "done: soon" is swallowed as the done: key — which fails parseTime and
// makes the board unopenable from the CLI, the TUI and kando web alike.
func escapeNote(line string) string {
	if structuralNote.MatchString(line) {
		return "\\" + line
	}
	return line
}

// unescapeNote is escapeNote's inverse, applied to every note line read.
func unescapeNote(line string) string {
	if strings.HasPrefix(line, "\\") && structuralNote.MatchString(line[1:]) {
		return line[1:]
	}
	return line
}

func cardBlock(c *board.Card) string {
	lines := []string{"### " + c.Title}
	if c.Tag != "" {
		lines = append(lines, "tag: "+c.Tag)
	}
	if !c.CreatedAt.IsZero() {
		lines = append(lines, "created: "+formatTime(c.CreatedAt))
	}
	if !c.MovedAt.IsZero() {
		lines = append(lines, "moved: "+formatTime(c.MovedAt))
	}
	if !c.DoneAt.IsZero() {
		lines = append(lines, "done: "+formatTime(c.DoneAt))
	}
	if c.Blocked {
		if c.BlockedReason == "" {
			lines = append(lines, "blocked:")
		} else {
			lines = append(lines, "blocked: "+c.BlockedReason)
		}
	}
	if c.ID != "" {
		lines = append(lines, "id: "+c.ID)
	}
	for _, n := range trimBlank(strings.Split(c.Notes, "\n")) {
		lines = append(lines, escapeNote(n))
	}
	for _, it := range c.Checklist {
		mark := " "
		if it.Done {
			mark = "x"
		}
		lines = append(lines, "- ["+mark+"] "+it.Text)
	}
	return strings.Join(lines, "\n")
}

func joinBlocks(blocks []string) []byte {
	return []byte(strings.Join(blocks, "\n\n") + "\n")
}

// Marshal renders the canonical board.md.
func Marshal(b *board.Board) []byte {
	var blocks []string
	for _, l := range board.Lanes {
		blocks = append(blocks, "## "+l.String())
		for _, c := range b.Lanes[l] {
			blocks = append(blocks, cardBlock(c))
		}
	}
	return joinBlocks(blocks)
}

// MarshalArchive renders archive.md grouped under "## <ISO week>" headings, newest first.
func MarshalArchive(a *board.Archive) []byte {
	cards := append([]*board.Card(nil), a.Cards...)
	sort.SliceStable(cards, func(i, j int) bool { return cards[i].DoneAt.After(cards[j].DoneAt) })
	var blocks []string
	last := ""
	for _, c := range cards {
		key := "undated"
		if !c.DoneAt.IsZero() {
			key = board.ISOWeekKey(c.DoneAt)
		}
		if key != last {
			blocks = append(blocks, "## "+key)
			last = key
		}
		blocks = append(blocks, cardBlock(c))
	}
	if len(blocks) == 0 {
		return []byte{}
	}
	return joinBlocks(blocks)
}
