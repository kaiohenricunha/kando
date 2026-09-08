package board

import (
	"strings"
	"testing"
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
			// covers them as well as the C0 block. U+009B is CSI, the
			// single-byte form of ESC [, so it starts a control sequence on
			// its own; U+0085 is NEL. Written as escapes deliberately: a
			// literal C1 byte is invisible in a diff and in every editor, so
			// a normalizing hook could delete it and leave the case asserting
			// "abc" == "abc" while still passing.
			name: "c1 control",
			in:   "a\u0085b\u009b2Jc",
			want: "ab2Jc",
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
				if r != '\n' && r != '\t' && UnsafeRune(r) {
					t.Errorf("unsafe rune %q survived in %q", r, c.Notes)
				}
			}
		})
	}
}

// TestSetNotesStepOrdering pins the order of the three steps in SetNotes,
// because inserting the strip in the middle of them created two ways to get
// it wrong that no existing test can see.
//
// TestSetNotesNormalizesCRLF (ops_test.go) and TestFieldsAreCapped
// (board_test.go) already cover CRLF and the byte cap, so repeating them here
// would add nothing: "a\r\nb" yields "a\nb" under either ordering, and a
// plain ASCII string cannot outgrow the cap. Each case below is chosen
// because it fails if the steps are reordered.
func TestSetNotesStepOrdering(t *testing.T) {
	c := &Card{}

	// Normalize must run BEFORE the strip. A lone CR is an old-Mac line
	// ending and CR is itself a control rune, so stripping first deletes it
	// and silently joins two lines into one. This is the only input that
	// tells the two orderings apart.
	c.SetNotes("a\rb")
	if c.Notes != "a\nb" {
		t.Errorf("CR normalization must precede the strip: got %q, want %q", c.Notes, "a\nb")
	}

	// A control rune touching a line ending must not consume it.
	c.SetNotes("a\x1b\r\n\x00b")
	if c.Notes != "a\nb" {
		t.Errorf("control rune beside a line ending: got %q, want %q", c.Notes, "a\nb")
	}

	// The blank-line trim must run AFTER the strip. TrimSpace does not see a
	// control or bidi rune as space, so a first line holding only one is
	// non-blank before the strip and blank after it. Trimming first would
	// store a leading blank line that store.trimBlank drops on save — the
	// row-vanishes-on-reload asymmetry the trim exists to close.
	c.SetNotes("\u0001\nfoo")
	if c.Notes != "foo" {
		t.Errorf("the trim must follow the strip: got %q, want %q", c.Notes, "foo")
	}
	// And AFTER the separator conversion: "\u2028foo" holds no "\n" until
	// U+2028 becomes one, so an earlier trim would find nothing to do.
	c.SetNotes("\u2028foo")
	if c.Notes != "foo" {
		t.Errorf("the trim must follow the separator conversion: got %q, want %q", c.Notes, "foo")
	}

	// The strip must run BEFORE the clip. strings.Map rewrites each invalid
	// UTF-8 byte to U+FFFD, which is 3 bytes, so sanitizing can grow the
	// value threefold. Clipping first would spend the budget on bytes that
	// are about to expand and blow the cap wide open.
	c.SetNotes(strings.Repeat("\xff", maxNotesBytes))
	if len(c.Notes) > maxNotesBytes {
		t.Errorf("strip must precede the clip: %d bytes exceeds the %d cap", len(c.Notes), maxNotesBytes)
	}
}
