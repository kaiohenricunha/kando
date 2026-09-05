package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

// boardsState is the board-picker screen state.
type boardsState struct {
	names  []string
	cursor int
	in     textinput.Model // the new-board name, while mode == modeBoardName
	err    string          // why the last switch or create failed
}

// openBoards lists the boards under the root and shows the picker with the
// cursor on the open board. Without a root there is only the open board.
func (m *Model) openBoards() {
	m.boards = boardsState{}
	names := []string{m.b.Name}
	if m.root != "" {
		listed, err := store.ListBoards(m.root)
		if err != nil {
			m.boards.err = err.Error()
		} else if len(listed) > 0 {
			names = listed
		}
	}
	m.boards.names = names
	for i, n := range names {
		if n == m.b.Name {
			m.boards.cursor = i
		}
	}
	m.scr = screenBoards
	m.mode = modeNormal
}

// renderBoards draws the picker: one row per board, the new-board input while
// naming, and the last error if any.
func (m Model) renderBoards() []string {
	s := m.styles
	cw := m.w - 2
	left := s.Brand.Render("kando") + "  " + s.Muted.Render(sanitize(m.b.Name)) + s.Muted.Render(" › boards")
	header := hsplit(left, s.Muted.Render(fmt.Sprintf("%d boards", len(m.boards.names))), cw)
	var body []string
	for i, n := range m.boards.names {
		if i == m.boards.cursor {
			body = append(body, s.SelFgBold.Render("▸ "+fit(sanitize(n), cw-2)))
		} else {
			body = append(body, s.Fg.Render("• ")+s.Fg.Render(sanitize(n)))
		}
	}
	if m.mode == modeBoardName {
		body = append(body, "", s.Muted.Render("new board:")+" "+m.boards.in.View())
	}
	if m.boards.err != "" {
		body = append(body, "", s.Accent2.Render("⊘ "+sanitize(m.boards.err)))
	}
	footer := fit(m.groups(boardsFooterGroups), cw)
	if m.mode == modeBoardName {
		footer = fit(m.groups(boardNameFooterGroups), cw)
	}
	return m.screenRows(header, body, footer)
}

func (m Model) updateBoards(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.boards.names)
	switch msg.String() {
	case "esc":
		m.scr = screenBoard
	case "j", "down":
		if n > 0 {
			m.boards.cursor = (m.boards.cursor + 1) % n
		}
	case "k", "up":
		if n > 0 {
			m.boards.cursor = (m.boards.cursor - 1 + n) % n
		}
	case "enter":
		if n > 0 {
			cmd := m.switchBoard(m.boards.names[m.boards.cursor])
			return m, cmd
		}
	case "n":
		m.boards.in = m.newInput(m.styles.Fg, m.styles.Fg)
		m.boards.in.Width = m.w - 2 - 12
		m.boards.err = ""
		m.mode = modeBoardName
	case "?":
		m.help = true
	}
	return m, nil
}

// updateBoardName handles the new-board prompt: enter creates and opens, esc cancels.
func (m Model) updateBoardName(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		return m, nil
	case "enter":
		m.mode = modeNormal
		name := strings.TrimSpace(m.boards.in.Value())
		if name == "" {
			return m, nil
		}
		cmd := m.switchBoard(name)
		return m, cmd
	}
	var cmd tea.Cmd
	m.boards.in, cmd = m.boards.in.Update(msg)
	return m, cmd
}

// switchBoard opens (creating if needed) the named board under the root and
// makes it the model's board: fresh store, lane and selection reset, filter
// cleared, archive dropped. The old watcher is stopped and, when watching is
// on, a new one is started through the same path Init uses; its generation
// makes anything still in flight from the old watcher ignorable. On failure
// the picker stays open showing the error.
func (m *Model) switchBoard(name string) tea.Cmd {
	if name == m.b.Name || m.st == nil || m.root == "" {
		m.scr, m.mode = screenBoard, modeNormal
		return nil
	}
	st, b, err := store.Open(m.root, name)
	if err != nil {
		m.boards.err = err.Error()
		return nil
	}
	m.teardown()
	m.watchGen++
	m.st, m.b, m.archive = st, b, nil
	m.filter, m.filterQuery = board.Filter{}, ""
	m.setLane(board.Todo)
	m.scr, m.mode = screenBoard, modeNormal
	if m.watch {
		return startWatch(st, m.watchGen)
	}
	return nil
}
