package main

import (
	"io"
	"testing"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

func TestAddArgs(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantVal  string
		wantName string
		wantLane board.Lane
		wantTag  string
		wantErr  bool
	}{
		{name: "title only", args: []string{"Renew passport"}, wantVal: "Renew passport", wantLane: board.Todo},
		{name: "title and board", args: []string{"Renew passport", "work"}, wantVal: "Renew passport", wantName: "work", wantLane: board.Todo},
		{name: "lane flag", args: []string{"Renew passport", "--lane", "Done"}, wantVal: "Renew passport", wantLane: board.Done},
		{name: "tag flag", args: []string{"Renew passport", "--tag", "#errand"}, wantVal: "Renew passport", wantLane: board.Todo, wantTag: "#errand"},
		{name: "empty title", args: []string{""}, wantErr: true},
		{name: "no args", wantErr: true},
		{name: "invalid lane", args: []string{"x", "--lane", "Sprint"}, wantErr: true},
		{name: "title starting with a dash needs --", args: []string{"--", "-1 point bug"}, wantVal: "-1 point bug", wantLane: board.Todo},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			title, name, lane, tag, err := addArgs(tc.args, io.Discard)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want an error, got title=%q lane=%v", title, lane)
				}
				return
			}
			if err != nil {
				t.Fatalf("addArgs: %v", err)
			}
			if title != tc.wantVal || name != tc.wantName || lane != tc.wantLane || tag != tc.wantTag {
				t.Errorf("got title=%q name=%q lane=%v tag=%q, want title=%q name=%q lane=%v tag=%q",
					title, name, lane, tag, tc.wantVal, tc.wantName, tc.wantLane, tc.wantTag)
			}
		})
	}
}

func TestAddCard(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

	t.Run("lands at the top of the lane", func(t *testing.T) {
		root := t.TempDir()
		st, b, err := store.Open(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		b.Lanes[board.Todo] = []*board.Card{{ID: "existing", Title: "Already there"}}
		if err := st.SaveBoard(b); err != nil {
			t.Fatal(err)
		}
		c, err := addCard(root, "life", "New card", board.Todo, "", now)
		if err != nil {
			t.Fatal(err)
		}
		got, err := store.Load(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Lanes[board.Todo]) != 2 || got.Lanes[board.Todo][0].ID != c.ID {
			t.Fatalf("new card should be at index 0: %+v", got.Lanes[board.Todo])
		}
	})

	t.Run("lane done stamps DoneAt", func(t *testing.T) {
		root := t.TempDir()
		if _, _, err := store.Open(root, "life"); err != nil {
			t.Fatal(err)
		}
		c, err := addCard(root, "life", "Finished already", board.Done, "", now)
		if err != nil {
			t.Fatal(err)
		}
		if !c.DoneAt.Equal(now) {
			t.Errorf("DoneAt = %v, want %v", c.DoneAt, now)
		}
	})

	t.Run("tag strips a leading hash", func(t *testing.T) {
		root := t.TempDir()
		if _, _, err := store.Open(root, "life"); err != nil {
			t.Fatal(err)
		}
		c, err := addCard(root, "life", "Tagged", board.Todo, "#errand", now)
		if err != nil {
			t.Fatal(err)
		}
		if c.Tag != "errand" {
			t.Errorf("Tag = %q, want %q", c.Tag, "errand")
		}
	})

	t.Run("control-only title is rejected, board unchanged", func(t *testing.T) {
		root := t.TempDir()
		if _, _, err := store.Open(root, "life"); err != nil {
			t.Fatal(err)
		}
		before := readBoardFile(t, root, "life")
		if _, err := addCard(root, "life", "\x00\x01", board.Todo, "", now); err == nil {
			t.Fatal("want an error")
		}
		if after := readBoardFile(t, root, "life"); before != after {
			t.Errorf("a rejected add must not change board.md")
		}
	})

	t.Run("nonexistent board is not created", func(t *testing.T) {
		root := t.TempDir()
		if _, err := addCard(root, "ghost", "X", board.Todo, "", now); err == nil {
			t.Fatal("want an error")
		}
	})
}
