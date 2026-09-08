package tui

import (
	"strings"
	"testing"

	"github.com/kaiohenricunha/kando/internal/board"
)

// TestOneCardCannotInflateTheFrame covers the two read-path bounds, which
// bound different things and are worth stating separately rather than under
// one number.
//
// The notes preview is bounded tightly: FirstNoteLine clips to 512 bytes, so a
// 16 KiB note adds about half a kilobyte to a frame instead of sixteen.
//
// A title is bounded loosely. store.parseSections assigns it verbatim with no
// cap, so a hand-edited board.md can supply any size, and sanitize bounds that
// to maxRenderBytes. That converts unbounded into bounded; it does not make a
// large title cheap, because sanitize cannot be tightened below the largest
// legitimate value it sees — wrapNotes hands it whole notes paragraphs.
// Tightening it per-surface is a separate change; see the PR.
func TestOneCardCannotInflateTheFrame(t *testing.T) {
	t.Run("a 16 KiB note costs half a kilobyte", func(t *testing.T) {
		m := newTestModel(t, 120, 40)
		clean := len(m.View())
		c := m.b.Lanes[board.Todo][0]
		c.SetNotes("a" + strings.Repeat("\u0301", 8000))
		if len(c.Notes) < 8000 {
			t.Fatalf("test premise wrong: notes are %d bytes", len(c.Notes))
		}
		if grew := len(m.View()) - clean; grew > 2048 {
			t.Errorf("a 16 KiB note grew the frame by %d bytes (clean %d)", grew, clean)
		}
	})

	t.Run("a megabyte title is bounded, not emitted", func(t *testing.T) {
		m := newTestModel(t, 120, 40)
		clean := len(m.View())
		c := m.b.Lanes[board.Todo][0]
		// Assigned as a field, the way store.parseSections assigns it —
		// verbatim, uncapped. SetTitle would clip it to 512 and prove nothing.
		c.Title = "a" + strings.Repeat("\u0301", 500_000)
		if len(c.Title) < 500_000 {
			t.Fatalf("test premise wrong: title is %d bytes", len(c.Title))
		}
		// The guarantee is that a megabyte does not reach the terminal, not
		// that the row is free. The literal is deliberate: asserting against
		// maxRenderBytes would move with the constant.
		if grew := len(m.View()) - clean; grew > 80_000 {
			t.Errorf("a 1 MiB title grew the frame by %d bytes (clean %d)", grew, clean)
		}
		for i, l := range strings.Split(m.View(), "\n") {
			if width(l) != 120 {
				t.Fatalf("row %d is %d cells, want 120", i, width(l))
			}
		}
	})
}
