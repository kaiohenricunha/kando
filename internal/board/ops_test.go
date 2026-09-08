package board

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCardSetTagStripsHash(t *testing.T) {
	cases := []struct{ in, want string }{
		{"errand", "errand"},
		{"#errand", "errand"},
		{"  #errand  ", "errand"},
		{"", ""},
		{"#", ""},
	}
	for _, c := range cases {
		card := &Card{}
		card.SetTag(c.in)
		if card.Tag != c.want {
			t.Errorf("SetTag(%q): Tag = %q, want %q", c.in, card.Tag, c.want)
		}
	}
}

func TestCardSetNotes(t *testing.T) {
	card := &Card{}
	card.SetNotes("line one\nline two\n\n  ")
	if card.Notes != "line one\nline two" {
		t.Errorf("Notes = %q", card.Notes)
	}
	card.SetNotes("")
	if card.Notes != "" {
		t.Errorf("Notes = %q, want empty", card.Notes)
	}
}

func TestCardSetBlockedEmptyClears(t *testing.T) {
	card := &Card{}
	card.SetBlocked("  waiting on pads  ")
	if !card.Blocked || card.BlockedReason != "waiting on pads" {
		t.Fatalf("SetBlocked(reason): Blocked=%v Reason=%q", card.Blocked, card.BlockedReason)
	}
	card.SetBlocked("   ")
	if card.Blocked || card.BlockedReason != "" {
		t.Errorf("SetBlocked(empty) should clear: Blocked=%v Reason=%q", card.Blocked, card.BlockedReason)
	}
}

func TestChecklistAddAtCursor(t *testing.T) {
	card := &Card{}
	// Empty checklist: the first item always lands at index 0, regardless of cursor.
	at := card.InsertChecklistItem(7, "  first  ")
	if at != 0 || len(card.Checklist) != 1 || card.Checklist[0] != (Item{Text: "first"}) {
		t.Fatalf("first insert: at=%d checklist=%+v", at, card.Checklist)
	}
	// Non-empty checklist: new item lands right after the cursor.
	at = card.InsertChecklistItem(0, "second")
	want := []Item{{Text: "first"}, {Text: "second"}}
	if at != 1 || !reflect.DeepEqual(card.Checklist, want) {
		t.Fatalf("insert after cursor: at=%d checklist=%+v", at, card.Checklist)
	}
	at = card.InsertChecklistItem(0, "third")
	want = []Item{{Text: "first"}, {Text: "third"}, {Text: "second"}}
	if at != 1 || !reflect.DeepEqual(card.Checklist, want) {
		t.Fatalf("insert at cursor 0 again: at=%d checklist=%+v", at, card.Checklist)
	}
	// Empty (after trim) text is a no-op, signalled by -1.
	before := append([]Item(nil), card.Checklist...)
	if at := card.InsertChecklistItem(0, "   "); at != -1 {
		t.Errorf("empty text should return -1, got %d", at)
	}
	if !reflect.DeepEqual(card.Checklist, before) {
		t.Errorf("empty text must not modify the checklist: %+v", card.Checklist)
	}
}

func TestChecklistToggle(t *testing.T) {
	card := &Card{Checklist: []Item{{Text: "a"}, {Text: "b", Done: true}}}
	card.ToggleChecklistItem(0)
	if !card.Checklist[0].Done {
		t.Errorf("toggle should set Done")
	}
	card.ToggleChecklistItem(0)
	if card.Checklist[0].Done {
		t.Errorf("toggle again should clear Done")
	}
	card.ToggleChecklistItem(1)
	if card.Checklist[1].Done {
		t.Errorf("toggling index 1 should clear it (was true)")
	}
	// Out-of-range indices are a no-op, not a panic, and change nothing.
	before := append([]Item(nil), card.Checklist...)
	card.ToggleChecklistItem(-1)
	card.ToggleChecklistItem(2)
	if !reflect.DeepEqual(card.Checklist, before) {
		t.Errorf("out-of-range toggle modified the checklist: %+v", card.Checklist)
	}
}

func TestChecklistInsertClampsCursor(t *testing.T) {
	card := &Card{Checklist: []Item{{Text: "a"}, {Text: "b"}}}
	if at := card.InsertChecklistItem(99, "end"); at != 2 || card.Checklist[2].Text != "end" {
		t.Errorf("over-large cursor should clamp to the end: at=%d %+v", at, card.Checklist)
	}
	if at := card.InsertChecklistItem(-5, "start"); at != 0 || card.Checklist[0].Text != "start" {
		t.Errorf("negative cursor should clamp to the start: at=%d %+v", at, card.Checklist)
	}
}

