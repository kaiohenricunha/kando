package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

// newRootModel builds a store-backed model over a KANDO_HOME with two boards:
// "life" (the sample data, open) and "work" (empty).
func newRootModel(t *testing.T, w, h int) (Model, string) {
	t.Helper()
	root := t.TempDir()
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes = sampleBoard(t).Lanes
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Open(root, "work"); err != nil {
		t.Fatal(err)
	}
	m := New(Options{Store: st, Board: b, Root: root, Styles: testStyles, Now: func() time.Time { return fixedNow }, Width: w, Height: h})
	return m, root
}

func TestBoardPickerListsBoards(t *testing.T) {
	m, _ := newRootModel(t, 120, 40)
	m = press(m, "B")
	if m.scr != screenBoards {
		t.Fatal("B opens the board picker")
	}
	lines := plainLines(m)
	if !strings.HasPrefix(lines[0], " kando  life › boards") || !strings.HasSuffix(lines[0], "2 boards ") {
		t.Errorf("header = %q", lines[0])
	}
	body := bodyRows(lines)
	if body[0] != "▸ life" || body[1] != "• work" {
		t.Errorf("rows = %q %q (cursor should start on the open board)", body[0], body[1])
	}
	if !strings.HasPrefix(lines[39], " j/k move  enter open  n new board  esc back") {
		t.Errorf("footer = %q", lines[39])
	}
	m = press(m, "esc")
	if m.scr != screenBoard || m.b.Name != "life" {
		t.Errorf("esc returns to the current board")
	}
}

func TestBoardPickerEnterSwitches(t *testing.T) {
	m, root := newRootModel(t, 120, 40)
	m = press(m, "j", "l", "B") // move selection/lane first, so the reset is observable
	m = press(m, "j", "enter")
	if m.scr != screenBoard || m.b.Name != "work" || m.b.Count() != 0 {
		t.Fatalf("enter should open 'work': scr=%v name=%q count=%d", m.scr, m.b.Name, m.b.Count())
	}
	if m.lane != board.Todo || m.sel != 0 || m.first != 0 {
		t.Errorf("switching resets lane/selection: lane=%v sel=%d first=%d", m.lane, m.sel, m.first)
	}
	if !strings.HasPrefix(plainLines(m)[0], " kando  work ") {
		t.Errorf("header should show the new board: %q", plainLines(m)[0])
	}
	m = press(m, "a")
	m = typeText(m, "Work item")
	m = press(m, "enter")
	data, _ := os.ReadFile(filepath.Join(root, "work", "board.md"))
	if !strings.Contains(string(data), "### Work item") {
		t.Errorf("edits after switching must go to the new board's file:\n%s", data)
	}
	life, _ := os.ReadFile(filepath.Join(root, "life", "board.md"))
	if strings.Contains(string(life), "Work item") {
		t.Errorf("the old board must not be touched")
	}
	m = press(m, "B", "k", "enter") // wraps to "life"? k from index 1 → 0
	if m.b.Name != "life" || m.b.Count() != 11 {
		t.Errorf("switching back reloads the sample board: %q %d", m.b.Name, m.b.Count())
	}
}

func TestBoardPickerCreateNewBoard(t *testing.T) {
	m, root := newRootModel(t, 120, 40)
	m = press(m, "B", "n")
	if m.mode != modeBoardName {
		t.Fatal("n opens the new-board input")
	}
	if !strings.HasPrefix(plainLines(m)[39], " type a name  enter create  esc cancel") {
		t.Errorf("naming footer = %q", plainLines(m)[39])
	}
	m = typeText(m, "side")
	m = press(m, "enter")
	if m.scr != screenBoard || m.b.Name != "side" || m.mode != modeNormal {
		t.Fatalf("enter creates and opens the board: scr=%v name=%q mode=%v", m.scr, m.b.Name, m.mode)
	}
	if _, err := os.Stat(filepath.Join(root, "side", "board.md")); err != nil {
		t.Errorf("board.md should exist for the new board: %v", err)
	}
	names, _ := store.ListBoards(root)
	if len(names) != 3 {
		t.Errorf("ListBoards = %v", names)
	}
}

func TestBoardPickerNamingEscAndInvalid(t *testing.T) {
	m, root := newRootModel(t, 120, 40)
	m = press(m, "B", "n")
	m = typeText(m, "nope")
	m = press(m, "esc")
	if m.mode != modeNormal || m.scr != screenBoards || m.b.Name != "life" {
		t.Errorf("esc cancels naming and stays on the picker: mode=%v scr=%v", m.mode, m.scr)
	}
	m = press(m, "n")
	m = typeText(m, "../evil")
	m = press(m, "enter")
	if m.b.Name != "life" || m.scr != screenBoards {
		t.Errorf("an invalid name must not switch: name=%q scr=%v", m.b.Name, m.scr)
	}
	if !strings.Contains(plainView(m), "invalid board name") {
		t.Errorf("the picker should say why: %s", plainLines(m)[39])
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "evil")); err == nil {
		t.Fatal("traversal name escaped root")
	}
	m = press(m, "n", "enter") // empty name is a no-op
	if m.b.Name != "life" || m.mode != modeNormal {
		t.Errorf("empty name: name=%q mode=%v", m.b.Name, m.mode)
	}
}

func TestBoardPickerWithoutStore(t *testing.T) {
	m := newTestModel(t, 120, 40) // no Store, no Root
	m = press(m, "B")
	if body := bodyRows(plainLines(m)); body[0] != "▸ life" {
		t.Errorf("without a root the picker lists just the open board: %q", body[0])
	}
	m = press(m, "enter")
	if m.scr != screenBoard || m.b.Count() != 11 {
		t.Errorf("re-opening the same board keeps it: %v %d", m.scr, m.b.Count())
	}
}

var boardsStates = map[string][]string{
	"boards":        {"B"},
	"boards naming": {"B", "n", "s"},
	"boards help":   {"B", "?"},
	"boards end":    {"B", "k"},
}

func TestWidthInvariantsBoards(t *testing.T) {
	for name, keys := range boardsStates {
		for _, sz := range invariantSizes {
			m, _ := newRootModel(t, sz[0], sz[1])
			m = press(m, keys...)
			checkInvariants(t, name, m, sz[0], sz[1])
		}
	}
}

func TestGoldenBoards(t *testing.T) {
	m, _ := newRootModel(t, 120, 40)
	m = press(m, "B")
	got := plainView(m) + "\n"
	path := "testdata/boards_120x40.txt"
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	assertFrame(t, string(want), got, 120, 40)
}
