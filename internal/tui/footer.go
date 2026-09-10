package tui

import (
	"fmt"
	"strings"
)

// keyGroup is one "key label" pair in a footer.
type keyGroup struct{ key, label string }

var (
	boardFooterFull = []keyGroup{
		{"j/k", "move"}, {"h/l tab", "lane"}, {"a", "add"}, {"enter", "open"},
		{"H/L", "move card"}, {"d", "done"}, {"/", "filter"}, {"?", "help"}, {"q", "quit"},
	}
	boardFooterReduced = []keyGroup{
		{"j/k", "move"}, {"h/l", "lane"}, {"a", "add"}, {"enter", "open"},
		{"d", "done"}, {"/", "filter"}, {"?", "help"}, {"q", "quit"},
	}
	filterFooterGroups = []keyGroup{
		{"type", "to filter title or #tag"}, {"enter", "keep filter"}, {"esc", "clear"},
	}
	detailFooterGroups = []keyGroup{
		{"j/k", "item"}, {"x", "toggle"}, {"o", "new item"}, {"T", "title"},
		{"e", "edit notes"}, {"t", "tag"}, {"m", "move"}, {"b", "block"}, {"esc", "back"},
	}
	archiveFooterGroups = []keyGroup{
		{"j/k", "move"}, {"u", "undo (back to Doing)"}, {"enter", "open"}, {"/", "filter"}, {"esc", "board"},
	}
	boardsFooterGroups = []keyGroup{
		{"j/k", "move"}, {"enter", "open"}, {"n", "new board"}, {"esc", "back"},
	}
	boardNameFooterGroups = []keyGroup{
		{"type", "a name"}, {"enter", "create"}, {"esc", "cancel"},
	}
)

// groups renders key groups joined by two spaces: bold key, space, muted label.
func (m Model) groups(gs []keyGroup) string {
	parts := make([]string, 0, len(gs))
	for _, g := range gs {
		parts = append(parts, m.styles.Bold.Render(g.key)+" "+m.styles.Muted.Render(g.label))
	}
	return strings.Join(parts, "  ")
}

// boardFooter applies the fallback chain: full+count, reduced+count, reduced,
// truncated. With an error the message takes the count's place and is never
// the part dropped for lack of room: the hints give way instead, as they do on
// the archive and detail footers.
func (m Model) boardFooter(cw int) string {
	full := m.groups(boardFooterFull)
	reduced := m.groups(boardFooterReduced)
	if m.err != nil {
		// A failed save, the watcher, or a refused key, until the next
		// successful write. The old chain dropped a message that did not fit
		// beside the reduced hints — at 120 wide, any refusal naming a card
		// with a title much over twenty characters.
		e := m.errPart(cw)
		left := full
		if width(full)+2+width(e) > cw {
			left = reduced
		}
		return hsplit(left, e, cw)
	}
	count := m.styles.Muted.Render(fmt.Sprintf("%d cards", m.visibleCount()))
	switch {
	case width(full)+2+width(count) <= cw:
		return hsplit(full, count, cw)
	case width(reduced)+2+width(count) <= cw:
		return hsplit(reduced, count, cw)
	default:
		return fit(reduced, cw)
	}
}

// errPart is the footer's report slot: "⊘ message" in accent2, held to half
// the row so the hints beside it keep at least the other half.
func (m Model) errPart(cw int) string {
	return m.styles.Accent2.Render(trunc("⊘ "+sanitize(m.err.Error()), cw/2))
}

// footerWithErr is the archive and detail footers: the key hints, and at the
// right end the message whenever a save, the watcher or a refused key has
// something to say — the slot boardFooter gives the card count. The message
// wins over the hints (hsplit shrinks the left part, never the right), so a
// refused key can say so at every size. With nothing to report it is
// fit(left, cw), byte for byte what these footers rendered before, so no golden
// moves.
func (m Model) footerWithErr(left string, cw int) string {
	if m.err == nil {
		return fit(left, cw)
	}
	return hsplit(left, m.errPart(cw), cw)
}

// footerWithRight draws groups on the left and an optional right part that is
// dropped when it does not fit; the left part is truncated as a last resort.
func (m Model) footerWithRight(left, rightPart string, cw int) string {
	if rightPart != "" && width(left)+2+width(rightPart) <= cw {
		return hsplit(left, rightPart, cw)
	}
	return fit(left, cw)
}
