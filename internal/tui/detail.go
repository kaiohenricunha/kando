package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/kaiohenricunha/kando/internal/board"
)

type editKind int

const (
	editNone editKind = iota
	editTitle
	editNotes
	editTag
	editNewItem
	editItem
	editBlock
)

// detailState is the card detail screen state.
type detailState struct {
	id       string
	archived bool // card lives in the archive, not on the board
	cursor   int  // checklist cursor
	edit     editKind
	in       textinput.Model
	ta       textarea.Model
}

const (
	detailLeftWidth = 30
	detailGap       = 4 + 1 + 3 // gap, rule column, padding
	notesWrap       = 64
)

func (m Model) rightPaneWidth() int {
	w := m.w - 2 - detailLeftWidth - detailGap
	if w < 1 {
		w = 1
	}
	return w
}

func (m Model) notesWidth() int {
	w := m.rightPaneWidth()
	if w > notesWrap {
		w = notesWrap
	}
	return w
}

func (d *detailState) resize(m *Model) {
	if m.scr != screenDetail {
		return
	}
	switch d.edit {
	case editNotes:
		d.ta.SetWidth(m.notesWidth())
	case editTitle, editTag, editNewItem, editItem, editBlock:
		d.in.Width = m.rightPaneWidth() - 4
	}
}

func (m *Model) openDetail(id string, archived bool) {
	m.detail = detailState{id: id, archived: archived}
	m.scr = screenDetail
	m.mode = modeNormal
}

// closeDetail returns to the board (or archive) with the card selected.
func (m *Model) closeDetail() {
	m.mode = modeNormal
	if m.detail.archived {
		m.scr = screenArchive
		m.selectArchiveByID(m.detail.id)
		return
	}
	m.scr = screenBoard
	m.selectByID(m.detail.id)
	m.ensureVisible()
}

// detailCard finds the card being shown, or nil if it vanished.
func (m Model) detailCard() *board.Card {
	if m.detail.archived {
		if m.archive == nil {
			return nil
		}
		for _, c := range m.archive.Cards {
			if c.ID == m.detail.id {
				return c
			}
		}
		return nil
	}
	_, _, c := m.b.Find(m.detail.id)
	return c
}

// detailList is the navigation list in the left pane and the cursor position in
// it. The cursor is the FIRST card carrying the open id, the same card
// detailCard shows through Board.Find: with two cards sharing an id, taking the
// last put the cursor on one card and the pane on the other.
func (m Model) detailList() (cards []*board.Card, cur int) {
	if m.detail.archived {
		cards = m.visibleArchive()
	} else {
		cards = m.visible(m.lane)
	}
	for i, c := range cards {
		if c.ID == m.detail.id {
			cur = i
			break
		}
	}
	return cards, cur
}

func (m Model) detailListName() string {
	if m.detail.archived {
		return "archive"
	}
	return m.lane.String()
}

// wrapNotes word-wraps notes at w cells, one entry per output row.
//
// The split runs on the raw value and sanitize runs per paragraph, in that
// order. Reversed, this does nothing: sanitize maps "\n" to a space like the
// rest of C0 (CR is the exception, it is dropped) — correct for its two dozen
// other call sites, each of which renders one exact-width row and must not
// gain a line — so splitting afterwards can never find a newline. Notes are
// the one multi-line value the TUI renders, and this is the only place in the
// repo where the order is load-bearing.
//
// cmd/kando/show.go:80 also splits the raw value, but it is immune either
// way: it sanitizes once over the assembled block with board.SafeForDisplay,
// which keeps newlines. Only the TUI has a sanitizer that destroys them.
func wrapNotes(notes string, w int) []string {
	var out []string
	for _, para := range strings.Split(notes, "\n") {
		para = sanitize(para)
		if strings.TrimSpace(para) == "" {
			out = append(out, "")
			continue
		}
		out = append(out, strings.Split(ansi.Wrap(para, w, ""), "\n")...)
	}
	return out
}

