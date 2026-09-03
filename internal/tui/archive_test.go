package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

func bodyRows(lines []string) []string {
	var out []string
	for _, l := range lines[2 : len(lines)-2] {
		out = append(out, strings.TrimRight(l[1:], " "))
	}
	return out
}

func archiveItem(title, tag, date string) string {
	return strings.TrimRight("✓ "+fit(title, 56)+"  "+fit(tag, 8)+"  "+right(date, 10), " ")
}

func TestArchiveLayout(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = press(m, "D")
	if m.scr != screenArchive {
		t.Fatal("D opens the archive")
	}
	lines := plainLines(m)
	if !strings.HasPrefix(lines[0], " kando  life › archive") || !strings.HasSuffix(lines[0], "10 done ") {
		t.Errorf("header = %q", lines[0])
	}
	body := bodyRows(lines)
	want := []string{
		"THIS WEEK 3",
		strings.Repeat("─", 80),
		archiveItem("Cancel gym membership", "#money", "Wed 2 Sep"),
		archiveItem("Return library books", "#errand", "Tue 1 Sep"),
		archiveItem("Pay electric bill", "#money", "Mon 31 Aug"),
		"",
		"LAST WEEK 4",
		strings.Repeat("─", 80),
		archiveItem("Descale the kettle", "#home", "Sat 29 Aug"),
		archiveItem("Finish The Overstory", "#read", "Thu 27 Aug"),
		archiveItem("Send insurance renewal", "#money", "Mon 24 Aug"),
		archiveItem("Reply to Sam about the trip", "", "Mon 24 Aug"),
		"",
		"EARLIER 3",
		strings.Repeat("─", 80),
		archiveItem("Hem the grey trousers", "#home", "Wed 19 Aug"),
		archiveItem("Renew car tax", "#money", "Fri 14 Aug"),
		archiveItem("Eye test", "#health", "Tue 11 Aug"),
		"",
		"Older entries live in ~/.kando/life/archive.md",
	}
	for i, w := range want {
		if body[i] != w {
			t.Errorf("body row %d\n got %q\nwant %q", i, body[i], w)
		}
	}
	if !strings.HasPrefix(lines[39], " j/k move  u undo (back to Doing)  enter open  / filter  esc board") {
		t.Errorf("footer = %q", lines[39])
	}
	// Item cell geometry: date ends at the 80th cell of the body.
	if r := []rune(lines[4]); string(r[1+80-9:1+80]) != "Wed 2 Sep" {
		t.Errorf("date column misaligned: %q", string(r[1:81]))
	}
	m = press(m, "esc")
	if m.scr != screenBoard {
		t.Errorf("esc returns to the board")
	}
}

func TestArchiveNarrowAndEmpty(t *testing.T) {
	m := newTestModel(t, 60, 16)
	m = press(m, "D")
	body := bodyRows(plainLines(m))
	if body[1] != strings.Repeat("─", 58) || body[2] != strings.TrimRight("✓ "+fit("Cancel gym membership", 34)+"  "+fit("#money", 8)+"  "+right("Wed 2 Sep", 10), " ") {
		t.Errorf("narrow archive rows: %q / %q", body[1], body[2])
	}
	empty := New(Options{Board: sampleBoard(t), Archive: &board.Archive{}, Styles: testStyles, Now: func() time.Time { return fixedNow }, Width: 120, Height: 40})
	empty = press(empty, "D")
	lines := plainLines(empty)
	if !strings.HasSuffix(lines[0], "0 done ") || bodyRows(lines)[0] != "Older entries live in ~/.kando/life/archive.md" {
		t.Errorf("empty archive: %q / %q", lines[0], bodyRows(lines)[0])
	}
	empty = press(empty, "j", "u", "enter")
	if empty.scr != screenArchive {
		t.Errorf("keys on an empty archive are harmless")
	}
}

