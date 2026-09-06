package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

func TestMoveArgs(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantCard string
		wantLane string
		wantName string
		wantErr  bool
	}{
		{name: "no arguments", wantErr: true},
		{name: "one argument", args: []string{"card"}, wantErr: true},
		{name: "card and lane", args: []string{"Renew passport", "Doing"}, wantCard: "Renew passport", wantLane: "Doing"},
		{name: "card, lane and board", args: []string{"Renew passport", "Doing", "work"}, wantCard: "Renew passport", wantLane: "Doing", wantName: "work"},
		{name: "too many arguments", args: []string{"a", "b", "c", "d"}, wantErr: true},
		{name: "empty card", args: []string{"", "Doing"}, wantErr: true},
		{name: "whitespace-only card", args: []string{"   ", "Doing"}, wantErr: true},
		{name: "board starting with a dash", args: []string{"card", "Doing", "--oops"}, wantErr: true},
		{name: "card starting with a dash is not a flag", args: []string{"-1 point bug", "Doing"}, wantCard: "-1 point bug", wantLane: "Doing"},
		{name: "empty board name", args: []string{"card", "Doing", ""}, wantErr: true},
		{name: "whitespace-only board name", args: []string{"card", "Doing", "   "}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			card, lane, name, err := moveArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want an error, got card=%q lane=%q name=%q", card, lane, name)
				}
				return
			}
			if err != nil {
				t.Fatalf("moveArgs: %v", err)
			}
			if card != tc.wantCard || lane != tc.wantLane || name != tc.wantName {
				t.Errorf("card=%q lane=%q name=%q, want card=%q lane=%q name=%q", card, lane, name, tc.wantCard, tc.wantLane, tc.wantName)
			}
		})
	}
}

func TestFindCard(t *testing.T) {
	passport := &board.Card{ID: "aaaaaaaa", Title: "Renew passport"}
	milk := &board.Card{ID: "bbbbbbbb", Title: "Buy milk"}
	dup1 := &board.Card{ID: "cccccccc", Title: "Dup"}
	dup2 := &board.Card{ID: "dddddddd", Title: "Dup"}
	sameLane1 := &board.Card{ID: "eeeeeeee", Title: "Same"}
	sameLane2 := &board.Card{ID: "ffffffff", Title: "Same"}
	collide := &board.Card{ID: "Buy milk", Title: "id equals another card's title"}

	b := &board.Board{Name: "life"}
	b.Lanes[board.Todo] = []*board.Card{passport, dup1, sameLane1, sameLane2}
	b.Lanes[board.Doing] = []*board.Card{milk, dup2}
	b.Lanes[board.Done] = []*board.Card{collide}

	t.Run("exact id", func(t *testing.T) {
		_, _, c, err := findCard(b, "aaaaaaaa")
		if err != nil || c != passport {
			t.Fatalf("got c=%v err=%v, want passport", c, err)
		}
	})

	t.Run("unique case-insensitive title", func(t *testing.T) {
		_, _, c, err := findCard(b, "renew PASSPORT")
		if err != nil || c != passport {
			t.Fatalf("got c=%v err=%v, want passport", c, err)
		}
	})

	t.Run("zero matches", func(t *testing.T) {
		_, i, c, err := findCard(b, "nope")
		if err == nil || c != nil || i != -1 {
			t.Fatalf("got i=%d c=%v err=%v, want a miss", i, c, err)
		}
	})

	t.Run("ambiguous across different lanes", func(t *testing.T) {
		_, i, c, err := findCard(b, "dup")
		if err == nil || c != nil || i != -1 {
			t.Fatalf("got i=%d c=%v err=%v, want an ambiguity error", i, c, err)
		}
		if !strings.Contains(err.Error(), dup1.ID) || !strings.Contains(err.Error(), dup2.ID) {
			t.Errorf("error %q should list both ids so the ambiguity is self-service", err.Error())
		}
	})

	t.Run("ambiguous within the same lane", func(t *testing.T) {
		_, i, c, err := findCard(b, "same")
		if err == nil || c != nil || i != -1 {
			t.Fatalf("got i=%d c=%v err=%v, want an ambiguity error", i, c, err)
		}
		if !strings.Contains(err.Error(), sameLane1.ID) || !strings.Contains(err.Error(), sameLane2.ID) {
			t.Errorf("error %q should list both ids so the ambiguity is self-service", err.Error())
		}
	})

	t.Run("id takes priority over a colliding title", func(t *testing.T) {
		// collide.ID == "Buy milk" == milk.Title: the id match must win.
		_, _, c, err := findCard(b, "Buy milk")
		if err != nil || c != collide {
			t.Fatalf("got c=%v err=%v, want the id match (collide)", c, err)
		}
	})

	t.Run("empty board", func(t *testing.T) {
		_, i, c, err := findCard(&board.Board{Name: "empty"}, "anything")
		if err == nil || c != nil || i != -1 {
			t.Fatalf("got i=%d c=%v err=%v, want a miss", i, c, err)
		}
	})
}

