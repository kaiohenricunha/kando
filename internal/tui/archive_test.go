package tui

import (
	"errors"
	"fmt"
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
	"archived one":   {"l", "l", "A", "D"},
}

func TestArchiveKeyMovesDoneCardToArchive(t *testing.T) {
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
	m = press(m, "l", "l") // Done lane
	if m.lane != board.Done {
		t.Fatalf("l,l should reach Done, got %v", m.lane)
	}
	top := m.b.Lanes[board.Done][0]
	if top.Title != "Cancel gym membership" {
		t.Fatalf("fixture drifted: top of Done is %q", top.Title)
	}
	doneAt := top.DoneAt

	m = press(m, "A")
	if got := len(m.b.Lanes[board.Done]); got != 2 {
		t.Fatalf("Done should shrink to 2, has %d", got)
	}
	if m.archive == nil || len(m.archive.Cards) != 11 {
		t.Fatalf("archive should grow to 11, has %v", m.archive)
	}
	got := m.archive.Cards[0]
	if got.Title != "Cancel gym membership" || !got.DoneAt.Equal(doneAt) {
		t.Errorf("archived card: %+v, want the fixture's DoneAt %v (not restamped)", got, doneAt)
	}

	// A outside Done must be a no-op.
	before := len(m.b.Lanes[board.Doing])
	m2 := press(m, "h", "A")
	if len(m2.b.Lanes[board.Doing]) != before || len(m2.archive.Cards) != 11 {
		t.Errorf("A outside Done must do nothing")
	}

	boardData, _ := os.ReadFile(st.BoardPath())
	archiveData, _ := os.ReadFile(st.ArchivePath())
	if strings.Contains(string(boardData), "Cancel gym membership") {
		t.Errorf("board.md should no longer have the archived card")
	}
	if !strings.Contains(string(archiveData), "### Cancel gym membership") {
		t.Errorf("archive.md should have the archived card")
	}
}

func TestArchiveKeyFromDetail(t *testing.T) {
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
	m = press(m, "l", "l", "enter")
	if m.scr != screenDetail || m.detail.archived {
		t.Fatalf("enter should open the Done card in detail")
	}
	m = press(m, "A")
	if m.scr != screenBoard {
		t.Fatalf("A from detail should return to the board, got %v", m.scr)
	}
	if len(m.b.Lanes[board.Done]) != 2 || m.archive == nil || len(m.archive.Cards) != 11 {
		t.Errorf("archiving from detail: done=%d archive=%v", len(m.b.Lanes[board.Done]), m.archive)
	}

	// A on an already-open archived card is a no-op.
	m = press(m, "D", "enter")
	if !m.detail.archived {
		t.Fatalf("expected an archived detail view")
	}
	before := len(m.archive.Cards)
	m = press(m, "A")
	if m.scr != screenDetail || len(m.archive.Cards) != before {
		t.Errorf("A on an archived card's detail must be a no-op")
	}
	if !strings.Contains(m.notice, "is already archived") {
		t.Errorf("A on an archived card must say why nothing happened: notice = %q", m.notice)
	}
}

func TestArchiveKeyLoadsArchiveLazily(t *testing.T) {
	root := t.TempDir()
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes = sampleBoard(t).Lanes
	if err := os.WriteFile(st.ArchivePath(), []byte(store.MarshalArchive(sampleArchive(t))), 0o644); err != nil {
		t.Fatal(err)
	}
	// Archive intentionally left nil: A must load it, not silently no-op
	// (saveRestore's own nil-archive guard is the trap this pins).
	m := New(Options{Store: st, Board: b, Styles: testStyles, Now: func() time.Time { return fixedNow }, Width: 120, Height: 40})
	if m.archive != nil {
		t.Fatal("test setup: archive should start nil")
	}
	m = press(m, "l", "l", "A")
	if m.archive == nil || len(m.archive.Cards) != 11 {
		t.Fatalf("A should load the existing archive.md and then add to it, got %v", m.archive)
	}
}

