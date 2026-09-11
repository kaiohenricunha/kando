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

// A failed reload left its error in the footer until the next successful save,
// even after a later reload read the files fine. An error that only reading can
// fix is now cleared by the read that fixes it. A failed save is not: reading
// the files again does not put the lost edit on disk.
func TestAReloadClearsTheErrorOfAFailedReload(t *testing.T) {
	m, root := newRootModel(t, 120, 40)
	path := filepath.Join(root, "life", "board.md")
	data := mustReadFile(t, path)
	// board.md replaced by a directory: the next reload cannot read it.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	if m.errs.reload == nil {
		t.Fatal("setup: a reload that cannot read board.md must say so")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	if m.errs.reload != nil || strings.Contains(plainLines(m)[39], "⊘") {
		t.Errorf("the reload that reads the file again must clear its error: err=%v footer=%q", m.errs.reload, plainLines(m)[39])
	}
}

func TestAReloadKeepsTheErrorOfAFailedSave(t *testing.T) {
	m, root := newRootModel(t, 120, 40)
	dir := filepath.Join(root, "life")
	// The board directory replaced by a file: the next save cannot write.
	if err := os.Rename(dir, dir+".away"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	m = press(m, "d")
	if m.errs.save == nil {
		t.Fatal("setup: the save must fail")
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dir+".away", dir); err != nil {
		t.Fatal(err)
	}
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	if m.errs.save == nil {
		t.Errorf("reading the files again does not put the failed save on disk, so its error must stay")
	}
}

func TestAReloadThatReadsTheArchiveClearsItsReadError(t *testing.T) {
	m, root := newRootModel(t, 120, 40)
	arch := filepath.Join(root, "life", "archive.md")
	if err := os.WriteFile(arch, []byte("### Orphan card\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m = press(m, "l", "l", "A")
	if m.errs.archive == nil {
		t.Fatal("setup: A must report an archive it cannot read")
	}

	// A reload that changes only board.md reads no usable archive, so the
	// archive's error must stay.
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes[0][0].Title = "Learn to make rye sourdough"
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	later := time.Unix(2_000_000_000, 0)
	os.Chtimes(st.BoardPath(), later, later)
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	if m.errs.archive == nil {
		t.Fatalf("the archive is still unreadable, so its error must stay")
	}

	if err := os.WriteFile(arch, []byte("## 2026-W36\n\n### Orphan card\nid: aaaaaaaa\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(arch, later.Add(time.Hour), later.Add(time.Hour))
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	if m.errs.archive != nil || strings.Contains(plainLines(m)[39], "⊘") {
		t.Errorf("the reload that reads the fixed archive must clear its error: err=%v footer=%q", m.errs.archive, plainLines(m)[39])
	}
}

// The origin has to be set wherever err is. A save that fails after a failed
// reload is a save error: the next good reload must not clear it as if it were
// still the reload's.
func TestAFailedSaveAfterAFailedReloadSurvivesTheNextReload(t *testing.T) {
	m, root := newRootModel(t, 120, 40)
	path := filepath.Join(root, "life", "board.md")
	data := mustReadFile(t, path)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	if m.errs.reload == nil {
		t.Fatal("setup: the reload must fail")
	}
	// board.md is still a directory, so the rename over it fails.
	m = press(m, "d")
	if m.errs.save == nil {
		t.Fatal("setup: the save must fail")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	if m.errs.save == nil {
		t.Errorf("a failed save must survive a reload, even one that follows a failed reload")
	}
}

// breakBoardDir replaces the board's directory with a plain file, so every read
// and write inside it fails, and returns the function that puts it back.
func breakBoardDir(t *testing.T, root string) func() {
	t.Helper()
	dir := filepath.Join(root, "life")
	if err := os.Rename(dir, dir+".away"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	return func() {
		t.Helper()
		if err := os.Remove(dir); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(dir+".away", dir); err != nil {
			t.Fatal(err)
		}
	}
}

// One error slot took its origin from the last writer, so a read failure could
// replace a failed save, and the next good read then cleared it while the edit
// was still only in memory. Each condition now has its own slot, and the footer
// shows the failed save first.
func TestAFailedReloadDoesNotHideAFailedSave(t *testing.T) {
	m, root := newRootModel(t, 120, 40)
	restore := breakBoardDir(t, root)
	m = press(m, "d")
	saved := plainLines(m)[39]
	if !strings.Contains(saved, "⊘") {
		t.Fatalf("setup: the save must fail: %q", saved)
	}
	m, _ = feed(m, changeMsg{gen: m.watchGen}) // board.md cannot be read either
	if got := plainLines(m)[39]; got != saved {
		t.Errorf("a failed reload must not replace the failed save in the footer:\n got %q\nwant %q", got, saved)
	}
	restore()
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	if got := plainLines(m)[39]; got != saved {
		t.Errorf("a good reload must not clear a save that never reached disk:\n got %q\nwant %q", got, saved)
	}
}

func TestAFailedArchiveReadDoesNotHideAFailedSave(t *testing.T) {
	m, root := newRootModel(t, 120, 40)
	restore := breakBoardDir(t, root)
	m = press(m, "d")
	saved := plainLines(m)[39]
	if !strings.Contains(saved, "⊘") {
		t.Fatalf("setup: the save must fail: %q", saved)
	}
	m = press(m, "D") // archive.md cannot be read: its directory is a file
	if m.scr != screenBoard {
		m = press(m, "esc")
	}
	if got := plainLines(m)[39]; got != saved {
		t.Errorf("an unreadable archive must not replace the failed save in the footer:\n got %q\nwant %q", got, saved)
	}
	restore()
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	if got := plainLines(m)[39]; got != saved {
		t.Errorf("reading the archive again must not clear a save that never reached disk:\n got %q\nwant %q", got, saved)
	}
}

// CheckReload can read a changed board.md and then fail on archive.md. It
// returns the board with the error, and reload used to drop both, so the TUI
// kept a stale board that its next unchecked save would write over the change.
func TestAReloadAppliesTheBoardItReadBeforeAnArchiveError(t *testing.T) {
	m, root := newRootModel(t, 120, 40)
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes[board.Backlog][0].Title = "Learn to make rye sourdough"
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	later := time.Unix(2_000_000_000, 0)
	os.Chtimes(st.BoardPath(), later, later)
	arch := filepath.Join(root, "life", "archive.md")
	if err := os.Mkdir(arch, 0o755); err != nil { // archive.md cannot be read
		t.Fatal(err)
	}
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	if got := m.b.Lanes[board.Backlog][0].Title; got != "Learn to make rye sourdough" {
		t.Errorf("the board read before the archive error must be applied, got %q", got)
	}
	if !strings.Contains(plainLines(m)[39], "⊘") {
		t.Errorf("the archive error must still be reported: %q", plainLines(m)[39])
	}
	if err := os.Remove(arch); err != nil {
		t.Fatal(err)
	}
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	if strings.Contains(plainLines(m)[39], "⊘") || m.b.Lanes[board.Backlog][0].Title != "Learn to make rye sourdough" {
		t.Errorf("a good reload must clear the error and keep the change: footer %q title %q", plainLines(m)[39], m.b.Lanes[board.Backlog][0].Title)
	}
}

// A reload that shortens the open card's checklist used to leave the cursor past
// the end, and enter then indexed past it and crashed the TUI.
func TestAReloadClampsTheChecklistCursor(t *testing.T) {
	m, root := newRootModel(t, 120, 40)
	m = press(m, "enter", "j", "j", "j")
	if m.scr != screenDetail || m.detail.cursor != 3 {
		t.Fatalf("setup: expected the cursor on the fourth item, got screen %v cursor %d", m.scr, m.detail.cursor)
	}
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	_, _, c := b.Find("k7q2m9ab")
	c.Checklist = c.Checklist[:2]
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	later := time.Unix(2_000_000_000, 0)
	os.Chtimes(st.BoardPath(), later, later)
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	if m.detail.cursor != 1 {
		t.Fatalf("the cursor must move onto the last item left, got %d", m.detail.cursor)
	}
	if m = press(m, "enter"); m.mode != modeEdit {
		t.Errorf("enter must edit the item under the cursor, got mode %v", m.mode)
	}
}

// A load that succeeds is the plainest proof that archive.md reads again.
func TestASuccessfulArchiveLoadClearsItsReadError(t *testing.T) {
	m, root := newRootModel(t, 120, 40)
	arch := filepath.Join(root, "life", "archive.md")
	if err := os.WriteFile(arch, []byte("### Orphan card\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m = press(m, "D")
	if !strings.Contains(plainLines(m)[39], "⊘") {
		t.Fatalf("setup: an archive that cannot be read must be reported: %q", plainLines(m)[39])
	}
	if err := os.WriteFile(arch, []byte("## 2026-W36\n\n### Orphan card\nid: aaaaaaaa\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if m.scr != screenBoard {
		m = press(m, "esc")
	}
	m = press(m, "D")
	if m.archive == nil || strings.Contains(plainLines(m)[39], "⊘") {
		t.Errorf("the load that succeeds must clear the archive's read error: archive %v footer %q", m.archive != nil, plainLines(m)[39])
	}
}

// An undo can put back bytes the store has already seen. The check then reports
// no change, so a reload retries the load instead of waiting for one.
func TestAReloadRetriesAnArchiveThatAnUndoRestored(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "life")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	arch := filepath.Join(dir, "archive.md")
	good := []byte("## 2026-W36\n\n### Orphan card\nid: aaaaaaaa\n")
	if err := os.WriteFile(arch, good, 0o644); err != nil {
		t.Fatal(err)
	}
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes = sampleBoard(t).Lanes
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	m := New(Options{Store: st, Board: b, Root: root, Styles: testStyles, Now: func() time.Time { return fixedNow }, Width: 120, Height: 40})
	t1, t2 := time.Unix(2_000_000_000, 0), time.Unix(2_000_003_600, 0)
	if err := os.WriteFile(arch, []byte("### Orphan card\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(arch, t1, t1)
	m = press(m, "D")
	if m.scr != screenBoard {
		m = press(m, "esc")
	}
	if !strings.Contains(plainLines(m)[39], "⊘") {
		t.Fatalf("setup: the broken archive must be reported: %q", plainLines(m)[39])
	}
	if err := os.WriteFile(arch, good, 0o644); err != nil { // the undo
		t.Fatal(err)
	}
	os.Chtimes(arch, t2, t2)
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	if strings.Contains(plainLines(m)[39], "⊘") {
		t.Errorf("the reload after the undo must clear the archive's read error: %q", plainLines(m)[39])
	}
}

// Every write path keeps its failure across a reload that reads fine: reading
// the files again does not put the lost edit on disk.
func TestEverySaveKeepsItsErrorAcrossAGoodReload(t *testing.T) {
	cases := []struct {
		name   string
		before []string
		fail   func(Model) Model
	}{
		{"save", nil, func(m Model) Model { return press(m, "d") }},
		{"saveRestore", []string{"D"}, func(m Model) Model { return press(m, "u") }},
		{"saveArchival", []string{"D", "esc"}, func(m Model) Model { return press(m, "l", "l", "A") }},
		{"saveArchive", []string{"D", "enter"}, func(m Model) Model { return press(typeText(press(m, "t"), "x"), "enter") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, root := newRootModel(t, 120, 40)
			if err := m.st.SaveArchive(sampleArchive(t)); err != nil {
				t.Fatal(err)
			}
			m = press(m, c.before...)
			restore := breakBoardDir(t, root)
			m = c.fail(m)
			failed := plainLines(m)[39]
			if !strings.Contains(failed, "⊘") {
				t.Fatalf("setup: the %s write must fail: %q", c.name, failed)
			}
			restore()
			m, _ = feed(m, changeMsg{gen: m.watchGen})
			if got := plainLines(m)[39]; got != failed {
				t.Errorf("%s: a good reload must not clear the failed write:\n got %q\nwant %q", c.name, got, failed)
			}
		})
	}
}

// A successful save used to clear "file watching disabled" too, while live
// reload stayed off, and a watcher that did start never cleared it.
func TestAWatcherThatStartsClearsItsWarning(t *testing.T) {
	m, _ := newRootModel(t, 120, 40)
	m.watch = true
	m, _ = feed(m, watchStartedMsg{gen: 0, err: os.ErrPermission})
	if !strings.Contains(plainLines(m)[39], "⊘ file watching disabled") {
		t.Fatalf("setup: the watcher failure must be reported: %q", plainLines(m)[39])
	}
	m, _ = feed(m, watchStartedMsg{gen: 0, ch: make(chan struct{}), stop: func() {}})
	if strings.Contains(plainLines(m)[39], "⊘") {
		t.Errorf("a watcher that starts must clear the warning: %q", plainLines(m)[39])
	}
}
