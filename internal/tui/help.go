package tui

import "github.com/charmbracelet/lipgloss"

const helpWidth = 58

var helpLeft = []keyGroup{
	{"j/k ↓↑", "select card"}, {"h/l ←→", "change lane"}, {"tab", "next lane"},
	{"H/L", "move card"}, {"u", "undo (Done → Doing)"}, {"x", "delete card"},
	{"/", "filter"}, {"q", "quit"},
	// Appended, not slotted next to H/L: helpRows pairs the two columns by
	// index, so inserting in the middle would re-pair every row below it and
	// break TestHelpOverlay, which asserts whole rows by their exact spacing
	// ("u        undo (Done → Doing) D        archive view"). It also goes on
	// the left because a right-column entry with no left partner renders as an
	// orphan behind 29 blank cells.
	{"J/K", "reorder in lane"},
}

var helpRight = []keyGroup{
	{"a", "quick add"}, {"enter", "open card"}, {"⇧tab", "previous lane"},
	{"d", "move to Done"}, {"D", "archive view"}, {"A", "archive (Done)"},
	{"B", "boards"}, {"?", "close help"},
}

// helpTable is one screen's keys, drawn as two columns paired by index.
type helpTable struct{ left, right []keyGroup }

// The other screens' tables. The overlay used to draw helpLeft and helpRight
// over every screen, which on the detail screen said x deletes a card and J/K
// reorder the lane, when there x toggles an item and J/K open the next card.
// Every left label stays within 19 cells: helpRows pads the left column to 29,
// and the key takes 9 of them, so a longer label would touch the right column.
// Every right label stays within 16, so the row fits the box.
// TestHelpTablesCoverTheFooterAndFitTheBox checks both.
var (
	detailHelp = helpTable{
		left: []keyGroup{
			{"j/k ↓↑", "select item"}, {"J/K", "next/prev card"}, {"x", "toggle item"},
			{"enter", "edit item"}, {"o", "new item"}, {"e", "edit notes"}, {"A", "archive (Done)"},
		},
		right: []keyGroup{
			{"T", "title"}, {"t", "tag"}, {"b", "block"}, {"m", "move to lane"},
			{"esc", "back"}, {"ctrl+s", "save notes"}, {"?", "close help"},
		},
	}
	archiveHelp = helpTable{
		left:  []keyGroup{{"j/k ↓↑", "select card"}, {"u", "back to Doing"}, {"enter", "open card"}},
		right: []keyGroup{{"/", "filter"}, {"esc", "board"}, {"?", "close help"}},
	}
	boardsHelp = helpTable{
		left:  []keyGroup{{"j/k ↓↑", "select board"}, {"enter", "open board"}, {"n", "new board"}},
		right: []keyGroup{{"esc", "back"}, {"?", "close help"}},
	}
)

// helpFor is the table for the screen the overlay is drawn over.
func (m Model) helpFor() helpTable {
	switch m.scr {
	case screenDetail:
		return detailHelp
	case screenArchive:
		return archiveHelp
	case screenBoards:
		return boardsHelp
	}
	return helpTable{left: helpLeft, right: helpRight}
}

// helpRows draws the help box: KEYS, a blank row, then the current screen's keys
// in two columns.
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
	t := m.helpFor()
	for i := 0; i < len(t.left) || i < len(t.right); i++ {
		var left, right string
		if i < len(t.left) {
			left = entry(t.left[i])
		}
		if i < len(t.right) {
			right = entry(t.right[i])
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
