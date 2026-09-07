package main

import (
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
}

const poisoned = "safe\u202egnp.exe\x1b]52;c;cGF5bG9hZA==\x07\u009b2J"

func poisonedCard() *board.Card {
	c := &board.Card{ID: "poison01", Title: "T" + poisoned}
	c.Notes = "N" + poisoned
	c.Tag = "G" + poisoned
	c.Blocked, c.BlockedReason = true, "B"+poisoned
	c.Checklist = []board.Item{{Text: "C" + poisoned}}
	c.CreatedAt = time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	return c
}

// TestTerminalFormattersNeverEmitUnsafeRunes covers every formatter that
// builds a block of text for the terminal with %s. These are the ones that
// need the guard: the per-verb success lines use %q, which escapes both
// control runes and the bidi controls (they are all non-printable), so a raw
// ESC cannot reach the terminal through those.
//
// The point of asserting on all of them together is that a formatter added
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
