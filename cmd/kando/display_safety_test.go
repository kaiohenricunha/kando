package main

import (
	"github.com/kaiohenricunha/kando/internal/store"
	"strings"
	"testing"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
)

// poisoned is what a card looks like when it was written by a build from
// before the write-time sanitizers existed, or typed straight into board.md.
// The store parses that file verbatim, so none of these fields has been
// through sanitizeLine or SetNotes by the time a formatter sees it.
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
	0x2028: "LINE SEPARATOR (would forge an output row)",
	0x2029: "PARAGRAPH SEPARATOR",
}

const poisoned = "safe\u202egnp.exe\x1b]52;c;cGF5bG9hZA==\x07\u009b2J\u2028id: FORGED"

func poisonedCard() *board.Card {
	c := &board.Card{ID: "poison01", Title: "T" + poisoned}
	c.Notes = "N" + poisoned
	c.Tag = "G" + poisoned
	c.Blocked, c.BlockedReason = true, "B"+poisoned
	c.Checklist = []board.Item{{Text: "C" + poisoned}}
	c.CreatedAt = time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	return c
}

// TestTerminalFormattersNeverEmitUnsafeRunes covers the formatters that build
// a block of text for the terminal with %s.
//
// Most per-verb success lines are safe without a guard because they use %q,
// which escapes anything failing unicode.IsPrint — that covers Cc and Cf, so
// both a raw ESC and U+202E come out as escapes. But "they all use %q" was
// not true: checklist toggle printed item text read straight back off the
// board with %s, and the add and block lines echoed raw argv. Those are
// guarded at their call sites now; TestVerbSuccessLinesAreSafe pins them.
//
// The point of asserting on all of these together is that a formatter added
// later is the easy thing to forget, and forgetting is silent — the output
// looks right and the escape sequence runs.
func TestTerminalFormattersNeverEmitUnsafeRunes(t *testing.T) {
	c := poisonedCard()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	var lanes [4][]*board.Card
	lanes[board.Todo] = []*board.Card{c}

	archived := poisonedCard()
	archived.DoneAt = now
	groups := []board.ArchiveGroup{{Label: "THIS WEEK", Cards: []*board.Card{archived}}}

	outputs := map[string]string{
		"formatCard":        formatCard(board.Todo, c, now),
		"formatList":        formatList(lanes, "", 1, 1, now),
		"formatArchiveList": formatArchiveList(t.TempDir(), "life", groups, "", 1, 1, 1),
		// The cap footnote is a second return in the same function and would
		// otherwise be unguarded without anything noticing.
		"formatArchiveList capped": formatArchiveList(t.TempDir(), "life", groups, "", 1, 1, 2),
		// A filtered list prints its own summary line and takes a different
		// path through formatList.
		"formatList filtered": formatList(lanes, "safe"+string(rune(0x202e))+"query", 1, 3, now),
	}

	for name, out := range outputs {
		t.Run(name, func(t *testing.T) {
			if out == "" {
				t.Fatal("no output, so this case asserts nothing")
			}
			// The content must still be shown, only de-fanged: a formatter
			// that dropped the whole field would pass the rune scan below
			// while being useless.
			if !strings.Contains(out, "gnp.exe") {
				t.Errorf("payload text vanished entirely rather than being de-fanged:\n%s", out)
			}
			for _, r := range out {
				if what, bad := forbidden[r]; bad {
					t.Errorf("emitted U+%04X (%s) in:\n%s", r, what, out)
					break
				}
			}
		})
	}
}

// TestCardJSONIsDefanged covers the fourth terminal output path. encoding/json
// is not a guard here: it escapes bytes below 0x20 and U+2028/U+2029, but
// copies everything from 0x80 up verbatim, so the C1 block and the bidi
// controls pass straight through. That matters because a complete OSC 52
// needs no ESC byte at all — U+009D opens it, U+009C ends it — and
// `kando show --json` writes to the same terminal as `kando show`.
func TestCardJSONIsDefanged(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	cj := cardToJSON(board.Todo, poisonedCard(), now)

	var b strings.Builder
	if err := writeJSON(&b, cj); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "gnp.exe") {
		t.Errorf("payload text vanished rather than being de-fanged:\n%s", out)
	}
	for _, r := range out {
		if what, bad := forbidden[r]; bad {
			t.Errorf("writeJSON emitted U+%04X (%s) in:\n%s", r, what, out)
			break
		}
	}
}

