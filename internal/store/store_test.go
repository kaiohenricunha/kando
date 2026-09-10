package store

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
)

func TestOpenCreatesBoardFile(t *testing.T) {
	root := t.TempDir()
	s, b, err := Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	if b.Name != "life" || b.Count() != 0 {
		t.Errorf("board = %+v", b)
	}
	data, err := os.ReadFile(filepath.Join(root, "life", "board.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, Marshal(&board.Board{})) {
		t.Errorf("new board file not canonical:\n%s", data)
	}
	if s.BoardPath() != filepath.Join(root, "life", "board.md") || s.ArchivePath() != filepath.Join(root, "life", "archive.md") {
		t.Errorf("paths: %s %s", s.BoardPath(), s.ArchivePath())
	}
}

func TestOpenRewritesMissingIDs(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "life")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "board.md"), []byte("## Todo\n\n### No id\n"), 0o644)
	_, b, err := Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "board.md"))
	if !strings.Contains(string(data), "id: "+b.Lanes[board.Todo][0].ID+"\n") {
		t.Errorf("file should have been rewritten with the id:\n%s", data)
	}
}

func TestSaveIsAtomicAndCanonical(t *testing.T) {
	root := t.TempDir()
	s, b, err := Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes[board.Todo] = []*board.Card{{ID: "abcdefgh", Title: "Hello", CreatedAt: ts(2026, 9, 1), MovedAt: ts(2026, 9, 1)}}
	if err := s.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "life"))
	for _, e := range entries {
		if e.Name() != "board.md" {
			t.Errorf("unexpected file left behind: %s", e.Name())
		}
	}
	data, _ := os.ReadFile(s.BoardPath())
	if !bytes.Equal(data, Marshal(b)) {
		t.Errorf("saved bytes differ from Marshal")
	}
}

func TestCheckReload(t *testing.T) {
	root := t.TempDir()
	s, b, err := Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes[board.Todo] = []*board.Card{{ID: "abcdefgh", Title: "Hello", CreatedAt: ts(2026, 9, 1), MovedAt: ts(2026, 9, 1)}}
	if err := s.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	// Own write: nothing to reload.
	r, err := s.CheckReload()
	if err != nil || r.Board != nil || r.Archive != nil {
		t.Errorf("after own save: %+v %v", r, err)
	}
	// Identical bytes rewritten externally: nothing to reload.
	data, _ := os.ReadFile(s.BoardPath())
	os.WriteFile(s.BoardPath(), data, 0o644)
	r, _ = s.CheckReload()
	if r.Board != nil {
		t.Errorf("identical external write should not reload")
	}
	// External edit: reload.
	edited := bytes.Replace(data, []byte("### Hello"), []byte("### Hello edited"), 1)
	os.WriteFile(s.BoardPath(), edited, 0o644)
	r, err = s.CheckReload()
	if err != nil || r.Board == nil || r.Board.Lanes[board.Todo][0].Title != "Hello edited" {
		t.Fatalf("external edit not reloaded: %+v %v", r, err)
	}
	// Same content again: nothing.
	r, _ = s.CheckReload()
	if r.Board != nil {
		t.Errorf("second check should be quiet")
	}
	// Partial write: ignored, then a valid write is picked up.
	os.WriteFile(s.BoardPath(), []byte("## Todo\n\n### A\ncreated: garbage\n"), 0o644)
	r, err = s.CheckReload()
	if r.Board != nil {
		t.Errorf("invalid content should not reload: %+v %v", r, err)
	}
	os.WriteFile(s.BoardPath(), data, 0o644)
	r, _ = s.CheckReload()
	if r.Board == nil || r.Board.Lanes[board.Todo][0].Title != "Hello" {
		t.Errorf("valid content after invalid should reload: %+v", r)
	}
	// Archive appears externally.
	os.WriteFile(s.ArchivePath(), readSample(t, "sample_archive.md"), 0o644)
	r, err = s.CheckReload()
	if err != nil || r.Archive == nil || len(r.Archive.Cards) != 10 {
		t.Errorf("archive reload: %+v %v", r, err)
	}
}

