package tui

import (
	"strings"
	"testing"

	"github.com/kaiohenricunha/kando/internal/board"
)

// A section size that does not add up to its footer would drop keys off the
// end of the row, or panic slicing past it.
func TestFooterSectionsCoverTheirFooters(t *testing.T) {
	for _, c := range []struct {
		name  string
		gs    []keyGroup
		sizes []int
	}{
		{"board full", boardFooterFull, boardFullSections},
		{"board reduced", boardFooterReduced, boardReducedSections},
		{"detail", detailFooterGroups, detailSections},
	} {
		sum := 0
		for _, n := range c.sizes {
			sum += n
		}
		if sum != len(c.gs) {
			t.Errorf("%s: sections add up to %d, the footer has %d groups", c.name, sum, len(c.gs))
		}
	}
}

func TestProgressCells(t *testing.T) {
	for _, c := range []struct{ done, total, on, off int }{
		{0, 0, 0, 0},
		{0, 4, 0, 4},
		{1, 4, 1, 3},
		{4, 4, 4, 0},
		{1, 20, 1, 9},  // scaled to 10 cells
		{3, 20, 2, 8},  // 1.5 rounds up
		{19, 20, 9, 1}, // would round to full; one remaining cell stays
		{1, 30, 1, 9},  // would round to empty; one done cell stays
	} {
		on, off := progressCells(c.done, c.total, progressBarMax)
		if on != c.on || off != c.off {
			t.Errorf("progressCells(%d, %d) = %d, %d; want %d, %d", c.done, c.total, on, off, c.on, c.off)
		}
	}
}

// d on an open card is m then 4: the card goes to Done and the detail follows.
func TestDetailDMovesToDone(t *testing.T) {
	m := press(newTestModel(t, 120, 40), "enter")
	id := m.detail.id
	m = press(m, "d")
	if l, _, c := m.b.Find(id); c == nil || l != board.Done {
		t.Fatalf("d must move the open card to Done, found it in %v", l)
	}
	if m.scr != screenDetail || m.detail.id != id || m.lane != board.Done {
		t.Errorf("the detail must follow the card: scr=%v id=%q lane=%v", m.scr, m.detail.id, m.lane)
	}
}

// Only the active lane is boxed. An open lane is a rule down its left edge
// with its header on the active lane's header row, and a card with a
// checklist shows its progress there.
func TestOpenLaneHasNoBoxAndShowsProgress(t *testing.T) {
	lines := plainLines(press(newTestModel(t, 120, 40), "h")) // Backlog active, Todo open
	h := -1
	for i, l := range lines {
		if strings.Contains(l, "BACKLOG") && strings.Contains(l, "TODO") {
			h = i
			break
		}
	}
	if h < 1 {
		t.Fatal("no lane header row")
	}
	if n := strings.Count(lines[h-1], "╭"); n != 1 {
		t.Errorf("the row above the headers should open one box, the active lane's; it opens %d: %q", n, lines[h-1])
	}
	for _, name := range []string{"│ TODO", "│ DOING", "│ DONE"} {
		if !strings.Contains(lines[h], name) {
			t.Errorf("open lane header %q missing: %q", name, lines[h])
		}
	}
	var row string
	for _, l := range lines {
		if strings.Contains(l, "Renew pass") {
			row = l
		}
	}
	if !strings.Contains(row, "1/4") {
		t.Errorf("the open Todo lane should show Renew passport's progress: %q", row)
	}
}
