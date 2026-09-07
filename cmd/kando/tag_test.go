package main

import (
	"testing"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

func TestTagArgs(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantCard string
		wantTag  string
		wantName string
		wantErr  bool
	}{
		{name: "card and tag", args: []string{"Renew passport", "errand"}, wantCard: "Renew passport", wantTag: "errand"},
		{name: "empty tag clears", args: []string{"Renew passport", ""}, wantCard: "Renew passport", wantTag: ""},
		{name: "card tag and board", args: []string{"Renew passport", "errand", "work"}, wantCard: "Renew passport", wantTag: "errand", wantName: "work"},
		{name: "no args", wantErr: true},
		{name: "one arg", args: []string{"Renew passport"}, wantErr: true},
		{name: "empty card", args: []string{"", "errand"}, wantErr: true},
		{name: "too many", args: []string{"a", "b", "c", "d"}, wantErr: true},
		{name: "board starting with a dash", args: []string{"a", "b", "--oops"}, wantErr: true},
		{name: "empty board name", args: []string{"a", "b", ""}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			card, tag, name, err := tagArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want an error, got card=%q tag=%q", card, tag)
				}
				return
			}
			if err != nil {
				t.Fatalf("tagArgs: %v", err)
			}
			if card != tc.wantCard || tag != tc.wantTag || name != tc.wantName {
				t.Errorf("card=%q tag=%q name=%q, want card=%q tag=%q name=%q", card, tag, name, tc.wantCard, tc.wantTag, tc.wantName)
			}
		})
	}
}

func TestTagCard(t *testing.T) {
	seed := func(t *testing.T) string {
		t.Helper()
		root := t.TempDir()
		st, b, err := store.Open(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		b.Lanes[board.Todo] = []*board.Card{{ID: "aaaaaaaa", Title: "Renew passport", Tag: "old"}}
		if err := st.SaveBoard(b); err != nil {
			t.Fatal(err)
		}
		return root
	}

	t.Run("sets a new tag", func(t *testing.T) {
		root := seed(t)
		title, tag, err := tagCard(root, "life", "aaaaaaaa", "#errand")
		if err != nil || title != "Renew passport" || tag != "errand" {
			t.Fatalf("title=%q tag=%q err=%v", title, tag, err)
		}
	})

	t.Run("empty tag clears it", func(t *testing.T) {
		root := seed(t)
		_, tag, err := tagCard(root, "life", "aaaaaaaa", "")
		if err != nil || tag != "" {
			t.Fatalf("tag=%q err=%v", tag, err)
		}
	})

	t.Run("same tag leaves the file unchanged", func(t *testing.T) {
		root := seed(t)
		before := readBoardFile(t, root, "life")
		if _, _, err := tagCard(root, "life", "aaaaaaaa", "old"); err != nil {
			t.Fatal(err)
		}
		if after := readBoardFile(t, root, "life"); before != after {
			t.Errorf("setting the same tag should not rewrite the file")
		}
	})

	t.Run("nonexistent card", func(t *testing.T) {
		root := seed(t)
		if _, _, err := tagCard(root, "life", "nope", "x"); err == nil {
			t.Fatal("want an error")
		}
	})
}
