package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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

// runCmd executes a tea.Cmd with a timeout and returns its message.
func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command")
	}
	out := make(chan tea.Msg, 1)
	go func() { out <- cmd() }()
	select {
	case msg := <-out:
		return msg
	case <-time.After(5 * time.Second):
		t.Fatal("command did not complete in time")
		return nil
	}
}

func feed(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func TestWatcherLifecycleAcrossBoardSwitch(t *testing.T) {
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
	m := New(Options{Store: st, Board: b, Root: root, Styles: testStyles, Watch: true, Now: func() time.Time { return fixedNow }, Width: 120, Height: 40})

	// Init starts the first watcher; the model records it and arms a listener.
	started := runCmd(t, m.Init())
	if _, ok := started.(watchStartedMsg); !ok {
		t.Fatalf("Init should yield watchStartedMsg, got %T", started)
	}
	m, listen := feed(m, started)
	if m.changes == nil || m.stopWatch == nil || m.err != nil {
		t.Fatalf("watcher not recorded: changes=%v stop=%v err=%v", m.changes != nil, m.stopWatch != nil, m.err)
	}
	// An external edit reaches the model through that listener.
	edited := strings.Replace(string(store.Marshal(b)), "### Renew passport", "### Renew passport soon", 1)
	if err := os.WriteFile(st.BoardPath(), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	msg := runCmd(t, listen)
	if cm, ok := msg.(changeMsg); !ok || cm.gen != 0 {
		t.Fatalf("expected changeMsg{gen:0}, got %#v", msg)
	}
	m, listen = feed(m, msg)
	if m.b.Lanes[board.Todo][0].Title != "Renew passport soon" {
		t.Fatalf("reload did not apply: %v", titles(m.b.Lanes[board.Todo]))
	}

	// Switching boards stops the old watcher (its listener sees the close) and
	// starts a new one with the next generation.
	oldCh := m.changes
	m = press(m, "B", "j")
	m, startNew := feed(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.b.Name != "work" || m.watchGen != 1 || m.changes != nil {
		t.Fatalf("after switch: name=%q gen=%d changes=%v", m.b.Name, m.watchGen, m.changes != nil)
	}
	// Drain contract (store.Watch): one coalesced signal buffered before the
	// stop may still arrive ahead of the close; the model ignores it (stale
	// gen) and the listener keeps receiving until the channel closes.
	stopped := runCmd(t, listen)
	for _, stale := stopped.(changeMsg); stale; _, stale = stopped.(changeMsg) {
		stopped = runCmd(t, waitChange(oldCh, 0))
	}
	if stopped != (watchStoppedMsg{gen: 0}) {
		t.Fatalf("old listener should observe the close, got %#v", stopped)
	}
	if _, cmd := feed(m, watchStoppedMsg{gen: 0}); cmd != nil {
		t.Errorf("watchStoppedMsg must not re-arm anything")
	}
	if _, cmd := feed(m, changeMsg{gen: 0}); cmd != nil {
		t.Errorf("a stale changeMsg from the old watcher must be ignored")
	}
	started = runCmd(t, startNew)
	if sm, ok := started.(watchStartedMsg); !ok || sm.gen != 1 || sm.err != nil {
		t.Fatalf("expected watchStartedMsg{gen:1}, got %#v", started)
	}
	m, listen = feed(m, started)
	if m.changes == nil || m.stopWatch == nil {
		t.Fatal("new watcher not recorded")
	}
	// A stale start result (gen 0) arriving late is discarded, not adopted.
	dummyStopped := false
	m2, cmd := feed(m, watchStartedMsg{gen: 0, ch: make(chan struct{}), stop: func() { dummyStopped = true }})
	if cmd != nil || !dummyStopped || m2.changes != m.changes {
		t.Errorf("stale watchStartedMsg should be stopped and ignored")
	}
	// Quit tears the watcher down: the listener sees the close.
	if _, cmd := feed(m, tea.KeyMsg{Type: tea.KeyCtrlC}); cmd == nil {
		t.Fatal("ctrl+c should quit")
	}
	if stopped := runCmd(t, listen); stopped != (watchStoppedMsg{gen: 1}) {
		t.Fatalf("quit should stop the watcher, got %#v", stopped)
	}
}

func TestWatchFailureIsShownAndRetried(t *testing.T) {
	m, _ := newRootModel(t, 120, 40)
	m.watch = true
	m, cmd := feed(m, watchStartedMsg{gen: 0, err: os.ErrPermission})
	if cmd != nil || m.err == nil || !strings.Contains(plainLines(m)[39], "⊘ file watching disabled") {
		t.Errorf("a watch failure should be surfaced in the footer: err=%v footer=%q", m.err, plainLines(m)[39])
	}
	m = press(m, "B", "j")
	m, cmd = feed(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || m.watchGen != 1 {
		t.Errorf("the next switch must retry watching: cmd=%v gen=%d", cmd != nil, m.watchGen)
	}
}

func TestDeleteActsOnThePaintedFilterSet(t *testing.T) {
	clock := fixedNow
	m := New(Options{Board: sampleBoard(t), Styles: testStyles, Now: func() time.Time { return clock }, Width: 120, Height: 40})
	m = press(m, "/")
	m = typeText(m, "age<3d")
	m = press(m, "enter") // Todo shows Tax docs (2d) and Book dentist (1d); Tax docs is selected
	if c := m.selectedCard(); c == nil || c.Title != "Tax docs to accountant" {
		t.Fatalf("setup: selected %v", c)
	}
	clock = clock.Add(24 * time.Hour) // Tax docs is now 3d and would fall out of a fresh evaluation
	m = press(m, "x")
	if got := titles(m.b.Lanes[board.Todo]); len(got) != 2 || got[0] != "Renew passport" || got[1] != "Book dentist" {
		t.Errorf("x must delete the card that was highlighted when the frame was painted: %v", got)
	}
}

func TestFooterShowsSaveError(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m.err = os.ErrPermission
	if !strings.Contains(plainLines(m)[39], "⊘ permission denied") {
		t.Errorf("footer = %q", plainLines(m)[39])
	}
}

func TestBoardPickerHidesUnopenableDirs(t *testing.T) {
	m, root := newRootModel(t, 120, 40)
	os.MkdirAll(filepath.Join(root, ".trash"), 0o755)
	os.WriteFile(filepath.Join(root, ".trash", "board.md"), store.Marshal(&board.Board{}), 0o644)
	m = press(m, "B")
	if view := plainView(m); strings.Contains(view, ".trash") || !strings.HasSuffix(plainLines(m)[0], "2 boards ") {
		t.Errorf("picker must list only openable boards: %q", plainLines(m)[0])
	}
}

func TestBoardPickerReportsListError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	os.WriteFile(file, []byte("x"), 0o644)
	m := New(Options{Board: sampleBoard(t), Root: file, Styles: testStyles, Now: func() time.Time { return fixedNow }, Width: 120, Height: 40})
	m = press(m, "B")
	if body := bodyRows(plainLines(m)); body[0] != "▸ life" || !strings.Contains(plainView(m), "⊘ ") {
		t.Errorf("an unreadable root should still show the open board and say why: %q", plainView(m))
	}
}
