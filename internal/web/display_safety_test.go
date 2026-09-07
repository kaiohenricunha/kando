package web

import (
	"html"
	"net/http"
	"net/url"
	"regexp"
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

	// Put a SECOND poisoned card into the archive, so /b/life/archive has
	// something to render while the first stays on the board for the board
	// and card pages. Done is replaced rather than appended to: the sample
	// fixture already has Done cards, so appending would leave
	// ArchiveDone(a, 0, ...) archiving one of those instead.
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	a, err := st.LoadArchive()
	if err != nil {
		t.Fatal(err)
	}
	archived := &board.Card{ID: "poison02", Title: "A" + poison, DoneAt: fixedNow}
	b.Lanes[board.Done] = []*board.Card{archived}
	if c := b.ArchiveDone(a, 0, fixedNow); c == nil {
		t.Fatal("could not archive the second poisoned card")
	}
	if err := st.SaveArchival(b, a); err != nil {
		t.Fatal(err)
	}

	// wantPayload marks the routes that render card text; /boards lists only
	// board names, so requiring the payload there would be wrong.
	for _, tc := range []struct {
		path        string
		wantPayload bool
	}{
		{"/b/life", true},
		{"/b/life/cards/poison01", true},
		{"/b/life/archive", true},
		{"/boards", false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			rec, body := get(t, h, tc.path)
			if rec.Code != 200 {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			// Where card text is rendered it must still be shown, de-fanged
			// rather than dropped: a page that swallowed the field entirely
			// would pass the rune scan below while being useless.
			if tc.wantPayload && !strings.Contains(body, "gnp.exe") {
				t.Errorf("payload text was dropped entirely, not just de-fanged")
			}
			for _, r := range body {
				if what, bad := forbidden[r]; bad {
					// One offending rune usually means many; report the
					// first and stop rather than printing a wall.
					t.Errorf("page served U+%04X — %s", r, what)
					break
				}
			}
		})
	}
}

// TestChecklistWitnessStillMatchesAfterDefanging is a regression test for the
// trap that de-fanging the view model sets: card.html renders the hidden "was"
// witness from the same .Text it displays, and checklistIndex compares that
// witness against the value on disk. Guard one side and not the other and the
// two can never be equal, so every toggle and every edit on the affected card
// returns 409 forever and the message tells the user to reload, which cannot
// help. The cards that would break are exactly the hand-edited ones this whole
// change exists to protect.
func TestChecklistWitnessStillMatchesAfterDefanging(t *testing.T) {
	root := newRoot(t)
	writePoisonedBoard(t, root)
	h := newHandler(t, root, "life")

	// Read the witness back out of the rendered page rather than constructing
	// it, so the test breaks if the template and the handler ever disagree.
	_, body := get(t, h, "/b/life/cards/poison01")
	m := regexp.MustCompile(`name="was" value="([^"]*)"`).FindStringSubmatch(body)
	if m == nil {
		t.Fatal("no was witness in the rendered card page")
	}
	witness := html.UnescapeString(m[1])

	rec := post(t, h, "/b/life/cards/poison01/checklist/0/toggle",
		url.Values{"was": {witness}})
	if rec.Code == http.StatusConflict {
		t.Fatalf("toggle rejected its own witness with 409: the page renders a de-fanged "+
			"value but checklistIndex compares the raw one\nwitness = %q", witness)
	}
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if got := reload(t, root, "life").Lanes[board.Todo][0]; !got.Checklist[0].Done {
		t.Error("item was not toggled")
	}
}