func TestArchiveLoadSave(t *testing.T) {
	root := t.TempDir()
	s, _, err := Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.LoadArchive()
	if err != nil || len(a.Cards) != 0 {
		t.Fatalf("missing archive should load empty: %v %v", a, err)
	}
	os.WriteFile(s.ArchivePath(), readSample(t, "sample_archive.md"), 0o644)
	a, err = s.LoadArchive()
	if err != nil || len(a.Cards) != 10 || a.Cards[0].Title != "Cancel gym membership" {
		t.Fatalf("archive: %v %v", a, err)
	}
	a.Remove(0)
	if err := s.SaveArchive(a); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(s.ArchivePath())
	if bytes.Contains(data, []byte("Cancel gym")) || !bytes.HasPrefix(data, []byte("## 2026-W36\n\n### Return library books\n")) {
		t.Errorf("archive after remove:\n%s", data)
	}
	r, _ := s.CheckReload()
	if r.Archive != nil {
		t.Errorf("own archive save should not reload")
	}
}

func TestDisplayPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	s := &Store{dir: filepath.Join(home, ".kando", "life")}
	if got := s.ArchiveDisplayPath(); got != "~/.kando/life/archive.md" {
		t.Errorf("display path = %q", got)
	}
	s = &Store{dir: "/srv/kando/work"}
	if got := s.ArchiveDisplayPath(); got != "/srv/kando/work/archive.md" {
		t.Errorf("display path = %q", got)
	}
}

func TestListBoardsEmpty(t *testing.T) {
	root := t.TempDir()
	got, err := ListBoards(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("ListBoards on an empty root = %v, want none", got)
	}
}

func TestListBoardsMissingRoot(t *testing.T) {
	got, err := ListBoards(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("ListBoards on a missing root = %v, want none", got)
	}
}

func TestListBoardsFindsExisting(t *testing.T) {
	root := t.TempDir()
	if _, _, err := Open(root, "work"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Open(root, "life"); err != nil {
		t.Fatal(err)
	}
	got, err := ListBoards(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"life", "work"} // sorted, deterministic for a stable picker/list UI
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListBoards = %v, want %v", got, want)
	}
}

