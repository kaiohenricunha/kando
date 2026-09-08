package tui

import (
	"strings"
	"testing"
)

// Legitimate single- and double-cell grapheme clusters, kept here as concrete
// values rather than described, because the byte ceiling below must not touch
// any of them. Measured: the flag is 28 bytes over 2 cells, the family 18 over
// 2 — 14 bytes per cell is the densest legitimate text this repo knows of.
const (
	englandFlag = "\U0001F3F4\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F"
	familyEmoji = "\U0001F468\u200d\U0001F469\u200d\U0001F467"
)

// TestTruncBoundsBytes covers the hole that made a pathological value cheap to
// measure and expensive to emit: trunc returned s verbatim whenever it already
// fitted the cell count, and a base character followed by hundreds of combining
// marks is one grapheme cluster of width 1. A 16 KiB note preview therefore
// went into a one-cell slot in full, on every frame.
//
// The cell arithmetic was never wrong — ansi.StringWidth clusters correctly and
// fit() pads from the same number. Only the byte count was unbounded.
func TestTruncBoundsBytes(t *testing.T) {
	zalgo := "a" + strings.Repeat("\u0301", 500)
	if got := width(zalgo); got != 1 {
		t.Fatalf("test premise wrong: width = %d, want 1 (grapheme clustering)", got)
	}

	for _, w := range []int{1, 10, 64} {
		got := trunc(zalgo, w)
		if len(got) > w*maxSlotBytes {
			t.Errorf("trunc(zalgo, %d) emitted %d bytes, ceiling is %d", w, len(got), w*maxSlotBytes)
		}
		if fitted := fit(zalgo, w); width(fitted) != w {
			t.Errorf("fit(zalgo, %d) is %d cells wide — the ceiling broke the cell contract", w, width(fitted))
		}
	}
}

// TestSlotCeilingLeavesLegitimateClustersAlone is the other half. A ceiling
// that clipped a flag or a ZWJ family would be worse than the problem it
// solves, and neither appears in any fixture, so nothing else would catch it.
func TestSlotCeilingLeavesLegitimateClustersAlone(t *testing.T) {
	for _, tc := range []struct{ name, s string }{
		{"england flag tag sequence", englandFlag},
		{"ZWJ family emoji", familyEmoji},
		{"repeated flags", strings.Repeat(englandFlag, 8)},
		{"accented latin", "Café résumé for the tax office"},
		{"cjk", "日本語のタイトルをここに書く"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := width(tc.s)
			if got := trunc(tc.s, w); got != tc.s {
				t.Errorf("trunc clipped legitimate text at its own width:\n got %q\nwant %q", got, tc.s)
			}
		})
	}
}
