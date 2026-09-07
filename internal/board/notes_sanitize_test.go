package board

import (
	"strings"
	"testing"
	"unicode"
)

// TestSetNotesStripsControlRunesKeepingNewlinesAndTabs pins the one field that
// used to keep control runes. The realistic way in is `kando notes X --file`
// or its stdin form: a note piped from an issue body or a web page carries
// whatever escape sequences that source had, and an unsanitized note is
// replayed on every later read by kando show, the TUI and kando web alike.
func TestSetNotesStripsControlRunesKeepingNewlinesAndTabs(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{
			// OSC 52 — writes the reader's clipboard from a card they merely
			// opened. ESC and BEL go; the payload text is left visible rather
			// than silently swallowed, so the note still shows what it holds.
			name: "osc clipboard sequence",
			in:   "before\x1b]52;c;cGF5bG9hZA==\x07after",
			want: "before]52;c;cGF5bG9hZA==after",
		},
		{
			name: "csi cursor and screen control",
			in:   "line one\x1b[2J\x1b[Hline two",
			want: "line one[2J[Hline two",
		},
		{
			name: "nul and bell",
			in:   "a\x00b\x07c",
			want: "abc",
		},
		{
			// C1 controls are real runes in UTF-8, and unicode.IsControl
			// covers them as well as the C0 block.
			name: "c1 control",
			in:   "abc",
			want: "abc",
		},
		{
			name: "newlines and tabs are content, not control",
			in:   "para one\n\n\tindented\tcolumns\npara two",
			want: "para one\n\n\tindented\tcolumns\npara two",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Card{}
			c.SetNotes(tc.in)
			if c.Notes != tc.want {
				t.Errorf("SetNotes(%q)\n got %q\nwant %q", tc.in, c.Notes, tc.want)
			}
			for _, r := range c.Notes {
				if r != '\n' && r != '\t' && unicode.IsControl(r) {
					t.Errorf("control rune %q survived in %q", r, c.Notes)
				}
			}
		})
	}
}

// TestSetNotesStillNormalizesAndTrims guards the behaviour that was already
// there, since the sanitizing step was inserted into the middle of it.
func TestSetNotesStillNormalizesAndTrims(t *testing.T) {
	c := &Card{}
	c.SetNotes("a\r\nb\r\n\r\n")
	if c.Notes != "a\nb" {
		t.Errorf("CRLF normalization or trailing trim changed: %q", c.Notes)
	}

	c.SetNotes(strings.Repeat("x", maxNotesBytes+100))
	if len(c.Notes) > maxNotesBytes {
		t.Errorf("notes exceed the %d byte cap: %d", maxNotesBytes, len(c.Notes))
	}
}
