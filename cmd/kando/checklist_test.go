package main

import (
	"io"
	"testing"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

func TestChecklistAddArgs(t *testing.T) {
	card, text, name, err := checklistAddArgs([]string{"Renew passport", "Fill in the form", "work"})
	if err != nil || card != "Renew passport" || text != "Fill in the form" || name != "work" {
		t.Fatalf("card=%q text=%q name=%q err=%v", card, text, name, err)
	}
	if _, _, _, err := checklistAddArgs([]string{"Renew passport", ""}); err == nil {
		t.Fatal("blank text should be rejected")
	}
}

func seedChecklistBoard(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes[board.Todo] = []*board.Card{{
		ID: "aaaaaaaa", Title: "Renew passport",
		Checklist: []board.Item{{Text: "Photos", Done: true}, {Text: "Form", Done: false}},
	}}
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestAddChecklistItem(t *testing.T) {
	root := seedChecklistBoard(t)
	title, n, err := addChecklistItem(root, "life", "aaaaaaaa", "Passport photo appointment")
	if err != nil || title != "Renew passport" || n != 3 {
		t.Fatalf("title=%q n=%d err=%v", title, n, err)
	}
	got, err := store.Load(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	items := got.Lanes[board.Todo][0].Checklist
	if len(items) != 3 || items[2].Text != "Passport photo appointment" {
		t.Errorf("checklist: %+v", items)
	}

	t.Run("blank text is rejected, board unchanged", func(t *testing.T) {
		before := readBoardFile(t, root, "life")
		if _, _, err := addChecklistItem(root, "life", "aaaaaaaa", "\x00"); err == nil {
			t.Fatal("want an error")
		}
		if after := readBoardFile(t, root, "life"); before != after {
			t.Errorf("board.md should be unchanged")
		}
	})
}

func TestParseItemNumber(t *testing.T) {
	if n, err := parseItemNumber("2"); err != nil || n != 2 {
		t.Errorf("n=%d err=%v", n, err)
	}
	for _, s := range []string{"0", "-1", "x", ""} {
		if _, err := parseItemNumber(s); err == nil {
			t.Errorf("parseItemNumber(%q) should error", s)
		}
	}
}

func TestChecklistIndex(t *testing.T) {
	c := &board.Card{Title: "X", Checklist: []board.Item{{Text: "a"}, {Text: "b"}}}

	if i, err := checklistIndex(c, 1, nil); err != nil || i != 0 {
		t.Errorf("i=%d err=%v", i, err)
	}
	if i, err := checklistIndex(c, 2, nil); err != nil || i != 1 {
		t.Errorf("i=%d err=%v", i, err)
	}
	if _, err := checklistIndex(c, 3, nil); err == nil {
		t.Error("out of range should error")
	}
	if _, err := checklistIndex(c, 0, nil); err == nil {
		t.Error("zero should error")
	}

	t.Run("was matches", func(t *testing.T) {
		was := "a"
		if i, err := checklistIndex(c, 1, &was); err != nil || i != 0 {
			t.Errorf("i=%d err=%v", i, err)
		}
	})
	t.Run("was mismatches", func(t *testing.T) {
		was := "different"
		if _, err := checklistIndex(c, 1, &was); err == nil {
			t.Error("a stale --was should error")
		}
	})
}

func TestChecklistToggleArgs(t *testing.T) {
	card, n, was, name, err := checklistToggleArgs([]string{"Renew passport", "2", "work", "--was", "Form"}, io.Discard)
	if err != nil || card != "Renew passport" || n != 2 || name != "work" || was == nil || *was != "Form" {
		t.Fatalf("card=%q n=%d name=%q was=%v err=%v", card, n, name, was, err)
	}
	if _, _, was, _, err := checklistToggleArgs([]string{"Renew passport", "2"}, io.Discard); err != nil || was != nil {
		t.Errorf("was should be nil when --was is not given: was=%v err=%v", was, err)
	}
	if _, _, _, _, err := checklistToggleArgs([]string{"Renew passport", "zero"}, io.Discard); err == nil {
		t.Error("a non-numeric item number should error")
	}
}

func TestToggleChecklistItem(t *testing.T) {
	root := seedChecklistBoard(t)

	title, item, err := toggleChecklistItem(root, "life", "aaaaaaaa", 2, nil)
	if err != nil || title != "Renew passport" || item.Text != "Form" || !item.Done {
		t.Fatalf("title=%q item=%+v err=%v", title, item, err)
	}

	t.Run("out of range", func(t *testing.T) {
		if _, _, err := toggleChecklistItem(root, "life", "aaaaaaaa", 99, nil); err == nil {
			t.Fatal("want an error")
		}
	})

	t.Run("stale --was leaves the file unchanged", func(t *testing.T) {
		before := readBoardFile(t, root, "life")
		was := "not the real text"
		if _, _, err := toggleChecklistItem(root, "life", "aaaaaaaa", 1, &was); err == nil {
			t.Fatal("want an error")
		}
		if after := readBoardFile(t, root, "life"); before != after {
			t.Errorf("board.md should be unchanged")
		}
	})
}

func TestChecklistEditArgs(t *testing.T) {
	card, n, text, was, name, err := checklistEditArgs([]string{"Renew passport", "1", "New text", "work"}, io.Discard)
	if err != nil || card != "Renew passport" || n != 1 || text != "New text" || name != "work" || was != nil {
		t.Fatalf("card=%q n=%d text=%q name=%q was=%v err=%v", card, n, text, name, was, err)
	}
	if _, _, _, _, _, err := checklistEditArgs([]string{"Renew passport", "1", ""}, io.Discard); err == nil {
		t.Error("blank text should be rejected")
	}
}

func TestEditChecklistItem(t *testing.T) {
	root := seedChecklistBoard(t)
	title, item, err := editChecklistItem(root, "life", "aaaaaaaa", 2, "Fill in the passport form", nil)
	if err != nil || title != "Renew passport" || item.Text != "Fill in the passport form" || item.Done {
		t.Fatalf("title=%q item=%+v err=%v", title, item, err)
	}

	t.Run("editing preserves Done", func(t *testing.T) {
		_, item, err := editChecklistItem(root, "life", "aaaaaaaa", 1, "New photos text", nil)
		if err != nil || !item.Done {
			t.Fatalf("item=%+v err=%v, want Done still true", item, err)
		}
	})
}