func TestListBoardsIgnoresNonBoardDirs(t *testing.T) {
	root := t.TempDir()
	if _, _, err := Open(root, "life"); err != nil {
		t.Fatal(err)
	}
	// A directory with no board.md is not a board.
	if err := os.MkdirAll(filepath.Join(root, "not-a-board"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A stray file at the root is not a board either.
	if err := os.WriteFile(filepath.Join(root, "README.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ListBoards(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"life"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ListBoards = %v, want %v", got, want)
	}
}

func TestValidBoardName(t *testing.T) {
	// Emoji and non-Latin scripts stay valid: the predicate targets the bidi
	// controls, not everything non-ASCII.
	valid := []string{"life", "work-2024", "a", "My Board", "日本", "\U0001F468\u200d\U0001F469", "עברית"}
	invalid := []string{"", ".", "..", ".hidden", "a/b", "../evil", "a\\b", "/etc", "a\nb", "tab\there", strings.Repeat("x", 65), "bad\xffutf8", "q#1", "a?b", "50%", "a&b", "a+b", "a;b",
		// A name is a stored value that every surface renders, so it is held
		// to board.UnsafeRune rather than unicode.IsControl. A bidi override
		// in a directory name would display a board as a name it does not
		// have. An existing directory named this way stops being listed:
		// ListBoards skips it, which is the intended outcome.
		"life\u202egnp.exe", "a\u200fb", "\u2066spoof\u2069", "c1\u009bhere",
		// Zl/Zp: a directory name is a single-line value, and the TUI header
		// renders it into a row of an exact cell count.
		"life\u2028work", "a\u2029b"}
	for _, n := range valid {
		if !ValidBoardName(n) {
			t.Errorf("ValidBoardName(%q) = false, want true", n)
		}
	}
	for _, n := range invalid {
		if ValidBoardName(n) {
			t.Errorf("ValidBoardName(%q) = true, want false", n)
		}
	}
}

func TestOpenRejectsInvalidName(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"../evil", "..", ".", "", "a/b", ".hidden"} {
		if _, _, err := Open(root, n); err == nil {
			t.Errorf("Open(root, %q) should have been rejected", n)
		}
	}
	// Nothing should have been created outside root.
	entries, _ := os.ReadDir(filepath.Dir(root))
	for _, e := range entries {
		if e.Name() == "evil" {
			t.Fatal("traversal name escaped root")
		}
	}
}

func TestListBoardsFollowsSymlinkedBoardDir(t *testing.T) {
	root := t.TempDir()
	if _, _, err := Open(root, "real"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	got, err := ListBoards(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"linked", "real"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListBoards = %v, want %v (a symlinked board directory must list the same as a real one)", got, want)
	}
}

func TestListBoardsIgnoresInvalidNames(t *testing.T) {
	root := t.TempDir()
	if _, _, err := Open(root, "life"); err != nil {
		t.Fatal(err)
	}
	// A dot-directory with a board.md would be listed but never openable.
	os.MkdirAll(filepath.Join(root, ".trash"), 0o755)
	os.WriteFile(filepath.Join(root, ".trash", "board.md"), Marshal(&board.Board{}), 0o644)
	got, err := ListBoards(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"life"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ListBoards = %v, want %v (the listed set must equal the openable set)", got, want)
	}
}

func TestExists(t *testing.T) {
	root := t.TempDir()
	if _, _, err := Open(root, "life"); err != nil {
		t.Fatal(err)
	}
	if !Exists(root, "life") || Exists(root, "nope") || Exists(root, ".hidden") || Exists(root, "../life") {
		t.Errorf("Exists: life=%v nope=%v hidden=%v traversal=%v", Exists(root, "life"), Exists(root, "nope"), Exists(root, ".hidden"), Exists(root, "../life"))
	}
	if _, err := os.Stat(filepath.Join(root, "nope")); err == nil {
		t.Errorf("Exists must not create anything")
	}
}

func TestLoadIsReadOnly(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "life")
	os.MkdirAll(dir, 0o755)
	idless := []byte("## Todo\n\n### No id yet\n")
	os.WriteFile(filepath.Join(dir, "board.md"), idless, 0o644)
	b, err := Load(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	if b.Name != "life" || b.Count() != 1 || len(b.Lanes[board.Todo][0].ID) != 8 {
		t.Errorf("Load: %+v", b)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "board.md")); !bytes.Equal(got, idless) {
		t.Errorf("Load must not rewrite board.md:\n%s", got)
	}
	if _, err := Load(root, "nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing board should be fs.ErrNotExist, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "nope")); err == nil {
		t.Errorf("Load must not create a directory")
	}
	if _, err := Load(root, "../evil"); err == nil {
		t.Errorf("invalid name should error")
	}
}

func TestLoadArchiveIsReadOnly(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "life")
	os.MkdirAll(dir, 0o755)
	if a, err := LoadArchive(root, "life"); err != nil || len(a.Cards) != 0 {
		t.Fatalf("missing archive should be empty: %v %v", a, err)
	}
	idless := []byte("## 2026-W36\n\n### Done by hand\ndone: 2026-09-02\n")
	os.WriteFile(filepath.Join(dir, "archive.md"), idless, 0o644)
	a, err := LoadArchive(root, "life")
	if err != nil || len(a.Cards) != 1 || len(a.Cards[0].ID) != 8 {
		t.Fatalf("LoadArchive: %+v %v", a, err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "archive.md")); !bytes.Equal(got, idless) {
		t.Errorf("LoadArchive must not rewrite archive.md:\n%s", got)
	}
	if _, err := LoadArchive(root, "../evil"); err == nil {
		t.Errorf("invalid name should error")
	}
}

func TestSaveBoardIfUnchangedRefusesAMidAirCollision(t *testing.T) {
	root := t.TempDir()
	st, b, err := Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Insert(board.Todo, 0, &board.Card{ID: "aaaaaaaa", Title: "mine"})
	if err := st.SaveBoardIfUnchanged(b); err != nil {
		t.Fatalf("an unchanged file should save: %v", err)
	}
	// Somebody else (the TUI, an editor) rewrites board.md.
	other := &board.Board{Name: "life"}
	other.Insert(board.Todo, 0, &board.Card{ID: "bbbbbbbb", Title: "theirs"})
	if err := os.WriteFile(st.BoardPath(), Marshal(other), 0o644); err != nil {
		t.Fatal(err)
	}
	b.Insert(board.Todo, 0, &board.Card{ID: "cccccccc", Title: "mine too"})
	if err := st.SaveBoardIfUnchanged(b); !errors.Is(err, ErrConflict) {
		t.Errorf("a changed file should be ErrConflict, got %v", err)
	}
	got, _, err := Parse(mustRead(t, st.BoardPath()))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Lanes[board.Todo]) != 1 || got.Lanes[board.Todo][0].Title != "theirs" {
		t.Errorf("the refused save overwrote the other writer: %v", titlesOf(got.Lanes[board.Todo]))
	}
	if err := st.SaveBoard(b); err != nil { // the unchecked writer still wins on purpose
		t.Fatal(err)
	}
}

