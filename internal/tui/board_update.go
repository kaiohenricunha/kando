package tui

import (
	"strings"

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
		title := strings.TrimSpace(m.quick.Value())
		m.mode = modeNormal
		if title != "" {
			now := m.now()
			c := &board.Card{ID: board.NewID(), Title: title, CreatedAt: now, MovedAt: now}
			if m.lane == board.Done {
				c.DoneAt = now
			}
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
