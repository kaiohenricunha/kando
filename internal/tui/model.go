package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

type screen int

const (
	screenBoard screen = iota
	screenDetail
	screenArchive
	screenBoards
)

type mode int

const (
	modeNormal mode = iota
	modeQuickAdd
	modeFilter
	modeEdit
	modeLanePick
	modeBoardName
)

// Options configures a Model. Store may be nil (pure in-memory model). Root is
// the KANDO_HOME directory the board picker lists and opens boards from. With
// Watch set, the model owns the file watcher end to end: Init starts one on
// the current store, a board switch stops it and starts another, quit tears
// it down, and a failure to watch is shown in the footer and retried on the
// next switch.
type Options struct {
	Store   *store.Store
	Board   *board.Board
	Archive *board.Archive
	Root    string
	Styles  Styles
	Now     func() time.Time
	Watch   bool
	Width   int
	Height  int
}

// Watcher messages carry the generation they belong to; a board switch bumps
// the generation so anything from a previous watcher is ignored.
type (
	// watchStartedMsg is the result of startWatch.
	watchStartedMsg struct {
		gen  int
		ch   <-chan struct{}
		stop func()
		err  error
	}
	// changeMsg says the board directory changed on disk.
	changeMsg struct{ gen int }
	// watchStoppedMsg says the watcher channel closed.
	watchStoppedMsg struct{ gen int }
)

// Model is the whole UI state. Every View() call renders the full frame from it.
type Model struct {
	st      *store.Store
	b       *board.Board
	archive *board.Archive
	root    string
	styles  Styles
	now     func() time.Time

	watch     bool            // policy: keep a watcher on the current store
	watchGen  int             // bumped on every board switch
	changes   <-chan struct{} // live watcher channel, nil while none is running
	stopWatch func()
	tick      time.Time // instant the last frame was evaluated at (filters, ages)

	w, h int
	scr  screen
	mode mode
	help bool

	lane  board.Lane // active lane
	sel   int        // selected index within the visible cards of the active lane
	first int        // first visible card index (whole-card scrolling)

	filterIn    textinput.Model
	filter      board.Filter
	filterQuery string // kept query shown in the header

	quick textinput.Model

	detail detailState
	arch   archiveState
	boards boardsState

	err error
}

// New builds a Model. The Todo lane starts active with its first card selected.
func New(o Options) Model {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Board == nil {
		o.Board = &board.Board{Name: "life"}
	}
	return Model{
		st:      o.Store,
		b:       o.Board,
		archive: o.Archive,
		root:    o.Root,
		styles:  o.Styles,
		now:     o.Now,
		watch:   o.Watch,
		tick:    o.Now(),
		w:       o.Width,
		h:       o.Height,
		lane:    board.Todo,
	}
}

// Init starts the watcher on the current store when watching is enabled.
func (m Model) Init() tea.Cmd {
	if !m.watch || m.st == nil {
		return nil
	}
	return startWatch(m.st, m.watchGen)
}

// startWatch opens a watcher on st and reports it (or the failure) tagged with gen.
func startWatch(st *store.Store, gen int) tea.Cmd {
	return func() tea.Msg {
		ch, stop, err := st.Watch()
		return watchStartedMsg{gen: gen, ch: ch, stop: stop, err: err}
	}
}

// waitChange blocks until the watcher signals (changeMsg) or is stopped
// (watchStoppedMsg), tagging the result with the watcher's generation.
func waitChange(ch <-chan struct{}, gen int) tea.Cmd {
	return func() tea.Msg {
		if _, ok := <-ch; !ok {
			return watchStoppedMsg{gen: gen}
		}
		return changeMsg{gen: gen}
	}
}

// teardown releases the current watcher, if any.
func (m *Model) teardown() {
	if m.stopWatch != nil {
		m.stopWatch()
		m.stopWatch = nil
	}
	m.changes = nil
}

