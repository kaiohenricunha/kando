package store

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
)

var tz = time.FixedZone("-03", -3*3600)

func ts(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 12, 0, 0, 0, tz) }

func readSample(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// A card archived without a done: date files under "## undated", after every
// real week, and reads back with a zero DoneAt — the shape a hand-edited
// archive.md takes, and the one ArchiveDone avoids by stamping DoneAt.
func TestArchiveWithoutDoneAtRoundTripsAsUndated(t *testing.T) {
	a := &board.Archive{Cards: []*board.Card{
		{ID: "aaaaaaaa", Title: "dated", DoneAt: ts(2026, 9, 2)},
		{ID: "bbbbbbbb", Title: "no date at all"},
	}}
	data := MarshalArchive(a)
	week := []byte("## " + board.ISOWeekKey(ts(2026, 9, 2)))
	if !bytes.Contains(data, []byte("## undated")) || !bytes.Contains(data, week) {
		t.Fatalf("headings:\n%s", data)
	}
	if bytes.Index(data, []byte("## undated")) < bytes.Index(data, []byte("### dated")) {
		t.Errorf("undated must come after the dated weeks:\n%s", data)
	}
	got, _, err := ParseArchive(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Cards) != 2 || got.Cards[0].ID != "aaaaaaaa" || !got.Cards[1].DoneAt.IsZero() {
		t.Errorf("round trip: %+v", got.Cards)
	}
	if !bytes.Equal(MarshalArchive(got), data) {
		t.Errorf("second marshal differs:\n%s\n%s", data, MarshalArchive(got))
	}
}

func TestParseSampleBoard(t *testing.T) {
	b, rewrite, err := Parse(readSample(t, "sample_board.md"))
	if err != nil {
		t.Fatal(err)
	}
	if rewrite {
		t.Errorf("sample has ids; rewrite should be false")
	}
	want := [4]int{3, 3, 2, 3}
	for _, l := range board.Lanes {
		if len(b.Lanes[l]) != want[l] {
			t.Errorf("lane %s has %d cards, want %d", l, len(b.Lanes[l]), want[l])
		}
	}
	rp := b.Lanes[board.Todo][0]
	if rp.Title != "Renew passport" || rp.Tag != "errand" || rp.ID != "k7q2m9ab" {
		t.Errorf("renew passport head: %+v", rp)
	}
	if !rp.CreatedAt.Equal(ts(2026, 8, 31)) || !rp.MovedAt.Equal(ts(2026, 9, 1)) || !rp.DoneAt.IsZero() {
		t.Errorf("renew passport times: %v %v %v", rp.CreatedAt, rp.MovedAt, rp.DoneAt)
	}
	if rp.Notes != "Expires 14 Nov. Two photos, old passport, printed form. Appointment slots open on Mondays." {
		t.Errorf("notes = %q", rp.Notes)
	}
	wantItems := []board.Item{{Text: "Photos from the pharmacy", Done: true}, {Text: "Fill in the form", Done: false}, {Text: "Book appointment", Done: false}, {Text: "Post the old one back", Done: false}}
	if !reflect.DeepEqual(rp.Checklist, wantItems) {
		t.Errorf("checklist = %+v", rp.Checklist)
	}
	brake := b.Lanes[board.Doing][0]
	if !brake.Blocked || brake.BlockedReason != "waiting on pads" {
		t.Errorf("brake blocked = %v %q", brake.Blocked, brake.BlockedReason)
	}
	bday := b.Lanes[board.Doing][1]
	if bday.Tag != "" || bday.Blocked || bday.Notes != "Post by Friday." {
		t.Errorf("birthday: %+v", bday)
	}
	gym := b.Lanes[board.Done][0]
	if !gym.DoneAt.Equal(ts(2026, 9, 2)) || gym.Notes != "" || len(gym.Checklist) != 0 {
		t.Errorf("gym: %+v", gym)
	}
}

func TestRoundTripCanonical(t *testing.T) {
	for _, name := range []string{"sample_board.md", "sample_archive.md"} {
		data := readSample(t, name)
		var out []byte
		if name == "sample_board.md" {
			b, _, err := Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			out = Marshal(b)
		} else {
			a, _, err := ParseArchive(data)
			if err != nil {
				t.Fatal(err)
			}
			if len(a.Cards) != 10 {
				t.Fatalf("archive cards = %d", len(a.Cards))
			}
			out = MarshalArchive(a)
		}
		if !bytes.Equal(out, data) {
			t.Errorf("%s: Marshal(Parse(x)) != x\n--- got ---\n%s\n--- want ---\n%s", name, out, data)
		}
	}
}

func TestNotesVerbatimAndDeepRoundTrip(t *testing.T) {
	b := &board.Board{}
	c := &board.Card{
		ID:        "abcdefgh",
		Title:     "Weird notes",
		Notes:     "first line\n\nafter a blank line\ntag: looks like a key but is notes\n  indented  ",
		CreatedAt: ts(2026, 9, 1),
		MovedAt:   ts(2026, 9, 1),
		Checklist: []board.Item{{Text: "one", Done: false}, {Text: "two", Done: true}},
	}
	d := &board.Card{ID: "zzzzzzzz", Title: "Bare", CreatedAt: ts(2026, 9, 2), MovedAt: ts(2026, 9, 2), Blocked: true}
	b.Lanes[board.Backlog] = []*board.Card{c}
	b.Lanes[board.Done] = []*board.Card{d}
	d.DoneAt = ts(2026, 9, 3)
	out := Marshal(b)
	got, rewrite, err := Parse(out)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if rewrite {
		t.Errorf("no rewrite expected")
	}
	for _, l := range board.Lanes {
		if len(got.Lanes[l]) != len(b.Lanes[l]) {
			t.Fatalf("lane %s: %d cards, want %d\n%s", l, len(got.Lanes[l]), len(b.Lanes[l]), out)
		}
		for i := range b.Lanes[l] {
			if !sameCard(got.Lanes[l][i], b.Lanes[l][i]) {
				t.Errorf("round trip mismatch in %s[%d]\n--- marshalled ---\n%s\n--- got ---\n%+v\n--- want ---\n%+v", l, i, out, got.Lanes[l][i], b.Lanes[l][i])
			}
		}
	}
	if !bytes.Contains(out, []byte("\nblocked:\n")) {
		t.Errorf("blocked without reason should serialise as a bare key:\n%s", out)
	}
}

func TestParseDateForms(t *testing.T) {
	src := "## Todo\n\n### A\ncreated: 2026-08-31\nmoved: 2026-08-31T09:12\ndone: 2026-08-31T09:12:33\nid: aaaaaaaa\n"
	b, _, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	c := b.Lanes[board.Todo][0]
	if c.CreatedAt != time.Date(2026, 8, 31, 0, 0, 0, 0, time.Local) {
		t.Errorf("date-only = %v", c.CreatedAt)
	}
	if c.MovedAt != time.Date(2026, 8, 31, 9, 12, 0, 0, time.Local) {
		t.Errorf("minute form = %v", c.MovedAt)
	}
	if c.DoneAt != time.Date(2026, 8, 31, 9, 12, 33, 0, time.Local) {
		t.Errorf("second form = %v", c.DoneAt)
	}
	out := string(Marshal(b))
	if !bytes.Contains([]byte(out), []byte("created: 2026-08-31\n")) {
		t.Errorf("midnight should marshal date-only:\n%s", out)
	}
	if !bytes.Contains([]byte(out), []byte("done: 2026-08-31T09:12:33")) {
		t.Errorf("non-midnight should marshal with time:\n%s", out)
	}
}

func TestParseAssignsMissingIDs(t *testing.T) {
	src := "## Backlog\n\n### No id here\ntag: x\n\n## Todo\n\n### Has id\nid: bbbbbbbb\n"
	b, rewrite, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if !rewrite {
		t.Errorf("rewrite should be true when an id was assigned")
	}
	if len(b.Lanes[board.Backlog][0].ID) != 8 || b.Lanes[board.Todo][0].ID != "bbbbbbbb" {
		t.Errorf("ids: %q %q", b.Lanes[board.Backlog][0].ID, b.Lanes[board.Todo][0].ID)
	}
}

func TestParseErrors(t *testing.T) {
	if _, _, err := Parse([]byte("## Nope\n\n### x\n")); err == nil {
		t.Errorf("unknown lane should error")
	}
	if _, _, err := Parse([]byte("### orphan card\n")); err == nil {
		t.Errorf("card before any lane should error")
	}
	if _, _, err := Parse([]byte("## Todo\n\n### A\ncreated: not-a-date\n")); err == nil {
		t.Errorf("bad date should error")
	}
	b, _, err := Parse(nil)
	if err != nil || b.Count() != 0 {
		t.Errorf("empty input should be an empty board: %v %v", err, b)
	}
}

func TestMarshalEmptyBoard(t *testing.T) {
	want := "## Backlog\n\n## Todo\n\n## Doing\n\n## Done\n"
	if got := string(Marshal(&board.Board{})); got != want {
		t.Errorf("empty board:\n%q\nwant\n%q", got, want)
	}
}

// sameCard compares cards field by field, using time.Equal for timestamps.
func sameCard(a, b *board.Card) bool {
	if a.ID != b.ID || a.Title != b.Title || a.Notes != b.Notes || a.Tag != b.Tag ||
		a.Blocked != b.Blocked || a.BlockedReason != b.BlockedReason ||
		!a.CreatedAt.Equal(b.CreatedAt) || !a.MovedAt.Equal(b.MovedAt) || !a.DoneAt.Equal(b.DoneAt) {
		return false
	}
	if len(a.Checklist) != len(b.Checklist) {
		return false
	}
	for i := range a.Checklist {
		if a.Checklist[i] != b.Checklist[i] {
			return false
		}
	}
	return true
}

func TestSanitizedFieldsRoundTripAsOneCard(t *testing.T) {
	b := &board.Board{}
	c := board.NewCard("Injected", board.Todo, ts(2026, 9, 1))
	c.SetTag("home\n## Bogus")
	c.SetBlocked("wait\ncreated: nope")
	c.InsertChecklistItem(0, "a\n### Injected")
	b.Lanes[board.Todo] = []*board.Card{c}
	got, _, err := Parse(Marshal(b))
	if err != nil {
		t.Fatalf("Parse failed: %v\n%s", err, Marshal(b))
	}
	if got.Count() != 1 || got.Lanes[board.Todo][0].Tag != "home## Bogus" || len(got.Lanes[board.Todo][0].Checklist) != 1 {
		t.Errorf("sanitized fields should round-trip as exactly one card: %+v", got.Lanes[board.Todo][0])
	}
}

// TestNotesFirstLineLookingLikeAKeyRoundTrips pins the case that made a board
// unopenable: parseSections is still in its inKeys state when it reaches the
// first notes line, so an unescaped "done: soon" was read back as the done:
// key rather than as prose. parseTime then rejected it and Parse failed, which
// took the CLI, the TUI and kando web down together — a note the user typed
// could brick their own board.
func TestNotesFirstLineLookingLikeAKeyRoundTrips(t *testing.T) {
	for _, notes := range []string{
		"done: soon",
		"tag: hijacked",
		"id: 00000000",
		"blocked: nope",
		"created: yesterday\nmoved: never",
		"done: soon\n\nand a second paragraph",
	} {
		t.Run(notes, func(t *testing.T) {
			b := &board.Board{}
			c := &board.Card{
				ID:        "abcdefgh",
				Title:     "Keyish notes",
				Tag:       "real",
				Notes:     notes,
				CreatedAt: ts(2026, 9, 1),
				MovedAt:   ts(2026, 9, 1),
			}
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
			if r.Notes != notes {
				t.Errorf("notes = %q, want %q\n%s", r.Notes, notes, out)
			}
			// The real keys must survive untouched: a note that looks like a
			// key must not be able to overwrite the card's own tag or id.
			if r.Tag != "real" || r.ID != "abcdefgh" {
				t.Errorf("a note overwrote a real key: tag=%q id=%q\n%s", r.Tag, r.ID, out)
			}
		})
	}
}

// TestLineSeparatorsRoundTripAsOneCard is the Zl/Zp twin of
// TestSanitizedFieldsRoundTripAsOneCard, which has this exact shape but only
// uses "\n". A separator reaching a single-line field must not be able to add
// a line the parser would read as structure.
func TestLineSeparatorsRoundTripAsOneCard(t *testing.T) {
	c := board.NewCard("Renew passport", board.Todo, ts(2026, 9, 1))
	c.SetTag("home\u2028## Bogus")
	c.SetBlocked("wait\u2029created: nope")
	c.InsertChecklistItem(-1, "a\u2028### Injected")
	c.SetNotes("first\u2028second")

	b := &board.Board{Name: "life"}
	b.Insert(board.Todo, 0, c)

	got, _, err := Parse(Marshal(b))
	if err != nil {
		t.Fatal(err)
	}
	if n := got.Count(); n != 1 {
		t.Fatalf("round trip produced %d cards, want 1", n)
	}
	out := got.Lanes[board.Todo][0]
	// Notes keep the break as a real newline; nothing else may hold one.
	if out.Notes != "first\nsecond" {
		t.Errorf("Notes = %q, want %q", out.Notes, "first\nsecond")
	}
	for name, v := range map[string]string{
		"Tag": out.Tag, "BlockedReason": out.BlockedReason, "Title": out.Title,
		"Checklist[0]": out.Checklist[0].Text,
	} {
		if strings.ContainsAny(v, "\n\u2028\u2029") {
			t.Errorf("%s = %q, must be a single line", name, v)
		}
	}
}
