package main

import (
	"io"
	"testing"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

func TestBoardListArgs(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantJSON bool
		wantErr  bool
	}{
		{name: "no args"},
		{name: "json flag", args: []string{"--json"}, wantJSON: true},
		{name: "unexpected positional", args: []string{"work"}, wantErr: true},
		{name: "unknown flag", args: []string{"--nope"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			jsonOut, err := boardListArgs(tc.args, io.Discard)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want an error, got jsonOut=%v", jsonOut)
				}
				return
			}
			if err != nil {
				t.Fatalf("boardListArgs: %v", err)
			}
			if jsonOut != tc.wantJSON {
				t.Errorf("jsonOut=%v, want %v", jsonOut, tc.wantJSON)
			}
		})
	}
}

func TestSummarizeBoards(t *testing.T) {
	root := t.TempDir()
	st, life, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	life.Lanes[board.Todo] = []*board.Card{{ID: "aaaaaaaa", Title: "One"}, {ID: "bbbbbbbb", Title: "Two"}}
	life.Lanes[board.Done] = []*board.Card{{ID: "cccccccc", Title: "Three"}}
	if err := st.SaveBoard(life); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Open(root, "work"); err != nil {
		t.Fatal(err)
	}

	got, err := summarizeBoards(root, []string{"life", "work"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d boards, want 2", len(got))
	}
	if got[0].Name != "life" || got[0].Cards != 3 || got[0].Lanes["todo"] != 2 || got[0].Lanes["done"] != 1 {
		t.Errorf("life: %+v", got[0])
	}
	if got[1].Name != "work" || got[1].Cards != 0 {
		t.Errorf("work: %+v", got[1])
	}

	t.Run("nonexistent board is an error", func(t *testing.T) {
		if _, err := summarizeBoards(root, []string{"ghost"}); err == nil {
			t.Fatal("want an error")
		}
	})
}
