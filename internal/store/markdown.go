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

var (
	keyRe  = regexp.MustCompile(`^(tag|created|moved|done|blocked|id):[ \t]?(.*)$`)
	itemRe = regexp.MustCompile(`^- \[( |x|X)\] ?(.*)$`)
)

type section struct {
	heading string
	cards   []*board.Card
}

// parseSections reads "## heading" sections containing "### title" card blocks.
// It assigns ids to cards that lack one and reports that in needsRewrite.
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
		if card.ID == "" {
			card.ID = board.NewID()
			needsRewrite = true
		}
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
				notes = append(notes, line)
			}
		}
	}
	flush()
	return secs, needsRewrite, nil
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
	if notes := trimBlank(strings.Split(c.Notes, "\n")); len(notes) > 0 {
		lines = append(lines, notes...)
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
