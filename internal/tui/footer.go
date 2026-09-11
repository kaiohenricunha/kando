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
		{"j/k", "item"}, {"x", "toggle"}, {"o", "new item"},
		{"T", "title"}, {"e", "edit notes"}, {"t", "tag"}, {"b", "block"},
		{"m", "move"}, {"d", "done"}, {"esc", "back"},
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

// The board and detail footers are drawn in sections divided by a rule: moving
// around, acting on the card, and the rest. Each list is the size of each
// section in order. The footers stay flat lists because
// TestHelpTablesCoverTheFooterAndFitTheBox checks them group by group against
// the help tables; sections change only how they are drawn.
var (
	boardFullSections    = []int{4, 2, 3}
	boardReducedSections = []int{4, 1, 3}
	detailSections       = []int{3, 4, 3}
)

// sections renders gs in runs of the given sizes, each run as groups renders
// it, joined by a border-coloured "│" with three spaces either side. The sizes
// must add up to len(gs); TestFooterSectionsCoverTheirFooters checks that.
func (m Model) sections(gs []keyGroup, sizes []int) string {
	parts := make([]string, 0, len(sizes))
	i := 0
	for _, n := range sizes {
		parts = append(parts, m.groups(gs[i:i+n]))
		i += n
	}
	return strings.Join(parts, "   "+m.styles.Border.Render("│")+"   ")
}

// boardFooter applies the fallback chain: full+count, reduced+count, reduced,
// reduced without its section rules, then truncated. With something to
// report — an error, or a refused key's notice — the message takes the
// count's place and is never the part dropped for lack of room: the hints
// give way instead, as they do on the archive and detail footers.
func (m Model) boardFooter(cw int) string {
	full := m.sections(boardFooterFull, boardFullSections)
	reduced := m.sections(boardFooterReduced, boardReducedSections)
	// The rules cost ten cells; at 80 wide that is the difference between the
	// row keeping "q quit" and losing it.
	flat := m.groups(boardFooterReduced)
	if m.hasReport() {
		// A standing condition until the success that fixes it (see standing);
		// a refused key until the next key. The old chain dropped a message that
		// did not fit beside the reduced hints — at 120 wide, any refusal naming
		// a card with a title much over twenty characters.
		e := m.reportPart(cw)
		left := full
		if width(left)+2+width(e) > cw {
			left = reduced
		}
		if width(left)+2+width(e) > cw {
			left = flat
		}
		return hsplit(left, e, cw)
	}
	count := m.styles.Muted.Render(fmt.Sprintf("%d cards", m.visibleCount()))
	switch {
	case width(full)+2+width(count) <= cw:
		return hsplit(full, count, cw)
	case width(reduced)+2+width(count) <= cw:
		return hsplit(reduced, count, cw)
	case width(reduced) <= cw:
		return fit(reduced, cw)
	default:
		return fit(flat, cw)
	}
}

// hasReport says whether the footer's report slot has anything to show.
func (m Model) hasReport() bool { return m.notice != "" || m.errs.first() != nil }

// reportPart is the footer's report slot: "⊘ message" in accent2, held to half
// the row so the hints beside it keep at least the other half. A refused key's
// notice goes ahead of the most urgent standing condition. It lasts only until
// the next key, after which that condition is shown again rather than lost.
func (m Model) reportPart(cw int) string {
	msg := m.notice
	if msg == "" {
		msg = m.errs.first().Error()
	}
	return m.styles.Accent2.Render(trunc("⊘ "+sanitize(msg), cw/2))
}

// footerWithReport is the archive and detail footers: the key hints, and at the
// right end the report slot whenever a standing condition or a refused key has
// something to say — the slot boardFooter gives the card count. The message
// wins over the hints (hsplit shrinks the left part, never the right), so a
// refused key can say so at every size. With nothing to report it is
// fit(left, cw), byte for byte what these footers rendered before, so no golden
// moves.
func (m Model) footerWithReport(left string, cw int) string {
	if !m.hasReport() {
		return fit(left, cw)
	}
	return hsplit(left, m.reportPart(cw), cw)
}

// footerWithRight draws groups on the left and an optional right part that is
// dropped when it does not fit; the left part is truncated as a last resort.
func (m Model) footerWithRight(left, rightPart string, cw int) string {
	if rightPart != "" && width(left)+2+width(rightPart) <= cw {
		return hsplit(left, rightPart, cw)
	}
	return fit(left, cw)
}
