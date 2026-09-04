package store

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

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
	valid := []string{"life", "work-2024", "a", "My Board"}
	invalid := []string{"", ".", "..", ".hidden", "a/b", "../evil", "a\\b", "/etc"}
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
