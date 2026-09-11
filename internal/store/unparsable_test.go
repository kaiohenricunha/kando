package store

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/kaiohenricunha/kando/internal/board"
)

// A hand edit that stops board.md or archive.md parsing must not be silently
// overwritten by an unchecked writer, and CheckReload must say so instead of
// pretending nothing happened. These pin the fix for that bug.

func TestSaveBoardRefusesAnUnparsableHandEdit(t *testing.T) {
	root := t.TempDir()
	st, b, err := Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	bad := []byte("## Todo\n\n### Mine\nid: aaaaaaaa\n\n### Call mum\ncreated: yesterday\n")
	if err := os.WriteFile(st.BoardPath(), bad, 0o644); err != nil {
		t.Fatal(err)
	}
	err = st.SaveBoard(b)
	if !errors.Is(err, ErrUnparsable) {
		t.Fatalf("want ErrUnparsable, got %v", err)
	}
	if want := `board.md does not parse: line 7: created: cannot parse time "yesterday"`; err.Error() != want {
		t.Errorf("message = %q, want %q", err.Error(), want)
	}
	if got := string(mustRead(t, st.BoardPath())); got != string(bad) {
		t.Errorf("board.md must be untouched:\n%s", got)
	}
	if err := st.SaveBoard(b); !errors.Is(err, ErrUnparsable) {
		t.Errorf("a second refused save: %v", err)
	}
	fixed := []byte("## Todo\n\n### Mine\nid: aaaaaaaa\n")
	if err := os.WriteFile(st.BoardPath(), fixed, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveBoard(b); err != nil {
		t.Errorf("a fixed file must save: %v", err)
	}
}

func TestSaveArchiveRefusesAnUnparsableHandEdit(t *testing.T) {
	root := t.TempDir()
	st, _, err := Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	a := &board.Archive{}
	bad := []byte("## undated\n\n### Old\nid: aaaaaaaa\n\n### Older\ndone: last week\n")
	if err := os.WriteFile(st.ArchivePath(), bad, 0o644); err != nil {
		t.Fatal(err)
	}
	err = st.SaveArchive(a)
	if !errors.Is(err, ErrUnparsable) {
		t.Fatalf("want ErrUnparsable, got %v", err)
	}
	if !strings.Contains(err.Error(), `archive.md does not parse: line 7: done: cannot parse time "last week"`) {
		t.Errorf("message = %q", err.Error())
	}
	if got := string(mustRead(t, st.ArchivePath())); got != string(bad) {
		t.Errorf("archive.md must be untouched:\n%s", got)
	}
}

func TestTwoFileSavesCheckBothFilesBeforeWritingEither(t *testing.T) {
	badBoard := []byte("## Todo\n\n### A\ncreated: yesterday\n")
	badArchive := []byte("## undated\n\n### A\ndone: last week\n")
	for _, tc := range []struct {
		name string
		save func(st *Store, b *board.Board, a *board.Archive) error
	}{
		{"SaveArchival", func(st *Store, b *board.Board, a *board.Archive) error { return st.SaveArchival(b, a) }},
		{"SaveRestore", func(st *Store, b *board.Board, a *board.Archive) error { return st.SaveRestore(b, a) }},
	} {
		for _, which := range []string{"board.md", "archive.md"} {
			t.Run(tc.name+"/"+which, func(t *testing.T) {
				root := t.TempDir()
				st, b, err := Open(root, "life")
				if err != nil {
					t.Fatal(err)
				}
				a := &board.Archive{}
				if err := st.SaveArchive(a); err != nil {
					t.Fatal(err)
				}
				boardBefore, archiveBefore := mustRead(t, st.BoardPath()), mustRead(t, st.ArchivePath())
				var bad []byte
				switch which {
				case "board.md":
					bad = badBoard
					os.WriteFile(st.BoardPath(), bad, 0o644)
				case "archive.md":
					bad = badArchive
					os.WriteFile(st.ArchivePath(), bad, 0o644)
				}
				err = tc.save(st, b, a)
				if !errors.Is(err, ErrUnparsable) || !strings.HasPrefix(err.Error(), which) {
					t.Fatalf("%s: want an ErrUnparsable naming %s, got %v", tc.name, which, err)
				}
				if got := string(mustRead(t, st.BoardPath())); which == "board.md" {
					if got != string(bad) {
						t.Errorf("board.md must be untouched:\n%s", got)
					}
				} else if got != string(boardBefore) {
					t.Errorf("board.md must not be written when archive.md is the broken one:\n%s", got)
				}
				if got := string(mustRead(t, st.ArchivePath())); which == "archive.md" {
					if got != string(bad) {
						t.Errorf("archive.md must be untouched:\n%s", got)
					}
				} else if got != string(archiveBefore) {
					t.Errorf("archive.md must not be written when board.md is the broken one:\n%s", got)
				}
			})
		}
	}
}

func TestCheckReloadReportsAnUnparsableFileUntilItIsFixed(t *testing.T) {
	root := t.TempDir()
	st, _, err := Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	bad := []byte("## Todo\n\n### Mine\nid: aaaaaaaa\n\n### Call mum\ncreated: yesterday\n")
	baseline := mustRead(t, st.BoardPath())
	os.WriteFile(st.BoardPath(), bad, 0o644)

	r, err := st.CheckReload()
	if r.Board != nil || !errors.Is(err, ErrUnparsable) || !strings.HasPrefix(err.Error(), "board.md does not parse: line 7") {
		t.Fatalf("board=%v err=%v", r.Board, err)
	}
	r, err = st.CheckReload()
	if r.Board != nil || !errors.Is(err, ErrUnparsable) {
		t.Fatalf("a second check must still report it: board=%v err=%v", r.Board, err)
	}
	os.WriteFile(st.BoardPath(), baseline, 0o644)
	r, err = st.CheckReload()
	if err != nil || r.Board != nil {
		t.Errorf("restoring the exact baseline must report nothing: board=%v err=%v", r.Board, err)
	}
	fixed := []byte("## Todo\n\n### Mine\nid: aaaaaaaa\n\n### Call mum\n")
	os.WriteFile(st.BoardPath(), fixed, 0o644)
	r, err = st.CheckReload()
	if err != nil || r.Board == nil || len(r.Board.Lanes[board.Todo]) != 2 {
		t.Errorf("a fixed file must reload: board=%v err=%v", r.Board, err)
	}
}

func TestCheckReloadAppliesWhatParsesBesideWhatDoesNot(t *testing.T) {
	badBoard := []byte("## Todo\n\n### A\ncreated: yesterday\n")
	badArchive := []byte("## undated\n\n### A\ndone: last week\n")
	goodBoard := []byte("## Todo\n\n### Renamed\nid: aaaaaaaa\n")
	goodArchive := []byte("## undated\n\n### Renamed\nid: bbbbbbbb\n")

	t.Run("bad board, good archive", func(t *testing.T) {
		root := t.TempDir()
		st, _, err := Open(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(st.BoardPath(), badBoard, 0o644)
		os.WriteFile(st.ArchivePath(), goodArchive, 0o644)
		r, err := st.CheckReload()
		if r.Board != nil || r.Archive == nil || r.Archive.Cards[0].Title != "Renamed" {
			t.Fatalf("board=%v archive=%v", r.Board, r.Archive)
		}
		if !errors.Is(err, ErrUnparsable) || !strings.Contains(err.Error(), "board.md") {
			t.Errorf("err = %v", err)
		}
		r2, err2 := st.CheckReload()
		if r2.Archive != nil || !errors.Is(err2, ErrUnparsable) {
			t.Errorf("the archive must not reload twice: archive=%v err=%v", r2.Archive, err2)
		}
	})
	t.Run("good board, bad archive", func(t *testing.T) {
		root := t.TempDir()
		st, _, err := Open(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(st.BoardPath(), goodBoard, 0o644)
		os.WriteFile(st.ArchivePath(), badArchive, 0o644)
		r, err := st.CheckReload()
		if r.Archive != nil || r.Board == nil || r.Board.Lanes[board.Todo][0].Title != "Renamed" {
			t.Fatalf("board=%v archive=%v", r.Board, r.Archive)
		}
		if !errors.Is(err, ErrUnparsable) || !strings.Contains(err.Error(), "archive.md") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("both bad", func(t *testing.T) {
		root := t.TempDir()
		st, _, err := Open(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(st.BoardPath(), badBoard, 0o644)
		os.WriteFile(st.ArchivePath(), badArchive, 0o644)
		_, err = st.CheckReload()
		if !errors.Is(err, ErrUnparsable) || !strings.Contains(err.Error(), "board.md") || !strings.Contains(err.Error(), "archive.md") {
			t.Errorf("err = %v, want it to name both files", err)
		}
	})
}

func TestAnUnparsableWriteRightAfterOurOwnSaveIsRefused(t *testing.T) {
	root := t.TempDir()
	st, b, err := Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	bad := []byte("## Todo\n\n### A\ncreated: yesterday\n### enough padding to change the size\n")
	interleaveAt(t, "wrote", func() {
		if err := os.WriteFile(st.BoardPath(), bad, 0o644); err != nil {
			t.Fatal(err)
		}
	})
	if err := st.SaveBoard(b); err != nil {
		t.Fatalf("our own save must not see the interleaved write: %v", err)
	}
	if err := st.SaveBoard(b); !errors.Is(err, ErrUnparsable) {
		t.Fatalf("the write that landed right after ours must be refused on the next save: %v", err)
	}
	if got := string(mustRead(t, st.BoardPath())); got != string(bad) {
		t.Errorf("the interleaved write must survive:\n%s", got)
	}
	if fi, err := os.Stat(st.BoardPath()); err != nil || fi.Size() != int64(len(bad)) {
		t.Fatalf("setup: the interleaved write must change the file size (stat err %v)", err)
	}
}
