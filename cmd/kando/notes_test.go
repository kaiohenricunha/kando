package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

func TestNotesArgs(t *testing.T) {
	t.Run("set", func(t *testing.T) {
		card, name, src, err := notesArgs([]string{"Renew passport", "--set", "New notes"}, io.Discard)
		if err != nil || card != "Renew passport" || name != "" || src.kind != "set" || src.value != "New notes" {
			t.Fatalf("card=%q name=%q src=%+v err=%v", card, name, src, err)
		}
	})

	t.Run("file", func(t *testing.T) {
		card, _, src, err := notesArgs([]string{"Renew passport", "--file", "notes.txt"}, io.Discard)
		if err != nil || card != "Renew passport" || src.kind != "file" || src.value != "notes.txt" {
			t.Fatalf("card=%q src=%+v err=%v", card, src, err)
		}
	})

	t.Run("stdin via trailing dash", func(t *testing.T) {
		card, name, src, err := notesArgs([]string{"Renew passport", "work", "-"}, io.Discard)
		if err != nil || card != "Renew passport" || name != "work" || src.kind != "stdin" {
			t.Fatalf("card=%q name=%q src=%+v err=%v", card, name, src, err)
		}
	})

	t.Run("no source is an error", func(t *testing.T) {
		if _, _, _, err := notesArgs([]string{"Renew passport"}, io.Discard); err == nil {
			t.Fatal("want an error")
		}
	})

	t.Run("two sources is an error", func(t *testing.T) {
		if _, _, _, err := notesArgs([]string{"Renew passport", "--set", "x", "--file", "y"}, io.Discard); err == nil {
			t.Fatal("want an error")
		}
	})

	t.Run("set and dash together is an error", func(t *testing.T) {
		if _, _, _, err := notesArgs([]string{"Renew passport", "--set", "x", "-"}, io.Discard); err == nil {
			t.Fatal("want an error")
		}
	})

	t.Run("empty --set is a valid clear, not 'no source'", func(t *testing.T) {
		_, _, src, err := notesArgs([]string{"Renew passport", "--set", ""}, io.Discard)
		if err != nil || src.kind != "set" || src.value != "" {
			t.Fatalf("src=%+v err=%v", src, err)
		}
	})

	t.Run("no card", func(t *testing.T) {
		if _, _, _, err := notesArgs([]string{"--set", "x"}, io.Discard); err == nil {
			t.Fatal("want an error")
		}
	})
}

func TestReadNotes(t *testing.T) {
	t.Run("set returns the value directly", func(t *testing.T) {
		got, err := readNotes(notesSource{kind: "set", value: "hello"}, strings.NewReader(""))
		if err != nil || got != "hello" {
			t.Fatalf("got=%q err=%v", got, err)
		}
	})

	t.Run("file reads the path", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "notes.txt")
		if err := os.WriteFile(path, []byte("from a file"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := readNotes(notesSource{kind: "file", value: path}, nil)
		if err != nil || got != "from a file" {
			t.Fatalf("got=%q err=%v", got, err)
		}
	})

	t.Run("file too large is rejected", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "big.txt")
		if err := os.WriteFile(path, make([]byte, maxNotesInputBytes+1), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := readNotes(notesSource{kind: "file", value: path}, nil); err == nil {
			t.Fatal("want an error")
		}
	})

	t.Run("stdin reads from the reader", func(t *testing.T) {
		got, err := readNotes(notesSource{kind: "stdin"}, strings.NewReader("piped in"))
		if err != nil || got != "piped in" {
			t.Fatalf("got=%q err=%v", got, err)
		}
	})

	t.Run("stdin too large is rejected", func(t *testing.T) {
		if _, err := readNotes(notesSource{kind: "stdin"}, strings.NewReader(strings.Repeat("x", maxNotesInputBytes+1))); err == nil {
			t.Fatal("want an error")
		}
	})
}

func TestSetNotes(t *testing.T) {
	root := t.TempDir()
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes[board.Todo] = []*board.Card{{ID: "aaaaaaaa", Title: "Renew passport", Notes: "old notes"}}
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}

	t.Run("sets new notes, normalizing CRLF", func(t *testing.T) {
		title, stored, err := setNotes(root, "life", "aaaaaaaa", "line1\r\nline2")
		if err != nil || title != "Renew passport" || stored != "line1\nline2" {
			t.Fatalf("title=%q stored=%q err=%v", title, stored, err)
		}
		got, err := store.Load(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		if got.Lanes[board.Todo][0].Notes != "line1\nline2" {
			t.Errorf("Notes = %q", got.Lanes[board.Todo][0].Notes)
		}
	})

	t.Run("a trailing newline is trimmed from the stored value", func(t *testing.T) {
		_, stored, err := setNotes(root, "life", "aaaaaaaa", "one line\n")
		if err != nil || stored != "one line" {
			t.Fatalf("stored=%q err=%v, want the trailing newline trimmed", stored, err)
		}
	})

	t.Run("empty text clears the notes", func(t *testing.T) {
		_, stored, err := setNotes(root, "life", "aaaaaaaa", "")
		if err != nil || stored != "" {
			t.Fatalf("stored=%q err=%v", stored, err)
		}
		got, err := store.Load(root, "life")
		if err != nil {
			t.Fatal(err)
		}
		if got.Lanes[board.Todo][0].Notes != "" {
			t.Errorf("Notes = %q, want empty", got.Lanes[board.Todo][0].Notes)
		}
	})

	t.Run("nonexistent card", func(t *testing.T) {
		if _, _, err := setNotes(root, "life", "nope", "x"); err == nil {
			t.Fatal("want an error")
		}
	})

	// The claim this verb's sanitizing exists to make is about bytes on disk,
	// not about what setNotes returns: a note piped in from an issue body must
	// not leave an escape sequence in board.md for kando show, the TUI and
	// kando web to replay on every later read. Assert that end to end, across
	// readNotes -> SetNotes -> Marshal, because no single package's tests span
	// it. OSC 52 is the payload that matters — it writes the reader's
	// clipboard from a card they merely opened.
	t.Run("an OSC 52 payload from --file leaves no escape bytes in board.md", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "note.txt")
		if err := os.WriteFile(path, []byte("before\x1b]52;c;cGF5bG9hZA==\x07after"), 0o644); err != nil {
			t.Fatal(err)
		}
		text, err := readNotes(notesSource{kind: "file", value: path}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := setNotes(root, "life", "aaaaaaaa", text); err != nil {
			t.Fatal(err)
		}
		md := readBoardFile(t, root, "life")
		if strings.ContainsAny(md, "\x1b\x07") {
			t.Errorf("board.md still holds an ESC or BEL byte: %q", md)
		}
		// The payload text stays visible rather than being silently swallowed,
		// so the note still shows what it carried.
		if !strings.Contains(md, "]52;c;cGF5bG9hZA==") {
			t.Errorf("payload text was dropped, not just de-fanged: %q", md)
		}
	})
}