func TestArchiveKeyRefusesToWriteAnUnreadableArchive(t *testing.T) {
	root := t.TempDir()
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes = sampleBoard(t).Lanes
	// A card heading before any "## " week section: parseSections rejects it,
	// so LoadArchive fails instead of returning an empty archive. archive.md is
	// documented as hand-editable, so this is a reachable state, and A is the
	// first key that writes the archive without the archive screen being opened
	// first — where a failed load shows nothing to press u on.
	bad := "### Orphan card\n\nnotes\n"
	if err := os.WriteFile(st.ArchivePath(), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	boardBefore, err := os.ReadFile(st.BoardPath())
	if err != nil {
		t.Fatal(err)
	}

	m := New(Options{Store: st, Board: b, Styles: testStyles, Now: func() time.Time { return fixedNow }, Width: 120, Height: 40})
	m = press(m, "l", "l")
	done := len(m.b.Lanes[board.Done])
	m = press(m, "A")

	if got := len(m.b.Lanes[board.Done]); got != done {
		t.Errorf("Done went from %d to %d cards: A must not touch the board when the archive cannot be read", done, got)
	}
	if m.err == nil {
		t.Error("the load failure should reach m.err")
	}
	archiveAfter, err := os.ReadFile(st.ArchivePath())
	if err != nil {
		t.Fatal(err)
	}
	if string(archiveAfter) != bad {
		t.Errorf("archive.md was rewritten to:\n%s\nit must be byte-identical to:\n%s", archiveAfter, bad)
	}
	boardAfter, err := os.ReadFile(st.BoardPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(boardAfter) != string(boardBefore) {
		t.Error("board.md was rewritten")
	}
}

func TestArchiveKeyRefusesADuplicate(t *testing.T) {
	m := newTestModel(t, 120, 40)
	dup := m.archive.Cards[0].ID
	m.b.Lanes[board.Done][0].ID = dup // a previous half-failed archive
	title := m.b.Lanes[board.Done][0].Title
	m = press(m, "l", "l", "A")
	if len(m.b.Lanes[board.Done]) != 3 || len(m.archive.Cards) != 10 {
		t.Errorf("a duplicate id must refuse the archive: done=%d archive=%d", len(m.b.Lanes[board.Done]), len(m.archive.Cards))
	}
	// The web answers this with a 409 and the CLI with an error; the TUI must
	// not be the one surface where the key just appears dead.
	want := fmt.Sprintf("%q is already archived", title)
	if !strings.Contains(m.notice, want) {
		t.Errorf("the refusal must say why: notice = %q, want %q", m.notice, want)
	}
	if footer := plainLines(m)[39]; !strings.Contains(footer, "⊘ "+want) {
		t.Errorf("the board footer must show the refusal: %q", footer)
	}
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

// twinByTitle gives the board card titled title the id of the archived card
// with the same title — the state a half-failed restore or archive leaves, where
// one card sits in both files. The web and CLI guards refuse to move it either
// way; these tests pin that the TUI does too.
func twinByTitle(t *testing.T, b *board.Board, a *board.Archive, title string) string {
	t.Helper()
	var live, arch *board.Card
	for _, l := range board.Lanes {
		for _, c := range b.Lanes[l] {
			if c.Title == title {
				live = c
			}
		}
	}
	for _, c := range a.Cards {
		if c.Title == title {
			arch = c
		}
	}
	if live == nil || arch == nil {
		t.Fatalf("fixture has no board and archive card both titled %q", title)
	}
	live.ID = arch.ID
	return arch.ID
}

func TestRestoreRefusesACardAlreadyOnTheBoard(t *testing.T) {
	root := t.TempDir()
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes = sampleBoard(t).Lanes
	a := sampleArchive(t)
	const title = "Pay electric bill"
	dup := twinByTitle(t, b, a, title)
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(st.ArchivePath(), []byte(store.MarshalArchive(a)), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(Options{Store: st, Board: b, Styles: testStyles, Now: func() time.Time { return fixedNow }, Width: 120, Height: 40})
	m = press(m, "D")
	for i, c := range m.visibleArchive() {
		if c.ID == dup {
			m.arch.cursor = i
		}
	}
	cursor := m.arch.cursor
	archived, doing := len(m.archive.Cards), len(m.b.Lanes[board.Doing])
	boardBefore, archiveBefore := mustReadFile(t, st.BoardPath()), mustReadFile(t, st.ArchivePath())
	past := pinMtimes(t, st.BoardPath(), st.ArchivePath())

	m = press(m, "u")

	if len(m.archive.Cards) != archived || len(m.b.Lanes[board.Doing]) != doing {
		t.Errorf("the restore must be refused: archive %d -> %d, Doing %d -> %d",
			archived, len(m.archive.Cards), doing, len(m.b.Lanes[board.Doing]))
	}
	if string(mustReadFile(t, st.BoardPath())) != string(boardBefore) || string(mustReadFile(t, st.ArchivePath())) != string(archiveBefore) {
		t.Error("a refused restore must write nothing")
	}
	if !unwritten(past, st.BoardPath(), st.ArchivePath()) {
		t.Error("a refused restore must not touch either file — a rewrite of identical bytes still moves the mtime")
	}
	if m.arch.cursor != cursor {
		t.Errorf("a refused restore must leave the cursor: %d -> %d", cursor, m.arch.cursor)
	}
	want := fmt.Sprintf("%q is already on the board", title)
	if !strings.Contains(m.notice, want) {
		t.Errorf("the refusal must say why, in the CLI's words: notice = %q, want %q", m.notice, want)
	}
	if footer := plainLines(m)[39]; !strings.HasPrefix(footer, " j/k move  u undo") || !strings.Contains(footer, "⊘ "+want) {
		t.Errorf("the archive footer must keep its hints and show the refusal: %q", footer)
	}
	// The reason the guard exists after #23: had the restore gone through, the
	// next open would keep the shared id on the Doing copy (Board.Find reaches
	// Doing before Done) and give the live Done card a new one.
	reopened, err := store.Load(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	if l, _, c := reopened.Find(dup); c == nil || l != board.Done || c.Title != title {
		t.Errorf("the live Done card must keep its id on reopen: Find(%s) = %v in %v", dup, c, l)
	}
}

func TestArchiveKeyFromDetailRefusalStaysOnTheCard(t *testing.T) {
	// (a) A on a Done card that is already archived.
	m := newTestModel(t, 120, 40)
	title := m.b.Lanes[board.Done][0].Title
	m.b.Lanes[board.Done][0].ID = m.archive.Cards[0].ID
	m = press(m, "l", "l", "enter", "A")
	if m.scr != screenDetail {
		t.Errorf("a refused archive must stay on the card, not look like it worked: screen = %v", m.scr)
	}
	if len(m.b.Lanes[board.Done]) != 3 || len(m.archive.Cards) != 10 {
		t.Errorf("nothing may move: done=%d archive=%d", len(m.b.Lanes[board.Done]), len(m.archive.Cards))
	}
	want := fmt.Sprintf("%q is already archived", title)
	if !strings.Contains(m.notice, want) {
		t.Errorf("notice = %q, want %q", m.notice, want)
	}
	if footer := plainLines(m)[39]; !strings.HasPrefix(footer, " j/k item") || !strings.Contains(footer, "⊘ "+want) {
		t.Errorf("the detail footer must keep its hints and show the refusal: %q", footer)
	}

	// (b) A on a card that is not in Done at all.
	m = newTestModel(t, 120, 40)
	m = press(m, "enter", "A")
	if m.scr != screenDetail {
		t.Errorf("A outside Done must stay on the card: screen = %v", m.scr)
	}
	if !strings.Contains(m.notice, `"Renew passport" is in Todo, not Done`) {
		t.Errorf("A outside Done must say where the card is: notice = %q", m.notice)
	}
}

func TestArchiveAndDetailFootersShowError(t *testing.T) {
	cases := []struct {
		name   string
		keys   []string
		prefix string
	}{
		{"board", nil, " j/k move"},
		{"archive", []string{"D"}, " j/k move  u undo"},
		{"detail", []string{"enter"}, " j/k item"},
		{"archived detail", []string{"D", "enter"}, " j/k item"},
	}
	for _, c := range cases {
		// A save failure on the archive screen was invisible before: only the
		// board footer ever read m.err.
		m := press(newTestModel(t, 120, 40), c.keys...)
		m.err = os.ErrPermission
		if footer := plainLines(m)[39]; !strings.HasPrefix(footer, c.prefix) || !strings.Contains(footer, "⊘ permission denied") {
			t.Errorf("%s at 120x40: footer = %q", c.name, footer)
		}
		// A refusal names its card, so it is longer than a save error. The board
		// footer used to drop any message that did not fit beside its hints.
		m.err, m.notice = nil, `"Tax docs to accountant before the end of the month" is already archived`
		if footer := plainLines(m)[39]; !strings.Contains(footer, `⊘ "Tax docs to accountant before`) {
			t.Errorf("%s at 120x40 with a refusal: footer = %q", c.name, footer)
		}

		m = press(newTestModel(t, 60, 16), c.keys...)
		m.err = errors.New(strings.Repeat("x", 200))
		checkInvariants(t, c.name+" error at 60x16", m, 60, 16)
		if footer := plainLines(m)[15]; !strings.Contains(footer, "⊘ xxx") {
			t.Errorf("%s at 60x16: a refused key must be able to say so at every size: %q", c.name, footer)
		}
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// pinMtimes sets each file's mtime in the past, so that any later write is
// detectable — including a canonical rewrite, which reproduces the same bytes
// and so is invisible to a byte comparison.
func pinMtimes(t *testing.T, paths ...string) time.Time {
	t.Helper()
	past := time.Unix(1_000_000_000, 0)
	for _, p := range paths {
		if err := os.Chtimes(p, past, past); err != nil {
			t.Fatal(err)
		}
	}
	return past
}

// unwritten reports whether every file still carries the mtime pinMtimes set.
func unwritten(past time.Time, paths ...string) bool {
	for _, p := range paths {
		if fi, err := os.Stat(p); err != nil || !fi.ModTime().Equal(past) {
			return false
		}
	}
	return true
}

func TestARefusalDoesNotHideFileWatchingDisabled(t *testing.T) {
	// "file watching disabled" is a standing condition that nothing reports twice.
	// A refusal is about one keypress. It must show, and then give the slot back —
	// not overwrite the only notice that live reload is off.
	m, _ := newRootModel(t, 120, 40)
	m.watch = true
	m, _ = feed(m, watchStartedMsg{gen: 0, err: os.ErrPermission})
	m = press(m, "enter")
	if m.scr != screenDetail {
		t.Fatalf("setup: expected the detail screen, got %v", m.scr)
	}
	m = press(m, "A") // refused: the open card is not in Done
	if footer := plainLines(m)[39]; !strings.Contains(footer, "not Done") {
		t.Fatalf("the refusal must show first: %q", footer)
	}
	m = press(m, "esc")
	if footer := plainLines(m)[39]; !strings.Contains(footer, "⊘ file watching disabled") {
		t.Errorf("after the next key the watcher's notice must be back: %q", footer)
	}
}

func TestARefusalClearsOnTheNextKey(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = press(m, "enter", "A") // refused: Renew passport is in Todo
	if footer := plainLines(m)[39]; !strings.Contains(footer, "is in Todo, not Done") {
		t.Fatalf("setup: the refusal must show: %q", footer)
	}
	m = press(m, "j")
	if footer := plainLines(m)[39]; strings.Contains(footer, "⊘") {
		t.Errorf("a refusal is about the key that caused it; the next key clears it: %q", footer)
	}
}
