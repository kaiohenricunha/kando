package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kaiohenricunha/kando/internal/board"
)

func (m Model) filterWidth() int {
	w := m.w - 2 - 4 - 20
	if w < 8 {
		w = 8
	}
	return w
}

func (m *Model) openFilter() {
	m.filterIn = m.newInput(m.styles.Fg, m.styles.Fg)
	m.filterIn.SetValue(m.filterQuery)
	m.filterIn.CursorEnd()
	m.filterIn.Width = m.filterWidth()
	m.mode = modeFilter
	m.applyFilter(m.filterIn.Value())
}

// applyFilter re-parses the query and moves the selection to the first visible card.
func (m *Model) applyFilter(q string) {
	m.filter = board.Parse(q)
	m.sel, m.first = 0, 0
	if m.scr == screenArchive {
		m.arch.cursor, m.arch.first = 0, 0
	}
}

// updateFilter handles live typing; enter keeps the filter, esc clears it.
func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filterQuery = ""
		m.mode = modeNormal
		m.applyFilter("")
		return m, nil
	case "enter":
		m.filterQuery = m.filterIn.Value()
		m.mode = modeNormal
		m.applyFilter(m.filterQuery)
		return m, nil
	}
	var cmd tea.Cmd
	m.filterIn, cmd = m.filterIn.Update(msg)
	m.applyFilter(m.filterIn.Value())
	return m, cmd
}

// filterHeader replaces the header while typing: "/ query" and "N of M match".
func (m Model) filterHeader(cw int) string {
	left := m.styles.AccentBold.Render("/") + " " + m.filterIn.View()
	total := m.b.Count()
	if m.scr == screenArchive && m.archive != nil {
		total = len(m.archive.Cards)
	}
	matched := m.visibleCount()
	if m.scr == screenArchive {
		matched = len(m.visibleArchive())
	}
	right := m.styles.Muted.Render(fmt.Sprintf("%d of %d match", matched, total))
	return hsplit(left, right, cw)
}

func (m Model) filterFooter(cw int) string {
	s := m.styles
	rightPart := s.Bold.Render("!blocked") + " " + s.Bold.Render("age>7d") + " " + s.Muted.Render("also work")
	return m.footerWithRight(m.groups(filterFooterGroups), rightPart, cw)
}
