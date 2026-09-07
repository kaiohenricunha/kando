package web

import (
	"strings"
	"testing"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

// poison is what a board.md holds when it was written by a build from before
// the write-time sanitizers existed, or edited by hand. U+202E reorders the
// text the browser shows; the ESC and C1 sequences matter less in a browser
// than in a terminal, but they have no business in the page either and the
// same guard removes all of them.
// forbidden is spelled out rather than taken from board.UnsafeRune on purpose.
// Asserting with the same predicate the code under test uses would make this
// tautological: weaken UnsafeRune and the assertion weakens with it, so the
// test would keep passing while the guard stopped guarding. These are the
// concrete runes that must never reach a reader.
var forbidden = map[rune]string{
	0x1B:   "ESC (opens OSC 52 and every CSI sequence)",
	0x07:   "BEL (terminates an OSC string)",
	0x00:   "NUL",
	0x9B:   "U+009B CSI, the single-rune form of ESC [",
	0x9D:   "U+009D OSC",
	0x202A: "LEFT-TO-RIGHT EMBEDDING",
	0x202B: "RIGHT-TO-LEFT EMBEDDING",
	0x202C: "POP DIRECTIONAL FORMATTING",
	0x202D: "LEFT-TO-RIGHT OVERRIDE",
	0x202E: "RIGHT-TO-LEFT OVERRIDE",
	0x2066: "LEFT-TO-RIGHT ISOLATE",
	0x2067: "RIGHT-TO-LEFT ISOLATE",
	0x2068: "FIRST STRONG ISOLATE",
	0x2069: "POP DIRECTIONAL ISOLATE",
	0x200E: "LEFT-TO-RIGHT MARK",
	0x200F: "RIGHT-TO-LEFT MARK",
	0x061C: "ARABIC LETTER MARK",
}

const poison = "safe\u202egnp.exe\x1b]52;c;cGF5bG9hZA==\x07\u009b2J"

// writePoisonedBoard bypasses every Set* helper and writes the raw bytes
// straight to board.md, which is the only way to reproduce the case that
// matters: the store parses a file verbatim, so nothing on the read path has
// been through sanitizeLine or SetNotes.
func writePoisonedBoard(t *testing.T, root string) {
	t.Helper()
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	c := &board.Card{ID: "poison01", Title: "T" + poison}
	c.Notes = "N" + poison
	c.Tag = "G" + poison
	c.Blocked, c.BlockedReason = true, "B"+poison
	c.Checklist = []board.Item{{Text: "C" + poison}}
	b.Lanes[board.Todo] = []*board.Card{c}
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	// Guard the premise: if a future change starts sanitizing on the write or
	// read path, this fixture stops testing anything and should fail loudly
	// rather than pass vacuously.
	got, err := store.Load(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Lanes[board.Todo][0].Notes, "\u202e") {
		t.Fatal("premise broken: the store no longer round-trips raw bytes, so this test proves nothing")
	}
}

// TestPagesNeverServeUnsafeRunes walks the pages that render card fields and
// asserts none of them emits a rune the display guard is supposed to drop.
// The bidi overrides are the reason this matters in a browser: html/template
// escapes HTML metacharacters, which does nothing about U+202E, so the page
// would render text in an order the file does not contain.
func TestPagesNeverServeUnsafeRunes(t *testing.T) {
	root := newRoot(t)
	writePoisonedBoard(t, root)
	h := newHandler(t, root, "life")

	for _, path := range []string{"/b/life", "/b/life/cards/poison01"} {
		t.Run(path, func(t *testing.T) {
			rec, body := get(t, h, path)
			if rec.Code != 200 {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			// The page must still show the content, just de-fanged.
			if !strings.Contains(body, "gnp.exe") {
				t.Errorf("payload text was dropped entirely, not just de-fanged")
			}
			for _, r := range body {
				if what, bad := forbidden[r]; bad {
					t.Errorf("page served U+%04X — %s", r, what)
				}
			}
		})
	}
}