func TestSingleLineFieldsDropControlRunes(t *testing.T) {
	card := &Card{Checklist: []Item{{Text: "x"}}}
	card.SetTag("home\n## Bogus")
	card.SetBlocked("wait\r\ncreated: nope")
	card.SetTitle("Ti\ttle\x00")
	card.InsertChecklistItem(0, "item\n### Injected")
	card.SetChecklistItemText(0, "edit\nid: zzzzzzzz")
	for name, got := range map[string]string{"tag": card.Tag, "blocked": card.BlockedReason, "title": card.Title, "item0": card.Checklist[0].Text, "item1": card.Checklist[1].Text} {
		if strings.ContainsAny(got, "\n\r\t\x00") {
			t.Errorf("%s kept a control rune: %q", name, got)
		}
	}
	if card.Tag != "home## Bogus" || card.BlockedReason != "waitcreated: nope" || card.Title != "Title" {
		t.Errorf("unexpected sanitized values: tag=%q blocked=%q title=%q", card.Tag, card.BlockedReason, card.Title)
	}
}

func TestSetNotesNormalizesCRLF(t *testing.T) {
	card := &Card{}
	card.SetNotes("a\r\nb\r\n\r\n")
	if card.Notes != "a\nb" {
		t.Errorf("Notes = %q, want %q", card.Notes, "a\nb")
	}
}

func TestNewCardAndSetTitle(t *testing.T) {
	if NewCard("   ", Todo, now) != nil {
		t.Errorf("empty title should yield nil")
	}
	c := NewCard("  Buy milk  ", Todo, now)
	if c == nil || c.Title != "Buy milk" || len(c.ID) != 8 || !c.CreatedAt.Equal(now) || !c.MovedAt.Equal(now) || !c.DoneAt.IsZero() {
		t.Fatalf("NewCard(Todo) = %+v", c)
	}
	if d := NewCard("x", Done, now); !d.DoneAt.Equal(now) {
		t.Errorf("NewCard(Done) should stamp DoneAt")
	}
	if !c.SetTitle(" Renamed ") || c.Title != "Renamed" {
		t.Errorf("SetTitle: %q", c.Title)
	}
	if c.SetTitle("  ") || c.Title != "Renamed" {
		t.Errorf("empty SetTitle must be a no-op: %q", c.Title)
	}
}

func TestChecklistEditText(t *testing.T) {
	card := &Card{Checklist: []Item{{Text: "old", Done: true}}}
	if ok := card.SetChecklistItemText(0, "  new text  "); !ok || card.Checklist[0].Text != "new text" {
		t.Fatalf("edit: ok=%v text=%q", ok, card.Checklist[0].Text)
	}
	if card.Checklist[0].Done != true {
		t.Errorf("editing text must not touch Done")
	}
	if ok := card.SetChecklistItemText(0, "   "); ok || card.Checklist[0].Text != "new text" {
		t.Errorf("empty text must be a no-op: ok=%v text=%q", ok, card.Checklist[0].Text)
	}
	if ok := card.SetChecklistItemText(5, "x"); ok {
		t.Errorf("out-of-range index must be a no-op")
	}
}

func TestBoardDeleteCard(t *testing.T) {
	b := &Board{}
	a := &Card{ID: "a"}
	c := &Card{ID: "c"}
	b.Lanes[Todo] = []*Card{a, c}
	got := b.DeleteCard(Todo, 0)
	if got != a || len(b.Lanes[Todo]) != 1 || b.Lanes[Todo][0] != c {
		t.Fatalf("DeleteCard: got=%v lane=%v", got, b.Lanes[Todo])
	}
	if got := b.DeleteCard(Todo, 5); got != nil {
		t.Errorf("out-of-range delete should return nil, got %v", got)
	}
}

// archiveIDs is the ids of the archive in order, the readable form for an
// ordering assertion — laneIDs for the archive.
func archiveIDs(a *Archive) string {
	ids := make([]string, len(a.Cards))
	for i, c := range a.Cards {
		ids[i] = c.ID
	}
	return strings.Join(ids, " ")
}

func TestArchiveDoneMovesCardToTopOfArchive(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	b := &Board{}
	keep := &Card{ID: "keep", DoneAt: now.AddDate(0, 0, -1)}
	done := &Card{ID: "done", DoneAt: now.AddDate(0, 0, -2)}
	b.Lanes[Done] = []*Card{keep, done}
	a := &Archive{Cards: []*Card{{ID: "older", DoneAt: now.AddDate(0, 0, -10)}}}
	if c := b.ArchiveDone(a, 1, now); c != done {
		t.Fatalf("ArchiveDone returned %v, want the Done[1] card", c)
	}
	if got := laneIDs(b, Done); got != "keep" {
		t.Errorf("Done after archiving: %q, want %q", got, "keep")
	}
	if got := archiveIDs(a); got != "done older" {
		t.Errorf("archive should be newest-first: %q, want %q", got, "done older")
	}
}

