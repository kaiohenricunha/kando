package main

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

func TestShowArgs(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantCard string
		wantName string
		wantJSON bool
		wantErr  bool
	}{
		{name: "no args", wantErr: true},
		{name: "card only", args: []string{"Renew passport"}, wantCard: "Renew passport"},
		{name: "card and board", args: []string{"Renew passport", "work"}, wantCard: "Renew passport", wantName: "work"},
		{name: "json flag before card", args: []string{"--json", "Renew passport"}, wantCard: "Renew passport", wantJSON: true},
		{name: "json flag after card", args: []string{"Renew passport", "--json"}, wantCard: "Renew passport", wantJSON: true},
		{name: "empty card", args: []string{""}, wantErr: true},
		{name: "too many args", args: []string{"a", "b", "c"}, wantErr: true},
		{name: "card starting with a dash needs --", args: []string{"--", "-urgent"}, wantCard: "-urgent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			card, name, jsonOut, err := showArgs(tc.args, io.Discard)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want an error, got card=%q name=%q json=%v", card, name, jsonOut)
				}
				return
			}
			if err != nil {
				t.Fatalf("showArgs: %v", err)
			}
			if card != tc.wantCard || name != tc.wantName || jsonOut != tc.wantJSON {
				t.Errorf("card=%q name=%q json=%v, want card=%q name=%q json=%v", card, name, jsonOut, tc.wantCard, tc.wantName, tc.wantJSON)
			}
		})
	}
}

func seedShowBoard(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes[board.Doing] = []*board.Card{{
		ID: "aaaaaaaa", Title: "Renew passport", Tag: "errand", Notes: "line1\nline2",
		Checklist: []board.Item{{Text: "Photos", Done: true}, {Text: "Form", Done: false}},
	}}
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestShowCard(t *testing.T) {
	root := seedShowBoard(t)

	t.Run("by id", func(t *testing.T) {
		lane, c, err := showCard(root, "life", "aaaaaaaa")
		if err != nil || lane != board.Doing || c.Title != "Renew passport" {
			t.Fatalf("lane=%v c=%v err=%v", lane, c, err)
		}
	})

	t.Run("by title, case-insensitive", func(t *testing.T) {
		_, c, err := showCard(root, "life", "renew PASSPORT")
		if err != nil || c.ID != "aaaaaaaa" {
			t.Fatalf("c=%v err=%v", c, err)
		}
	})

	t.Run("does not rewrite the board", func(t *testing.T) {
		before := readBoardFile(t, root, "life")
		if _, _, err := showCard(root, "life", "aaaaaaaa"); err != nil {
			t.Fatal(err)
		}
		if after := readBoardFile(t, root, "life"); before != after {
			t.Errorf("show must never rewrite board.md")
		}
	})

	t.Run("nonexistent card", func(t *testing.T) {
		if _, _, err := showCard(root, "life", "nope"); err == nil {
			t.Fatal("want an error")
		}
	})

	t.Run("nonexistent board", func(t *testing.T) {
		if _, _, err := showCard(root, "ghost", "anything"); err == nil {
			t.Fatal("want an error")
		}
	})
}

func TestFormatCard(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

	t.Run("bare card", func(t *testing.T) {
		c := &board.Card{ID: "aaaaaaaa", Title: "Bare"}
		out := formatCard(board.Todo, c, now)
		if !strings.HasPrefix(out, "Bare\nlane: Todo\nid: aaaaaaaa") {
			t.Errorf("got:\n%s", out)
		}
		if strings.Contains(out, "tag:") || strings.Contains(out, "notes:") || strings.Contains(out, "checklist") || strings.Contains(out, "blocked:") {
			t.Errorf("empty sections should be omitted:\n%s", out)
		}
	})

	t.Run("full card", func(t *testing.T) {
		c := &board.Card{
			ID: "aaaaaaaa", Title: "Full", Tag: "errand", Notes: "line1\nline2",
			Blocked: true, BlockedReason: "waiting",
			Checklist: []board.Item{{Text: "a", Done: true}, {Text: "b", Done: false}},
		}
		out := formatCard(board.Doing, c, now)
		for _, want := range []string{
			"tag: #errand", "blocked: waiting", "notes:\n  line1\n  line2",
			"checklist 1/2:", "1. [x] a", "2. [ ] b",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("missing %q in:\n%s", want, out)
			}
		}
	})
}
