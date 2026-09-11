package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

func rightPane(lines []string) []string {
	// body rows 3..H-2 (1-based), right pane starts after " " + 30 + 4 + "│" + 3 = cell 39
	var out []string
	for _, l := range lines[2 : len(lines)-2] {
		r := []rune(l)
		if len(r) > 39 {
			out = append(out, strings.TrimRight(string(r[39:]), " "))
		}
	}
	return out
}

func leftPane(lines []string) []string {
	var out []string
	for _, l := range lines[2 : len(lines)-2] {
		r := []rune(l)
		if len(r) > 31 {
			out = append(out, strings.TrimRight(string(r[1:31]), " "))
		}
	}
	return out
}

func TestDetailLayout(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = press(m, "enter")
	if m.scr != screenDetail {
		t.Fatal("enter opens the detail screen")
	}
	lines := plainLines(m)
	if !strings.HasPrefix(lines[0], " kando  life › Todo › Renew passport") || !strings.HasSuffix(lines[0], "Thu 3 Sep ") {
		t.Errorf("header = %q", lines[0])
	}
	left := leftPane(lines)
	wantLeft := []string{"Todo 3", strings.Repeat("─", 30), "", "▸ Renew passport", "• Tax docs to accountant", "• Book dentist"}
	for i, w := range wantLeft {
		if left[i] != w {
			t.Errorf("left row %d = %q, want %q", i, left[i], w)
		}
	}
	for _, l := range lines[2 : len(lines)-2] {
		if []rune(l)[35] != '│' {
			t.Errorf("rule column missing in %q", l)
		}
	}
	right := rightPane(lines)
	wantRight := []string{
		"Renew passport",
		"#errand  created Mon 31 Aug  in Todo since Tue 1 Sep",
		"",
		"NOTES",
		"Expires 14 Nov. Two photos, old passport, printed form.",
		"Appointment slots open on Mondays.",
		"",
		"CHECKLIST 1/4",
		"▣ Photos from the pharmacy",
		"▢ Fill in the form",
		"▢ Book appointment",
		"▢ Post the old one back",
		"",
		"BLOCKED",
		"— not blocked. b to set a reason",
	}
	for i, w := range wantRight {
		if right[i] != w {
			t.Errorf("right row %d = %q, want %q", i, right[i], w)
		}
	}
	if !strings.HasPrefix(lines[39], " j/k item  x toggle  o new item  T title  e edit notes  t tag  m move  b block  esc back") {
		t.Errorf("footer = %q", lines[39])
	}
}

func TestDetailEmptySections(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = press(m, "l", "l", "enter") // Cancel gym membership: no notes, no checklist
	right := rightPane(plainLines(m))
	joined := strings.Join(right, "\n")
	for _, w := range []string{"— no notes. e to write", "\nCHECKLIST\n", "— not blocked. b to set a reason"} {
		if !strings.Contains(joined, w) {
			t.Errorf("missing %q in:\n%s", w, joined)
		}
	}
	m = newTestModel(t, 120, 40)
	m = press(m, "l", "enter") // Fix bike brake: blocked
	if joined := strings.Join(rightPane(plainLines(m)), "\n"); !strings.Contains(joined, "BLOCKED\n⊘ waiting on pads") {
		t.Errorf("blocked reason missing:\n%s", joined)
	}
}

func TestDetailChecklistCursorAndToggle(t *testing.T) {
	root := t.TempDir()
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes = sampleBoard(t).Lanes
	m := New(Options{Store: st, Board: b, Styles: testStyles, Now: func() time.Time { return fixedNow }, Width: 120, Height: 40})
	m = press(m, "enter", "j", "x")
	c := m.b.Lanes[board.Todo][0]
	if !c.Checklist[1].Done {
		t.Fatalf("x should toggle the second item: %+v", c.Checklist)
	}
	if right := rightPane(plainLines(m)); right[7] != "CHECKLIST 2/4" || right[9] != "▣ Fill in the form" {
		t.Errorf("progress/row not updated: %q %q", right[7], right[9])
	}
	data, _ := os.ReadFile(st.BoardPath())
	if !strings.Contains(string(data), "- [x] Fill in the form") {
		t.Errorf("toggle should be saved:\n%s", data)
	}
	m = press(m, "k", "k") // wraps to the last item
	if m.detail.cursor != 3 {
		t.Errorf("cursor should wrap, got %d", m.detail.cursor)
	}
	m = press(m, "x", "x")
	if c.Checklist[3].Done {
		t.Errorf("double toggle restores")
	}
}

