package tui

import "github.com/charmbracelet/lipgloss"

const helpWidth = 58

var helpLeft = []keyGroup{
	{"j/k ↓↑", "select card"}, {"h/l ←→", "change lane"}, {"tab", "next lane"},
	{"H/L", "move card"}, {"u", "undo (Done → Doing)"}, {"x", "delete card"},
	{"/", "filter"}, {"q", "quit"},
}

var helpRight = []keyGroup{
	{"a", "quick add"}, {"enter", "open card"}, {"H/L", "move card ±lane"},
	{"d", "move to Done"}, {"D", "archive view"}, {"A", "archive (Done)"},
	{"B", "boards"}, {"?", "close help"},
}

// helpRows draws the help box: KEYS, a blank row, then two key columns.
func (m Model) helpRows() []string {
	s := m.styles
	canvas := helpWidth - 4 // border + 1-col padding each side
	entry := func(g keyGroup) string {
		if g.key == "" {
			return ""
		}
		return s.Bold.Render(fit(g.key, 9)) + s.Muted.Render(g.label)
	}
	rows := []string{fit(s.Bold.Render("KEYS"), canvas), spaces(canvas)}
	for i := 0; i < len(helpLeft) || i < len(helpRight); i++ {
		var left, right string
		if i < len(helpLeft) {
			left = entry(helpLeft[i])
		}
		if i < len(helpRight) {
			right = entry(helpRight[i])
		}
		row := left + spaces(29-width(left)) + right
		rows = append(rows, fit(row, canvas))
	}
	return box(rows, helpWidth, lipgloss.RoundedBorder(), s.Accent, s.Plain, 1)
}

// overlayHelp draws the help box centred over the screen rows.
func (m Model) overlayHelp(rows []string) []string {
	h := m.helpRows()
	x := (m.w - helpWidth) / 2
	y := (m.h - len(h)) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	out := append([]string(nil), rows...)
	for i, hr := range h {
		if y+i < len(out) {
			out[y+i] = splice(out[y+i], x, hr)
		}
	}
	return out
}
