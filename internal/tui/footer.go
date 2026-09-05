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
		{"j/k", "item"}, {"x", "toggle"}, {"o", "new item"}, {"e", "edit notes"},
		{"t", "tag"}, {"m", "move"}, {"b", "block"}, {"esc", "back"},
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

// boardFooter applies the fallback chain: full+count, reduced+count, reduced, truncated.
func (m Model) boardFooter(cw int) string {
	count := m.styles.Muted.Render(fmt.Sprintf("%d cards", m.visibleCount()))
	full := m.groups(boardFooterFull)
	reduced := m.groups(boardFooterReduced)
	switch {
	case width(full)+2+width(count) <= cw:
		return hsplit(full, count, cw)
	case width(reduced)+2+width(count) <= cw:
		return hsplit(reduced, count, cw)
	default:
		return fit(reduced, cw)
	}
}

// footerWithRight draws groups on the left and an optional right part that is
// dropped when it does not fit; the left part is truncated as a last resort.
func (m Model) footerWithRight(left, rightPart string, cw int) string {
	if rightPart != "" && width(left)+2+width(rightPart) <= cw {
		return hsplit(left, rightPart, cw)
	}
	return fit(left, cw)
}
