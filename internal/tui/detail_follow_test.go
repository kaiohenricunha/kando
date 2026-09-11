package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

// When a reload moved the open card to another lane, the detail pane kept
// listing the old lane. detailList found no card with the open id and reported
// cursor 0, so the pane highlighted a card that was not the one shown, and J
// stepped from that card.
func TestDetailFollowsItsCardToAnotherLaneOnReload(t *testing.T) {
	m, root := newRootModel(t, 120, 40)
	m = press(m, "enter")
	if m.scr != screenDetail || m.detail.id != "k7q2m9ab" {
		t.Fatalf("setup: expected Renew passport open, got screen %v id %q", m.scr, m.detail.id)
	}
	// Another process moves the card to the top of Doing.
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	l, i, c := b.Find("k7q2m9ab")
	if c == nil {
		t.Fatal("setup: the card is not on disk")
	}
	b.Move(l, i, board.Doing, fixedNow)
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	later := time.Unix(2_000_000_000, 0)
	if err := os.Chtimes(st.BoardPath(), later, later); err != nil {
		t.Fatal(err)
	}
	m, _ = feed(m, changeMsg{gen: m.watchGen})

	if m.scr != screenDetail || m.detail.id != "k7q2m9ab" {
		t.Fatalf("the card still exists, so its detail must stay open: screen %v id %q", m.scr, m.detail.id)
	}
	if m.lane != board.Doing {
		t.Errorf("the list must follow the card to Doing, got %v", m.lane)
	}
	if cards, cur := m.detailList(); cur < 0 || cur >= len(cards) || cards[cur].ID != "k7q2m9ab" {
		t.Errorf("the cursor must be on the open card: cur=%d", cur)
	}
	if !strings.Contains(plainView(m), "▸ Renew passport") {
		t.Errorf("the left pane must highlight the card shown")
	}
	if next := press(m, "J").detailCard(); next == nil || next.Title != "Fix bike brake" {
		t.Errorf("J must step from the open card to the one below it in Doing, got %v", next)
	}
}

// A card can also stay in its lane but leave the list the pane shows, when it
// stops matching the filter. There is then no row to put the cursor on, so the
// pane shows none, and J and K start from the ends of the list.
func TestDetailShowsNoCursorWhenTheOpenCardLeavesTheList(t *testing.T) {
	open := func(t *testing.T) Model {
		t.Helper()
		m := newTestModel(t, 120, 40)
		m = press(m, "/")
		m = typeText(m, "o")
		m = press(m, "enter", "enter")
		if m.scr != screenDetail || m.detail.id != "k7q2m9ab" || len(m.visible(board.Todo)) != 3 {
			t.Fatalf("setup: screen %v id %q visible %d", m.scr, m.detail.id, len(m.visible(board.Todo)))
		}
		m.b.Lanes[board.Todo][0].Title = "Renew pass"
		if len(m.visible(board.Todo)) != 2 {
			t.Fatalf("setup: the renamed card must stop matching the filter")
		}
		return m
	}
	m := open(t)
	if cards, cur := m.detailList(); len(cards) != 2 || cur != -1 {
		t.Errorf("no row in the list is the open card: len=%d cur=%d", len(cards), cur)
	}
	if strings.Contains(plainView(m), "▸") {
		t.Errorf("the left pane must not highlight a card that is not the one shown")
	}
	if c := press(m, "J").detailCard(); c == nil || c.Title != "Tax docs to accountant" {
		t.Errorf("J must start from the top of the list, got %v", c)
	}
	if c := press(open(t), "K").detailCard(); c == nil || c.Title != "Book dentist" {
		t.Errorf("K must start from the bottom of the list, got %v", c)
	}
}

// archivedGym is "Cancel gym membership" in the sample archive: done Sep 2,
// created Aug 28. It shares its title with a different card in the board's
// Done lane, on purpose, so the two must never be confused by title.
const archivedGym = "s4t5u6v7"

// openArchived opens archivedGym's detail from the archive screen, optionally
// through a kept filter, on a store-backed root with the sample archive saved.
func openArchived(t *testing.T, filter string) (Model, string) {
	t.Helper()
	m, root := newRootModel(t, 120, 40)
	if err := m.st.SaveArchive(sampleArchive(t)); err != nil {
		t.Fatal(err)
	}
	m = press(m, "D")
	if filter != "" {
		m = press(m, "/")
		m = typeText(m, filter)
		m = press(m, "enter")
	}
	for i, c := range m.visibleArchive() {
		if c.ID == archivedGym {
			m.arch.cursor = i
		}
	}
	m = press(m, "enter")
	if m.scr != screenDetail || !m.detail.archived || m.detail.id != archivedGym {
		t.Fatalf("setup: screen %v archived %v id %q", m.scr, m.detail.archived, m.detail.id)
	}
	return m, root
}