func TestArchiveDoneRejectsOutOfRange(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	b := &Board{}
	b.Lanes[Done] = []*Card{{ID: "only", DoneAt: now}}
	a := &Archive{}
	for _, i := range []int{-1, 1, 5} {
		if c := b.ArchiveDone(a, i, now); c != nil {
			t.Errorf("index %d: got %v, want nil", i, c)
		}
	}
	if len(b.Lanes[Done]) != 1 || len(a.Cards) != 0 {
		t.Errorf("an out-of-range archive must change nothing: done=%d archive=%d", len(b.Lanes[Done]), len(a.Cards))
	}
}

func TestArchiveDoneKeepsDoneAtAndMovedAt(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	then := now.AddDate(0, 0, -3)
	b := &Board{}
	c := &Card{ID: "c", DoneAt: then, MovedAt: then}
	b.Lanes[Done] = []*Card{c}
	b.ArchiveDone(&Archive{}, 0, now)
	if !c.DoneAt.Equal(then) || !c.MovedAt.Equal(then) {
		t.Errorf("archiving must not restamp: done=%v moved=%v, want both %v", c.DoneAt, c.MovedAt, then)
	}
}

func TestArchiveDoneStampsAMissingDoneAt(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	then := now.AddDate(0, 0, -3)
	b := &Board{}
	c := &Card{ID: "c", MovedAt: then} // hand-edited: in Done without a done: date
	b.Lanes[Done] = []*Card{c}
	b.ArchiveDone(&Archive{}, 0, now)
	if !c.DoneAt.Equal(now) {
		t.Errorf("a zero DoneAt should become now so the card gets a real week, got %v", c.DoneAt)
	}
	if !c.MovedAt.Equal(then) {
		t.Errorf("MovedAt must still be untouched: %v", c.MovedAt)
	}
}

func TestArchiveInsertKeepsNewestFirst(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	day := func(n int) time.Time { return now.AddDate(0, 0, -n) }
	a := &Archive{Cards: []*Card{{ID: "d1", DoneAt: day(1)}, {ID: "d3", DoneAt: day(3)}, {ID: "d5", DoneAt: day(5)}}}
	cases := []struct {
		id   string
		at   time.Time
		want int
	}{
		{"newest", day(0), 0},
		{"between", day(2), 2},
		{"tie", day(3), 3}, // a tie lands ahead of the card already there
		{"oldest", day(9), 6},
	}
	for _, tc := range cases {
		if got := a.Insert(&Card{ID: tc.id, DoneAt: tc.at}); got != tc.want {
			t.Errorf("Insert(%s): index %d, want %d (order now %q)", tc.id, got, tc.want, archiveIDs(a))
		}
	}
	if got, want := archiveIDs(a), "newest d1 between tie d3 d5 oldest"; got != want {
		t.Errorf("order: %q, want %q", got, want)
	}
}

func TestArchiveInsertIsVisibleAtTheCap(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	a := &Archive{}
	for i := 0; i < ArchiveMax; i++ {
		a.Cards = append(a.Cards, &Card{ID: DeriveID(string(rune('a' + i%26))), Title: "old", DoneAt: now.AddDate(0, 0, -30-i)})
	}
	fresh := &Card{ID: "fresh", Title: "just done", DoneAt: now}
	a.Insert(fresh)
	groups, _, scanned, total := ArchiveView(a, Parse(""), now)
	if total != ArchiveMax+1 || scanned != ArchiveMax {
		t.Fatalf("scanned=%d total=%d", scanned, total)
	}
	// ArchiveView takes the first ArchiveMax cards without sorting, so an
	// appended card would be exactly the one it drops.
	if len(groups) == 0 || groups[0].Label != ThisWeek.Label() || groups[0].Cards[0] != fresh {
		t.Errorf("a freshly archived card must show under THIS WEEK even at the cap; got %+v", groups)
	}
}

func TestArchiveFind(t *testing.T) {
	x, y := &Card{ID: "x"}, &Card{ID: "y"}
	a := &Archive{Cards: []*Card{x, y}}
	if i, c := a.Find("y"); i != 1 || c != y {
		t.Errorf("Find(y) = %d,%v, want 1,y", i, c)
	}
	if i, c := a.Find("nope"); i != -1 || c != nil {
		t.Errorf("Find(nope) = %d,%v, want -1,nil", i, c)
	}
	if i, c := (&Archive{}).Find("x"); i != -1 || c != nil {
		t.Errorf("Find on an empty archive = %d,%v, want -1,nil", i, c)
	}
}