func TestSaveRestoreWritesTheBoardFirst(t *testing.T) {
	root := t.TempDir()
	st, b, err := Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	a := &board.Archive{Cards: []*board.Card{{ID: "aaaaaaaa", Title: "was done"}}}
	b.Restore(a, 0, time.Now())
	if err := st.SaveRestore(b, a); err != nil {
		t.Fatal(err)
	}
	got, _, _ := Parse(mustRead(t, st.BoardPath()))
	left, _ := LoadArchive(root, "life")
	if len(got.Lanes[board.Doing]) != 1 || len(left.Cards) != 0 {
		t.Errorf("after restore: board=%v archive=%d", titlesOf(got.Lanes[board.Doing]), len(left.Cards))
	}
}

func TestSaveArchivalWritesTheArchiveFirst(t *testing.T) {
	root := t.TempDir()
	st, b, err := Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	b.Insert(board.Done, 0, &board.Card{ID: "aaaaaaaa", Title: "was done", DoneAt: now})
	a := &board.Archive{}
	b.ArchiveDone(a, 0, now)
	if err := st.SaveArchival(b, a); err != nil {
		t.Fatal(err)
	}
	got, _, _ := Parse(mustRead(t, st.BoardPath()))
	left, _ := LoadArchive(root, "life")
	if len(got.Lanes[board.Done]) != 0 || len(left.Cards) != 1 || left.Cards[0].ID != "aaaaaaaa" {
		t.Errorf("after archiving: done=%v archive=%v", titlesOf(got.Lanes[board.Done]), titlesOf(left.Cards))
	}
	data := string(mustRead(t, st.ArchivePath()))
	if !strings.Contains(data, "## "+board.ISOWeekKey(now)) || !strings.Contains(data, "id: aaaaaaaa") {
		t.Errorf("archive.md should file the card under its ISO week with its id:\n%s", data)
	}
}

