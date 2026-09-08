package tui

import (
	"strings"
	"testing"
)

// TestWrapNotes is the regression net for a bug that survived because nothing
// tested this function at all: wrapNotes called sanitize(notes) and then split
// the result on "\n", but sanitize maps "\n" to a space, so the split could
// never find one. Every note rendered as a single flattened paragraph and the
// blank-line branch was unreachable.
//
// The fix is the shape cmd/kando/show.go already uses: split the raw value
// first, sanitize each paragraph after.
func TestWrapNotes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		notes string
		w     int
		want  []string
	}{
		{
			name:  "two paragraphs stay two rows",
			notes: "first para\nsecond para",
			w:     40,
			want:  []string{"first para", "second para"},
		},
		{
			// The branch that was dead: a blank line between paragraphs is a
			// blank row, not a space joining them.
			name:  "blank line between paragraphs is its own row",
			notes: "first\n\nsecond",
			w:     40,
			want:  []string{"first", "", "second"},
		},
		{
			name:  "a long paragraph still wraps",
			notes: "aaaa bbbb cccc dddd",
			w:     9,
			want:  []string{"aaaa bbbb", "cccc dddd"},
		},
		{
			name:  "wrapping and paragraphs compose",
			notes: "aaaa bbbb cccc\nshort",
			w:     9,
			want:  []string{"aaaa bbbb", "cccc", "short"},
		},
		{
			// Control runes inside a paragraph are still sanitized; only the
			// line structure is preserved.
			name:  "control runes inside a paragraph are still handled",
			notes: "a\x01b\nc",
			w:     40,
			want:  []string{"a b", "c"},
		},
		{
			name:  "tabs become spaces, as everywhere else in the TUI",
			notes: "a\tb",
			w:     40,
			want:  []string{"a b"},
		},
		{
			name:  "empty notes",
			notes: "",
			w:     40,
			want:  []string{""},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := wrapNotes(tc.notes, tc.w)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d rows %q, want %d rows %q", len(got), got, len(tc.want), tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("row %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestNewTextareaHeightCountsParagraphs pins the second symptom of the same
// bug: the editor is sized from len(wrapNotes(...)), so while newlines were
// being flattened, opening a multi-paragraph note gave a textarea too short to
// hold it.
func TestNewTextareaHeightCountsParagraphs(t *testing.T) {
	m := newTestModel(t, 120, 40)
	one := m.newTextarea("one line")
	three := m.newTextarea("first\n\nsecond")
	if three.Height() <= one.Height() {
		t.Errorf("a three-row note sized %d, a one-row note sized %d — paragraphs are not counted",
			three.Height(), one.Height())
	}
	if got, want := strings.Count("first\n\nsecond", "\n")+1, 3; got != want {
		t.Fatalf("test premise wrong: %d", got)
	}
}
