package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
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
// It reports whether the archive is usable: on a load failure it records the
// error and leaves m.archive nil rather than an empty archive, because a
// caller that writes would otherwise persist that empty archive over the very
// file it could not read. nil also makes the next call retry the load instead
// of inheriting a fabricated state; every reader already treats nil as an
// empty archive (visibleArchive).
func (m *Model) ensureArchive() bool {
	if m.archive != nil {
		return true
	}
	if m.st == nil {
		m.archive = &board.Archive{}
		return true
	}
	a, err := m.st.LoadArchive()
	if err != nil {
		m.errs.archive = err
		return false
	}
	m.errs.archive = nil
	m.archive = a
	return true
}

// openArchive loads archive.md on first use and shows the archive screen.
// A failed load still opens the screen — it renders empty with the error in
// the footer, as it did before A existed — because nothing on this path writes.
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
		footer = m.footerWithReport(m.groups(archiveFooterGroups), cw)
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

// restoreRefused reports whether restoring c must be refused because a card
// with its id is already on the board, and records why in m.notice in the CLI's
// words — the guard the web's restore route answers with a 409 and `kando
// archive restore` with an error. After a restore or archive that half failed,
// the card sits in both files. Restoring anyway would put two cards with one id
// on the board, and the next open keeps the id on the card Board.Find reaches
// first (store.assignIDs): a restored card lands in Doing, ahead of a live twin
// in Done, so the card that silently lost its id would be the live one that
// links and scripts point at.
func (m *Model) restoreRefused(c *board.Card) bool {
	if _, _, dup := m.b.Find(c.ID); dup == nil {
		return false
	}
	m.notice = fmt.Sprintf("%q is already on the board", c.Title)
	return true
}

// moveSnapshot copies what a two-file move is about to change, so a save that
// the store refuses can be undone in memory: the move already happened before
// the save runs, and a refusal must not leave the card split between the
// board and the archive in RAM while neither file was touched. Insert and
// Remove shift a slice's backing array in place, so a copy of the slice
// headers would not restore what they pointed at — the slices themselves have
// to be copied, and c's own fields, which Unarchive and ArchiveDone stamp in
// place, restored by value.
type moveSnapshot struct {
	lanes   [4][]*board.Card
	archive []*board.Card
	card    board.Card
}

// snapshotBeforeMove copies the board's four lanes, the archive list, and c's
// own fields, all of which restoreArchived, archiveDone and moveDetailCard's
// archived branch are about to change.
func (m *Model) snapshotBeforeMove(c *board.Card) moveSnapshot {
	var snap moveSnapshot
	for l := range m.b.Lanes {
		snap.lanes[l] = append([]*board.Card(nil), m.b.Lanes[l]...)
	}
	snap.archive = append([]*board.Card(nil), m.archive.Cards...)
	snap.card = *c
	snap.card.Checklist = append([]board.Item(nil), c.Checklist...)
	return snap
}

// undoMove puts back what snapshotBeforeMove copied. c keeps its identity, so
// the selection and any open detail still resolve to it afterward.
func (m *Model) undoMove(c *board.Card, snap moveSnapshot) {
	m.b.Lanes = snap.lanes
	m.archive.Cards = snap.archive
	*c = snap.card
}

// restoreArchived moves an archived card back to the top of Doing — u on the
// archive screen. A refusal that wrote nothing — the id is already on the
// board, or the save fails before either file is touched — undoes the move
// in memory and leaves the cursor where it was, with the reason in the
// footer. A save that lands board.md but fails archive.md (ErrPartialWrite)
// keeps the move: board.md, the file that succeeded, already agrees with it,
// and undoing would only make memory disagree with the file on disk.
func (m *Model) restoreArchived(c *board.Card) {
	if m.restoreRefused(c) {
		return
	}
	snap := m.snapshotBeforeMove(c)
	for i, x := range m.archive.Cards {
		if x == c {
			m.b.Restore(m.archive, i, m.now())
			break
		}
	}
	if !m.saveRestore() && !errors.Is(m.errs.save, store.ErrPartialWrite) {
		m.undoMove(c, snap)
		return
	}
	m.clampArchive()
	if m.lane == board.Doing {
		m.clampSel()
	}
}

// archiveDone moves c — which must be in Done — into the archive, saves both
// files, and reports whether it did. It refuses, with the reason in m.notice in
// the CLI's words, when c is not in Done or is already archived: a stale
// selection racing an external edit, or an archive that half failed after
// archive.md was written. Those are the two guards the web's archive route
// answers with a 409 and `kando archive` with an error. It also refuses when
// archive.md cannot be read (ensureArchive records that error): writing then
// would replace an archive we never saw with a one-card file, which is the one
// way this feature could lose a card rather than duplicate it. The web route
// refuses that case with a 500.
//
// A save that lands archive.md but fails board.md (ErrPartialWrite) still
// reports true: archive.md, the file that succeeded, already agrees with the
// move, and undoing it in memory would only make memory disagree with the
// file on disk. The stale board.md is a duplicate the next board save
// replaces; the error itself still stands in m.errs.save until that save
// clears it.
func (m *Model) archiveDone(c *board.Card) bool {
	i := m.laneIndex(board.Done, c)
	if i < 0 {
		// Both callers hand over a card that is on the board, so it is in some
		// lane; find which by pointer, as laneIndex does, rather than by id.
		for _, l := range board.Lanes {
			if m.laneIndex(l, c) >= 0 {
				m.notice = fmt.Sprintf("%q is in %s, not Done", c.Title, l)
			}
		}
		return false
	}
	if !m.ensureArchive() {
		return false
	}
	if _, dup := m.archive.Find(c.ID); dup != nil {
		m.notice = fmt.Sprintf("%q is already archived", c.Title)
		return false
	}
	snap := m.snapshotBeforeMove(c)
	m.b.ArchiveDone(m.archive, i, m.now())
	if !m.saveArchival() && !errors.Is(m.errs.save, store.ErrPartialWrite) {
		m.undoMove(c, snap)
		return false
	}
	return true
}