// renderDetail draws the two-pane card screen.
func (m Model) renderDetail() []string {
	c := m.detailCard()
	if c == nil {
		return m.renderBoard()
	}
	s := m.styles
	cw := m.w - 2
	rp := m.rightPaneWidth()
	bodyRows := m.h - 4
	sep := s.Muted.Render(" › ")
	crumb := s.Brand.Render("kando") + "  " + s.Muted.Render(sanitize(m.b.Name)) + sep +
		s.Muted.Render(m.detailListName()) + sep + s.Fg.Render(sanitize(c.Title))
	header := hsplit(crumb, m.dateStr(), cw)
	left := m.detailLeft(bodyRows)
	right := m.detailRight(c, rp)
	body := make([]string, bodyRows)
	for i := range body {
		var l, r string
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		body[i] = fit(l, detailLeftWidth) + spaces(4) + s.Border.Render("│") + spaces(3) + fit(r, rp)
	}
	var footer string
	if m.mode == modeLanePick {
		picks := []keyGroup{{"1", "Backlog"}, {"2", "Todo"}, {"3", "Doing"}, {"4", "Done"}}
		footer = fit(s.Muted.Render("move to:")+"  "+m.groups(picks), cw)
	} else {
		footer = m.footerWithReport(m.groups(detailFooterGroups), cw)
	}
	return m.screenRows(header, body, footer)
}

// detailLeft lists the cards of the current lane with a cursor on the open card.
func (m Model) detailLeft(rows int) []string {
	s := m.styles
	cards, cur := m.detailList()
	name := m.detailListName()
	if m.detail.archived {
		name = "Archive"
	}
	out := []string{
		s.Bold.Render(name) + " " + s.Accent.Render(fmt.Sprint(len(cards))),
		s.Accent.Render(strings.Repeat("─", detailLeftWidth)),
		"",
	}
	avail := rows - len(out)
	top := 0
	if avail > 0 && cur >= avail {
		top = cur - avail + 1
	}
	for i := top; i < len(cards) && i-top < avail; i++ {
		title := fit(sanitize(cards[i].Title), detailLeftWidth-2)
		if i == cur {
			out = append(out, s.SelFgBold.Render("▸ "+title))
		} else {
			out = append(out, s.Muted.Render("• "+title))
		}
	}
	return out
}

// detailRight draws title, meta, notes, checklist and blocked sections.
func (m Model) detailRight(c *board.Card, rp int) []string {
	s := m.styles
	d := m.detail
	var rows []string
	if d.edit == editTitle {
		rows = append(rows, fit(d.in.View(), rp))
	} else {
		rows = append(rows, s.Bold.Render(fit(sanitize(c.Title), rp)))
	}

	var meta []string
	switch {
	case d.edit == editTag:
		meta = append(meta, s.Accent.Render("#")+d.in.View())
	case c.Tag != "":
		meta = append(meta, s.Accent.Render("#"+sanitize(c.Tag)))
	}
	if !c.CreatedAt.IsZero() {
		meta = append(meta, s.Muted.Render("created "+board.DayLabel(c.CreatedAt)))
	}
	if d.archived {
		if !c.DoneAt.IsZero() {
			meta = append(meta, s.Muted.Render("done "+board.DayLabel(c.DoneAt)))
		}
	} else {
		since := c.MovedAt
		if since.IsZero() {
			since = c.CreatedAt
		}
		if !since.IsZero() {
			meta = append(meta, s.Muted.Render("in "+m.lane.String()+" since "+board.DayLabel(since)))
		}
	}
	rows = append(rows, strings.Join(meta, "  "), "", s.Muted.Render("NOTES"))

	switch {
	case d.edit == editNotes:
		rows = append(rows, strings.Split(d.ta.View(), "\n")...)
	case strings.TrimSpace(c.Notes) == "":
		rows = append(rows, s.Muted.Render("— no notes. ")+s.Bold.Render("e")+s.Muted.Render(" to write"))
	default:
		for _, l := range wrapNotes(c.Notes, m.notesWidth()) {
			// render() rather than s.Fg.Render(): a blank paragraph row would
			// otherwise emit an SGR open/close pair around an empty string.
			rows = append(rows, render(s.Fg, l))
		}
	}
	rows = append(rows, "")

	total := len(c.Checklist)
	if pl := c.ProgressLabel(); pl != "" {
		rows = append(rows, s.Muted.Render("CHECKLIST")+" "+s.Accent.Render(pl))
	} else {
		rows = append(rows, s.Muted.Render("CHECKLIST"))
	}
	newItem := s.Fg.Render("▢ ") + d.in.View()
	for i, it := range c.Checklist {
		text := sanitize(it.Text)
		atCursor := i == d.cursor
		var row string
		switch {
		case d.edit == editItem && atCursor:
			row = s.Fg.Render("▢ ") + d.in.View()
		case it.Done && atCursor:
			row = s.SelMuted.Render("▣ ") + s.SelStrike.Render(text)
		case it.Done:
			row = s.Muted.Render("▣ ") + s.Strike.Render(text)
		case atCursor:
			row = s.SelFg.Render("▢ " + text)
		default:
			row = s.Fg.Render("▢ " + text)
		}
		rows = append(rows, row)
		if d.edit == editNewItem && atCursor {
			rows = append(rows, newItem)
		}
	}
	if d.edit == editNewItem && total == 0 {
		rows = append(rows, newItem)
	}
	rows = append(rows, "", s.Muted.Render("BLOCKED"))

	switch {
	case d.edit == editBlock:
		rows = append(rows, s.Accent2.Render("⊘ ")+d.in.View())
	case c.Blocked:
		rows = append(rows, s.Accent2.Render("⊘ "+sanitize(c.BlockedLabel())))
	default:
		rows = append(rows, s.Muted.Render("— not blocked. ")+s.Bold.Render("b")+s.Muted.Render(" to set a reason"))
	}
	return rows
}

