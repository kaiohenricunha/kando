package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
)

// A key line kando does not write, such as a priority: added by hand, used to
// end the key block: every key line after it, id: included, was read as a
// note. The card lost its written id to a derived one, and the next write
// escaped the old id into its notes. The line is now kept as a note, and the
// keys after it still count.
func TestAnUnknownKeyLineDoesNotEndTheKeyBlock(t *testing.T) {
	data := []byte("## Todo\n\n### Renew passport\ntag: errand\npriority: high\ncreated: 2026-08-31\nid: k7q2m9ab\nExpires 14 Nov.\n- [ ] Fill in the form\n")
	b, rewrite, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if rewrite {
		t.Errorf("a card with a written id needs no repair")
	}
	c := b.Lanes[board.Todo][0]
	if c.ID != "k7q2m9ab" || c.Tag != "errand" || formatTime(c.CreatedAt) != "2026-08-31" {
		t.Errorf("the keys after the unknown one must still be keys: id=%q tag=%q created=%q", c.ID, c.Tag, formatTime(c.CreatedAt))
	}
	if c.Notes != "priority: high\nExpires 14 Nov." {
		t.Errorf("the unknown key line must be kept, as a note: %q", c.Notes)
	}
	if len(c.Checklist) != 1 {
		t.Errorf("checklist: %+v", c.Checklist)
	}
	out := Marshal(b)
	if !strings.Contains(string(out), "id: k7q2m9ab\npriority: high\nExpires 14 Nov.\n") {
		t.Errorf("the next write must move the unknown key below the keys, unescaped:\n%s", out)
	}
	again, _, err := Parse(out)
	if err != nil || !sameCard(again.Lanes[board.Todo][0], c) {
		t.Errorf("the rewritten card must read back the same: %v\n%s", err, out)
	}
}

// The same bug through Open, which also wrote the derived id to disk.
func TestOpenKeepsAWrittenIDAfterAnUnknownKey(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "life")
	os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "board.md")
	os.WriteFile(path, []byte("## Todo\n\n### Renew passport\npriority: high\nid: k7q2m9ab\n"), 0o644)
	// A rewrite always replaces the file and moves its mtime, so pin it in the past.
	past := time.Unix(1_000_000_000, 0)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	_, b, err := Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	if got := b.Lanes[board.Todo][0].ID; got != "k7q2m9ab" {
		t.Errorf("Open replaced the written id with %q", got)
	}
	if fi, err := os.Stat(path); err != nil || !fi.ModTime().Equal(past) {
		t.Errorf("a board whose ids are all written must not be rewritten on open (stat err %v)", err)
	}
}

// The key block still ends at the first line of prose. The unknown-key shape is
// narrow on purpose — a lowercase word, a colon, then a space or the end of the
// line — so a note that opens with a URL or a capitalised label stays prose, and
// a key written after prose is still a note, as the README's format says.
func TestProseStillEndsTheKeyBlock(t *testing.T) {
	for _, first := range []string{"https://example.com/form", "Note: bring photos", "call them first", "todo:tomorrow"} {
		t.Run(first, func(t *testing.T) {
			b, _, err := Parse([]byte("## Todo\n\n### A\n" + first + "\ntag: nope\n"))
			if err != nil {
				t.Fatal(err)
			}
			c := b.Lanes[board.Todo][0]
			if c.Tag != "" || c.Notes != first+"\ntag: nope" {
				t.Errorf("tag=%q notes=%q", c.Tag, c.Notes)
			}
		})
	}
}

// A key line after an unknown key counts only when it sets a key the card does
// not have yet, with a value that parses. Anything else was prose before
// unknown keys were kept, and it stays prose. Failing the parse instead would
// stop a board that opened before from opening on any surface, and a TUI that
// already had it open would skip the unparsable reload without a word and
// overwrite the hand edit on its next save.
func TestAnUnparsableKeyAfterAnUnknownKeyStaysProse(t *testing.T) {
	data := []byte("## Todo\n\n### Call the bank\nid: k7q2m9ab\nupdate: called on Monday\ndone: sent the form\ntag: late\n")
	b, rewrite, err := Parse(data)
	if err != nil {
		t.Fatalf("a board that opened before unknown keys were kept must still open: %v", err)
	}
	c := b.Lanes[board.Todo][0]
	if rewrite || c.ID != "k7q2m9ab" || !c.DoneAt.IsZero() || c.Tag != "" {
		t.Errorf("the lines from done: on must stay prose: rewrite=%v id=%q done=%v tag=%q", rewrite, c.ID, c.DoneAt, c.Tag)
	}
	if c.Notes != "update: called on Monday\ndone: sent the form\ntag: late" {
		t.Errorf("notes = %q", c.Notes)
	}
	out := Marshal(b)
	if again, _, err := Parse(out); err != nil || !sameCard(again.Lanes[board.Todo][0], c) {
		t.Errorf("the rewritten card must read back the same: %v\n%s", err, out)
	}
	// With no unknown key before it, a bad date is still an error.
	if _, _, err := Parse([]byte("## Todo\n\n### A\ncreated: not-a-date\n")); err == nil {
		t.Errorf("a bad date directly under the heading must still fail")
	}
}

