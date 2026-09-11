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
// unopenable on every surface. unknownKeyRe raises the stakes: it keeps the
// parser reading keys past a line kando does not write, so structuralNote has to
// escape a key-shaped note line wherever it sits in the notes, not only on the
// first line.
const cardKeys = `tag|created|moved|done|blocked|id`

var (
	keyRe  = regexp.MustCompile(`^(` + cardKeys + `):[ \t]?(.*)$`)
	itemRe = regexp.MustCompile(`^- \[( |x|X)\] ?(.*)$`)
	// unknownKeyRe matches a line shaped like a key that kando does not write:
	// a lowercase name, a colon, then a space, a tab or the end of the line,
	// such as "priority: high". parseSections keeps it as a note but does not
	// let it end the key block, so the keys written after it still count. The
	// shape is narrow on purpose: "Note: ..." and "https://..." stay prose.
	//
	// Three things keep that safe. keyRe is tried first, because this shape
	// also matches the known keys. The writer has no need to escape such a line,
	// since it reads back as the note it was, but structuralNote must escape a
	// key-shaped note line at every position, because the lines after one are
	// read in the key block too. And after one, parseSections keeps a key line
	// that repeats a key or does not parse as a note, as it was before unknown
	// keys were kept: a board that opened then still opens, and a stray id:
	// under a label cannot replace the card's id.
	unknownKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_-]*:([ \t]|$)`)
)

type section struct {
	heading string
	cards   []*board.Card
}

// parseSections reads "## heading" sections containing "### title" card blocks.
// It assigns no ids. It records each card's derivation seed in FILE order, and
// the caller assigns ids with assignIDs in LOOKUP order, which only the caller
// knows: lane order for a board, newest DoneAt first for an archive.
func parseSections(data []byte) (secs []section, seeds map[*board.Card]string, err error) {
	var (
		cur      *section
		card     *board.Card
		inKeys   bool
		notes    []string
		blanks   []string        // blank lines in the key block, held until the next line places them
		keysSet  map[string]bool // the keys this card's key block has set
		lineNo   int
		ordinal  int
		checkist []board.Item
	)
	seeds = make(map[*board.Card]string)
	flush := func() {
		if card == nil {
			return
		}
		card.Notes = strings.Join(trimBlank(notes), "\n")
		card.Checklist = checkist
		// The seed is fixed here, in file order, so every id derived before
		// shared ids were repaired stays byte-identical. The ordinal keeps two
		// identical card blocks apart.
		seeds[card] = fmt.Sprintf("%d\x00%s\x00%s\x00%s\x00%s",
			ordinal, cur.heading, card.Title, formatTime(card.CreatedAt), card.Notes)
		ordinal++
		cur.cards = append(cur.cards, card)
		card, notes, blanks, checkist = nil, nil, nil, nil
	}
	// keepAsNote ends the key block and adds line to the notes, after any blank
	// lines held before it.
	keepAsNote := func(line string) {
		inKeys = false
		notes = append(append(notes, blanks...), line)
		blanks = nil
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
				return nil, nil, fmt.Errorf("line %d: card before any \"## \" section", lineNo)
			}
			card = &board.Card{Title: strings.TrimSpace(line[4:])}
			inKeys = true
			keysSet = map[string]bool{}
		case card == nil:
			// Text outside a card (preamble, stray lines): ignored.
		case inKeys && strings.TrimSpace(line) == "":
			// A blank line in the key block is dropped if a key follows it and
			// kept if a note does, so it waits for the next line to decide.
			blanks = append(blanks, line)
		case inKeys && keyRe.MatchString(line):
			// notes is non-empty here only after an unknown key: see unknownKeyRe.
			m := keyRe.FindStringSubmatch(line)
			if len(notes) > 0 && keysSet[m[1]] {
				keepAsNote(line)
			} else if err := setKey(card, m[1], strings.TrimSpace(m[2])); err == nil {
				keysSet[m[1]] = true
				blanks = nil
			} else if len(notes) > 0 {
				keepAsNote(line)
			} else {
				return nil, nil, fmt.Errorf("line %d: %w", lineNo, err)
			}
		case inKeys && unknownKeyRe.MatchString(line):
			// A key kando does not know is kept, as a note, without ending the
			// key block: see unknownKeyRe.
			notes = append(append(notes, blanks...), line)
			blanks = nil
		default:
			notes = append(notes, blanks...)
			blanks = nil
			inKeys = false
			if m := itemRe.FindStringSubmatch(line); m != nil {
				checkist = append(checkist, board.Item{Text: m[2], Done: m[1] != " "})
			} else {
				notes = append(notes, unescapeNote(line))
			}
		}
	}
	flush()
	return secs, seeds, nil
}

// assignIDs gives every card an id no other card holds, and reports whether it
// changed any: a card with no id, or one sharing an id with a card before it.
//
// order must be the order the lookups walk, not file order: Board.Find goes
// Backlog, Todo, Doing, Done, and Archive.Find goes newest DoneAt first. The
// card that keeps a shared id has to be the one those lookups already reached,
// or a saved script, a bookmark or an open tab would start acting on a
// different card. A hand-edited file can list its lanes in any order, so the
// two orders genuinely differ.
//
// Written ids claim their values in a first pass, before anything is derived.
// Deciding card by card would let an id-less card derive the very value a later
// card wrote by hand, and the written id would be the one replaced.
//
// A shared id is reassigned rather than refused. This is the one value parsing
// changes instead of preserving (see the policy note in internal/board/ops.go):
// an id naming two cards identifies neither, and the lookups only ever reached
// one of them, so the other was already unreachable.
func assignIDs(order []*board.Card, seeds map[*board.Card]string) (changed bool) {
	seen := make(map[string]bool, len(order))
	var need []*board.Card
	for _, c := range order {
		if c.ID != "" && !seen[c.ID] {
			seen[c.ID] = true
			continue
		}
		need = append(need, c)
	}
	taken := func(id string) bool { return seen[id] }
	for _, c := range need {
		c.ID = board.DeriveFreeID(seeds[c], taken)
		seen[c.ID] = true
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

// Parse reads a board.md. A card without an id, or sharing one, gets one;
// needsRewrite reports that.
func Parse(data []byte) (b *board.Board, needsRewrite bool, err error) {
	secs, seeds, err := parseSections(data)
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
	// The order Board.Find walks, whatever order the file listed its lanes in.
	var order []*board.Card
	for _, l := range board.Lanes {
		order = append(order, b.Lanes[l]...)
	}
	return b, assignIDs(order, seeds), nil
}

// ParseArchive reads an archive.md; cards are ordered newest DoneAt first.
func ParseArchive(data []byte) (a *board.Archive, needsRewrite bool, err error) {
	secs, seeds, err := parseSections(data)
	if err != nil {
		return nil, false, err
	}
	a = &board.Archive{}
	for _, s := range secs {
		a.Cards = append(a.Cards, s.cards...)
	}
	sort.SliceStable(a.Cards, func(i, j int) bool { return a.Cards[i].DoneAt.After(a.Cards[j].DoneAt) })
	// After the sort: the order Archive.Find walks.
	return a, assignIDs(a.Cards, seeds), nil
}

// structuralNote matches a note line that would otherwise be read back as
// structure: a heading, a checklist item, a card key, or an already-escaped
// line.
//
// The key alternation is load-bearing for every note line the parser can reach
// while it is still in its inKeys state: the first line, and every line after
// a leading run of unknown-key lines (see unknownKeyRe) and the blank lines
// among them. Escaping every matching line covers all of those, and
// unescapeNote strips the backslash back off wherever it appears.
//
// The item alternation matches whatever itemRe reads as an item, with or
// without a space after the bracket: a note line "- [ ]" or "- [x]done" written
// unescaped would come back as a checklist item.
var structuralNote = regexp.MustCompile(`^\\*(#{1,6} |- \[[ xX]\]|(` + cardKeys + `):)`)

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