// TestVerbSuccessLinesAreSafe drives the real verbs through the subprocess
// harness and reads their actual stdout. Building the format string here
// instead would assert only that SafeForDisplay works, which is already
// covered — it would not notice the guard being removed from the call site,
// which is the regression worth catching.
//
// checklist toggle is the one that mattered: its item text is read back off
// the board after the toggle, so it is whatever the file held and has never
// been through a write-time sanitizer.
func TestVerbSuccessLinesAreSafe(t *testing.T) {
	home := t.TempDir()

	// Build a board through the CLI, then poison it on disk the way a
	// hand-edit or a pre-fix build would, bypassing every Set* helper.
	if _, _, code := runCLI(t, home, "board", "create", "life"); code != 0 {
		t.Fatal("board create failed")
	}
	if _, _, code := runCLI(t, home, "add", "Card", "life"); code != 0 {
		t.Fatal("add failed")
	}
	root := home // KANDO_HOME is the boards root itself
	b, err := store.Load(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	// Poison every user-controlled field, not just one: a fixture that
	// poisons only the checklist would let an unguarded Notes or Title slip
	// through every assertion below.
	c := b.Lanes[board.Todo][0]
	c.Title = "T" + poisoned
	c.Notes = "N" + poisoned
	c.Tag = "G" + poisoned
	c.Checklist = []board.Item{{Text: "item " + poisoned}}
	st, _, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	// Address the card by id: its title is poisoned now, so a title lookup
	// would fail and every verb below would exit non-zero.
	id := c.ID

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"checklist toggle", []string{"checklist", "toggle", id, "1", "life"}},
		{"checklist add", []string{"checklist", "add", id, "new " + poisoned, "life"}},
		{"block", []string{"block", id, "--reason", "why " + poisoned, "life"}},
		{"show", []string{"show", id, "life"}},
		{"list", []string{"list", "life"}},
		{"show --json", []string{"show", id, "life", "--json"}},
		{"list --json", []string{"list", "life", "--json"}},
		// The filter is echoed back in the summary line and in the JSON
		// Filter field; a query is user-controlled like any card text.
		{"list --filter", []string{"list", "life", "--filter", poisoned}},
		{"list --filter --json", []string{"list", "life", "--filter", poisoned, "--json"}},
		{"archive list --filter", []string{"archive", "list", "life", "--filter", poisoned}},
		{"archive list --filter --json", []string{"archive", "list", "life", "--filter", poisoned, "--json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runCLI(t, home, tc.args...)
			if code != 0 {
				t.Fatalf("exit %d, stderr: %s", code, errOut)
			}
			for _, r := range out + errOut {
				if what, bad := forbidden[r]; bad {
					t.Errorf("emitted U+%04X (%s) in:\n%s", r, what, out+errOut)
					break
				}
			}
		})
	}
}

// TestFormattersNeverGainALine is the structural guard the membership scan
// above cannot be: it counts lines rather than looking for a forbidden rune.
//
// That distinction is the whole point. These formatters build a block of
// "key: value" lines and then guard the assembled block at the return, so any
// read-path helper that turns a rune inside a field into "\n" forges an output
// line indistinguishable from a real one — a card tagged "x\u2028id: FORGED"
// printing its own id: line. A scan for unsafe runes sees nothing wrong,
// because the emitted rune is a perfectly legitimate newline.
func TestFormattersNeverGainALine(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

	clean := &board.Card{ID: "7f3a11", Title: "Innocent", Tag: "home"}
	// Same card, but every single-line field carries a line separator plus
	// text shaped like the structure the formatter itself emits.
	forged := &board.Card{ID: "7f3a11", Title: "Innocent\u2028lane: Done"}
	forged.Tag = "home\u2028id: FORGED"
	forged.Blocked, forged.BlockedReason = true, "why\u2028created: 1999-01-01"
	clean.Blocked, clean.BlockedReason = true, "why"

	if got, want := lines(formatCard(board.Todo, forged, now)), lines(formatCard(board.Todo, clean, now)); got != want {
		t.Errorf("formatCard: %d lines with separators, %d without — a field forged %d line(s):\n%s",
			got, want, got-want, formatCard(board.Todo, forged, now))
	}

	var lanesClean, lanesForged [4][]*board.Card
	lanesClean[board.Todo] = []*board.Card{clean}
	lanesForged[board.Todo] = []*board.Card{forged}
	if got, want := lines(formatList(lanesForged, "", 1, 1, now)), lines(formatList(lanesClean, "", 1, 1, now)); got != want {
		t.Errorf("formatList: %d lines with separators, %d without:\n%s",
			got, want, formatList(lanesForged, "", 1, 1, now))
	}

	gc := []board.ArchiveGroup{{Label: "THIS WEEK", Cards: []*board.Card{clean}}}
	gf := []board.ArchiveGroup{{Label: "THIS WEEK", Cards: []*board.Card{forged}}}
	if got, want := lines(formatArchiveList(t.TempDir(), "life", gf, "", 1, 1, 1)),
		lines(formatArchiveList(t.TempDir(), "life", gc, "", 1, 1, 1)); got != want {
		t.Errorf("formatArchiveList: %d lines with separators, %d without", got, want)
	}
}

func lines(s string) int { return strings.Count(s, "\n") + 1 }