// elsewhere opens a second Store on the same root, as a second process would.
func elsewhere(t *testing.T, root string) (*store.Store, *board.Board, *board.Archive) {
	t.Helper()
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	a, err := st.LoadArchive()
	if err != nil {
		t.Fatal(err)
	}
	return st, b, a
}

// touch moves the given files' mtimes forward by step hours from a fixed
// point, so the store's stat-based change check sees them as changed.
func touch(t *testing.T, step int, paths ...string) {
	t.Helper()
	at := time.Unix(2_000_000_000+int64(step)*3600, 0)
	for _, p := range paths {
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAnArchivedDetailFollowsItsCardOntoTheBoardWhenRestored(t *testing.T) {
	m, root := openArchived(t, "")
	st, b, a := elsewhere(t, root)
	i, _ := a.Find(archivedGym)
	b.Restore(a, i, fixedNow)
	if err := st.SaveRestore(b, a); err != nil {
		t.Fatal(err)
	}
	touch(t, 1, st.BoardPath(), st.ArchivePath())
	m, _ = feed(m, changeMsg{gen: m.watchGen})

	if m.scr != screenDetail || m.detail.id != archivedGym || m.detail.archived {
		t.Fatalf("the card still exists, so its detail must stay open, now on the board: screen %v id %q archived %v", m.scr, m.detail.id, m.detail.archived)
	}
	if m.lane != board.Doing {
		t.Errorf("the list must follow the card to Doing, got %v", m.lane)
	}
	if cards, cur := m.detailList(); cur < 0 || cur >= len(cards) || cards[cur].ID != archivedGym {
		t.Errorf("the cursor must be on the restored card: cur=%d", cur)
	}
	if !strings.HasPrefix(plainLines(m)[0], " kando  life › Doing › Cancel gym membership") {
		t.Errorf("header: %q", plainLines(m)[0])
	}
	if left := leftPane(plainLines(m)); len(left) == 0 || left[0] != "Doing 3" {
		t.Errorf("left pane header: %v", left)
	}
	if next := press(m, "J").detailCard(); next == nil || next.Title != "Fix bike brake" {
		t.Errorf("J must step to the card below it in Doing, got %v", next)
	}
	if m2 := press(m, "esc"); m2.scr != screenBoard || m2.lane != board.Doing {
		t.Errorf("esc must land on the board in Doing, got screen %v lane %v", m2.scr, m2.lane)
	}
}

func TestAnArchivedDetailWaitsForTheSecondWriteOfARestore(t *testing.T) {
	m, root := openArchived(t, "")
	st, b, a := elsewhere(t, root)
	i, _ := a.Find(archivedGym)
	b.Restore(a, i, fixedNow)
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	touch(t, 1, st.BoardPath())
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	if !m.detail.archived || m.detailCard() == nil {
		t.Fatalf("board.md alone must not restore the detail: archived=%v card=%v", m.detail.archived, m.detailCard())
	}
	if _, _, c := m.b.Find(archivedGym); c == nil {
		t.Fatalf("setup: the card must be on the board too by now")
	}

	if err := st.SaveArchive(a); err != nil {
		t.Fatal(err)
	}
	touch(t, 2, st.ArchivePath())
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	if m.detail.archived || m.lane != board.Doing {
		t.Errorf("archive.md landing must follow the card: archived=%v lane=%v", m.detail.archived, m.lane)
	}
}

func TestAnArchivedDetailClosesWhenItsCardIsInNeitherFile(t *testing.T) {
	for _, keys := range [][]string{nil, {"m"}} {
		m, root := openArchived(t, "")
		m = press(m, keys...)
		st, _, a := elsewhere(t, root)
		i, _ := a.Find(archivedGym)
		a.Remove(i)
		if err := st.SaveArchive(a); err != nil {
			t.Fatal(err)
		}
		touch(t, 3, st.ArchivePath())
		m, _ = feed(m, changeMsg{gen: m.watchGen})
		if m.scr != screenArchive || m.mode != modeNormal {
			t.Errorf("a card in neither file must close to the archive: screen %v mode %v", m.scr, m.mode)
		}
		if n := len(m.visibleArchive()); n > 0 && m.arch.cursor >= n {
			t.Errorf("the archive cursor must stay in range: cursor %d of %d", m.arch.cursor, n)
		}
	}
}

func TestAnArchivedDetailStaysWhileItsCardIsStillArchived(t *testing.T) {
	m, root := openArchived(t, "#money")
	st, _, a := elsewhere(t, root)
	_, c := a.Find(archivedGym)
	c.SetTag("errand")
	if err := st.SaveArchive(a); err != nil {
		t.Fatal(err)
	}
	touch(t, 1, st.ArchivePath())
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	if !m.detail.archived || m.detailCard() == nil || m.detailCard().Tag != "errand" {
		t.Errorf("an edit that keeps the card archived must not close or move its detail: archived=%v card=%v", m.detail.archived, m.detailCard())
	}
}

func TestARestoreDuringAnEditActsOnTheBoardCard(t *testing.T) {
	m, root := openArchived(t, "")
	m = press(m, "T")
	m = typeText(m, " today")
	st, b, a := elsewhere(t, root)
	i, _ := a.Find(archivedGym)
	b.Restore(a, i, fixedNow)
	if err := st.SaveRestore(b, a); err != nil {
		t.Fatal(err)
	}
	touch(t, 1, st.BoardPath(), st.ArchivePath())
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	m = press(m, "enter")
	if _, _, c := m.b.Find(archivedGym); c == nil || !strings.Contains(c.Title, "today") {
		t.Fatalf("the edit must land on the board card in memory: %v", c)
	}
	onDisk, err := store.Load(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, c := onDisk.Find(archivedGym); c == nil || !strings.Contains(c.Title, "today") {
		t.Errorf("the edit must be saved to board.md, not archive.md: %v", c)
	}
}

func TestARestoreDuringALanePickMovesTheBoardCard(t *testing.T) {
	m, root := openArchived(t, "")
	m = press(m, "m")
	st, b, a := elsewhere(t, root)
	i, _ := a.Find(archivedGym)
	b.Restore(a, i, fixedNow)
	if err := st.SaveRestore(b, a); err != nil {
		t.Fatal(err)
	}
	touch(t, 1, st.BoardPath(), st.ArchivePath())
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	m = press(m, "2")
	if l, _, c := m.b.Find(archivedGym); c == nil || l != board.Todo {
		t.Errorf("the lane pick must move the board card: lane=%v card=%v", l, c)
	}
}

func TestARestoredCardHiddenByTheFilterKeepsItsDetailOpen(t *testing.T) {
	m, root := openArchived(t, "age<2d")
	st, b, a := elsewhere(t, root)
	i, _ := a.Find(archivedGym)
	b.Restore(a, i, fixedNow)
	if err := st.SaveRestore(b, a); err != nil {
		t.Fatal(err)
	}
	touch(t, 1, st.BoardPath(), st.ArchivePath())
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	if m.detail.archived || m.lane != board.Doing || m.detailCard() == nil {
		t.Fatalf("setup: archived=%v lane=%v card=%v", m.detail.archived, m.lane, m.detailCard())
	}
	if cards, cur := m.detailList(); cur != -1 {
		t.Errorf("the filter hides the restored card, so no row is the cursor: cur=%d of %d", cur, len(cards))
	}
	if strings.Contains(plainView(m), "▸") {
		t.Errorf("no row should be highlighted")
	}
}

func TestARestoreThatShortensTheChecklistClampsTheCursor(t *testing.T) {
	m, root := openArchived(t, "")
	m = press(m, "o")
	m = typeText(m, "a")
	m = press(m, "enter", "o")
	m = typeText(m, "b")
	m = press(m, "enter")
	if m.detail.cursor != 1 {
		t.Fatalf("setup: cursor %d", m.detail.cursor)
	}
	st, b, a := elsewhere(t, root)
	i, ac := a.Find(archivedGym)
	ac.Checklist = ac.Checklist[:1]
	b.Restore(a, i, fixedNow)
	if err := st.SaveRestore(b, a); err != nil {
		t.Fatal(err)
	}
	touch(t, 1, st.BoardPath(), st.ArchivePath())
	m, _ = feed(m, changeMsg{gen: m.watchGen})
	if m.detail.archived || m.detail.cursor != 0 {
		t.Fatalf("archived=%v cursor=%d", m.detail.archived, m.detail.cursor)
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("enter on the clamped cursor must not panic: %v", r)
			}
		}()
		if m2 := press(m, "enter"); m2.mode != modeEdit {
			t.Errorf("enter must edit the item under the clamped cursor, got mode %v", m2.mode)
		}
	}()
}
