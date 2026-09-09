package tui

import (
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/kaiohenricunha/kando/internal/board"
)

// updateBoard handles keys on the board screen in normal mode.
func (m Model) updateBoard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.visible(m.lane))
	switch msg.String() {
	case "q":
		m.teardown()
		return m, tea.Quit
	case "j", "down":
		if n > 0 {
			m.sel = (m.sel + 1) % n
			m.ensureVisible()
		}
	case "k", "up":
		if n > 0 {
			m.sel = (m.sel - 1 + n) % n
			m.ensureVisible()
		}
	case "h", "left":
		if m.lane > board.Backlog {
			m.setLane(m.lane - 1)
		}
	case "l", "right":
		if m.lane < board.Done {
			m.setLane(m.lane + 1)
		}
	case "tab":
		m.setLane((m.lane + 1) % 4)
	case "shift+tab":
		m.setLane((m.lane + 3) % 4)
	case "H":
		if m.lane > board.Backlog {
			m.moveSelected(m.lane - 1)
		}
	case "L":
		if m.lane < board.Done {
			m.moveSelected(m.lane + 1)
		}
	case "J":
		m.reorderSelected(1)
	case "K":
		m.reorderSelected(-1)
	case "d":
		if m.lane != board.Done {
			if c := m.selectedCard(); c != nil {
				m.b.Move(m.lane, m.laneIndex(m.lane, c), board.Done, m.now())
				m.save()
				m.clampSel()
			}
		}
	case "u":
		if m.lane == board.Done {
			if c := m.selectedCard(); c != nil {
				m.b.Move(m.lane, m.laneIndex(m.lane, c), board.Doing, m.now())
				m.save()
				m.clampSel()
			}
		}
	case "A":
		if m.lane == board.Done {
			if c := m.selectedCard(); c != nil {
				m.archiveDone(c)
				m.clampSel()
			}
		}
	case "x":
		if c := m.selectedCard(); c != nil {
			m.b.DeleteCard(m.lane, m.laneIndex(m.lane, c))
			m.save()
			m.clampSel()
		}
	case "a":
		m.openQuickAdd()
	case "enter":
		if c := m.selectedCard(); c != nil {
			m.openDetail(c.ID, false)
		}
	case "/":
		m.openFilter()
	case "?":
		m.help = true
	case "D":
		m.openArchive()
	case "B":
		m.openBoards()
	}
	return m, nil
}

// moveSelected moves the selected card to lane `to`, activates that lane and
// keeps the moved card selected.
func (m *Model) moveSelected(to board.Lane) {
	c := m.selectedCard()
	if c == nil {
		return
	}
	m.b.Move(m.lane, m.laneIndex(m.lane, c), to, m.now())
	m.save()
	m.setLane(to)
	m.selectByID(c.ID)
	m.ensureVisible()
}

// reorderSelected moves the selected card one slot down (delta 1) or up
// (delta -1) inside its own lane, and keeps it selected. It clamps at the ends
// like H/L rather than wrapping like j/k: a card falling off the bottom and
// reappearing on top is not a gesture this board has anywhere else.
//
// The target is named against the neighbour the user can SEE, never a raw
// index. m.sel counts the filtered list while MoveAt needs positions in the
// lane itself, so with a filter active the visible neighbour is not the
// adjacent lane element, and stepping by lane index would hop over hidden
// cards — leaving the on-screen order unchanged. The web route names positions
// against a visible card for exactly this reason (movePos, internal/web/cards.go).
//
// MoveAt's `at` is insert-before against the lane as it stood BEFORE the move
// (board.go:209-219), so landing below a neighbour is that neighbour's index
// plus one. The natural-looking `at = i+1` is wrong: it names the position the
// card already holds and does nothing. A same-lane move deliberately leaves
// MovedAt and DoneAt alone — a reorder is not a lane change.
func (m *Model) reorderSelected(delta int) {
	c := m.selectedCard()
	if c == nil {
		return
	}
	v := m.visible(m.lane)
	j := m.sel + delta
	if j < 0 || j >= len(v) {
		return // at the end already: nothing to move, and nothing to save
	}
	i, at := m.laneIndex(m.lane, c), m.laneIndex(m.lane, v[j])
	if i < 0 || at < 0 {
		return
	}
	if delta > 0 {
		at++
	}
	m.b.MoveAt(m.lane, i, m.lane, at, m.now())
	m.save()
	// Re-select by identity, the way this function located the card, rather
	// than by id via selectByID. board.md is hand-editable and nothing dedupes
	// ids — parseSections takes a written "id:" verbatim and only derives one
	// for an empty field — so two cards in a lane can share an id. laneIndex
	// matches by pointer and selectByID by id, and where those disagree the
	// selection lands on the twin that did not move: the card then oscillates
	// instead of descending, never reaching the clamp, and every press writes
	// the board. MoveAt's return value cannot stand in here either — it is a
	// lane index, and m.sel counts the filtered list.
	for k, x := range m.visible(m.lane) {
		if x == c {
			m.sel = k
			break
		}
	}
	m.ensureVisible()
}

// newInput builds a single-line input with no prompt, no placeholder and a
// static (non-blinking) reverse-video cursor.
func (m Model) newInput(text, cursorStyle lipglossStyle) textinput.Model {
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = ""
	in.TextStyle = text
	in.Cursor.SetMode(cursor.CursorStatic)
	in.Cursor.Style = cursorStyle
	in.Cursor.TextStyle = text
	in.Focus()
	return in
}

func (m *Model) openQuickAdd() {
	m.quick = m.newInput(m.styles.SelFg, m.styles.SelFg)
	m.quick.Width = m.cardTextWidth() - 1
	m.mode = modeQuickAdd
	m.first = 0
}

// updateQuickAdd feeds keys to the input; enter creates, esc cancels.
func (m Model) updateQuickAdd(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.ensureVisible()
		return m, nil
	case "enter":
		m.mode = modeNormal
		if c := board.NewCard(m.quick.Value(), m.lane, m.now()); c != nil {
			m.b.Insert(m.lane, 0, c)
			m.save()
			m.sel, m.first = 0, 0
			m.selectByID(c.ID)
		}
		m.ensureVisible()
		return m, nil
	}
	var cmd tea.Cmd
	m.quick, cmd = m.quick.Update(msg)
	return m, cmd
}