// Update routes messages, then freezes the clock the next frame is evaluated
// at. Keys are handled against the instant of the frame the user is looking
// at, so a time-dependent filter cannot shift the selection between a paint
// and the key that acts on it.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.dispatch(msg)
	next.tick = next.now()
	return next, cmd
}

func (m Model) dispatch(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.resizeInputs()
		m.ensureVisible()
		return m, nil
	case watchStartedMsg:
		if msg.gen != m.watchGen { // a switch happened meanwhile; not ours
			if msg.stop != nil {
				msg.stop()
			}
			return m, nil
		}
		if msg.err != nil {
			m.err = fmt.Errorf("file watching disabled: %w", msg.err)
			return m, nil
		}
		m.changes, m.stopWatch = msg.ch, msg.stop
		return m, waitChange(msg.ch, msg.gen)
	case changeMsg:
		if msg.gen != m.watchGen { // from a watcher we no longer own
			return m, nil
		}
		m.reload()
		if m.changes == nil {
			return m, nil
		}
		return m, waitChange(m.changes, msg.gen)
	case watchStoppedMsg:
		return m, nil
	case tea.KeyMsg:
		next, cmd := m.handleKey(msg)
		return next.(Model), cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		m.teardown()
		return m, tea.Quit
	}
	switch m.mode {
	case modeQuickAdd:
		return m.updateQuickAdd(msg)
	case modeFilter:
		return m.updateFilter(msg)
	case modeEdit:
		return m.updateEdit(msg)
	case modeLanePick:
		return m.updateLanePick(msg)
	case modeBoardName:
		return m.updateBoardName(msg)
	}
	if m.help {
		if key == "?" || key == "esc" {
			m.help = false
		}
		return m, nil
	}
	switch m.scr {
	case screenDetail:
		return m.updateDetail(msg)
	case screenArchive:
		return m.updateArchive(msg)
	case screenBoards:
		return m.updateBoards(msg)
	}
	return m.updateBoard(msg)
}

// View renders exactly h rows of exactly w cells, joined by newlines.
func (m Model) View() string {
	return strings.Join(frame(m.render(), m.w, m.h), "\n")
}

// render produces the screen rows before frame() normalisation. Every row is
// meant to be exactly w cells already; tests verify that.
func (m Model) render() []string {
	if m.w < 60 || m.h < 16 {
		return frame([]string{"terminal too small (min 60x16)"}, m.w, m.h)
	}
	var rows []string
	switch m.scr {
	case screenDetail:
		rows = m.renderDetail()
	case screenArchive:
		rows = m.renderArchive()
	case screenBoards:
		rows = m.renderBoards()
	default:
		rows = m.renderBoard()
	}
	if m.help {
		rows = m.overlayHelp(rows)
	}
	return rows
}

// screenRows assembles the 4-row frame: header, blank, body, blank, footer,
// with one cell of padding left and right.
func (m Model) screenRows(header string, body []string, footer string) []string {
	cw := m.w - 2
	rows := make([]string, 0, m.h)
	rows = append(rows, " "+fit(header, cw)+" ", spaces(m.w))
	for i := 0; i < m.h-4; i++ {
		if i < len(body) {
			rows = append(rows, " "+fit(body[i], cw)+" ")
		} else {
			rows = append(rows, spaces(m.w))
		}
	}
	rows = append(rows, spaces(m.w), " "+fit(footer, cw)+" ")
	return rows
}

// --- board state helpers ---