// --- keys ---

func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	c := m.detailCard()
	if c == nil {
		m.closeDetail()
		return m, nil
	}
	n := len(c.Checklist)
	switch msg.String() {
	case "esc":
		m.closeDetail()
	case "j", "down":
		if n > 0 {
			m.detail.cursor = (m.detail.cursor + 1) % n
		}
	case "k", "up":
		if n > 0 {
			m.detail.cursor = (m.detail.cursor - 1 + n) % n
		}
	case "J":
		m.detailSwitch(1)
	case "K":
		m.detailSwitch(-1)
	case "x":
		if n > 0 {
			c.ToggleChecklistItem(m.detail.cursor)
			m.saveDetail()
		}
	case "o":
		m.startEdit(editNewItem, "")
	case "enter":
		if n > 0 {
			m.startEdit(editItem, board.SafeForDisplay(c.Checklist[m.detail.cursor].Text))
		}
	case "e":
		// Editors are seeded de-fanged. sanitize() guards the read-only
		// panes, but bubbles renders its own buffer, and its runeutil
		// sanitizer drops only unicode.IsControl — the bidi controls are Cf
		// and survive it. Without this, pressing a key to edit a hand-edited
		// field would show an override that the pane one keystroke earlier
		// had stripped. Nothing is lost: the Set* helpers strip the same
		// runes when the edit is committed.
		m.detail.ta = m.newTextarea(board.SafeForDisplay(c.Notes))
		m.detail.edit = editNotes
		m.mode = modeEdit
	case "T":
		m.startEdit(editTitle, board.SafeForDisplay(c.Title))
	case "t":
		m.startEdit(editTag, board.SafeForDisplay(c.Tag))
	case "b":
		m.startEdit(editBlock, board.SafeForDisplay(c.BlockedReason))
	case "m":
		m.mode = modeLanePick
	case "A":
		m.archiveDetailCard()
	case "?":
		m.help = true
	}
	return m, nil
}

// detailSwitch moves to the next/previous card of the navigation list (wrapping).
func (m *Model) detailSwitch(delta int) {
	cards, cur := m.detailList()
	if len(cards) == 0 {
		return
	}
	next := (cur + delta + len(cards)) % len(cards)
	m.detail.id = cards[next].ID
	m.detail.cursor = 0
	if m.detail.archived {
		m.arch.cursor = next
	} else {
		m.sel = next
		m.ensureVisible()
	}
}

func (m *Model) saveDetail() {
	if m.detail.archived {
		m.saveArchive()
	} else {
		m.save()
	}
}

func (m *Model) startEdit(kind editKind, initial string) {
	m.detail.in = m.newInput(m.styles.Fg, m.styles.Fg)
	m.detail.in.SetValue(initial)
	m.detail.in.CursorEnd()
	m.detail.in.Width = m.rightPaneWidth() - 4
	m.detail.edit = kind
	m.mode = modeEdit
}