// The write order is what makes a half failure recoverable: archive.md goes
// first, so if board.md then cannot be written the card is in both files (a
// duplicate both guards refuse) rather than in neither. A directory cannot be
// renamed over, which fails the second write for root and non-root alike.
func TestSaveArchivalHalfFailureDuplicatesRatherThanLoses(t *testing.T) {
	root := t.TempDir()
	st, b, err := Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	b.Insert(board.Done, 0, &board.Card{ID: "aaaaaaaa", Title: "was done", DoneAt: now})
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	a := &board.Archive{}
	b.ArchiveDone(a, 0, now)
	if err := os.Remove(st.BoardPath()); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(st.BoardPath(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveArchival(b, a); err == nil {
		t.Fatal("the board write should have failed")
	}
	left, err := LoadArchive(root, "life")
	if err != nil || len(left.Cards) != 1 || left.Cards[0].ID != "aaaaaaaa" {
		t.Errorf("archive.md must already hold the card when the board write fails: %v %v", titlesOf(left.Cards), err)
	}
}

func TestSaveArchivalIfUnchangedRefusesAStaleBoard(t *testing.T) {
	root := t.TempDir()
	st, b, err := Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	b.Insert(board.Done, 0, &board.Card{ID: "aaaaaaaa", Title: "mine", DoneAt: now})
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	a, err := st.LoadArchive()
	if err != nil {
		t.Fatal(err)
	}
	// Somebody else (the TUI, an editor) rewrites board.md in between.
	other := &board.Board{Name: "life"}
	other.Insert(board.Todo, 0, &board.Card{ID: "bbbbbbbb", Title: "theirs"})
	if err := os.WriteFile(st.BoardPath(), Marshal(other), 0o644); err != nil {
		t.Fatal(err)
	}
	before := mustRead(t, st.BoardPath())
	b.ArchiveDone(a, 0, now)
	if err := st.SaveArchivalIfUnchanged(b, a); !errors.Is(err, ErrConflict) {
		t.Errorf("a changed board.md should be ErrConflict, got %v", err)
	}
	if !bytes.Equal(mustRead(t, st.BoardPath()), before) {
		t.Errorf("a refused save must not touch board.md")
	}
	if _, err := os.Stat(st.ArchivePath()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a refused save must not create archive.md: %v", err)
	}
}

func TestSaveArchivalIfUnchangedRefusesAStaleArchive(t *testing.T) {
	root := t.TempDir()
	st, b, err := Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	b.Insert(board.Done, 0, &board.Card{ID: "aaaaaaaa", Title: "mine", DoneAt: now})
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveArchive(&board.Archive{Cards: []*board.Card{{ID: "cccccccc", Title: "old", DoneAt: now.AddDate(0, 0, -7)}}}); err != nil {
		t.Fatal(err)
	}
	a, err := st.LoadArchive()
	if err != nil {
		t.Fatal(err)
	}
	// Somebody else rewrites archive.md in between.
	theirs := &board.Archive{Cards: []*board.Card{{ID: "dddddddd", Title: "theirs", DoneAt: now}}}
	if err := os.WriteFile(st.ArchivePath(), MarshalArchive(theirs), 0o644); err != nil {
		t.Fatal(err)
	}
	beforeBoard, beforeArchive := mustRead(t, st.BoardPath()), mustRead(t, st.ArchivePath())
	b.ArchiveDone(a, 0, now)
	if err := st.SaveArchivalIfUnchanged(b, a); !errors.Is(err, ErrConflict) {
		t.Errorf("a changed archive.md should be ErrConflict, got %v", err)
	}
	if !bytes.Equal(mustRead(t, st.BoardPath()), beforeBoard) || !bytes.Equal(mustRead(t, st.ArchivePath()), beforeArchive) {
		t.Errorf("a refused save must touch neither file")
	}
}

// Open rewrites an id-less board.md and LoadArchive an id-less archive.md,
// each recording what it wrote — so the checked save that follows must not
// mistake our own rewrites for somebody else's edit, and the archived card
// must be written with an id.
func TestSaveArchivalPersistsHandWrittenArchiveIds(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "life")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "board.md"), []byte("## Done\n\n### Finished by hand\ndone: 2026-09-02\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "archive.md"), []byte("## 2026-W35\n\n### Older, by hand\ndone: 2026-08-26\n"), 0o644)
	st, b, err := Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	a, err := st.LoadArchive()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	if c := b.ArchiveDone(a, 0, now); c == nil {
		t.Fatal("nothing archived")
	}
	if err := st.SaveArchivalIfUnchanged(b, a); err != nil {
		t.Fatalf("our own rewrites must not read as a conflict: %v", err)
	}
	data := string(mustRead(t, st.ArchivePath()))
	if n := strings.Count(data, "\nid: "); n != 2 {
		t.Errorf("every archived card should carry an id, found %d:\n%s", n, data)
	}
	got, _, _ := Parse(mustRead(t, st.BoardPath()))
	if len(got.Lanes[board.Done]) != 0 {
		t.Errorf("the card should have left Done: %v", titlesOf(got.Lanes[board.Done]))
	}
}

func TestStructuralNotesSurviveARoundTrip(t *testing.T) {
	notes := "## Nope\n### Ghost\n- [ ] not an item\n#### deep\nplain"
	b := &board.Board{Name: "life"}
	b.Insert(board.Todo, 0, &board.Card{ID: "aaaaaaaa", Title: "notes", Notes: notes})
	data := Marshal(b)
	got, _, err := Parse(data)
	if err != nil {
		t.Fatalf("a board with structural note lines must still parse: %v\n%s", err, data)
	}
	if n := got.Count(); n != 1 {
		t.Fatalf("note lines became %d cards:\n%s", n, data)
	}
	c := got.Lanes[board.Todo][0]
	if c.Notes != notes {
		t.Errorf("notes round trip:\n got %q\nwant %q", c.Notes, notes)
	}
	if len(c.Checklist) != 0 {
		t.Errorf("a note line became a checklist item: %+v", c.Checklist)
	}
	if !bytes.Equal(Marshal(got), data) {
		t.Errorf("second marshal differs:\n%s\n%s", data, Marshal(got))
	}
}

func TestIdlessCardsGetTheSameIdOnEveryRead(t *testing.T) {
	data := []byte("## Todo\n\n### One\n\n### Two\n\n### One\n")
	first, rewrite, err := Parse(data)
	if err != nil || !rewrite {
		t.Fatalf("parse: %v rewrite=%v", err, rewrite)
	}
	second, _, _ := Parse(data)
	var ids []string
	for i, c := range first.Lanes[board.Todo] {
		if c.ID != second.Lanes[board.Todo][i].ID {
			t.Errorf("card %d: %q then %q — a rendered link would not resolve", i, c.ID, second.Lanes[board.Todo][i].ID)
		}
		if len(c.ID) != 8 {
			t.Errorf("card %d: id %q", i, c.ID)
		}
		ids = append(ids, c.ID)
	}
	if ids[0] == ids[2] {
		t.Errorf("two identical card blocks must still get distinct ids: %v", ids)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func titlesOf(cards []*board.Card) []string {
	var out []string
	for _, c := range cards {
		out = append(out, c.Title)
	}
	return out
}

func TestOpenRepairsADuplicateIDOnDisk(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "life")
	os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "board.md")
	os.WriteFile(path, []byte("## Todo\n\n### Original\nid: dup\n\n### Copy\nid: dup\n"), 0o644)

	_, b, err := Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	if b.Lanes[board.Todo][0].ID != "dup" {
		t.Errorf("the card Find reaches must keep its id, got %q", b.Lanes[board.Todo][0].ID)
	}
	second := b.Lanes[board.Todo][1].ID
	data := mustRead(t, path)
	if !strings.Contains(string(data), "### Original\nid: dup\n") {
		t.Errorf("the id must stay on Original in the file:\n%s", data)
	}
	if !strings.Contains(string(data), "id: dup\n") || !strings.Contains(string(data), "id: "+second+"\n") {
		t.Fatalf("Open must write both distinct ids to disk:\n%s", data)
	}
	if strings.Count(string(data), "id: dup\n") != 1 {
		t.Errorf("the duplicate must be gone from the file:\n%s", data)
	}

	// Idempotent: the repaired file is clean, so a second open must not write.
	// Comparing bytes cannot show that — the file came from Marshal, so a
	// rewrite would produce the same bytes. A rewrite always replaces the file
	// and moves its mtime, so pin the mtime in the past and check it stayed.
	past := time.Unix(1_000_000_000, 0)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Open(root, "life"); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path); err != nil || !fi.ModTime().Equal(past) {
		t.Errorf("a repaired board must not be rewritten on the next open (stat err %v)", err)
	}
}

