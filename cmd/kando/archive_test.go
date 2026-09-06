package main

import (
	"io"
	"os"
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

	// ArchiveView drops empty buckets, so no groups does not imply an empty
	// archive: a filter matching none of 12 archived cards lands here too,
	// and saying "nothing archived" there is simply false.
	t.Run("no groups because the filter matched nothing", func(t *testing.T) {
		out := formatArchiveList("/root", "life", nil, "#nomatch", 0, 12, 12)
		if strings.Contains(out, "nothing archived") {
			t.Errorf("a zero-match filter must not claim the archive is empty: %q", out)
		}
		if out != `0 of the newest 12 match "#nomatch"` {
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

func TestArchiveArgs(t *testing.T) {
	card, name, err := archiveArgs([]string{"Cancel gym membership", "work"})
	if err != nil || card != "Cancel gym membership" || name != "work" {
		t.Fatalf("card=%q name=%q err=%v", card, name, err)
	}
	if _, _, err := archiveArgs(nil); err == nil {
		t.Fatal("want an error for no args")
	}
	if _, _, err := archiveArgs([]string{""}); err == nil {
		t.Fatal("want an error for an empty card")
	}
}

func TestArchiveRestoreArgs(t *testing.T) {
	card, name, err := archiveRestoreArgs([]string{"Cancel gym membership", "work"})
	if err != nil || card != "Cancel gym membership" || name != "work" {
		t.Fatalf("card=%q name=%q err=%v", card, name, err)
	}
	if _, _, err := archiveRestoreArgs(nil); err == nil {
		t.Fatal("want an error for no args")
	}
}

func TestFindArchived(t *testing.T) {
	x := &board.Card{ID: "aaaaaaaa", Title: "Cancel gym membership"}
	dup1 := &board.Card{ID: "bbbbbbbb", Title: "Dup"}
	dup2 := &board.Card{ID: "cccccccc", Title: "Dup"}
	collide := &board.Card{ID: "Dup", Title: "id equals another card's title"}
	a := &board.Archive{Cards: []*board.Card{x, dup1, dup2, collide}}

	t.Run("exact id", func(t *testing.T) {
		_, c, err := findArchived(a, "aaaaaaaa")
		if err != nil || c != x {
			t.Fatalf("c=%v err=%v", c, err)
		}
	})

	t.Run("unique case-insensitive title", func(t *testing.T) {
		_, c, err := findArchived(a, "cancel GYM membership")
		if err != nil || c != x {
			t.Fatalf("c=%v err=%v", c, err)
		}
	})

	t.Run("zero matches", func(t *testing.T) {
		_, c, err := findArchived(a, "nope")
		if err == nil || c != nil {
			t.Fatalf("c=%v err=%v, want a miss", c, err)
		}
	})

	t.Run("ambiguous title lists both ids", func(t *testing.T) {
		_, c, err := findArchived(a, "dup")
		if err == nil || c != nil {
			t.Fatalf("c=%v err=%v, want an ambiguity error", c, err)
		}
		if !strings.Contains(err.Error(), dup1.ID) || !strings.Contains(err.Error(), dup2.ID) {
			t.Errorf("error %q should list both ids", err.Error())
		}
	})

	t.Run("id takes priority over a colliding title", func(t *testing.T) {
		_, c, err := findArchived(a, "Dup")
		if err != nil || c != collide {
			t.Fatalf("c=%v err=%v, want the id match (collide)", c, err)
		}
	})

	t.Run("empty archive", func(t *testing.T) {
		_, c, err := findArchived(&board.Archive{}, "anything")
		if err == nil || c != nil {
			t.Fatalf("c=%v err=%v, want a miss", c, err)
		}
	})
}

func seedArchiveCardBoard(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes[board.Done] = []*board.Card{{ID: "aaaaaaaa", Title: "Cancel gym membership", Tag: "money", DoneAt: now.AddDate(0, 0, -1)}}
	b.Lanes[board.Doing] = []*board.Card{{ID: "bbbbbbbb", Title: "Not done yet"}}
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestArchiveCard(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

	t.Run("by id", func(t *testing.T) {
		root := seedArchiveCardBoard(t)
		title, doneAt, err := archiveCard(root, "life", "aaaaaaaa", now)
		if err != nil || title != "Cancel gym membership" {
			t.Fatalf("title=%q err=%v", title, err)
		}
		if !doneAt.Equal(now.AddDate(0, 0, -1)) {
			t.Errorf("doneAt = %v, should be the fixture's, not restamped to now", doneAt)
		}
		b, err := store.Load(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		if len(b.Lanes[board.Done]) != 0 {
			t.Errorf("card should have left Done: %+v", b.Lanes[board.Done])
		}
		a, err := store.LoadArchive(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		if len(a.Cards) != 1 || a.Cards[0].ID != "aaaaaaaa" {
			t.Errorf("archive: %+v", a.Cards)
		}
	})

	t.Run("by title, case-insensitive", func(t *testing.T) {
		root := seedArchiveCardBoard(t)
		_, _, err := archiveCard(root, "life", "cancel GYM membership", now)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("not in Done leaves both files unchanged", func(t *testing.T) {
		root := seedArchiveCardBoard(t)
		beforeBoard := readBoardFile(t, root, "life")
		_, _, err := archiveCard(root, "life", "bbbbbbbb", now)
		if err == nil || !strings.Contains(err.Error(), "Doing") {
			t.Fatalf("err=%v, want a not-in-Done error naming the lane", err)
		}
		if after := readBoardFile(t, root, "life"); beforeBoard != after {
			t.Errorf("board.md should be unchanged")
		}
		// The rejection happens before LoadArchive is even called, so
		// archive.md — which nothing has created yet in this fixture —
		// must still not exist.
		if _, err := os.Stat(root + "/life/archive.md"); !os.IsNotExist(err) {
			t.Errorf("archive.md should not have been created, stat err = %v", err)
		}
	})

	t.Run("zero DoneAt is stamped to now", func(t *testing.T) {
		root := t.TempDir()
		if _, _, err := store.Open(root, "life"); err != nil {
			t.Fatal(err)
		}
		dir := root + "/life"
		if err := writeCLIFile(dir+"/board.md", "## Done\n\n### Done by hand\n"); err != nil {
			t.Fatal(err)
		}
		title, doneAt, err := archiveCard(root, "life", "Done by hand", now)
		if err != nil || title != "Done by hand" || !doneAt.Equal(now) {
			t.Fatalf("title=%q doneAt=%v err=%v", title, doneAt, err)
		}
	})

	t.Run("already archived is refused, both files unchanged", func(t *testing.T) {
		root := seedArchiveCardBoard(t)
		st, _, err := store.Open(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		dup := &board.Card{ID: "aaaaaaaa", Title: "duplicate id"}
		if err := st.SaveArchive(&board.Archive{Cards: []*board.Card{dup}}); err != nil {
			t.Fatal(err)
		}
		beforeBoard := readBoardFile(t, root, "life")
		beforeArchive := mustReadCLI(t, root+"/life/archive.md")
		_, _, err = archiveCard(root, "life", "aaaaaaaa", now)
		if err == nil || !strings.Contains(err.Error(), "already archived") {
			t.Fatalf("err=%v, want an already-archived error", err)
		}
		if after := readBoardFile(t, root, "life"); beforeBoard != after {
			t.Errorf("board.md should be unchanged")
		}
		if after := mustReadCLI(t, root+"/life/archive.md"); beforeArchive != after {
			t.Errorf("archive.md should be unchanged")
		}
	})

	t.Run("nonexistent board creates no directory", func(t *testing.T) {
		root := t.TempDir()
		if _, _, err := archiveCard(root, "ghost", "x", now); err == nil {
			t.Fatal("want an error")
		}
		if _, err := os.Stat(root + "/ghost"); !os.IsNotExist(err) {
			t.Errorf("board directory should not have been created")
		}
	})

	t.Run("ambiguous title leaves both files unchanged", func(t *testing.T) {
		root := t.TempDir()
		st, b, err := store.Open(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		b.Lanes[board.Done] = []*board.Card{{ID: "aaaaaaaa", Title: "Dup", DoneAt: now}, {ID: "bbbbbbbb", Title: "Dup", DoneAt: now}}
		if err := st.SaveBoard(b); err != nil {
			t.Fatal(err)
		}
		before := readBoardFile(t, root, "life")
		if _, _, err := archiveCard(root, "life", "Dup", now); err == nil {
			t.Fatal("want an ambiguity error")
		}
		if after := readBoardFile(t, root, "life"); before != after {
			t.Errorf("board.md should be unchanged")
		}
	})

	t.Run("newest-first after two archives", func(t *testing.T) {
		root := t.TempDir()
		st, b, err := store.Open(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		b.Lanes[board.Done] = []*board.Card{
			{ID: "older", Title: "Older", DoneAt: now.AddDate(0, 0, -5)},
			{ID: "newer", Title: "Newer", DoneAt: now.AddDate(0, 0, -1)},
		}
		if err := st.SaveBoard(b); err != nil {
			t.Fatal(err)
		}
		if _, _, err := archiveCard(root, "life", "older", now); err != nil {
			t.Fatal(err)
		}
		if _, _, err := archiveCard(root, "life", "newer", now); err != nil {
			t.Fatal(err)
		}
		a, err := store.LoadArchive(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		if len(a.Cards) != 2 || a.Cards[0].ID != "newer" || a.Cards[1].ID != "older" {
			t.Errorf("archive order: %v", []string{a.Cards[0].ID, a.Cards[1].ID})
		}
	})
}

func writeCLIFile(path, contents string) error {
	return os.WriteFile(path, []byte(contents), 0o644)
}

func TestArchiveRestore(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

	seedRestore := func(t *testing.T) string {
		t.Helper()
		root := t.TempDir()
		st, _, err := store.Open(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		a := &board.Archive{Cards: []*board.Card{{ID: "aaaaaaaa", Title: "Cancel gym membership", DoneAt: now.AddDate(0, 0, -3)}}}
		if err := st.SaveArchive(a); err != nil {
			t.Fatal(err)
		}
		return root
	}

	t.Run("lands at the top of Doing", func(t *testing.T) {
		root := seedRestore(t)
		title, err := archiveRestore(root, "life", "aaaaaaaa", now)
		if err != nil || title != "Cancel gym membership" {
			t.Fatalf("title=%q err=%v", title, err)
		}
		b, err := store.Load(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		c := b.Lanes[board.Doing][0]
		if c.ID != "aaaaaaaa" || !c.DoneAt.IsZero() || !c.MovedAt.Equal(now) {
			t.Errorf("restored card: %+v", c)
		}
		a, err := store.LoadArchive(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		if len(a.Cards) != 0 {
			t.Errorf("archive should be empty, has %d", len(a.Cards))
		}
	})

	t.Run("duplicate id on the board is refused, both files unchanged", func(t *testing.T) {
		root := seedRestore(t)
		st, b, err := store.Open(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		b.Lanes[board.Todo] = []*board.Card{{ID: "aaaaaaaa", Title: "already here"}}
		if err := st.SaveBoard(b); err != nil {
			t.Fatal(err)
		}
		beforeBoard := readBoardFile(t, root, "life")
		beforeArchive := mustReadCLI(t, root+"/life/archive.md")
		_, err = archiveRestore(root, "life", "aaaaaaaa", now)
		if err == nil || !strings.Contains(err.Error(), "already on the board") {
			t.Fatalf("err=%v, want an already-on-the-board error", err)
		}
		if after := readBoardFile(t, root, "life"); beforeBoard != after {
			t.Errorf("board.md should be unchanged")
		}
		if after := mustReadCLI(t, root+"/life/archive.md"); beforeArchive != after {
			t.Errorf("archive.md should be unchanged")
		}
	})

	t.Run("empty archive", func(t *testing.T) {
		root := t.TempDir()
		if _, _, err := store.Open(root, "life"); err != nil {
			t.Fatal(err)
		}
		_, err := archiveRestore(root, "life", "anything", now)
		if err == nil || !strings.Contains(err.Error(), "nothing archived") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("nonexistent board", func(t *testing.T) {
		root := t.TempDir()
		if _, err := archiveRestore(root, "ghost", "x", now); err == nil {
			t.Fatal("want an error")
		}
	})

	t.Run("ambiguous title", func(t *testing.T) {
		root := t.TempDir()
		st, _, err := store.Open(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		a := &board.Archive{Cards: []*board.Card{{ID: "aaaaaaaa", Title: "Dup"}, {ID: "bbbbbbbb", Title: "Dup"}}}
		if err := st.SaveArchive(a); err != nil {
			t.Fatal(err)
		}
		if _, err := archiveRestore(root, "life", "Dup", now); err == nil {
			t.Fatal("want an ambiguity error")
		}
	})
}

func TestArchiveReservedWordsAreReachableByID(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	root := t.TempDir()
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes[board.Done] = []*board.Card{{ID: "aaaaaaaa", Title: "list", DoneAt: now}}
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	// A card titled "list" cannot be reached by title through runArchive's
	// dispatch (it would be read as the list subcommand), but archiveCard
	// itself has no notion of reserved words — only the id path is exercised
	// through the real dispatcher, so this pins that the core function does
	// not special-case "list"/"restore" as titles either.
	title, _, err := archiveCard(root, "life", "aaaaaaaa", now)
	if err != nil || title != "list" {
		t.Fatalf("title=%q err=%v", title, err)
	}
}
