package tui

import tea "github.com/charmbracelet/bubbletea"

// detailState is the card detail screen state.
type detailState struct {
	id       string
	archived bool
}

func (d *detailState) resize(m *Model) {}

func (m *Model) openDetail(id string, archived bool) {}

func (m Model) renderDetail() []string { return m.renderBoard() }

func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd)   { return m, nil }
func (m Model) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd)     { return m, nil }
func (m Model) updateLanePick(msg tea.KeyMsg) (tea.Model, tea.Cmd) { return m, nil }
