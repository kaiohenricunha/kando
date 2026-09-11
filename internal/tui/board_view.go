package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/kaiohenricunha/kando/internal/board"
)

// renderBoard draws the board screen (also used underneath the filter prompt).
func (m Model) renderBoard() []string {
	L := m.layout()
	var body []string
	if L.narrow {
		body = append(body, m.tabStrip(L.cw), spaces(L.cw))
		body = append(body, m.renderLane(m.lane, L.cw, L.laneH)...)
	} else {
		var lanes [4][]string
		for _, l := range board.Lanes {
			lanes[l] = m.renderLane(l, L.laneW[l], L.laneH)
		}
		for i := 0; i < L.laneH; i++ {
			body = append(body, lanes[0][i]+"  "+lanes[1][i]+"  "+lanes[2][i]+"  "+lanes[3][i])
		}
	}
	var header string
	if m.mode == modeFilter {
		header = m.filterHeader(L.cw)
	} else {
		header = hsplit(m.headerLeft(), m.dateStr(), L.cw)
	}
	var footer string
	if m.mode == modeFilter {
		footer = m.filterFooter(L.cw)
	} else {
		footer = m.boardFooter(L.cw)
	}
	return m.screenRows(header, body, footer)
}

func (m Model) headerLeft() string {
	s := m.styles
	left := s.Brand.Render("kando") + "  " + s.Muted.Render(sanitize(m.b.Name))
	if m.filterQuery != "" && m.mode != modeFilter {
		left += "  " + s.Muted.Render("/"+sanitize(m.filterQuery))
	}
	return left
}

func (m Model) dateStr() string { return m.styles.Muted.Render(board.DayLabel(m.now())) }

// tabStrip is the narrow-mode lane strip: " NAME N " tabs, the active one in
// brackets on the accent background.
func (m Model) tabStrip(cw int) string {
	var tabs []string
	for _, l := range board.Lanes {
		label := fmt.Sprintf("%s %d", strings.ToUpper(l.String()), len(m.visible(l)))
		if l == m.lane {
			tabs = append(tabs, m.styles.TabActive.Render("["+label+"]"))
		} else {
			tabs = append(tabs, m.styles.Muted.Render(" "+label+" "))
		}
	}
	return fit(strings.Join(tabs, "  "), cw)
}

// renderLane draws one lane of the given outer size: the active lane boxed in
// the accent colour, every other lane an open column (openLane).
func (m Model) renderLane(l board.Lane, outerW, outerH int) []string {
	s := m.styles
	cards := m.visible(l)
	name := strings.ToUpper(l.String())
	count := fmt.Sprint(len(cards))
	if l != m.lane {
		return m.openLane(l, cards, name, count, outerW, outerH)
	}
	inner := outerW - 4
	header := hsplit(s.AccentBold.Render(name), s.AccentBold.Render(count), inner)
	rows := append([]string{header, spaces(inner)}, m.activeRows(l, cards, inner, outerH-4)...)
	return box(rows, outerW, lipgloss.RoundedBorder(), s.Accent, s.Plain, 1)
}

// openLane draws an inactive lane with no box: a rule and one padding cell down
// its left edge. The rows where a box would draw its top and bottom edges stay
// blank, so every lane's header and first row line up with the active lane's.
func (m Model) openLane(l board.Lane, cards []*board.Card, name, count string, outerW, outerH int) []string {
	s := m.styles
	inner := outerW - 2
	rows := make([]string, 0, outerH)
	rows = append(rows, spaces(inner), hsplit(s.Muted.Render(name), s.Muted.Render(count), inner), spaces(inner))
	rows = append(rows, m.collapsedRows(l, cards, inner, outerH-4)...)
	rows = append(rows, spaces(inner))
	rule := s.Border.Render("│") + " "
	for i := range rows {
		rows[i] = rule + rows[i]
	}
	return rows
}

func (m Model) emptyRow(inner int) string {
	return fit(m.styles.Border.Render("· · ·"), inner)
}

func (m Model) moreRow(n, inner int) string {
	return fit(m.styles.Border.Render(fmt.Sprintf("… +%d", n)), inner)
}

// minOpenTitle is the fewest title cells an open-lane row keeps before it drops
// its glyph: a glyph beside a title cut to a letter or two says less than the
// title would.
const minOpenTitle = 6

// collapsedRows lists "• Title" (or "✓ Title" in Done) one per row. A card
// outside Done also shows, right-aligned, the one glyph worth a glance: ⊘ when
// blocked, otherwise its checklist progress. The glyph is dropped when it
// would leave the title fewer than minOpenTitle cells.
func (m Model) collapsedRows(l board.Lane, cards []*board.Card, inner, R int) []string {
	s := m.styles
	rows := make([]string, R)
	for i := range rows {
		rows[i] = spaces(inner)
	}
	if R == 0 {
		return rows
	}
	if len(cards) == 0 {
		rows[0] = m.emptyRow(inner)
		return rows
	}
	show := len(cards)
	if show > R {
		show = R - 1
	}
	for i := 0; i < show; i++ {
		c := cards[i]
		tw := inner - 2
		if l == board.Done {
			rows[i] = s.Accent.Render("✓") + " " + s.Muted.Render(fit(sanitize(c.Title), tw))
			continue
		}
		glyph, glyphStyle := "⊘", s.Accent2
		if c.BlockedLabel() == "" {
			glyph, glyphStyle = c.ProgressLabel(), s.Muted
		}
		tail := ""
		if glyph != "" && tw-2-width(glyph) >= minOpenTitle {
			tw -= 2 + width(glyph)
			tail = "  " + glyphStyle.Render(glyph)
		}
		rows[i] = s.Fg.Render("•") + " " + s.Fg.Render(fit(sanitize(c.Title), tw)) + tail
	}
	if hidden := len(cards) - show; hidden > 0 {
		rows[show] = m.moreRow(hidden, inner)
	}
	return rows
}

// activeRows stacks full cards with one blank row between them, scrolling in
// whole-card steps and marking hidden cards with "… +N" rows.
func (m Model) activeRows(l board.Lane, cards []*board.Card, inner, R int) []string {
	rows := make([]string, R)
	for i := range rows {
		rows[i] = spaces(inner)
	}
	r := 0
	if m.mode == modeQuickAdd {
		for i, qr := range m.quickAddRows(inner) {
			if r+i < R {
				rows[r+i] = qr
			}
		}
		r += 6
	}
	if r >= R {
		return rows
	}
	if len(cards) == 0 {
		rows[r] = m.emptyRow(inner)
		return rows
	}
	avail := R - r
	first := m.first
	if first > len(cards) {
		first = len(cards)
	}
	win := laneWindow(avail, len(cards), first)
	base := r
	if first > 0 {
		rows[base] = m.moreRow(first, inner)
		base++
	}
	for i := 0; i < win.shown; i++ {
		idx := first + i
		selected := idx == m.sel && m.mode != modeQuickAdd
		for j, cr := range m.cardRows(cards[idx], l, inner, selected) {
			rows[base+i*6+j] = cr
		}
	}
	if win.hidden > 0 {
		idx := base + win.shown*6 - 1
		if win.shown == 0 {
			idx = base
		}
		if idx > R-1 {
			idx = R - 1
		}
		rows[idx] = m.moreRow(win.hidden, inner)
	}
	return rows
}
