package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	if m.err == nil {
		t.Fatal("setup: a reload that cannot read board.md must say so")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	if m.err != nil || strings.Contains(plainLines(m)[39], "⊘") {
		t.Errorf("the reload that reads the file again must clear its error: err=%v footer=%q", m.err, plainLines(m)[39])
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
	if m.err == nil {
		t.Fatal("setup: the save must fail")
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dir+".away", dir); err != nil {
		t.Fatal(err)
	}
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	if m.err == nil {
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
	if m.err == nil {
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
	if m.err == nil {
		t.Fatalf("the archive is still unreadable, so its error must stay")
	}

	if err := os.WriteFile(arch, []byte("## 2026-W36\n\n### Orphan card\nid: aaaaaaaa\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(arch, later.Add(time.Hour), later.Add(time.Hour))
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	if m.err != nil || strings.Contains(plainLines(m)[39], "⊘") {
		t.Errorf("the reload that reads the fixed archive must clear its error: err=%v footer=%q", m.err, plainLines(m)[39])
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
	if m.err == nil {
		t.Fatal("setup: the reload must fail")
	}
	// board.md is still a directory, so the rename over it fails.
	m = press(m, "d")
	if m.err == nil {
		t.Fatal("setup: the save must fail")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	if m.err == nil {
		t.Errorf("a failed save must survive a reload, even one that follows a failed reload")
	}
}
