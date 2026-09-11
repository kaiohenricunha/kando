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

// A refused u, A or archived-card move still changed memory before the save
// that was refused: the card moved between the board's lanes and the archive
// in RAM even though neither file was written. These pin that a failed save
// puts the move back, so a later save cannot lose the card that stayed on the
// side the disk never saw.

func TestARefusedRestoreLeavesTheArchiveUnchangedInMemory(t *testing.T) {
	m, root := newRootModel(t, 120, 40)
	if err := m.st.SaveArchive(sampleArchive(t)); err != nil {
		t.Fatal(err)
	}
	m, _ = feed(m, changeMsg{gen: m.watchGen}) // pick up the archive on disk
	m = press(m, "D")
	archived, doing := len(m.archive.Cards), len(m.b.Lanes[board.Doing])
	target := m.visibleArchive()[0]
	wantDoneAt, wantMovedAt := target.DoneAt, target.MovedAt
	restore := breakBoardDir(t, root)
	m = press(m, "u")
	if !strings.Contains(plainLines(m)[39], "⊘") {
		t.Fatalf("setup: the save must fail: %q", plainLines(m)[39])
	}
	if len(m.archive.Cards) != archived || len(m.b.Lanes[board.Doing]) != doing {
		t.Errorf("a refused restore must leave the archive and the board as they were: archive %d -> %d, Doing %d -> %d",
			archived, len(m.archive.Cards), doing, len(m.b.Lanes[board.Doing]))
	}
	if !target.DoneAt.Equal(wantDoneAt) || !target.MovedAt.Equal(wantMovedAt) {
		t.Errorf("a refused restore must undo the card's own stamp too: done=%v moved=%v", target.DoneAt, target.MovedAt)
	}
	restore()
	m = press(m, "u")
	if len(m.archive.Cards) != archived-1 || len(m.b.Lanes[board.Doing]) != doing+1 {
		t.Errorf("the retry must still be able to restore: archive %d doing %d", len(m.archive.Cards), len(m.b.Lanes[board.Doing]))
	}
}

func TestARefusedArchiveLeavesTheCardInDoneInMemory(t *testing.T) {
	m, root := newRootModel(t, 120, 40)
	m = press(m, "D", "esc") // loads the (empty) archive into memory first
	done := len(m.b.Lanes[board.Done])
	c := m.b.Lanes[board.Done][0]
	title := c.Title
	restore := breakBoardDir(t, root)
	m = press(m, "l", "l", "A")
	if !strings.Contains(plainLines(m)[39], "⊘") {
		t.Fatalf("setup: the save must fail: %q", plainLines(m)[39])
	}
	if len(m.b.Lanes[board.Done]) != done || m.b.Lanes[board.Done][0].Title != title {
		t.Errorf("a refused archive must leave the card in Done: %d cards, first %q", len(m.b.Lanes[board.Done]), m.b.Lanes[board.Done][0].Title)
	}
	if m.archive != nil {
		if _, dup := m.archive.Find(c.ID); dup != nil {
			t.Errorf("the card must not appear in the in-memory archive")
		}
	}
	restore()
	m = press(m, "l", "l", "A")
	if len(m.b.Lanes[board.Done]) != done-1 {
		t.Errorf("archiving after the fix must still work: %d cards left", len(m.b.Lanes[board.Done]))
	}
}

func TestARefusedArchiveFromDetailStaysOnTheCard(t *testing.T) {
	m, root := newRootModel(t, 120, 40)
	m = press(m, "D", "esc") // loads the (empty) archive into memory first
	m = press(m, "l", "l", "enter")
	if m.scr != screenDetail {
		t.Fatalf("setup: expected the detail screen")
	}
	restore := breakBoardDir(t, root)
	m = press(m, "A")
	if !strings.Contains(plainLines(m)[39], "⊘") {
		t.Fatalf("setup: the save must fail: %q", plainLines(m)[39])
	}
	if m.scr != screenDetail || m.detail.archived {
		t.Errorf("a refused archive from the detail screen must stay on the live card: scr=%v archived=%v", m.scr, m.detail.archived)
	}
	restore()
	m = press(m, "A")
	if m.scr != screenBoard {
		t.Errorf("archiving after the fix must still work: scr=%v", m.scr)
	}
}

func TestARefusedMoveOfAnArchivedCardKeepsItArchived(t *testing.T) {
	m, root := newRootModel(t, 120, 40)
	if err := m.st.SaveArchive(sampleArchive(t)); err != nil {
		t.Fatal(err)
	}
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	m = press(m, "D", "enter", "m")
	if m.scr != screenDetail || !m.detail.archived || m.mode != modeLanePick {
		t.Fatalf("setup: scr=%v archived=%v mode=%v", m.scr, m.detail.archived, m.mode)
	}
	id := m.detail.id
	archived := len(m.archive.Cards)
	restore := breakBoardDir(t, root)
	m = press(m, "2")
	if !strings.Contains(plainLines(m)[39], "⊘") {
		t.Fatalf("setup: the save must fail: %q", plainLines(m)[39])
	}
	if !m.detail.archived || m.detail.id != id {
		t.Errorf("a refused move must leave the detail archived and open: archived=%v id=%q", m.detail.archived, m.detail.id)
	}
	if len(m.archive.Cards) != archived {
		t.Errorf("the card must still be in the in-memory archive: %d cards", len(m.archive.Cards))
	}
	if _, _, c := m.b.Find(id); c != nil {
		t.Errorf("the card must not appear on the in-memory board")
	}
	restore()
}
