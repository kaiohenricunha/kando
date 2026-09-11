package store

import (
	"testing"

	"github.com/kaiohenricunha/kando/internal/board"
)

// TestSetNotesSurvivesTheStoreRoundTrip pins the invariant the two trimmers
// exist to hold, across the boundary where they drifted apart.
//
// board.SetNotes and store.trimBlank both decide what a blank line is, and they
// are in different packages, so nothing but this test connects them. When they
// disagree, SetNotes stores a value the renderer shows and Marshal then writes
// a *different* value to board.md — the row appears, then vanishes on the next
// reload, with no error anywhere. Every earlier test asserted on c.Notes alone
// and so could not see the second half of that.
//
// The inputs below are the disagreements that have actually happened: a leading
// blank line (SetNotes trimmed only the right end), and a trailing line holding
// one non-ASCII space (SetNotes' TrimRight cutset is ASCII, trimBlank's
// TrimSpace is unicode.IsSpace).
func TestSetNotesSurvivesTheStoreRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
	}{
		{"a leading blank line", "\nfirst\nsecond"},
		{"several leading blank lines", "\n\n  \nfirst"},
		{"a trailing blank line", "first\n\n"},
		{"a trailing non-breaking space", "first\n\u00a0"},
		{"a leading non-breaking space", "\u00a0\nfirst"},
		{"an ideographic space at both ends", "\u3000\nfirst\n\u3000"},
		{"a line separator at the front", "\u2028first"},
		{"interior blank lines, which must survive", "first\n\nsecond"},
		{"first-line indentation, which must survive", "  indented\nsecond"},
		{"a bare checklist marker, which SetNotes leaves without its trailing space", "todo\n- [ ] "},
		{"a checklist marker with no space before its text", "- [x]done"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &board.Card{ID: "abcdefgh", Title: "Notes", CreatedAt: ts(2026, 9, 1), MovedAt: ts(2026, 9, 1)}
			c.SetNotes(tc.input)
			stored := c.Notes

			b := &board.Board{}
			b.Lanes[board.Backlog] = []*board.Card{c}
			out := Marshal(b)
			got, _, err := Parse(out)
			if err != nil {
				t.Fatalf("parse: %v\n%s", err, out)
			}
			if len(got.Lanes[board.Backlog]) != 1 {
				t.Fatalf("card did not survive the round trip\n%s", out)
			}

			// The assertion is equality with what SetNotes stored, not with
			// any literal: whatever the write path decided to keep is exactly
			// what a reload must produce.
			if reloaded := got.Lanes[board.Backlog][0].Notes; reloaded != stored {
				t.Errorf("the store changed what SetNotes stored:\n stored:   %q\n reloaded: %q\n board.md:\n%s",
					stored, reloaded, out)
			}
		})
	}
}