// visible returns the lane's cards after the active filter.
func (m Model) visible(l board.Lane) []*board.Card {
	cards := m.b.Lanes[l]
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

// visibleCount counts cards across the four lanes after the filter.
func (m Model) visibleCount() int {
	n := 0
	for _, l := range board.Lanes {
		n += len(m.visible(l))
	}
	return n
}

// selectedCard is the card under the cursor in the active lane, or nil.
func (m Model) selectedCard() *board.Card {
	v := m.visible(m.lane)
	if m.sel >= 0 && m.sel < len(v) {
		return v[m.sel]
	}
	return nil
}

// laneIndex finds c's position in the unfiltered lane.
func (m Model) laneIndex(l board.Lane, c *board.Card) int {
	for i, x := range m.b.Lanes[l] {
		if x == c {
			return i
		}
	}
	return -1
}

// selectByID moves the selection to the card with the given id if it is visible
// in the active lane.
func (m *Model) selectByID(id string) {
	for i, c := range m.visible(m.lane) {
		if c.ID == id {
			m.sel = i
			return
		}
	}
}

// clampSel keeps sel and first inside the visible cards.
func (m *Model) clampSel() {
	n := len(m.visible(m.lane))
	if n == 0 {
		m.sel, m.first = 0, 0
		return
	}
	if m.sel >= n {
		m.sel = n - 1
	}
	if m.sel < 0 {
		m.sel = 0
	}
	if m.first > m.sel {
		m.first = m.sel
	}
	m.ensureVisible()
}

// ensureVisible scrolls in whole-card steps until the selected card is on screen.
func (m *Model) ensureVisible() {
	n := len(m.visible(m.lane))
	if n == 0 {
		m.first = 0
		return
	}
	if m.sel < m.first {
		m.first = m.sel
	}
	R := m.activeListRows()
	for m.first < m.sel {
		win := laneWindow(R, n, m.first)
		if win.shown == 0 || m.sel < m.first+win.shown {
			break
		}
		m.first++
	}
}

func (m *Model) setLane(l board.Lane) {
	m.lane = l
	m.sel, m.first = 0, 0
}

// save persists the board after a mutation.
func (m *Model) save() {
	if m.st == nil {
		return
	}
	if err := m.st.SaveBoard(m.b); err != nil {
		m.err = err
		return
	}
	m.err = nil
}

// saveArchive persists the archive after a mutation.
// saveRestore writes both files of a restore through the store, which owns
// the order that fails safely (board.md first): the same durability rule the
// web's restore button gets.
func (m *Model) saveRestore() {
	if m.st == nil || m.archive == nil {
		return
	}
	if err := m.st.SaveRestore(m.b, m.archive); err != nil {
		m.err = err
		return
	}
	m.err = nil
}

// saveArchival writes both files of an archive move through the store,
// which owns the order that fails safely (archive.md first) — A's
// counterpart to saveRestore.
func (m *Model) saveArchival() {
	if m.st == nil || m.archive == nil {
		return
	}
	if err := m.st.SaveArchival(m.b, m.archive); err != nil {
		m.err = err
		return
	}
	m.err = nil
}

func (m *Model) saveArchive() {
	if m.st == nil || m.archive == nil {
		return
	}
	if err := m.st.SaveArchive(m.archive); err != nil {
		m.err = err
		return
	}
	m.err = nil
}

// reload applies external file changes while keeping the UI state.
func (m *Model) reload() {
	if m.st == nil {
		return
	}
	r, err := m.st.CheckReload()
	if err != nil {
		m.err = err
		return
	}
	if r.Board != nil {
		var selID, detailID string
		if c := m.selectedCard(); c != nil {
			selID = c.ID
		}
		if m.scr == screenDetail && !m.detail.archived {
			detailID = m.detail.id
		}
		m.b = r.Board
		if selID != "" {
			m.selectByID(selID)
		}
		m.clampSel()
		if detailID != "" {
			if _, _, c := m.b.Find(detailID); c == nil {
				m.scr, m.mode = screenBoard, modeNormal
			}
		}
	}
	if r.Archive != nil {
		m.archive = r.Archive
		m.clampArchive()
	}
}

// resizeInputs keeps text inputs sized to the current frame.
func (m *Model) resizeInputs() {
	if m.mode == modeQuickAdd {
		m.quick.Width = m.cardTextWidth() - 1
	}
	if m.mode == modeFilter {
		m.filterIn.Width = m.filterWidth()
	}
	m.detail.resize(m)
}
