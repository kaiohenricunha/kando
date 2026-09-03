package tui

import "github.com/kaiohenricunha/kando/internal/board"

// layout is the board geometry for one terminal size.
type layout struct {
	tooSmall bool
	cw       int    // content width (w-2)
	narrow   bool   // w < 100: tab strip + full-width lane
	laneW    [4]int // outer lane widths (only the active lane in narrow mode)
	laneH    int    // outer lane height
}

func computeLayout(w, h int, active board.Lane) layout {
	if w < 60 || h < 16 {
		return layout{tooSmall: true}
	}
	L := layout{cw: w - 2}
	if w < 100 {
		L.narrow = true
		L.laneH = h - 6
		L.laneW[active] = L.cw
		return L
	}
	col := 22
	if w < 120 {
		col = 18
	}
	L.laneH = h - 4
	for l := range L.laneW {
		L.laneW[l] = col
	}
	L.laneW[active] = L.cw - 3*col - 6
	return L
}

func (m Model) layout() layout { return computeLayout(m.w, m.h, m.lane) }

// activeListRows is the number of content rows available to the card list of
// the active lane (after header, blank row and an open quick-add card).
func (m Model) activeListRows() int {
	L := m.layout()
	if L.tooSmall {
		return 0
	}
	R := L.laneH - 4
	if m.mode == modeQuickAdd {
		R -= 6
	}
	if R < 0 {
		R = 0
	}
	return R
}

// cardTextWidth is the text width inside a card of the active lane.
func (m Model) cardTextWidth() int {
	L := m.layout()
	if L.tooSmall {
		return 0
	}
	return L.laneW[m.lane] - 4 - 6
}

// window describes which whole cards fit in R content rows.
type window struct {
	start  int // 1 when a "… +N" row precedes the first card
	shown  int
	hidden int
}

// laneWindow computes the whole-card window: cards are 5 rows plus one blank
// separator; a "… +first" indicator takes the first row when scrolled.
func laneWindow(R, n, first int) window {
	if first > n {
		first = n
	}
	var w window
	if first > 0 {
		w.start = 1
	}
	avail := R - w.start
	if avail >= 5 {
		w.shown = (avail + 1) / 6
	}
	if w.shown > n-first {
		w.shown = n - first
	}
	w.hidden = n - first - w.shown
	return w
}
