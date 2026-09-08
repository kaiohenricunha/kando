package tui

import (
	"strings"
	"testing"
)

// Legitimate dense grapheme clusters. Measured with width() and len(), not
// assumed — the densest here is the skin-toned family at 20.5 bytes per cell,
// not the flag at 14, which an earlier revision of this file asserted as fact.
const (
	englandFlag  = "\U0001F3F4\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F"
	familyEmoji  = "\U0001F468\u200d\U0001F469\u200d\U0001F467"
	familySkin   = "\U0001F468\U0001F3FB\u200d\U0001F469\U0001F3FB\u200d\U0001F467\U0001F3FB\u200d\U0001F466\U0001F3FB"
	devanagari   = "\u0915\u094d\u0937\u093f"
	zalgoBase    = "a"
	zalgoMarkRun = 5000
)

// TestSanitizeBoundsBytes covers the read path the write path cannot: the
// store assigns Title, Tag and BlockedReason verbatim, so a hand-edited
// board.md can hand a megabyte to a row that renders a few dozen cells, and
// sanitize would otherwise allocate a builder that size on every frame.
//
// The budgets below are literals on purpose. Asserting against maxRenderBytes
// — the constant sanitize itself uses — would mean raising the constant also
// raised the bar, so the test would keep passing while the bound stopped
// bounding. Two earlier PRs in this series shipped exactly that defect.
func TestSanitizeBoundsBytes(t *testing.T) {
	// One cluster, one cell, ten thousand bytes.
	zalgo := zalgoBase + strings.Repeat("\u0301", zalgoMarkRun)
	if got := width(zalgo); got != 1 {
		t.Fatalf("test premise wrong: width = %d, want 1 (grapheme clustering)", got)
	}

	huge := strings.Repeat("x", 1<<20) // a megabyte, as a hand-edited title
	if got := len(sanitize(huge)); got > 20000 {
		t.Errorf("sanitize emitted %d bytes for a 1 MiB value, budget is 20000", got)
	}
	if got := len(sanitize(zalgo)); got > 20000 {
		t.Errorf("sanitize emitted %d bytes for a zalgo value, budget is 20000", got)
	}
	// Still valid UTF-8 after the cut.
	if !strings.ContainsRune(sanitize(huge), 'x') {
		t.Error("sanitize dropped everything")
	}
}

// TestSanitizeLeavesLegitimateTextAlone is the other half. A bound that clipped
// real content would be worse than the problem it solves, and none of these
// appear in any fixture, so nothing else would catch it.
func TestSanitizeLeavesLegitimateTextAlone(t *testing.T) {
	for _, tc := range []struct{ name, s string }{
		{"england flag tag sequence", englandFlag},
		{"ZWJ family emoji", familyEmoji},
		{"skin-toned family, the densest measured", familySkin},
		{"devanagari conjunct", devanagari},
		{"accented latin", "Café résumé for the tax office"},
		{"cjk", "日本語のタイトルをここに書く"},
		{"a full-size notes paragraph", strings.Repeat("lorem ipsum dolor ", 500)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitize(tc.s); got != tc.s {
				t.Errorf("sanitize altered legitimate text:\n got %q\nwant %q", got, tc.s)
			}
		})
	}
}

// TestTruncStaysAnsiSafe pins why the byte bound is not in trunc. Every caller
// passes an already-styled row; a byte cut there is rune-safe but not
// escape-safe, because every byte of a CSI sequence is ASCII and RuneStart is
// true for all of them. A severed escape is invisible to width(), so fit()
// pads and every cell assertion still passes while the terminal eats the
// padding — which is why this is asserted on the escape structure instead.
func TestTruncStaysAnsiSafe(t *testing.T) {
	styled := testStyles.Fg.Render("a" + strings.Repeat("\u0301", 5000))
	if !strings.HasSuffix(styled, "\x1b[0m") {
		t.Fatalf("test premise wrong: the style does not emit a reset")
	}
	for _, w := range []int{1, 10, 80} {
		got := trunc(styled, w)
		if strings.Contains(got, "\x1b[") && !strings.HasSuffix(got, "\x1b[0m") {
			t.Errorf("trunc(styled, %d) left an unterminated SGR: %q", w, got[max(0, len(got)-16):])
		}
	}
}
