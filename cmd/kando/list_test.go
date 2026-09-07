package main

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

func TestListArgs(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantName  string
		wantQuery string
		wantJSON  bool
		wantErr   bool
	}{
		{name: "no args"},
		{name: "board only", args: []string{"work"}, wantName: "work"},
		{name: "filter", args: []string{"--filter", "#errand age>7d"}, wantQuery: "#errand age>7d"},
		{name: "filter and board", args: []string{"work", "--filter", "#errand"}, wantName: "work", wantQuery: "#errand"},
		{name: "json", args: []string{"--json"}, wantJSON: true},
		{name: "too many positionals", args: []string{"a", "b"}, wantErr: true},
		{name: "unknown flag", args: []string{"--nope"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, query, jsonOut, err := listArgs(tc.args, io.Discard)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want an error, got name=%q query=%q json=%v", name, query, jsonOut)
				}
				return
			}
			if err != nil {
				t.Fatalf("listArgs: %v", err)
			}
			if name != tc.wantName || query != tc.wantQuery || jsonOut != tc.wantJSON {
				t.Errorf("got name=%q query=%q json=%v, want name=%q query=%q json=%v", name, query, jsonOut, tc.wantName, tc.wantQuery, tc.wantJSON)
			}
		})
	}
}

func seedListBoard(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes[board.Todo] = []*board.Card{{ID: "aaaaaaaa", Title: "Renew passport", Tag: "errand"}}
	b.Lanes[board.Doing] = []*board.Card{{ID: "bbbbbbbb", Title: "Fix bike", Tag: "home", Blocked: true, BlockedReason: "waiting on part"}}
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestListCards(t *testing.T) {
	root := seedListBoard(t)
	now := time.Now()

	t.Run("empty filter returns everything", func(t *testing.T) {
		b, lanes, matched, total, err := listCards(root, "life", "", now)
		if err != nil {
			t.Fatal(err)
		}
		if b.Name != "life" || matched != 2 || total != 2 {
			t.Fatalf("name=%q matched=%d total=%d", b.Name, matched, total)
		}
		if len(lanes[board.Todo]) != 1 || len(lanes[board.Doing]) != 1 {
			t.Errorf("lanes: %v", lanes)
		}
	})

	t.Run("filter narrows the result", func(t *testing.T) {
		_, lanes, matched, total, err := listCards(root, "life", "#home", now)
		if err != nil {
			t.Fatal(err)
		}
		if matched != 1 || total != 2 || len(lanes[board.Doing]) != 1 {
			t.Errorf("matched=%d total=%d doing=%v", matched, total, lanes[board.Doing])
		}
	})

	t.Run("does not rewrite the board", func(t *testing.T) {
		before := readBoardFile(t, root, "life")
		if _, _, _, _, err := listCards(root, "life", "", now); err != nil {
			t.Fatal(err)
		}
		if after := readBoardFile(t, root, "life"); before != after {
			t.Errorf("list must never rewrite board.md")
		}
	})

	t.Run("nonexistent board", func(t *testing.T) {
		if _, _, _, _, err := listCards(root, "ghost", "", now); err == nil {
			t.Fatal("want an error")
		}
	})
}

func TestFormatList(t *testing.T) {
	now := time.Now()
	var lanes [4][]*board.Card
	lanes[board.Todo] = []*board.Card{{ID: "aaaaaaaa", Title: "Renew passport", Tag: "errand"}}
	lanes[board.Doing] = []*board.Card{{ID: "bbbbbbbb", Title: "Fix bike", Blocked: true, BlockedReason: "waiting"}}

	t.Run("all four lanes always present", func(t *testing.T) {
		out := formatList(lanes, "", 2, 2, now)
		for _, l := range board.Lanes {
			if !strings.Contains(out, l.String()+" (") {
				t.Errorf("missing lane header for %v in:\n%s", l, out)
			}
		}
	})

	t.Run("row content", func(t *testing.T) {
		out := formatList(lanes, "", 2, 2, now)
		if !strings.Contains(out, "aaaaaaaa") || !strings.Contains(out, "Renew passport") || !strings.Contains(out, "#errand") {
			t.Errorf("todo row missing fields:\n%s", out)
		}
		if !strings.Contains(out, "blocked: waiting") {
			t.Errorf("doing row missing blocked reason:\n%s", out)
		}
	})

	t.Run("a card with no created date omits age instead of reporting millennia", func(t *testing.T) {
		c := &board.Card{ID: "aaaabbbb", Title: "Fix bike"} // hand-written: no created:, no done:
		if got, want := listRow(c, now), "aaaabbbb  Fix bike"; got != want {
			t.Errorf("listRow = %q, want %q (age must be omitted, not a multi-thousand-day figure)", got, want)
		}
	})

	t.Run("filter summary line only when filtered", func(t *testing.T) {
		if strings.Contains(formatList(lanes, "", 2, 2, now), "match") {
			t.Errorf("no summary line expected without a filter")
		}
		out := formatList(lanes, "#errand", 1, 2, now)
		if !strings.Contains(out, `1 of 2 cards match "#errand"`) {
			t.Errorf("missing filter summary:\n%s", out)
		}
	})
}
