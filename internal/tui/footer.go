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

// boardFooter shows the widest hint row that fits: the full keys, then the
// reduced keys (without H/L), each tried with its section rules first and
// without them second, so the rules always go before a key does. The count
// sits beside the widest row that leaves room for it; when none does, the
// widest row that fits goes alone, and the reduced row without rules is
// truncated as a last resort. With something to report — an error, or a
// refused key's notice — the message takes the count's place and is never
// the part dropped for lack of room: the hints give way instead, as they do
// on the archive and detail footers.
func (m Model) boardFooter(cw int) string {
	// The rules cost ten cells: from 102 to 111 wide they are the difference
	// between the row keeping H/L and losing it, and at 80 wide between it
	// keeping "q quit" and losing that.
	rows := []string{
		m.sections(boardFooterFull, boardFullSections), m.groups(boardFooterFull),
		m.sections(boardFooterReduced, boardReducedSections), m.groups(boardFooterReduced),
	}
	widest := func(room int) (string, bool) {
		for _, r := range rows {
			if width(r) <= room {
				return r, true
			}
		}
		return rows[len(rows)-1], false
	}
	if m.hasReport() {
		// A standing condition until the success that fixes it (see standing);
		// a refused key until the next key. The old chain dropped a message that
		// did not fit beside the reduced hints — at 120 wide, any refusal naming
		// a card with a title much over twenty characters.
		e := m.reportPart(cw)
		left, _ := widest(cw - 2 - width(e))
		return hsplit(left, e, cw)
	}
	count := m.styles.Muted.Render(fmt.Sprintf("%d cards", m.visibleCount()))
	if left, ok := widest(cw - 2 - width(count)); ok {
		return hsplit(left, count, cw)
	}
	left, _ := widest(cw)
	return fit(left, cw)
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
