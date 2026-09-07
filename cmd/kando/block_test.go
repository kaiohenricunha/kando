package main

import (
	"io"
	"testing"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

func TestBlockArgs(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantCard   string
		wantReason string
		wantName   string
		wantErr    bool
	}{
		{name: "card and reason", args: []string{"Renew passport", "--reason", "waiting on photos"}, wantCard: "Renew passport", wantReason: "waiting on photos"},
		{name: "reason before card", args: []string{"--reason", "waiting", "Renew passport"}, wantCard: "Renew passport", wantReason: "waiting"},
		{name: "card, reason and board", args: []string{"Renew passport", "work", "--reason", "waiting"}, wantCard: "Renew passport", wantReason: "waiting", wantName: "work"},
		{name: "missing reason", args: []string{"Renew passport"}, wantErr: true},
		{name: "blank reason", args: []string{"Renew passport", "--reason", "   "}, wantErr: true},
		{name: "no args", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			card, reason, name, err := blockArgs(tc.args, io.Discard)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want an error, got card=%q reason=%q", card, reason)
				}
				return
			}
			if err != nil {
				t.Fatalf("blockArgs: %v", err)
			}
			if card != tc.wantCard || reason != tc.wantReason || name != tc.wantName {
				t.Errorf("card=%q reason=%q name=%q, want card=%q reason=%q name=%q", card, reason, name, tc.wantCard, tc.wantReason, tc.wantName)
			}
		})
	}
}

func seedBlockBoard(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes[board.Todo] = []*board.Card{{ID: "aaaaaaaa", Title: "Renew passport"}}
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestBlockCard(t *testing.T) {
	root := seedBlockBoard(t)
	title, err := blockCard(root, "life", "aaaaaaaa", "waiting on photos")
	if err != nil || title != "Renew passport" {
		t.Fatalf("title=%q err=%v", title, err)
	}
	got, err := store.Load(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	c := got.Lanes[board.Todo][0]
	if !c.Blocked || c.BlockedReason != "waiting on photos" {
		t.Errorf("blocked=%v reason=%q", c.Blocked, c.BlockedReason)
	}

	t.Run("nonexistent card", func(t *testing.T) {
		if _, err := blockCard(root, "life", "nope", "x"); err == nil {
			t.Fatal("want an error")
		}
	})
}

func TestUnblockArgs(t *testing.T) {
	card, name, err := unblockArgs([]string{"Renew passport", "work"})
	if err != nil || card != "Renew passport" || name != "work" {
		t.Fatalf("card=%q name=%q err=%v", card, name, err)
	}
	if _, _, err := unblockArgs(nil); err == nil {
		t.Fatal("want an error for no args")
	}
}

func TestUnblockCard(t *testing.T) {
	t.Run("clears a blocked card", func(t *testing.T) {
		root := t.TempDir()
		st, b, err := store.Open(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		b.Lanes[board.Todo] = []*board.Card{{ID: "aaaaaaaa", Title: "Renew passport", Blocked: true, BlockedReason: "waiting"}}
		if err := st.SaveBoard(b); err != nil {
			t.Fatal(err)
		}
		title, wasBlocked, err := unblockCard(root, "life", "aaaaaaaa")
		if err != nil || title != "Renew passport" || !wasBlocked {
			t.Fatalf("title=%q wasBlocked=%v err=%v", title, wasBlocked, err)
		}
		got, err := store.Load(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		c := got.Lanes[board.Todo][0]
		if c.Blocked || c.BlockedReason != "" {
			t.Errorf("blocked=%v reason=%q", c.Blocked, c.BlockedReason)
		}
	})

	t.Run("not blocked leaves the file unchanged", func(t *testing.T) {
		root := seedBlockBoard(t)
		before := readBoardFile(t, root, "life")
		title, wasBlocked, err := unblockCard(root, "life", "aaaaaaaa")
		if err != nil || title != "Renew passport" || wasBlocked {
			t.Fatalf("title=%q wasBlocked=%v err=%v", title, wasBlocked, err)
		}
		if after := readBoardFile(t, root, "life"); before != after {
			t.Errorf("unblocking an already-unblocked card should not rewrite the file")
		}
	})
}