// loadBoard reloads root/name from disk, failing the test on any error.
func loadBoard(t *testing.T, root, name string) *board.Board {
	t.Helper()
	b, err := store.Load(root, name)
	if err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	return b
}

// readBoardFile returns root/name/board.md's contents, failing the test on
// any error.
func readBoardFile(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, name, "board.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestMoveCard(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

	seed := func(t *testing.T, root, name string, cards map[board.Lane][]*board.Card) {
		t.Helper()
		st, b, err := store.Open(root, name)
		if err != nil {
			t.Fatalf("seed: store.Open: %v", err)
		}
		for lane, cs := range cards {
			b.Lanes[lane] = cs
		}
		if err := st.SaveBoard(b); err != nil {
			t.Fatalf("seed: SaveBoard: %v", err)
		}
	}

	t.Run("happy path by id", func(t *testing.T) {
		root := t.TempDir()
		seed(t, root, "life", map[board.Lane][]*board.Card{
			board.Todo: {{ID: "aaaaaaaa", Title: "Renew passport"}},
		})
		title, from, err := moveCard(root, "life", "aaaaaaaa", board.Doing, now)
		if err != nil {
			t.Fatalf("moveCard: %v", err)
		}
		if title != "Renew passport" || from != board.Todo {
			t.Errorf("title=%q from=%v, want %q/%v", title, from, "Renew passport", board.Todo)
		}
		b := loadBoard(t, root, "life")
		if len(b.Lanes[board.Todo]) != 0 || len(b.Lanes[board.Doing]) != 1 {
			t.Fatalf("card did not move: todo=%d doing=%d", len(b.Lanes[board.Todo]), len(b.Lanes[board.Doing]))
		}
		if !b.Lanes[board.Doing][0].MovedAt.Equal(now) {
			t.Errorf("MovedAt = %v, want %v", b.Lanes[board.Doing][0].MovedAt, now)
		}
	})

	t.Run("happy path by title, and Done stamps DoneAt", func(t *testing.T) {
		root := t.TempDir()
		seed(t, root, "life", map[board.Lane][]*board.Card{
			board.Doing: {{ID: "aaaaaaaa", Title: "Renew Passport"}},
		})
		_, from, err := moveCard(root, "life", "renew passport", board.Done, now)
		if err != nil {
			t.Fatalf("moveCard: %v", err)
		}
		if from != board.Doing {
			t.Errorf("from=%v, want %v", from, board.Doing)
		}
		b := loadBoard(t, root, "life")
		if len(b.Lanes[board.Done]) != 1 || !b.Lanes[board.Done][0].DoneAt.Equal(now) {
			t.Fatalf("DoneAt not stamped: %+v", b.Lanes[board.Done])
		}
	})

	t.Run("nonexistent board does not create one", func(t *testing.T) {
		root := t.TempDir()
		_, _, err := moveCard(root, "ghost", "anything", board.Doing, now)
		if err == nil {
			t.Fatal("want an error for a nonexistent board")
		}
		if _, statErr := os.Stat(filepath.Join(root, "ghost")); !os.IsNotExist(statErr) {
			t.Errorf("board directory should not have been created, stat err = %v", statErr)
		}
	})

	t.Run("empty name defaults to life", func(t *testing.T) {
		root := t.TempDir()
		seed(t, root, "life", map[board.Lane][]*board.Card{
			board.Todo: {{ID: "aaaaaaaa", Title: "Renew passport"}},
		})
		_, _, err := moveCard(root, "", "aaaaaaaa", board.Doing, now)
		if err != nil {
			t.Fatalf("moveCard: %v", err)
		}
		b := loadBoard(t, root, "life")
		if len(b.Lanes[board.Doing]) != 1 {
			t.Fatalf("default board \"life\" was not used")
		}
	})

	t.Run("nonexistent card leaves the board unchanged", func(t *testing.T) {
		root := t.TempDir()
		seed(t, root, "life", map[board.Lane][]*board.Card{
			board.Todo: {{ID: "aaaaaaaa", Title: "Renew passport"}},
		})
		before := readBoardFile(t, root, "life")
		if _, _, err := moveCard(root, "life", "nope", board.Doing, now); err == nil {
			t.Fatal("want an error for a nonexistent card")
		}
		if after := readBoardFile(t, root, "life"); before != after {
			t.Errorf("board.md changed despite a failed move")
		}
	})

	t.Run("ambiguous title leaves the board unchanged", func(t *testing.T) {
		root := t.TempDir()
		seed(t, root, "life", map[board.Lane][]*board.Card{
			board.Todo:  {{ID: "aaaaaaaa", Title: "Dup"}},
			board.Doing: {{ID: "bbbbbbbb", Title: "Dup"}},
		})
		before := readBoardFile(t, root, "life")
		if _, _, err := moveCard(root, "life", "Dup", board.Done, now); err == nil {
			t.Fatal("want an ambiguity error")
		}
		if after := readBoardFile(t, root, "life"); before != after {
			t.Errorf("board.md changed despite a failed move")
		}
	})

	t.Run("same-lane move is a no-op", func(t *testing.T) {
		root := t.TempDir()
		created := now.Add(-24 * time.Hour)
		seed(t, root, "life", map[board.Lane][]*board.Card{
			board.Doing: {{ID: "aaaaaaaa", Title: "Renew passport", MovedAt: created}},
		})
		before := readBoardFile(t, root, "life")
		_, from, err := moveCard(root, "life", "aaaaaaaa", board.Doing, now)
		if err != nil {
			t.Fatalf("moveCard: %v", err)
		}
		if from != board.Doing {
			t.Errorf("from=%v, want %v", from, board.Doing)
		}
		b := loadBoard(t, root, "life")
		if !b.Lanes[board.Doing][0].MovedAt.Equal(created) {
			t.Errorf("MovedAt changed on a same-lane no-op: got %v, want %v", b.Lanes[board.Doing][0].MovedAt, created)
		}
		if after := readBoardFile(t, root, "life"); before != after {
			t.Errorf("board.md rewritten despite a same-lane no-op")
		}
	})

	t.Run("blocked state is untouched", func(t *testing.T) {
		root := t.TempDir()
		seed(t, root, "life", map[board.Lane][]*board.Card{
			board.Todo: {{ID: "aaaaaaaa", Title: "Renew passport", Blocked: true, BlockedReason: "waiting on photos"}},
		})
		if _, _, err := moveCard(root, "life", "aaaaaaaa", board.Doing, now); err != nil {
			t.Fatalf("moveCard: %v", err)
		}
		c := loadBoard(t, root, "life").Lanes[board.Doing][0]
		if !c.Blocked || c.BlockedReason != "waiting on photos" {
			t.Errorf("blocked state changed: blocked=%v reason=%q", c.Blocked, c.BlockedReason)
		}
	})

	t.Run("invalid board name", func(t *testing.T) {
		root := t.TempDir()
		_, _, err := moveCard(root, "has/slash", "anything", board.Doing, now)
		if err == nil || !strings.Contains(err.Error(), "invalid board name") {
			t.Fatalf("got err=%v, want an invalid board name error", err)
		}
	})
}
