package board

import (
	"strings"
	"testing"
	"unicode"
)

// The Trojan Source set (CVE-2021-42574) plus the plain direction marks. These
// are category Cf, not Cc, so unicode.IsControl is blind to every one of them
// and they survived the #17 filter.
var bidiRunes = []rune{
	0x061C, // ARABIC LETTER MARK
	0x200E, // LEFT-TO-RIGHT MARK
	0x200F, // RIGHT-TO-LEFT MARK
	0x202A, // LEFT-TO-RIGHT EMBEDDING
	0x202B, // RIGHT-TO-LEFT EMBEDDING
	0x202C, // POP DIRECTIONAL FORMATTING
	0x202D, // LEFT-TO-RIGHT OVERRIDE
	0x202E, // RIGHT-TO-LEFT OVERRIDE
	0x2066, // LEFT-TO-RIGHT ISOLATE
	0x2067, // RIGHT-TO-LEFT ISOLATE
	0x2068, // FIRST STRONG ISOLATE
	0x2069, // POP DIRECTIONAL ISOLATE
}

// Runes that are also category Cf, or look like control characters, but carry
// meaning and must survive. Stripping these is how a blanket unicode.Cf filter
// breaks real text, which is why the predicate targets Bidi_Control instead.
var mustSurvive = []struct {
	r    rune
	what string
}{
	{0x200C, "ZERO WIDTH NON-JOINER — Persian and Indic shaping"},
	{0x200D, "ZERO WIDTH JOINER — emoji sequences like family and flag"},
	{0xFE0F, "VARIATION SELECTOR-16 — emoji presentation"},
	{0x0301, "COMBINING ACUTE ACCENT"},
	{0x05D0, "HEBREW ALEF — real RTL text, not a control"},
	{0x0627, "ARABIC ALEF"},
}

func TestUnsafeRuneCoversControlAndBidi(t *testing.T) {
	for _, r := range bidiRunes {
		if !UnsafeRune(r) {
			t.Errorf("U+%04X is a bidi control and must be unsafe", r)
		}
		if unicode.IsControl(r) {
			t.Errorf("U+%04X: test premise wrong, IsControl already covers it", r)
		}
	}
	for _, r := range []rune{0x00, 0x07, 0x1B, 0x7F, 0x85, 0x9B} {
		if !UnsafeRune(r) {
			t.Errorf("U+%04X is a C0/C1 control and must be unsafe", r)
		}
	}
	for _, tc := range mustSurvive {
		if UnsafeRune(tc.r) {
			t.Errorf("U+%04X must survive: %s", tc.r, tc.what)
		}
	}
}

// TestWritePathStripsBidi covers the two write-time sanitizers together. They
// are separate functions with different newline rules, so the risk is that one
// gets widened and the other does not; asserting both here is what stops them
// drifting apart.
func TestWritePathStripsBidi(t *testing.T) {
	for _, r := range bidiRunes {
		in := "before" + string(r) + "after"

		c := &Card{}
		if !c.SetTitle(in) {
			t.Fatalf("U+%04X: SetTitle rejected the value", r)
		}
		if strings.ContainsRune(c.Title, r) {
			t.Errorf("U+%04X survived SetTitle: %q", r, c.Title)
		}

		c.SetNotes(in)
		if strings.ContainsRune(c.Notes, r) {
			t.Errorf("U+%04X survived SetNotes: %q", r, c.Notes)
		}

		c.SetTag(in)
		if strings.ContainsRune(c.Tag, r) {
			t.Errorf("U+%04X survived SetTag: %q", r, c.Tag)
		}

		c.SetBlocked(in)
		if strings.ContainsRune(c.BlockedReason, r) {
			t.Errorf("U+%04X survived SetBlocked: %q", r, c.BlockedReason)
		}

		c.Checklist = nil
		if i := c.InsertChecklistItem(-1, in); i < 0 {
			t.Fatalf("U+%04X: InsertChecklistItem rejected the value", r)
		}
		if strings.ContainsRune(c.Checklist[0].Text, r) {
			t.Errorf("U+%04X survived InsertChecklistItem: %q", r, c.Checklist[0].Text)
		}
	}
}

func TestWritePathKeepsMeaningfulRunes(t *testing.T) {
	for _, tc := range mustSurvive {
		in := "a" + string(tc.r) + "b"
		c := &Card{}
		c.SetNotes(in)
		if !strings.ContainsRune(c.Notes, tc.r) {
			t.Errorf("U+%04X dropped by SetNotes but must survive: %s", tc.r, tc.what)
		}
		c.SetTitle(in)
		if !strings.ContainsRune(c.Title, tc.r) {
			t.Errorf("U+%04X dropped by SetTitle but must survive: %s", tc.r, tc.what)
		}
	}
	// The canonical composed emoji: two people joined by ZWJ with a variation
	// selector. If a filter is too broad this renders as separate glyphs.
	family := "\U0001F468\u200d\U0001F469\u200d\U0001F467"
	c := &Card{}
	c.SetNotes(family)
	if c.Notes != family {
		t.Errorf("emoji ZWJ sequence mangled:\n got %q\nwant %q", c.Notes, family)
	}
}

// TestSafeForDisplayDefangsWhatTheWritePathNeverSaw is the read-side half.
// board.md is the user's file and is parsed verbatim, so a note a pre-fix
// build stored, or one typed in by hand, reaches a renderer with its escape
// sequences intact. This is what the renderers put it through.
func TestSafeForDisplayDefangsWhatTheWritePathNeverSaw(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{
			// The payload that motivated all of this: it writes the reader's
			// clipboard from a card they merely opened.
			name: "osc 52 from a hand-edited file",
			in:   "before\x1b]52;c;cGF5bG9hZA==\x07after",
			want: "before]52;c;cGF5bG9hZA==after",
		},
		{
			name: "c1 CSI, the single-byte ESC bracket",
			in:   "a\u009b2Jb",
			want: "a2Jb",
		},
		{
			name: "rtl override reorders the visible text",
			in:   "safe\u202egnp.exe",
			want: "safegnp.exe",
		},
		{
			name: "newlines and tabs are content and survive",
			in:   "one\n\ttwo",
			want: "one\n\ttwo",
		},
		{
			name: "ordinary text is returned unchanged",
			in:   "Renew passport #errand",
			want: "Renew passport #errand",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SafeForDisplay(tc.in); got != tc.want {
				t.Errorf("SafeForDisplay(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}