func TestArchiveCursorUndoAndDetail(t *testing.T) {
	root := t.TempDir()
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes = sampleBoard(t).Lanes
	if err := os.WriteFile(st.ArchivePath(), []byte(store.MarshalArchive(sampleArchive(t))), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(Options{Store: st, Board: b, Styles: testStyles, Now: func() time.Time { return fixedNow }, Width: 120, Height: 40})
	m = press(m, "D")
	if m.archive == nil || len(m.archive.Cards) != 10 {
		t.Fatalf("archive should load from the store on D")
	}
	m = press(m, "j", "j", "j") // Descale the kettle
	if m.arch.cursor != 3 {
		t.Fatalf("cursor = %d", m.arch.cursor)
	}
	m = press(m, "k", "k", "k", "k")
	if m.arch.cursor != 9 {
		t.Errorf("k wraps to the last item, got %d", m.arch.cursor)
	}
	m = press(m, "j") // wraps to the first item
	m = press(m, "u")
	if len(m.archive.Cards) != 9 || m.b.Lanes[board.Doing][0].Title != "Cancel gym membership" {
		t.Fatalf("u moves the item to the top of Doing: %v", titles(m.b.Lanes[board.Doing]))
	}
	c := m.b.Lanes[board.Doing][0]
	if !c.MovedAt.Equal(fixedNow) || !c.DoneAt.IsZero() {
		t.Errorf("restored card stamps: %v %v", c.MovedAt, c.DoneAt)
	}
	if !strings.HasSuffix(plainLines(m)[0], "9 done ") {
		t.Errorf("header count should update")
	}
	archiveData, _ := os.ReadFile(st.ArchivePath())
	boardData, _ := os.ReadFile(st.BoardPath())
	if strings.Contains(string(archiveData), "Cancel gym") || !strings.Contains(string(boardData), "### Cancel gym membership") {
		t.Errorf("restore should be saved to both files")
	}
	m = press(m, "j", "enter") // Pay electric bill (now second)
	if m.scr != screenDetail || !m.detail.archived {
		t.Fatalf("enter opens the archived card in the detail screen")
	}
	lines := plainLines(m)
	if !strings.HasPrefix(lines[0], " kando  life › archive › Pay electric bill") || leftPane(lines)[0] != "Archive 9" {
		t.Errorf("archived detail header/left: %q / %q", lines[0], leftPane(lines)[0])
	}
	if joined := strings.Join(rightPane(lines), "\n"); !strings.Contains(joined, "#money  created Tue 25 Aug  done Mon 31 Aug") {
		t.Errorf("archived meta:\n%s", joined)
	}
	m = press(m, "esc")
	if m.scr != screenArchive || m.arch.cursor != 1 {
		t.Errorf("esc returns to the archive with the cursor kept: scr=%v cursor=%d", m.scr, m.arch.cursor)
	}
}

func TestArchiveFilter(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = press(m, "D", "/")
	m = typeText(m, "#money")
	lines := plainLines(m)
	if !strings.HasPrefix(lines[0], " / #money") || !strings.HasSuffix(lines[0], "4 of 10 match ") {
		t.Errorf("archive filter header: %q", lines[0])
	}
	if len(m.visibleArchive()) != 4 {
		t.Errorf("visible = %d", len(m.visibleArchive()))
	}
	body := bodyRows(lines)
	if body[0] != "THIS WEEK 2" || body[5] != "LAST WEEK 1" || body[9] != "EARLIER 1" {
		t.Errorf("group counts follow the filter: %q %q %q", body[0], body[5], body[9])
	}
	m = press(m, "enter")
	if m.scr != screenArchive || m.mode != modeNormal || len(m.visibleArchive()) != 4 {
		t.Errorf("enter keeps the archive filter")
	}
	m = press(m, "esc") // back to board; the kept filter stays
	if m.scr != screenBoard || m.filterQuery != "#money" {
		t.Errorf("esc from archive returns to the board: scr=%v q=%q", m.scr, m.filterQuery)
	}
	m = press(m, "D", "/", "esc")
	if !m.filter.Empty() || len(m.visibleArchive()) != 10 {
		t.Errorf("esc while typing clears the filter")
	}
}

var archiveStates = map[string][]string{
	"archive":        {"D"},
	"archive filter": {"D", "/", "#", "m"},
	"archive detail": {"D", "enter"},
	"archive help":   {"D", "?"},
	"archive end":    {"D", "k"},
}

func TestWidthInvariantsArchive(t *testing.T) {
	for name, keys := range archiveStates {
		for _, sz := range invariantSizes {
			m := newTestModel(t, sz[0], sz[1])
			m = press(m, keys...)
			checkInvariants(t, name, m, sz[0], sz[1])
		}
	}
}