func TestARepeatedKeyAfterAnUnknownKeyStaysProse(t *testing.T) {
	data := []byte("## Todo\n\n### Book appointment\nid: aaaaaaaa\nsee: the passport card\nid: k7q2m9ab\n\n### Renew passport\nid: k7q2m9ab\n")
	b, rewrite, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	first, second := b.Lanes[board.Todo][0], b.Lanes[board.Todo][1]
	if rewrite || first.ID != "aaaaaaaa" || second.ID != "k7q2m9ab" {
		t.Errorf("a later id: under a label must not replace a written id or take another card's: rewrite=%v first=%q second=%q", rewrite, first.ID, second.ID)
	}
	if first.Notes != "see: the passport card\nid: k7q2m9ab" {
		t.Errorf("notes = %q", first.Notes)
	}
}

// The parser drops blank lines between keys. After an unknown key it has to
// hold them until the next line shows whether they sit between keys, where they
// are dropped, or between note lines, where they are kept.
func TestBlankLinesBetweenKeysAfterAnUnknownKeyAreDropped(t *testing.T) {
	data := []byte("## Todo\n\n### Renew passport\npriority: high\n\ncreated: 2026-08-31\n\nExpires 14 Nov.\n")
	b, _, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	c := b.Lanes[board.Todo][0]
	if formatTime(c.CreatedAt) != "2026-08-31" || c.Notes != "priority: high\n\nExpires 14 Nov." {
		t.Errorf("created=%q notes=%q", formatTime(c.CreatedAt), c.Notes)
	}
}

// Each edge of the shape is a branch of unknownKeyRe that no other test takes.
func TestUnknownKeyShapeEdges(t *testing.T) {
	for _, first := range []string{"due-date: 2026-09-10", "follow_up: call", "v2: yes", "priority:", "priority:\thigh", "priority: high\n"} {
		t.Run(first, func(t *testing.T) {
			b, _, err := Parse([]byte("## Todo\n\n### A\n" + first + "\nid: k7q2m9ab\n"))
			if err != nil {
				t.Fatal(err)
			}
			if c := b.Lanes[board.Todo][0]; c.ID != "k7q2m9ab" || c.Notes != strings.TrimSpace(first) {
				t.Errorf("id=%q notes=%q", c.ID, c.Notes)
			}
		})
	}
}

// archive.md goes through the same parser, and there a done: line decides the
// sort order and the week a card is filed under.
func TestArchiveKeysAfterAnUnknownKeyStillCount(t *testing.T) {
	data := []byte("## 2026-W31\n\n### Older\ndone: 2026-08-01\nid: aaaaaaaa\n\n## 2026-W36\n\n### Newer\npriority: high\ndone: 2026-09-01\nid: bbbbbbbb\n")
	a, rewrite, err := ParseArchive(data)
	if err != nil || rewrite {
		t.Fatalf("err=%v rewrite=%v", err, rewrite)
	}
	if len(a.Cards) != 2 || a.Cards[0].ID != "bbbbbbbb" || formatTime(a.Cards[0].DoneAt) != "2026-09-01" {
		t.Errorf("the done: after an unknown key must date the card and sort it first: %+v", a.Cards)
	}
}

// Notes that start with an unknown-key line, typed through any surface, must
// survive the write. The dangerous one is the blank line: the parser skips blank
// lines while it is still reading keys, so once an unknown key line keeps it
// there, a blank line after it has to go to the notes instead.
func TestNotesStartingWithAnUnknownKeyRoundTrip(t *testing.T) {
	for _, notes := range []string{
		"priority: high",
		"priority: high\n\nsecond paragraph",
		"priority: high\nid: 00000000",
		"priority: high\n- [ ] not an item",
		"see: https://example.com\ndone: soon",
	} {
		t.Run(notes, func(t *testing.T) {
			b := &board.Board{}
			c := &board.Card{ID: "abcdefgh", Title: "Unknown-key notes", Tag: "real", Notes: notes, CreatedAt: ts(2026, 9, 1)}
			b.Lanes[board.Todo] = []*board.Card{c}
			out := Marshal(b)
			got, _, err := Parse(out)
			if err != nil {
				t.Fatalf("board became unparsable: %v\n%s", err, out)
			}
			if len(got.Lanes[board.Todo]) != 1 {
				t.Fatalf("want one card, got %d\n%s", len(got.Lanes[board.Todo]), out)
			}
			r := got.Lanes[board.Todo][0]
			if r.Notes != notes || r.Tag != "real" || r.ID != "abcdefgh" || len(r.Checklist) != 0 {
				t.Errorf("round trip: notes=%q tag=%q id=%q checklist=%v\n%s", r.Notes, r.Tag, r.ID, r.Checklist, out)
			}
			if again := Marshal(got); string(again) != string(out) {
				t.Errorf("a second write differs:\n%s\n%s", out, again)
			}
		})
	}
}
