package tui

import (
	"strings"
	"testing"

	"github.com/kaiohenricunha/kando/internal/board"
)

// TestSanitizeDropsC1AndBidi pins the gap that the byte-wise fast path used to
// hide. sanitize() is the TUI's single render-time guard — every visible
// string in every view goes through it — but it only ever looked for bytes
// below 0x20, and the scan that decides whether to bother is byte-wise. C1
// controls and bidi overrides are multi-byte UTF-8, so a string containing
// nothing else took the "clean" early return and was rendered verbatim.
//
// U+009B is the case that matters most: it is CSI, the single-byte form of
// "ESC [", so it opens a control sequence with no ESC byte anywhere for the
// old check to find.
func TestSanitizeDropsC1AndBidi(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"c1 CSI alone in an otherwise clean string", "a\u009b2Jb", "a2Jb"},
		{"c1 NEL", "a\u0085b", "ab"},
		{"rtl override", "safe\u202egnp.exe", "safegnp.exe"},
		{"rtl isolate", "a\u2067b\u2069c", "abc"},
		{"direction mark", "a\u200fb", "ab"},
		// Existing behaviour, which must not regress: C0 becomes a space so
		// column alignment survives, and CR vanishes.
		{"c0 becomes a space", "a\x01b", "a b"},
		{"cr vanishes", "a\rb", "ab"},
		{"esc becomes a space", "a\x1bb", "a b"},
		// Meaningful runes must survive: the TUI renders emoji and RTL text.
		{"zwj emoji sequence", "\U0001F468\u200d\U0001F469", "\U0001F468\u200d\U0001F469"},
		{"hebrew text is not a control", "אב", "אב"},
		{"plain ascii is untouched", "Renew passport", "Renew passport"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitize(tc.in); got != tc.want {
				t.Errorf("sanitize(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestSanitizeAgreesWithTheSharedPredicate checks that sanitize() and
// UnsafeRune agree rune for rune. sanitize() keeps its own loop because it
// maps C0 to a space rather than dropping it — the TUI measures cells and
// needs the column — so the two implementations can drift.
//
// Agreement is all this proves. Both sides call UnsafeRune, so weakening the
// predicate moves the assertion with it and this test stays green; the floor
// below is what pins which runes are actually unsafe, and
// TestSanitizeDropsC1AndBidi carries the concrete expectations. Spelling the
// floor out by hand is the same discipline cmd/kando and internal/web use in
// their display-safety tests, and for the same reason.
func TestSanitizeAgreesWithTheSharedPredicate(t *testing.T) {
	// The floor: these must be dropped no matter what UnsafeRune says.
	for _, r := range []rune{
		0x1B, 0x00, 0x07, 0x7F, // C0 and DEL
		0x85, 0x9B, 0x9D, // C1: NEL, CSI, OSC
		0x061C, 0x200E, 0x200F, // direction marks
		0x202A, 0x202B, 0x202C, 0x202D, 0x202E, // embeddings and overrides
		0x2066, 0x2067, 0x2068, 0x2069, // isolates
	} {
		if strings.ContainsRune(sanitize(string(r)), r) {
			t.Errorf("U+%04X survived sanitize", r)
		}
	}

	for r := rune(0); r < 0x3000; r++ {
		if r == '\n' || r == '\t' {
			continue // laid out by the caller, not by sanitize
		}
		out := sanitize(string(r))
		if board.UnsafeRune(r) {
			if strings.ContainsRune(out, r) {
				t.Errorf("U+%04X is unsafe but survived sanitize as %q", r, out)
			}
			continue
		}
		if !strings.ContainsRune(out, r) {
			t.Errorf("U+%04X is safe but sanitize dropped it", r)
		}
	}
}