func TestDetailNewAndEditItem(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = press(m, "enter", "o")
	if m.mode != modeEdit {
		t.Fatal("o opens an input")
	}
	right := rightPane(plainLines(m))
	if right[9] != "▢" || right[10] != "▢ Fill in the form" {
		t.Errorf("new item row should appear under the cursor: %q / %q", right[9], right[10])
	}
	m = typeText(m, "Call the embassy")
	m = press(m, "enter")
	c := m.b.Lanes[board.Todo][0]
	if len(c.Checklist) != 5 || c.Checklist[1].Text != "Call the embassy" || m.detail.cursor != 1 {
		t.Fatalf("new item inserted after the cursor: %+v cursor=%d", c.Checklist, m.detail.cursor)
	}
	m = press(m, "o")
	m = typeText(m, "discard me")
	m = press(m, "esc")
	if len(c.Checklist) != 5 || m.mode != modeNormal {
		t.Errorf("esc cancels the new item")
	}
	m = press(m, "o", "enter")
	if len(c.Checklist) != 5 {
		t.Errorf("empty new item is dropped")
	}
	m = press(m, "enter") // edit "Call the embassy"
	m = typeText(m, " today")
	m = press(m, "enter")
	if c.Checklist[1].Text != "Call the embassy today" {
		t.Errorf("edit in place: %q", c.Checklist[1].Text)
	}
}

