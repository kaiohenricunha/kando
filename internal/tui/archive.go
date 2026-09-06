package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kaiohenricunha/kando/internal/board"
)

// archiveMaxItems caps the list at the most recent entries; the footnote points
// at the file for the rest.
const archiveMaxItems = 50

// archiveState is the archive screen state.
type archiveState struct {
	cursor int
}

// ensureArchive loads archive.md on first use, so a screen or a key that
// touches the archive works whether or not the archive screen was opened
// first — most notably A, which archives a card without ever showing it.
func (m *Model) ensureArchive() {
	if m.archive != nil {
		return
	}
	m.archive = &board.Archive{}
	if m.st != nil {
		if a, err := m.st.LoadArchive(); err != nil {
			m.err = err
		} else {
			m.archive = a
		}
	}
}

// openArchive loads archive.md on first use and shows the archive screen.
func (m *Model) openArchive() {
	m.ensureArchive()
	m.scr = screenArchive
	m.mode = modeNormal
	m.arch.cursor = 0
}

func (m *Model) clampArchive() {
	n := len(m.visibleArchive())
	if n == 0 {
		m.arch.cursor = 0
		return
	}
	if m.arch.cursor >= n {
		m.arch.cursor = n - 1
	}
	if m.arch.cursor < 0 {
		m.arch.cursor = 0
	}
}

func (m *Model) selectArchiveByID(id string) {
	for i, c := range m.visibleArchive() {
		if c.ID == id {
			m.arch.cursor = i
			return
		}
	}
	m.clampArchive()
}

// visibleArchive is the 50 most recent archived cards after the active filter.
func (m Model) visibleArchive() []*board.Card {
	if m.archive == nil {
		return nil
	}
	cards := m.archive.Cards
	if len(cards) > archiveMaxItems {
		cards = cards[:archiveMaxItems]
	}
	if m.filter.Empty() {
		return cards
	}
	now := m.tick
	out := make([]*board.Card, 0, len(cards))
	for _, c := range cards {
		if m.filter.Match(c, now) {
			out = append(out, c)
		}
	}
	return out
}

func (m Model) archivePath() string {
	if m.st != nil {
		return m.st.ArchiveDisplayPath()
	}
	return "~/.kando/" + m.b.Name + "/archive.md"
}

type archiveGroup struct {
	label string
	items []*board.Card
}

// archiveGroups buckets cards by week; empty groups are omitted.
func (m Model) archiveGroups(cards []*board.Card) []archiveGroup {
	now := m.now()
	var gs [3]archiveGroup
	for g := board.ThisWeek; g <= board.Earlier; g++ {
		gs[g].label = g.Label()
	}
	for _, c := range cards {
		g := board.GroupOf(now, c.DoneAt)
		gs[g].items = append(gs[g].items, c)
	}
	var out []archiveGroup
	for _, g := range gs {
		if len(g.items) > 0 {
			out = append(out, g)
		}
	}
	return out
}

// archiveItemRow draws "✓ title  tag  date" in w cells.
func (m Model) archiveItemRow(c *board.Card, w int, selected bool) string {
	p := m.palette(selected)
	title := fit(sanitize(c.Title), w-24)
	tag := ""
	if c.Tag != "" {
		tag = "#" + sanitize(c.Tag)
	}
	date := ""
	if !c.DoneAt.IsZero() {
		date = board.DayLabel(c.DoneAt)
	}
	return p.accent.Render("✓") + render(p.fill, " ") + p.fg.Render(title) + render(p.fill, "  ") +
		p.accent.Render(fit(tag, 8)) + render(p.fill, "  ") + p.muted.Render(right(date, 10))
}

// renderArchive draws the archive screen.
func (m Model) renderArchive() []string {
	s := m.styles
	cw := m.w - 2
	w := cw
	if w > 80 {
		w = 80
	}
	cards := m.visibleArchive()

	var header string
	if m.mode == modeFilter {
		header = m.filterHeader(cw)
	} else {
		left := s.Brand.Render("kando") + "  " + s.Muted.Render(sanitize(m.b.Name)) + s.Muted.Render(" › archive")
		if m.filterQuery != "" {
			left += "  " + s.Muted.Render("/"+sanitize(m.filterQuery))
		}
		total := 0
		if m.archive != nil {
			total = len(m.archive.Cards)
		}
		header = hsplit(left, s.Muted.Render(fmt.Sprintf("%d done", total)), cw)
	}

	var rows []string
	cursorRow, idx := 0, 0
	groups := m.archiveGroups(cards)
	for gi, g := range groups {
		if gi > 0 {
			rows = append(rows, "")
		}
		rows = append(rows, s.Muted.Render(g.label)+" "+s.Muted.Render(fmt.Sprint(len(g.items))))
		rows = append(rows, s.Border.Render(strings.Repeat("─", w)))
		for _, c := range g.items {
			selected := idx == m.arch.cursor
			rows = append(rows, m.archiveItemRow(c, w, selected))
			if selected {
				cursorRow = len(rows) - 1
			}
			idx++
		}
	}
	if len(groups) > 0 {
		rows = append(rows, "")
	}
	rows = append(rows, s.Muted.Render("Older entries live in ")+s.Fg.Render(m.archivePath()))

	bodyRows := m.h - 4
	top := 0
	if cursorRow >= bodyRows {
		top = cursorRow - bodyRows + 1
	}
	var footer string
	if m.mode == modeFilter {
		footer = m.filterFooter(cw)
	} else {
		footer = fit(m.groups(archiveFooterGroups), cw)
	}
	return m.screenRows(header, rows[top:], footer)
}

func (m Model) updateArchive(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cards := m.visibleArchive()
	n := len(cards)
	switch msg.String() {
	case "esc":
		m.scr = screenBoard
		m.mode = modeNormal
	case "j", "down":
		if n > 0 {
			m.arch.cursor = (m.arch.cursor + 1) % n
		}
	case "k", "up":
		if n > 0 {
			m.arch.cursor = (m.arch.cursor - 1 + n) % n
		}
	case "u":
		if n > 0 {
			m.restoreArchived(cards[m.arch.cursor])
		}
	case "enter":
		if n > 0 {
			m.openDetail(cards[m.arch.cursor].ID, true)
		}
	case "/":
		m.openFilter()
	case "?":
		m.help = true
	}
	return m, nil
}

// restoreArchived moves an archived card back to the top of Doing.
func (m *Model) restoreArchived(c *board.Card) {
	for i, x := range m.archive.Cards {
		if x == c {
			m.b.Restore(m.archive, i, m.now())
			break
		}
	}
	m.saveRestore()
	m.clampArchive()
	if m.lane == board.Doing {
		m.clampSel()
	}
}

// archiveDone moves c — which must be in Done — into the archive and saves
// both files. A no-op if c is not in Done or is already archived (a stale
// selection racing an external edit), the same class of guard the web's
// archive route enforces with a 409.
func (m *Model) archiveDone(c *board.Card) {
	i := m.laneIndex(board.Done, c)
	if i < 0 {
		return
	}
	m.ensureArchive()
	if _, dup := m.archive.Find(c.ID); dup != nil {
		return
	}
	m.b.ArchiveDone(m.archive, i, m.now())
	m.saveArchival()
}
