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

	// The other three read verbs each pin this; summarizeBoards is the one
	// that touches every board on disk, so a store.Open slipped in here
	// would rewrite them all — including stamping ids onto hand-written
	// cards — on a bare `kando board list --json`.
	t.Run("never rewrites any board.md", func(t *testing.T) {
		before := readBoardFile(t, root, "life")
		if _, err := summarizeBoards(root, []string{"life", "work"}); err != nil {
			t.Fatal(err)
		}
		if after := readBoardFile(t, root, "life"); before != after {
			t.Errorf("board list must never rewrite board.md:\nbefore:\n%s\nafter:\n%s", before, after)
		}
	})
}

func TestBoardCreateArgs(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantName string
		wantErr  bool
	}{
		{name: "no args", wantErr: true},
		{name: "one name", args: []string{"work"}, wantName: "work"},
		{name: "trimmed", args: []string{" work "}, wantName: "work"},
		{name: "too many", args: []string{"work", "extra"}, wantErr: true},
		{name: "empty", args: []string{""}, wantErr: true},
		{name: "whitespace-only", args: []string{"   "}, wantErr: true},
		{name: "starts with a dash", args: []string{"--oops"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, err := boardCreateArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want an error, got %q", name)
				}
				return
			}
			if err != nil {
				t.Fatalf("boardCreateArgs: %v", err)
			}
			if name != tc.wantName {
				t.Errorf("name=%q, want %q", name, tc.wantName)
			}
		})
	}
}

func TestCreateBoard(t *testing.T) {
	root := t.TempDir()

	t.Run("creates a new board", func(t *testing.T) {
		created, err := createBoard(root, "work")
		if err != nil || !created {
			t.Fatalf("created=%v err=%v", created, err)
		}
		if !store.Exists(root, "work") {
			t.Error("board should now exist")
		}
	})

	t.Run("idempotent: an existing board is reported, not recreated", func(t *testing.T) {
		before := mustReadCLI(t, root+"/work/board.md")
		created, err := createBoard(root, "work")
		if err != nil || created {
			t.Fatalf("created=%v err=%v, want created=false", created, err)
		}
		if after := mustReadCLI(t, root+"/work/board.md"); before != after {
			t.Errorf("an existing board's file should be untouched")
		}
	})

	t.Run("invalid name", func(t *testing.T) {
		if _, err := createBoard(root, "has/slash"); err == nil {
			t.Fatal("want an error")
		}
	})
}