// newTextarea builds the notes editor: no prompt, no line numbers, static cursor,
// wrapped at the notes width, tall enough for the current text plus one line.
func (m Model) newTextarea(text string) textarea.Model {
	s := m.styles
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.EndOfBufferCharacter = ' '
	ta.Cursor.SetMode(cursor.CursorStatic)
	ta.Cursor.Style = s.Fg
	ta.Cursor.TextStyle = s.Fg
	ta.FocusedStyle.Base = s.Plain
	ta.FocusedStyle.CursorLine = s.Fg
	ta.FocusedStyle.Text = s.Fg
	ta.FocusedStyle.EndOfBuffer = s.Plain
	ta.FocusedStyle.Placeholder = s.Muted
	ta.BlurredStyle = ta.FocusedStyle
	ta.SetWidth(m.notesWidth())
	h := len(wrapNotes(text, m.notesWidth())) + 1
	if h < 3 {
		h = 3
	}
	if max := m.h - 4 - 12; h > max && max >= 3 {
		h = max
	}
	ta.SetHeight(h)
	ta.SetValue(text)
	ta.Focus()
	return ta
}

// updateEdit routes keys to the active inline editor.
func (m Model) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "esc" {
		m.detail.edit = editNone
		m.mode = modeNormal
		return m, nil
	}
	if m.detail.edit == editNotes {
		if key == "ctrl+s" {
			m.commitEdit()
			return m, nil
		}
		var cmd tea.Cmd
		m.detail.ta, cmd = m.detail.ta.Update(msg)
		return m, cmd
	}
	if key == "enter" {
		m.commitEdit()
		return m, nil
	}
	var cmd tea.Cmd
	m.detail.in, cmd = m.detail.in.Update(msg)
	return m, cmd
}

// commitEdit applies the editor's value to the card and saves.
func (m *Model) commitEdit() {
	c := m.detailCard()
	kind := m.detail.edit
	m.detail.edit = editNone
	m.mode = modeNormal
	if c == nil {
		return
	}
	switch kind {
	case editTitle:
		if !c.SetTitle(m.detail.in.Value()) {
			return
		}
	case editNotes:
		c.SetNotes(m.detail.ta.Value())
	case editTag:
		c.SetTag(m.detail.in.Value())
	case editNewItem:
		at := c.InsertChecklistItem(m.detail.cursor, m.detail.in.Value())
		if at < 0 {
			return
		}
		m.detail.cursor = at
	case editItem:
		if !c.SetChecklistItemText(m.detail.cursor, m.detail.in.Value()) {
			return
		}
	case editBlock:
		c.SetBlocked(m.detail.in.Value())
	}
	m.saveDetail()
}

// updateLanePick handles the "move to" footer prompt.
func (m Model) updateLanePick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
	case "1", "2", "3", "4":
		m.mode = modeNormal
		m.moveDetailCard(board.Lane(msg.String()[0] - '1'))
	}
	return m, nil
}

// moveDetailCard moves the open card to lane `to` (out of the archive if needed)
// and follows it: the destination lane becomes active with the card selected.
// Moving an archived card out is a restore, so it takes u's guard: refused, with
// nothing written and the card left open, when its id is already on the board.
func (m *Model) moveDetailCard(to board.Lane) {
	c := m.detailCard()
	if c == nil {
		return
	}
	now := m.now()
	if m.detail.archived {
		if m.restoreRefused(c) {
			return
		}
		for i, x := range m.archive.Cards {
			if x == c {
				m.b.Unarchive(m.archive, i, to, now)
				break
			}
		}
		m.saveRestore()
		m.detail.archived = false
	} else {
		l, i, _ := m.b.Find(c.ID)
		m.b.Move(l, i, to, now)
		m.save()
	}
	m.setLane(to)
	m.selectByID(c.ID)
	m.ensureVisible()
}

// archiveDetailCard is A on an open card: archive it and return to the board,
// where the card's own page no longer exists — what the web's button does too.
// The screen changes only when the archive happened. A refusal (the card is
// already archived, is not in Done, or archive.md cannot be read) stays on the
// card with the reason in the footer, so a key that did nothing does not look
// like one that did.
func (m *Model) archiveDetailCard() {
	c := m.detailCard()
	if c == nil {
		return
	}
	if m.detail.archived {
		// An archived card is in no lane, so archiveDone's lane check would miss
		// it and find no lane to name in its refusal.
		m.notice = fmt.Sprintf("%q is already archived", c.Title)
		return
	}
	if !m.archiveDone(c) {
		return
	}
	m.scr = screenBoard
	m.mode = modeNormal
	m.clampSel()
}
