package tui

import (
	"strings"
	"testing"

	"github.com/kaiohenricunha/kando/internal/board"
)

// TestOneCardCannotInflateTheFrame is the end-to-end check on the byte bounds.
// It is deliberately about bytes rather than cells, because the cell contract
// was never the thing at risk — ansi.StringWidth clusters correctly, so a base
// character with thousands of combining marks measures one cell and every row
// stayed exactly w wide. What was unbounded was what got written: trunc
// returned a value verbatim whenever its cell count already fitted, and
// FirstNoteLine handed over a whole 16 KiB notes body.
//
// Measured on this fixture at 120x40: a clean frame is ~14 KB, and one
// pathological card used to add ~16 KB to every frame. It now adds ~0.5 KB.
func TestOneCardCannotInflateTheFrame(t *testing.T) {
	m := newTestModel(t, 120, 40)
	clean := len(m.View())

	c := m.b.Lanes[board.Todo][0]
	c.SetNotes("a" + strings.Repeat("\u0301", 8000))
	if len(c.Notes) < 8000 {
		t.Fatalf("test premise wrong: notes are %d bytes, the card is not pathological", len(c.Notes))
	}

	v := m.View()
	// Generous: the preview budget is 512 bytes, and styling adds a little.
	if grew := len(v) - clean; grew > 2048 {
		t.Errorf("one card grew the frame by %d bytes (clean %d, got %d)", grew, clean, len(v))
	}
	// The contract that was always fine, asserted so it stays that way.
	for i, l := range strings.Split(v, "\n") {
		if width(l) != 120 {
			t.Fatalf("row %d is %d cells, want 120", i, width(l))
		}
	}
}
