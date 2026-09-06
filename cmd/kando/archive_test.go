package main

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

func TestArchiveListArgs(t *testing.T) {
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
		{name: "filter", args: []string{"--filter", "#money"}, wantQuery: "#money"},
		{name: "json", args: []string{"--json"}, wantJSON: true},
		{name: "too many positionals", args: []string{"a", "b"}, wantErr: true},
		{name: "unknown flag", args: []string{"--nope"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, query, jsonOut, err := archiveListArgs(tc.args, io.Discard)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want an error, got name=%q query=%q json=%v", name, query, jsonOut)
				}
				return
			}
			if err != nil {
				t.Fatalf("archiveListArgs: %v", err)
			}
			if name != tc.wantName || query != tc.wantQuery || jsonOut != tc.wantJSON {
				t.Errorf("got name=%q query=%q json=%v, want name=%q query=%q json=%v", name, query, jsonOut, tc.wantName, tc.wantQuery, tc.wantJSON)
			}
		})
	}
}

// seedArchive writes root/life/board.md (empty) and archive.md with n cards
// spread across this week, last week and earlier, so ArchiveView's grouping
// has something in every bucket.
func seedArchive(t *testing.T, now time.Time) string {
	t.Helper()
	root := t.TempDir()
	st, _, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	a := &board.Archive{Cards: []*board.Card{
		{ID: "aaaaaaaa", Title: "This week", Tag: "money", DoneAt: now},
		{ID: "bbbbbbbb", Title: "Last week", Tag: "home", DoneAt: now.AddDate(0, 0, -8)},
		{ID: "cccccccc", Title: "Ages ago", DoneAt: now.AddDate(0, 0, -30)},
	}}
	if err := st.SaveArchive(a); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestArchiveList(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC) // a Sunday

	t.Run("groups and counts", func(t *testing.T) {
		root := seedArchive(t, now)
		name, groups, matched, scanned, total, err := archiveList(root, "life", "", now)
		if err != nil {
			t.Fatal(err)
		}
		if name != "life" || matched != 3 || scanned != 3 || total != 3 {
			t.Fatalf("name=%q matched=%d scanned=%d total=%d", name, matched, scanned, total)
		}
		if len(groups) == 0 {
			t.Fatal("expected at least one group")
		}
	})

	t.Run("filter narrows matched, not scanned or total", func(t *testing.T) {
		root := seedArchive(t, now)
		_, _, matched, scanned, total, err := archiveList(root, "life", "#money", now)
		if err != nil {
			t.Fatal(err)
		}
		if matched != 1 || scanned != 3 || total != 3 {
			t.Errorf("matched=%d scanned=%d total=%d", matched, scanned, total)
		}
	})

	t.Run("never rewrites archive.md", func(t *testing.T) {
		root := seedArchive(t, now)
		before := mustReadCLI(t, root+"/life/archive.md")
		if _, _, _, _, _, err := archiveList(root, "life", "", now); err != nil {
			t.Fatal(err)
		}
		if after := mustReadCLI(t, root+"/life/archive.md"); before != after {
			t.Errorf("archive list must never rewrite archive.md")
		}
	})

	t.Run("empty archive", func(t *testing.T) {
		root := t.TempDir()
		if _, _, err := store.Open(root, "life"); err != nil {
			t.Fatal(err)
		}
		name, groups, matched, scanned, total, err := archiveList(root, "life", "", now)
		if err != nil {
			t.Fatal(err)
		}
		if name != "life" || len(groups) != 0 || matched != 0 || scanned != 0 || total != 0 {
			t.Errorf("groups=%v matched=%d scanned=%d total=%d", groups, matched, scanned, total)
		}
	})

	t.Run("nonexistent board", func(t *testing.T) {
		root := t.TempDir()
		if _, _, _, _, _, err := archiveList(root, "ghost", "", now); err == nil {
			t.Fatal("want an error")
		}
	})
}

func TestFormatArchiveList(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

	t.Run("empty archive", func(t *testing.T) {
		out := formatArchiveList("/root", "life", nil, "", 0, 0, 0)
		if out != `nothing archived on "life"` {
			t.Errorf("got %q", out)
		}
	})

	t.Run("groups render with title, tag, date", func(t *testing.T) {
		groups := []board.ArchiveGroup{{
			Label: "THIS WEEK",
			Cards: []*board.Card{{Title: "Cancel gym membership", Tag: "money", DoneAt: now}},
		}}
		out := formatArchiveList("/root", "life", groups, "", 1, 1, 1)
		if !strings.Contains(out, "THIS WEEK 1") || !strings.Contains(out, "✓ Cancel gym membership") || !strings.Contains(out, "#money") {
			t.Errorf("got:\n%s", out)
		}
	})

	t.Run("filter summary line", func(t *testing.T) {
		groups := []board.ArchiveGroup{{Label: "THIS WEEK", Cards: []*board.Card{{Title: "X", DoneAt: now}}}}
		out := formatArchiveList("/root", "life", groups, "#money", 1, 4, 10)
		if !strings.Contains(out, `1 of the newest 4 match "#money"`) {
			t.Errorf("missing filter summary:\n%s", out)
		}
	})

	t.Run("cap footnote only when total exceeds scanned", func(t *testing.T) {
		groups := []board.ArchiveGroup{{Label: "THIS WEEK", Cards: []*board.Card{{Title: "X", DoneAt: now}}}}
		out := formatArchiveList("/root", "life", groups, "", 1, 50, 60)
		if !strings.Contains(out, "Older entries live in") {
			t.Errorf("missing cap footnote:\n%s", out)
		}
		out2 := formatArchiveList("/root", "life", groups, "", 1, 1, 1)
		if strings.Contains(out2, "Older entries live in") {
			t.Errorf("footnote should not appear when nothing is capped:\n%s", out2)
		}
	})
}
