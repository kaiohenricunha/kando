package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

func TestDispatch(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantVerb string
		wantRest []string
	}{
		{name: "bare", args: nil, wantVerb: "", wantRest: nil},
		{name: "bare board", args: []string{"work"}, wantVerb: "", wantRest: []string{"work"}},
		{name: "help flag", args: []string{"-h"}, wantVerb: "", wantRest: []string{"-h"}},
		{name: "web verb", args: []string{"web"}, wantVerb: "web", wantRest: []string{}},
		{name: "web verb with args", args: []string{"web", "work", "--port", "8080"}, wantVerb: "web", wantRest: []string{"work", "--port", "8080"}},
		{name: "move verb", args: []string{"move", "card", "Doing"}, wantVerb: "move", wantRest: []string{"card", "Doing"}},
		{name: "escape hatch", args: []string{"--", "move"}, wantVerb: "", wantRest: []string{"move"}},
		{name: "escape hatch bare", args: []string{"--"}, wantVerb: "", wantRest: []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verb, rest := dispatch(tc.args)
			if verb != tc.wantVerb || !equalStrings(rest, tc.wantRest) {
				t.Errorf("dispatch(%v) = %q,%v want %q,%v", tc.args, verb, rest, tc.wantVerb, tc.wantRest)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestUsageListsEveryVerb(t *testing.T) {
	for name := range verbs {
		if want := "kando " + name; !strings.Contains(usageText, want) {
			t.Errorf("usageText is missing verb %q (want to see %q)", name, want)
		}
	}
}

func TestReservedWordsAreNotVerbs(t *testing.T) {
	for _, w := range []string{"help", "-h", "--help", "-v", "--version", "version"} {
		if _, ok := verbs[w]; ok {
			t.Errorf("%q must not be a verbs table key — main's own switch handles it", w)
		}
	}
}

func TestResolveBoard(t *testing.T) {
	root := t.TempDir()
	if _, _, err := store.Open(root, "life"); err != nil {
		t.Fatal(err)
	}

	t.Run("empty defaults to life", func(t *testing.T) {
		got, err := resolveBoard(root, "")
		if err != nil || got != "life" {
			t.Fatalf("got %q, %v", got, err)
		}
	})
	t.Run("existing board", func(t *testing.T) {
		got, err := resolveBoard(root, "life")
		if err != nil || got != "life" {
			t.Fatalf("got %q, %v", got, err)
		}
	})
	t.Run("invalid name", func(t *testing.T) {
		if _, err := resolveBoard(root, "has/slash"); err == nil {
			t.Fatal("want an error")
		}
	})
	t.Run("nonexistent board creates no directory", func(t *testing.T) {
		if _, err := resolveBoard(root, "ghost"); err == nil {
			t.Fatal("want an error")
		}
		if _, statErr := os.Stat(filepath.Join(root, "ghost")); !os.IsNotExist(statErr) {
			t.Errorf("resolveBoard must not create a board directory, stat err = %v", statErr)
		}
	})
}

func TestSaveBoardMapsConflict(t *testing.T) {
	root := t.TempDir()
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Insert(board.Todo, 0, &board.Card{ID: "aaaaaaaa", Title: "mine"})
	if err := saveBoard(st, b); err != nil {
		t.Fatalf("first save: %v", err)
	}
	other := &board.Board{Name: "life"}
	other.Insert(board.Todo, 0, &board.Card{ID: "bbbbbbbb", Title: "theirs"})
	if err := os.WriteFile(st.BoardPath(), store.Marshal(other), 0o644); err != nil {
		t.Fatal(err)
	}
	b.Insert(board.Todo, 0, &board.Card{ID: "cccccccc", Title: "mine too"})
	if err := saveBoard(st, b); !errors.Is(err, errConflict) {
		t.Errorf("got %v, want errConflict", err)
	}
}

func TestWithCard(t *testing.T) {
	seed := func(t *testing.T) (root string) {
		t.Helper()
		root = t.TempDir()
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

	t.Run("op error leaves the file untouched", func(t *testing.T) {
		root := seed(t)
		before := mustReadCLI(t, filepath.Join(root, "life", "board.md"))
		_, _, err := withCard(root, "life", "aaaaaaaa", func(b *board.Board, lane board.Lane, i int, c *board.Card) (bool, error) {
			return false, errors.New("boom")
		})
		if err == nil || err.Error() != "boom" {
			t.Fatalf("got %v", err)
		}
		if after := mustReadCLI(t, filepath.Join(root, "life", "board.md")); before != after {
			t.Errorf("board.md changed despite an op error")
		}
	})

	t.Run("changed false does not save", func(t *testing.T) {
		root := seed(t)
		before := mustReadCLI(t, filepath.Join(root, "life", "board.md"))
		lane, c, err := withCard(root, "life", "aaaaaaaa", func(b *board.Board, lane board.Lane, i int, c *board.Card) (bool, error) {
			c.Tag = "mutated-in-memory-only"
			return false, nil
		})
		if err != nil || lane != board.Todo || c.ID != "aaaaaaaa" {
			t.Fatalf("lane=%v c=%v err=%v", lane, c, err)
		}
		if after := mustReadCLI(t, filepath.Join(root, "life", "board.md")); before != after {
			t.Errorf("board.md changed despite changed=false")
		}
	})

	t.Run("changed true saves", func(t *testing.T) {
		root := seed(t)
		_, _, err := withCard(root, "life", "aaaaaaaa", func(b *board.Board, lane board.Lane, i int, c *board.Card) (bool, error) {
			c.Tag = "errand"
			return true, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		got, err := store.Load(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		if got.Lanes[board.Todo][0].Tag != "errand" {
			t.Errorf("tag not saved: %+v", got.Lanes[board.Todo][0])
		}
	})

	t.Run("nonexistent card", func(t *testing.T) {
		root := seed(t)
		if _, _, err := withCard(root, "life", "nope", func(b *board.Board, lane board.Lane, i int, c *board.Card) (bool, error) {
			t.Fatal("op must not run when the card is not found")
			return false, nil
		}); err == nil {
			t.Fatal("want an error")
		}
	})

	t.Run("card still readable after a delete op", func(t *testing.T) {
		root := seed(t)
		_, c, err := withCard(root, "life", "aaaaaaaa", func(b *board.Board, lane board.Lane, i int, c *board.Card) (bool, error) {
			b.DeleteCard(lane, i)
			return true, nil
		})
		if err != nil || c.Title != "Renew passport" {
			t.Errorf("card should still be readable after delete: c=%v err=%v", c, err)
		}
	})
}

func mustReadCLI(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestParseMixed(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantPos []string
		wantErr bool
	}{
		{name: "no args", args: nil, wantPos: nil},
		{name: "positional only", args: []string{"a", "b"}, wantPos: []string{"a", "b"}},
		{name: "flag before positional", args: []string{"--x", "1", "a"}, wantPos: []string{"a"}},
		{name: "flag after positional", args: []string{"a", "--x", "1"}, wantPos: []string{"a"}},
		{name: "flag between positionals", args: []string{"a", "--x", "1", "b"}, wantPos: []string{"a", "b"}},
		{name: "dash-dash terminator keeps a flag-like positional", args: []string{"a", "--", "-1 point bug"}, wantPos: []string{"a", "-1 point bug"}},
		{name: "unknown flag", args: []string{"--nope"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			fs.String("x", "", "")
			pos, err := parseMixed(fs, tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want an error, got %v", pos)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseMixed: %v", err)
			}
			if !equalStrings(pos, tc.wantPos) {
				t.Errorf("pos = %v, want %v", pos, tc.wantPos)
			}
		})
	}
}

func TestSplitBoard(t *testing.T) {
	cases := []struct {
		name     string
		pos      []string
		n        int
		wantVals []string
		wantName string
		wantErr  bool
	}{
		{name: "too few", pos: []string{"a"}, n: 2, wantErr: true},
		{name: "exact n, no board", pos: []string{"a", "b"}, n: 2, wantVals: []string{"a", "b"}},
		{name: "n plus board", pos: []string{"a", "b", "work"}, n: 2, wantVals: []string{"a", "b"}, wantName: "work"},
		{name: "too many", pos: []string{"a", "b", "work", "extra"}, n: 2, wantErr: true},
		{name: "blank required value", pos: []string{"", "b"}, n: 2, wantErr: true},
		{name: "whitespace-only required value", pos: []string{"   ", "b"}, n: 2, wantErr: true},
		{name: "required value trimmed", pos: []string{" a ", "b"}, n: 2, wantVals: []string{"a", "b"}},
		{name: "board starting with a dash", pos: []string{"a", "b", "--oops"}, n: 2, wantErr: true},
		{name: "blank board", pos: []string{"a", "b", ""}, n: 2, wantErr: true},
		{name: "whitespace-only board", pos: []string{"a", "b", "   "}, n: 2, wantErr: true},
		{name: "required value starting with a dash is not a flag", pos: []string{"-1 point bug", "b"}, n: 2, wantVals: []string{"-1 point bug", "b"}},
		{name: "n=1 exact", pos: []string{"only"}, n: 1, wantVals: []string{"only"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vals, name, err := splitBoard("test", "a and b", tc.pos, tc.n)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want an error, got vals=%v name=%q", vals, name)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitBoard: %v", err)
			}
			if !equalStrings(vals, tc.wantVals) || name != tc.wantName {
				t.Errorf("vals=%v name=%q, want vals=%v name=%q", vals, name, tc.wantVals, tc.wantName)
			}
		})
	}
}

// TestMoveOnAnIdLessBoard pins the interplay between store.Open's own
// id-repair rewrite and the checked save every mutating verb now uses:
// Open's rewrite calls saveBoard internally (store.go), which updates the
// Store's tracked file state, so the very next SaveBoardIfUnchanged must not
// mistake that rewrite for a concurrent external edit.
func TestMoveOnAnIdLessBoard(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "life")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "board.md"), []byte("## Todo\n\n### Typed by hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	title, from, err := moveCard(root, "life", "Typed by hand", board.Doing, time.Now())
	if err != nil {
		t.Fatalf("moveCard on an id-less board: %v", err)
	}
	if title != "Typed by hand" || from != board.Todo {
		t.Errorf("title=%q from=%v", title, from)
	}
	got, err := store.Load(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Lanes[board.Doing]) != 1 {
		t.Errorf("card did not move: %+v", got.Lanes)
	}
}