func TestLoadDoesNotRepairADuplicateOnDisk(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "life")
	os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "board.md")
	dup := []byte("## Todo\n\n### Original\nid: dup\n\n### Copy\nid: dup\n")
	os.WriteFile(path, dup, 0o644)

	b, err := Load(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	if b.Lanes[board.Todo][0].ID == b.Lanes[board.Todo][1].ID {
		t.Errorf("Load must still give the twins distinct ids in memory")
	}
	if got := mustRead(t, path); !bytes.Equal(got, dup) {
		t.Errorf("Load must not rewrite board.md:\n%s", got)
	}
}

// interleave runs f inside the window between Open's (or LoadArchive's) read
// and the write that repairs what it read — where a concurrent writer lands.
// No store test runs in parallel, so package state is safe here.
func interleave(t *testing.T, f func()) {
	t.Helper()
	beforeRepair = f
	t.Cleanup(func() { beforeRepair = nil })
}

func TestOpenRepairDoesNotClobberAConcurrentWrite(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "life")
	os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "board.md")
	dup := "## Todo\n\n### Original\nid: dup\n\n### Copy\nid: dup\n"
	os.WriteFile(path, []byte(dup), 0o644)

	calls := 0
	interleave(t, func() {
		calls++
		if calls == 1 {
			// Another process lands an edit in the window. It adds a card, so
			// the file changes size (a same-size edit inside one mtime tick is a
			// pre-existing blind spot of every checked write, not this one) and
			// the retry still has a repair to do.
			if err := os.WriteFile(path, []byte(dup+"\n### Theirs\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	})

	st, b, err := Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	data := string(mustRead(t, path))
	if !strings.Contains(data, "### Theirs\n") {
		t.Fatalf("the repair overwrote a write that landed after Open's read:\n%s", data)
	}
	if calls != 2 {
		t.Errorf("a refused repair must re-read and retry: hook ran %d times, want 2", calls)
	}
	if got := titlesOf(b.Lanes[board.Todo]); !reflect.DeepEqual(got, []string{"Original", "Copy", "Theirs"}) {
		t.Errorf("Open must return the board it repaired on the retry, not the one it first read: %v", got)
	}
	if strings.Count(data, "\nid: ") != 3 || strings.Count(data, "id: dup\n") != 1 {
		t.Errorf("every card must carry its own id on disk:\n%s", data)
	}
	// The Store's baseline is the repaired file, so the caller's own next checked
	// save still goes through — TestMoveOnAnIdLessBoard's guarantee, on the retry.
	b.Insert(board.Todo, 0, &board.Card{ID: "aaaaaaaa", Title: "mine"})
	if err := st.SaveBoardIfUnchanged(b); err != nil {
		t.Errorf("our own repair must not read as a conflict: %v", err)
	}
}

func TestOpenGivesUpAfterRepeatedConflicts(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "life")
	os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "board.md")
	os.WriteFile(path, []byte("## Todo\n\n### Original\nid: dup\n\n### Copy\nid: dup\n"), 0o644)

	calls := 0
	interleave(t, func() {
		calls++
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString("\n### Theirs " + string(rune('0'+calls)) + "\n")
		f.Close()
	})

	_, _, err := Open(root, "life")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("losing the race every time must end in ErrConflict, got %v", err)
	}
	// A literal, not the constant the loop counts to: asserting against the
	// constant under test would pass at any value of it.
	if calls != 3 {
		t.Errorf("Open must give up after 3 attempts rather than spin: hook ran %d times", calls)
	}
	if data := string(mustRead(t, path)); !strings.Contains(data, "### Theirs 3\n") {
		t.Errorf("giving up must still lose nothing:\n%s", data)
	}
}

func TestOpenCreateDoesNotClobberAConcurrentCreate(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "life", "board.md")
	calls := 0
	interleave(t, func() {
		calls++
		if calls == 1 {
			if err := os.WriteFile(path, []byte("## Todo\n\n### Theirs\nid: bbbbbbbb\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	})

	_, b, err := Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	if data := string(mustRead(t, path)); !strings.Contains(data, "### Theirs\n") {
		t.Fatalf("creating the board overwrote one created in the meantime:\n%s", data)
	}
	if got := titlesOf(b.Lanes[board.Todo]); len(got) != 1 || got[0] != "Theirs" {
		t.Errorf("Open must return the board that won, got %v", got)
	}
	if calls != 1 {
		t.Errorf("the retry reads a clean file with nothing left to repair: hook ran %d times, want 1", calls)
	}
}

func TestLoadArchiveRepairDoesNotClobberAConcurrentWrite(t *testing.T) {
	root := t.TempDir()
	st, b, err := Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	one := "## undated\n\n### One\n"
	os.WriteFile(st.ArchivePath(), []byte(one), 0o644)

	calls := 0
	interleave(t, func() {
		calls++
		if calls == 1 {
			if err := os.WriteFile(st.ArchivePath(), []byte(one+"\n### Two\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	})

	a, err := st.LoadArchive()
	if err != nil {
		t.Fatal(err)
	}
	data := string(mustRead(t, st.ArchivePath()))
	if !strings.Contains(data, "### Two\n") {
		t.Fatalf("the archive repair overwrote a write that landed after its read:\n%s", data)
	}
	if calls != 2 {
		t.Errorf("a refused archive repair must re-read and retry: hook ran %d times, want 2", calls)
	}
	if got := titlesOf(a.Cards); len(got) != 2 {
		t.Errorf("LoadArchive must return the archive it repaired on the retry: %v", got)
	}
	if strings.Count(data, "\nid: ") != 2 {
		t.Errorf("both archived cards must carry an id on disk:\n%s", data)
	}
	if err := st.SaveArchivalIfUnchanged(b, a); err != nil {
		t.Errorf("our own archive repair must not read as a conflict: %v", err)
	}
}
