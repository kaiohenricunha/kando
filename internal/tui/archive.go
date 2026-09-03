package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/kaiohenricunha/kando/internal/board"
)

// archiveState is the archive screen state.
type archiveState struct {
	cursor int
	first  int
}

func (m *Model) openArchive() {}

func (m *Model) clampArchive() {}

func (m Model) visibleArchive() []*board.Card { return nil }

func (m Model) renderArchive() []string { return m.renderBoard() }

func (m Model) updateArchive(msg tea.KeyMsg) (tea.Model, tea.Cmd) { return m, nil }

func (m *Model) selectArchiveByID(id string) {}
