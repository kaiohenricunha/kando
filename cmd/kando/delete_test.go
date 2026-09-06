package main

import (
	"testing"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

func TestDeleteArgs(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantCard string
		wantName string
		wantErr  bool
	}{
		{name: "card only", args: []string{"Renew passport"}, wantCard: "Renew passport"},
		{name: "card and board", args: []string{"Renew passport", "work"}, wantCard: "Renew passport", wantName: "work"},
		{name: "no args", wantErr: true},
		{name: "empty card", args: []string{""}, wantErr: true},
		{name: "too many", args: []string{"a", "b", "c"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			card, name, err := deleteArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want an error, got card=%q", card)
				}
				return
			}
			if err != nil {
				t.Fatalf("deleteArgs: %v", err)
			}
			if card != tc.wantCard || name != tc.wantName {
				t.Errorf("card=%q name=%q, want card=%q name=%q", card, name, tc.wantCard, tc.wantName)
			}
		})
	}
}

func TestDeleteCard(t *testing.T) {
	seed := func(t *testing.T) string {
		t.Helper()
		root := t.TempDir()
		st, b, err := store.Open(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		b.Lanes[board.Todo] = []*board.Card{{ID: "aaaaaaaa", Title: "Renew passport"}, {ID: "bbbbbbbb", Title: "Other"}}
		if err := st.SaveBoard(b); err != nil {
			t.Fatal(err)
		}
		return root
	}

	t.Run("removes the card", func(t *testing.T) {
		root := seed(t)
		title, lane, err := deleteCard(root, "life", "aaaaaaaa")
		if err != nil || title != "Renew passport" || lane != board.Todo {
			t.Fatalf("title=%q lane=%v err=%v", title, lane, err)
		}
		got, err := store.Load(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Lanes[board.Todo]) != 1 || got.Lanes[board.Todo][0].ID != "bbbbbbbb" {
			t.Errorf("remaining: %+v", got.Lanes[board.Todo])
		}
	})

	t.Run("nonexistent card leaves the board unchanged", func(t *testing.T) {
		root := seed(t)
		before := readBoardFile(t, root, "life")
		if _, _, err := deleteCard(root, "life", "nope"); err == nil {
			t.Fatal("want an error")
		}
		if after := readBoardFile(t, root, "life"); before != after {
			t.Errorf("board.md should be unchanged")
		}
	})

	t.Run("nonexistent board", func(t *testing.T) {
		root := t.TempDir()
		if _, _, err := deleteCard(root, "ghost", "x"); err == nil {
			t.Fatal("want an error")
		}
	})
}
