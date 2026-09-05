package board

import (
	"reflect"
	"strings"
	"testing"
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
