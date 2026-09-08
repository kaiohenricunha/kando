package tui

import (
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
			// The classes sanitize's ASCII fast path deliberately falls
			// through for. These are the runes the whole guard series exists
			// to remove, and the ones a future refactor of this loop would
			// most plausibly lose — the C0 case above would not catch it.
			name:  "C1 and bidi controls are dropped inside every paragraph",
			notes: "a\u009bb\nc\u202ed",
			w:     40,
			want:  []string{"ab", "cd"},
		},
		{
			name:  "ESC becomes a space, it does not open an OSC",
			notes: "a\x1b]52;c;QQ==\x07b\nc",
			w:     40,
			want:  []string{"a ]52;c;QQ== b", "c"},
		},
		{
			name:  "tabs become spaces, as everywhere else in the TUI",
			notes: "a\tb",
			w:     40,
			want:  []string{"a b"},
		},
		{
			// wrapNotes' own contract. A card can no longer arrive with a
			// blank line at either *edge* — SetNotes trims both and
			// store.trimBlank trims both on read and write — but the
			// blank-row branch stays live in production for interior blank
			// lines, which are exactly what separates two paragraphs and are
			// the reason the branch exists at all. Only this edge case is
			// unreachable, and it is pinned because the function is a pure
			// helper a direct caller can still hand it.
			name:  "a leading newline is its own row",
			notes: "\nfoo",
			w:     40,
			want:  []string{"", "foo"},
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
}