func TestDetailNotesTagBlock(t *testing.T) {
	root := t.TempDir()
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes = sampleBoard(t).Lanes
	m := New(Options{Store: st, Board: b, Styles: testStyles, Now: func() time.Time { return fixedNow }, Width: 120, Height: 40})
	m = press(m, "enter", "e")
	if m.mode != modeEdit {
		t.Fatal("e opens the notes editor")
	}
	m = typeText(m, " Bring cash.")
	m = press(m, "enter")
	m = typeText(m, "Second line.")
	m = press(m, "ctrl+s")
	c := m.b.Lanes[board.Todo][0]
	wantNotes := "Expires 14 Nov. Two photos, old passport, printed form. Appointment slots open on Mondays. Bring cash.\nSecond line."
	if c.Notes != wantNotes || m.mode != modeNormal {
		t.Fatalf("notes = %q", c.Notes)
	}
	// The value above is two paragraphs. Assert the pane actually renders it
	// as two — this test has built multi-line notes since it was written and
	// never looked at the screen, which is how wrapNotes flattening every
	// newline went unnoticed.
	right := rightPane(plainLines(m))
	var notesRows []string
	for i, l := range right {
		if strings.TrimSpace(l) == "NOTES" {
			for _, r := range right[i+1:] {
				if strings.HasPrefix(strings.TrimSpace(r), "CHECKLIST") {
					break
				}
				notesRows = append(notesRows, strings.TrimSpace(r))
			}
			break
		}
	}
	if len(notesRows) == 0 {
		t.Fatalf("no NOTES section in the right pane:\n%s", strings.Join(right, "\n"))
	}
	if !strings.Contains(strings.Join(notesRows, "\n"), "Second line.") {
		t.Errorf("second paragraph missing from the pane:\n%s", strings.Join(notesRows, "\n"))
	}
	// "Second line." must start a row of its own, not be joined onto the tail
	// of the first paragraph by a space.
	var onOwnRow bool
	for _, r := range notesRows {
		if r == "Second line." {
			onOwnRow = true
		}
	}
	if !onOwnRow {
		t.Errorf("second paragraph did not start its own row:\n%s", strings.Join(notesRows, "\n"))
	}

	m = press(m, "e")
	m = typeText(m, "zzz")
	m = press(m, "esc")
	if c.Notes != wantNotes {
		t.Errorf("esc cancels notes edit: %q", c.Notes)
	}
	m = press(m, "t")
	m = typeText(m, "s")
	m = press(m, "enter")
	if c.Tag != "errands" {
		t.Errorf("tag edit appends to the existing tag: %q", c.Tag)
	}
	m = press(m, "t", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace")
	m = typeText(m, "#money")
	m = press(m, "enter")
	if c.Tag != "money" {
		t.Errorf("leading # is stripped: %q", c.Tag)
	}
	m = press(m, "b")
	m = typeText(m, "need photos")
	m = press(m, "enter")
	if !c.Blocked || c.BlockedReason != "need photos" {
		t.Errorf("b sets a reason: %v %q", c.Blocked, c.BlockedReason)
	}
	if joined := strings.Join(rightPane(plainLines(m)), "\n"); !strings.Contains(joined, "BLOCKED\n⊘ need photos") {
		t.Errorf("blocked row:\n%s", joined)
	}
	m = press(m, "b")
	for range "need photos" {
		m = press(m, "backspace")
	}
	m = press(m, "enter")
	if c.Blocked {
		t.Errorf("empty reason clears the block")
	}
	data, _ := os.ReadFile(st.BoardPath())
	parsed, _, err := store.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	got := parsed.Lanes[board.Todo][0]
	if got.Notes != wantNotes || got.Tag != "money" || got.Blocked {
		t.Errorf("edits should round-trip through the store: %+v", got)
	}
}

func TestDetailMovePicker(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = press(m, "enter", "m")
	if m.mode != modeLanePick {
		t.Fatal("m opens the lane picker")
	}
	lines := plainLines(m)
	if !strings.HasPrefix(lines[39], " move to:  1 Backlog  2 Todo  3 Doing  4 Done") {
		t.Errorf("picker footer = %q", lines[39])
	}
	m = press(m, "esc")
	if m.mode != modeNormal || m.lane != board.Todo {
		t.Errorf("esc cancels the picker")
	}
	m = press(m, "m", "3")
	if m.mode != modeNormal || m.scr != screenDetail || m.lane != board.Doing {
		t.Errorf("3 moves to Doing and stays in detail: mode=%v scr=%v lane=%v", m.mode, m.scr, m.lane)
	}
	if got := titles(m.b.Lanes[board.Doing]); got[0] != "Renew passport" {
		t.Errorf("card should be at the top of Doing: %v", got)
	}
	lines = plainLines(m)
	if !strings.HasPrefix(lines[0], " kando  life › Doing › Renew passport") || leftPane(lines)[0] != "Doing 3" {
		t.Errorf("header/left pane after move: %q / %q", lines[0], leftPane(lines)[0])
	}
	m = press(m, "m", "4")
	if !m.b.Lanes[board.Done][0].DoneAt.Equal(fixedNow) {
		t.Errorf("moving to Done stamps DoneAt")
	}
}

func TestDetailSwitchCardAndBack(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = press(m, "enter", "j", "J")
	lines := plainLines(m)
	if !strings.HasPrefix(lines[0], " kando  life › Todo › Tax docs to accountant") || m.detail.cursor != 0 {
		t.Errorf("J switches to the next card and resets the cursor: %q %d", lines[0], m.detail.cursor)
	}
	if left := leftPane(lines); left[3] != "• Renew passport" || left[4] != "▸ Tax docs to accountant" {
		t.Errorf("left pane cursor: %v", left[3:6])
	}
	m = press(m, "K", "K")
	if !strings.HasPrefix(plainLines(m)[0], " kando  life › Todo › Book dentist") {
		t.Errorf("K wraps to the last card")
	}
	m = press(m, "esc")
	if m.scr != screenBoard || m.sel != 2 {
		t.Errorf("esc returns to the board with the card selected: scr=%v sel=%d", m.scr, m.sel)
	}
}

var detailStates = map[string][]string{
	"detail":          {"enter"},
	"detail notes":    {"enter", "e"},
	"detail tag":      {"enter", "t"},
	"detail new item": {"enter", "o"},
	"detail picker":   {"enter", "m"},
	"detail block":    {"enter", "b"},
	"detail done":     {"l", "l", "enter"},
	"detail help":     {"enter", "?"},
}

func TestWidthInvariantsDetail(t *testing.T) {
	for name, keys := range detailStates {
		for _, sz := range invariantSizes {
			m := newTestModel(t, sz[0], sz[1])
			m = press(m, keys...)
			checkInvariants(t, name, m, sz[0], sz[1])
		}
	}
}

func TestDetailTitleEdit(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = press(m, "enter", "T")
	if m.mode != modeEdit || m.detail.edit != editTitle {
		t.Fatalf("T opens the title editor: mode=%v edit=%v", m.mode, m.detail.edit)
	}
	if right := rightPane(plainLines(m)); !strings.HasPrefix(right[0], "Renew passport") {
		t.Errorf("the input should start from the current title: %q", right[0])
	}
	m = typeText(m, " soon")
	m = press(m, "enter")
	c := m.b.Lanes[board.Todo][0]
	if c.Title != "Renew passport soon" || m.mode != modeNormal {
		t.Fatalf("title = %q, mode = %v", c.Title, m.mode)
	}
	m = press(m, "T")
	m = typeText(m, " discarded")
	m = press(m, "esc")
	if c.Title != "Renew passport soon" || m.mode != modeNormal {
		t.Errorf("esc cancels the title edit: %q", c.Title)
	}
	// An empty title is refused, as it is on the web (board.SetTitle).
	m = press(m, "T")
	for range "Renew passport soon" {
		m = press(m, "backspace")
	}
	m = press(m, "enter")
	if c.Title != "Renew passport soon" {
		t.Errorf("an empty title must be a no-op, got %q", c.Title)
	}
}

func TestArchiveRestoreWritesBothFiles(t *testing.T) {
	root := t.TempDir()
	st, _, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, 120, 40)
	m.st = st
	m.b.Name = "life"
	m = press(m, "D") // archive screen
	before := len(m.archive.Cards)
	m = press(m, "u")
	if len(m.archive.Cards) != before-1 || m.errs.first() != nil {
		t.Fatalf("restore: archive=%d err=%v", len(m.archive.Cards), m.errs.first())
	}
	b, err := store.Load(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	a, err := store.LoadArchive(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Lanes[board.Doing]) == 0 {
		t.Errorf("the restored card must be on the saved board")
	}
	if len(a.Cards) != before-1 {
		t.Errorf("archive.md has %d cards, want %d", len(a.Cards), before-1)
	}
}

func TestDetailMoveRefusesAnArchivedTwin(t *testing.T) {
	root := t.TempDir()
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes = sampleBoard(t).Lanes
	a := sampleArchive(t)
	const title = "Cancel gym membership"
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
	m = press(m, "enter")
	if !m.detail.archived {
		t.Fatal("expected an archived detail view")
	}
	lane := m.lane
	backlog, archived := len(m.b.Lanes[board.Backlog]), len(m.archive.Cards)
	boardBefore, archiveBefore := mustReadFile(t, st.BoardPath()), mustReadFile(t, st.ArchivePath())
	past := pinMtimes(t, st.BoardPath(), st.ArchivePath())

	// m then 1: move the open archived card to Backlog — the lane picker's
	// unarchive, the second restore path and the one with no web counterpart.
	m = press(m, "m", "1")

	if m.scr != screenDetail || !m.detail.archived || m.mode != modeNormal {
		t.Errorf("a refused move must leave the archived card open: screen=%v archived=%v mode=%v", m.scr, m.detail.archived, m.mode)
	}
	if len(m.b.Lanes[board.Backlog]) != backlog || len(m.archive.Cards) != archived {
		t.Errorf("nothing may move: Backlog %d -> %d, archive %d -> %d",
			backlog, len(m.b.Lanes[board.Backlog]), archived, len(m.archive.Cards))
	}
	if m.lane != lane {
		t.Errorf("a refused move must not switch lanes: %v -> %v", lane, m.lane)
	}
	if string(mustReadFile(t, st.BoardPath())) != string(boardBefore) || string(mustReadFile(t, st.ArchivePath())) != string(archiveBefore) {
		t.Error("a refused move must write nothing")
	}
	if !unwritten(past, st.BoardPath(), st.ArchivePath()) {
		t.Error("a refused move must not touch either file — a rewrite of identical bytes still moves the mtime")
	}
	want := fmt.Sprintf("%q is already on the board", title)
	if !strings.Contains(m.notice, want) {
		t.Errorf("notice = %q, want %q", m.notice, want)
	}
	if footer := plainLines(m)[39]; !strings.HasPrefix(footer, " j/k item") || !strings.Contains(footer, "⊘ "+want) {
		t.Errorf("the detail footer must show the refusal: %q", footer)
	}
}

func TestDetailListCursorAgreesWithTheCardShown(t *testing.T) {
	// Parsing keeps ids unique within a file, but a Board built in memory can
	// still hold two cards sharing one — the unguarded restore this branch
	// closed was one way in. detailCard resolves the open card through
	// Board.Find, which takes the first match; detailList has to take the same
	// one, or the left pane's cursor sits on one card while the right pane shows
	// another, and J/K step from the wrong place.
	m := newTestModel(t, 120, 40)
	todo := m.b.Lanes[board.Todo]
	todo[1].ID = todo[0].ID
	m = press(m, "enter")
	if c := m.detailCard(); c == nil || c.Title != "Renew passport" {
		t.Fatalf("detailCard = %v, want Renew passport", c)
	}
	if _, cur := m.detailList(); cur != 0 {
		t.Errorf("detailList cursor = %d, want 0 — the card detailCard shows", cur)
	}
	if left := leftPane(plainLines(m)); left[3] != "▸ Renew passport" {
		t.Errorf("the left pane's cursor must be on the card shown: %v", left[3:6])
	}
}