func TestArchiveDoneThenRestoreRoundTrip(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	then := now.AddDate(0, 0, -2)
	b := &Board{}
	c := &Card{ID: "c", Title: "round trip", DoneAt: then, MovedAt: then}
	b.Lanes[Done] = []*Card{c}
	a := &Archive{}
	if b.ArchiveDone(a, 0, now) != c {
		t.Fatal("archive")
	}
	i, _ := a.Find("c")
	if got := b.Restore(a, i, now); got != c {
		t.Fatalf("restore returned %v", got)
	}
	if len(a.Cards) != 0 || laneIDs(b, Doing) != "c" {
		t.Errorf("after the round trip: archive=%d doing=%q", len(a.Cards), laneIDs(b, Doing))
	}
	if !c.DoneAt.IsZero() || !c.MovedAt.Equal(now) {
		t.Errorf("restore must stamp like a move: done=%v moved=%v", c.DoneAt, c.MovedAt)
	}
}

// TestSetNotesTrimsLeadingBlankLines closes an asymmetry between the two
// trimmers. SetNotes trimmed only the right end, while store.trimBlank strips
// blank lines from both ends on read and on write — so a note that began with
// a blank line rendered an extra row in the TUI detail pane and then lost it
// on the next reload, when the parse path trimmed what the write path had
// kept. Now the stored value is what a reload would produce.
func TestSetNotesTrimsLeadingBlankLines(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"a leading newline", "\nfoo", "foo"},
		{"several blank lines, some with whitespace", "\n\n  \n\t\nfoo", "foo"},
		{"blank lines at both ends", "\n\nfoo\n\n", "foo"},
		{
			// The case a naive TrimLeft(value, "\n \t") would break:
			// trimBlank drops whole whitespace-only lines, never the
			// indentation of a line that has content.
			name: "indentation on the first line survives",
			in:   "  indented first\nsecond",
			want: "  indented first\nsecond",
		},
		{
			name: "a tab-indented first line survives",
			in:   "\n\t indented\nsecond",
			want: "\t indented\nsecond",
		},
		{"interior blank lines are untouched", "a\n\nb", "a\n\nb"},
		{"all blank collapses to empty", "\n\n  \n", ""},
		{
			// Both ends must use the same definition of "blank" that
			// store.trimBlank uses, which is unicode.IsSpace. A non-ASCII
			// space is the case that tells a line-based trim apart from a
			// TrimRight cutset of "\n \t", and it arrives readily: a note
			// pasted from a web page through kando notes --file often ends
			// in one.
			name: "a leading non-breaking-space line",
			in:   "\u00a0\nfoo",
			want: "foo",
		},
		{
			name: "a trailing non-breaking-space line",
			in:   "foo\n\u00a0",
			want: "foo",
		},
		{
			name: "a trailing ideographic-space line",
			in:   "foo\n\u3000",
			want: "foo",
		},
		{
			// Trailing spaces on a line that has content are still stripped;
			// that is TrimRight's job and it is unchanged.
			name: "trailing spaces on a content line still go",
			in:   "foo   ",
			want: "foo",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &Card{}
			c.SetNotes(tc.in)
			if c.Notes != tc.want {
				t.Errorf("SetNotes(%q)\n got %q\nwant %q", tc.in, c.Notes, tc.want)
			}
		})
	}
}

// TestSetNotesClipCannotLeaveABlankLastLine pins the ordering of the clip and
// the blank-line trim.
//
// The clip is what makes the order load-bearing: it cuts at a byte ceiling with
// no idea where the lines are, so a line that was interior text can become the
// last line. If the trim ran first, the value stored here would end in a blank
// line — the exact asymmetry with store.trimBlank that the trim exists to close,
// reintroduced at the 16 KiB boundary where no other test looks.
//
// The budget is a literal, not maxNotesBytes: asserting against the constant the
// code clips by would pass at any value of it.
func TestSetNotesClipCannotLeaveABlankLastLine(t *testing.T) {
	// Fill to just under the ceiling, then a blank line, then enough text that
	// the clip must land inside it.
	in := strings.Repeat("a", 16380) + "\n\u00a0\n" + strings.Repeat("b", 4096)

	c := &Card{}
	c.SetNotes(in)

	if len(c.Notes) > 16384 {
		t.Fatalf("notes not clipped: %d bytes", len(c.Notes))
	}
	lines := strings.Split(c.Notes, "\n")
	if last := lines[len(lines)-1]; strings.TrimSpace(last) == "" {
		t.Errorf("the clip left a blank last line: %q (%d lines)", last, len(lines))
	}
}
