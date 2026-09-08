package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

func titles(cards []*board.Card) []string {
	out := make([]string, len(cards))
	for i, c := range cards {
		out[i] = c.Title
	}
	return out
}

func TestLaneChangeResetsSelection(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = press(m, "j")
	if m.sel != 1 {
		t.Fatalf("sel = %d", m.sel)
	}
	m = press(m, "l")
	if m.lane != board.Doing || m.sel != 0 || m.first != 0 {
		t.Errorf("after l: lane=%v sel=%d first=%d", m.lane, m.sel, m.first)
	}
	m = press(m, "h", "h", "h")
	if m.lane != board.Backlog {
		t.Errorf("h clamps at Backlog, got %v", m.lane)
	}
	m = press(m, "h")
	if m.lane != board.Backlog {
		t.Errorf("h must not wrap")
	}
	m = press(m, "shift+tab")
	if m.lane != board.Done {
		t.Errorf("shift+tab wraps to Done, got %v", m.lane)
	}
	m = press(m, "tab")
	if m.lane != board.Backlog {
		t.Errorf("tab wraps to Backlog, got %v", m.lane)
	}
	m = press(m, "right", "right", "right", "right")
	if m.lane != board.Done {
		t.Errorf("arrows work too, got %v", m.lane)
	}
}

func TestJKWrap(t *testing.T) {
	m := newTestModel(t, 120, 40) // Todo has 3 cards
	m = press(m, "k")
	if m.sel != 2 {
		t.Errorf("k from first wraps to last, sel = %d", m.sel)
	}
	m = press(m, "j")
	if m.sel != 0 {
		t.Errorf("j from last wraps to first, sel = %d", m.sel)
	}
	m = press(m, "down", "down", "up")
	if m.sel != 1 {
		t.Errorf("arrows: sel = %d", m.sel)
	}
}

func TestMoveCardFollows(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = press(m, "j") // Tax docs
	m = press(m, "L")
	if m.lane != board.Doing {
		t.Fatalf("active lane should follow the card, got %v", m.lane)
	}
	if c := m.selectedCard(); c == nil || c.Title != "Tax docs to accountant" {
		t.Fatalf("moved card should stay selected, got %v", c)
	}
	if got := titles(m.b.Lanes[board.Doing]); got[0] != "Tax docs to accountant" || len(got) != 3 {
		t.Errorf("card goes to the top of the destination: %v", got)
	}
	if len(m.b.Lanes[board.Todo]) != 2 {
		t.Errorf("source lane should shrink")
	}
	c := m.selectedCard()
	if !c.MovedAt.Equal(fixedNow) || !c.DoneAt.IsZero() {
		t.Errorf("MovedAt/DoneAt: %v %v", c.MovedAt, c.DoneAt)
	}
	m = press(m, "H", "H", "H")
	if m.lane != board.Backlog || m.selectedCard().Title != "Tax docs to accountant" {
		t.Errorf("H moves back and clamps at Backlog: lane=%v sel=%v", m.lane, m.selectedCard())
	}
}

func TestDoneAndUndo(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = press(m, "d") // Renew passport → Done
	if m.lane != board.Todo || len(m.b.Lanes[board.Done]) != 4 || m.b.Lanes[board.Done][0].Title != "Renew passport" {
		t.Fatalf("d should move to the top of Done and keep the lane: %v", titles(m.b.Lanes[board.Done]))
	}
	if !m.b.Lanes[board.Done][0].DoneAt.Equal(fixedNow) {
		t.Errorf("DoneAt should be now")
	}
	if c := m.selectedCard(); c == nil || c.Title != "Tax docs to accountant" {
		t.Errorf("selection stays at the same index: %v", c)
	}
	m = press(m, "l", "l", "u") // Done lane, undo the first card
	if len(m.b.Lanes[board.Doing]) != 3 || m.b.Lanes[board.Doing][0].Title != "Renew passport" {
		t.Errorf("u should move back to the top of Doing: %v", titles(m.b.Lanes[board.Doing]))
	}
	if !m.b.Lanes[board.Doing][0].DoneAt.IsZero() {
		t.Errorf("DoneAt should clear on undo")
	}
	m = press(m, "d") // no-op in Done
	if len(m.b.Lanes[board.Done]) != 3 {
		t.Errorf("d in Done must be a no-op")
	}
}

func TestQuickAddCreatesAtTopAndSaves(t *testing.T) {
	root := t.TempDir()
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes[board.Todo] = sampleBoard(t).Lanes[board.Todo]
	m := New(Options{Store: st, Board: b, Styles: testStyles, Now: func() time.Time { return fixedNow }, Width: 120, Height: 40})
	m = press(m, "j", "a")
	if m.mode != modeQuickAdd {
		t.Fatalf("a should open quick-add")
	}
	m = typeText(m, "Buy milk")
	m = press(m, "j") // goes to the input, not the selection
	if m.quick.Value() != "Buy milkj" {
		t.Errorf("input should receive every key: %q", m.quick.Value())
	}
	m = press(m, "backspace", "enter")
	if m.mode != modeNormal {
		t.Errorf("enter closes quick-add")
	}
	got := titles(m.b.Lanes[board.Todo])
	if got[0] != "Buy milk" || len(got) != 4 {
		t.Fatalf("new card should be first: %v", got)
	}
	c := m.b.Lanes[board.Todo][0]
	if !c.CreatedAt.Equal(fixedNow) || !c.MovedAt.Equal(fixedNow) || len(c.ID) != 8 {
		t.Errorf("new card stamps: %+v", c)
	}
	if m.sel != 0 || m.selectedCard() != c {
		t.Errorf("new card should be selected, sel=%d", m.sel)
	}
	data, _ := os.ReadFile(st.BoardPath())
	if !strings.Contains(string(data), "### Buy milk\n") {
		t.Errorf("board.md should contain the new card:\n%s", data)
	}
	// Empty input and esc do not create anything.
	m = press(m, "a", "enter")
	m = press(m, "a")
	m = typeText(m, "nope")
	m = press(m, "esc")
	if len(m.b.Lanes[board.Todo]) != 4 || m.mode != modeNormal {
		t.Errorf("empty enter / esc must not create cards")
	}
}

func TestOverflowIndicators(t *testing.T) {
	m := newTestModel(t, 80, 24) // R=14: two cards fit
	lines := plainLines(m)
	if !strings.HasPrefix(lines[18], " │ … +1") {
		t.Errorf("bottom indicator missing: %q", lines[18])
	}
	m = press(m, "j", "j") // select the third card → scroll by one
	lines = plainLines(m)
	if !strings.HasPrefix(lines[7], " │ … +1") {
		t.Errorf("top indicator missing after scroll: %q", lines[7])
	}
	if !strings.Contains(lines[9], "Tax docs to accountant") || !strings.Contains(lines[15], "Book dentist") {
		t.Errorf("cards 2 and 3 should be visible:\n%s", strings.Join(lines, "\n"))
	}
	if m.first != 1 {
		t.Errorf("first = %d", m.first)
	}
	m = press(m, "j") // wraps to the first card → scroll back
	if m.first != 0 {
		t.Errorf("wrap should scroll back to the top, first = %d", m.first)
	}
	// Collapsed lane overflow: many cards in Backlog at a short height.
	m = newTestModel(t, 120, 16) // R = 8 rows
	for i := 0; i < 10; i++ {
		m.b.Insert(board.Backlog, 0, &board.Card{ID: board.NewID(), Title: "extra"})
	}
	lines = plainLines(m)
	if !strings.HasPrefix(lines[12], " │ … +6") {
		t.Errorf("collapsed overflow row: %q", lines[12])
	}
}

func TestHelpOverlay(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = press(m, "?")
	if !m.help {
		t.Fatal("? opens help")
	}
	view := plainView(m)
	for _, want := range []string{"KEYS", "j/k ↓↑   select card", "u        undo (Done → Doing) D        archive view", "?        close help"} {
		if !strings.Contains(view, want) {
			t.Errorf("help overlay missing %q", want)
		}
	}
	m = press(m, "j")
	if m.sel != 0 {
		t.Errorf("keys are swallowed while help is open")
	}
	m = press(m, "esc")
	if m.help {
		t.Errorf("esc closes help")
	}
	m = press(m, "?", "?")
	if m.help {
		t.Errorf("? toggles help closed")
	}
}

func TestFilterLiveKeepClear(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = press(m, "/")
	m = typeText(m, "#home")
	if m.visibleCount() != 3 || len(m.visible(board.Todo)) != 0 {
		t.Errorf("live filter: %d visible", m.visibleCount())
	}
	lines := plainLines(m)
	if !strings.HasPrefix(lines[0], " / #home") || !strings.HasSuffix(lines[0], "3 of 11 match ") {
		t.Errorf("filter header: %q", lines[0])
	}
	m = press(m, "enter")
	if m.mode != modeNormal || m.filterQuery != "#home" || m.visibleCount() != 3 {
		t.Errorf("enter keeps the filter: mode=%v q=%q n=%d", m.mode, m.filterQuery, m.visibleCount())
	}
	lines = plainLines(m)
	if !strings.HasPrefix(lines[0], " kando  life  /#home") || !strings.HasSuffix(lines[39], "3 cards ") {
		t.Errorf("kept filter header/footer: %q / %q", lines[0], lines[39])
	}
	m = press(m, "l") // Doing has one visible card
	if c := m.selectedCard(); c == nil || c.Title != "Fix bike brake" {
		t.Errorf("selection follows visible cards: %v", c)
	}
	m = press(m, "/")
	m = typeText(m, " !blocked")
	if m.visibleCount() != 1 {
		t.Errorf("operators combine: %d", m.visibleCount())
	}
	m = press(m, "esc")
	if m.filterQuery != "" || !m.filter.Empty() || m.visibleCount() != 11 {
		t.Errorf("esc clears everything: q=%q n=%d", m.filterQuery, m.visibleCount())
	}
	m = press(m, "/")
	m = typeText(m, "age>7d")
	if m.visibleCount() != 2 {
		t.Errorf("age>7d should match 2 cards, got %d", m.visibleCount())
	}
	m = press(m, "esc", "/")
	m = typeText(m, "zzz")
	if m.visibleCount() != 0 || !strings.Contains(plainView(m), "· · ·") {
		t.Errorf("no matches should show empty lanes")
	}
}

func TestReloadKeepsSelection(t *testing.T) {
	root := t.TempDir()
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes = sampleBoard(t).Lanes
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	m := New(Options{Store: st, Board: b, Styles: testStyles, Now: func() time.Time { return fixedNow }, Width: 120, Height: 40})
	m = press(m, "j") // Tax docs selected
	data, _ := os.ReadFile(st.BoardPath())
	edited := strings.Replace(string(data), "### Renew passport", "### Renew passport soon", 1)
	edited = strings.Replace(edited, "## Todo\n\n", "## Todo\n\n### Inserted at top\nid: zzzzzzzz\n\n", 1)
	if err := os.WriteFile(st.BoardPath(), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	next, _ := m.Update(changeMsg{})
	m = next.(Model)
	if got := titles(m.b.Lanes[board.Todo]); got[0] != "Inserted at top" || got[1] != "Renew passport soon" {
		t.Fatalf("reload did not apply: %v", got)
	}
	if c := m.selectedCard(); c == nil || c.Title != "Tax docs to accountant" || m.sel != 2 {
		t.Errorf("selection should follow the card id: sel=%d %v", m.sel, c)
	}
}

func TestQuitKeys(t *testing.T) {
	m := newTestModel(t, 120, 40)
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}); cmd == nil {
		t.Errorf("q should quit")
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); cmd == nil {
		t.Errorf("ctrl+c should quit")
	}
}

func TestDeleteKeyRemovesSelectedCard(t *testing.T) {
	root := t.TempDir()
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes = sampleBoard(t).Lanes
	m := New(Options{Store: st, Board: b, Styles: testStyles, Now: func() time.Time { return fixedNow }, Width: 120, Height: 40})
	m = press(m, "j", "x") // delete "Tax docs to accountant"
	if got := titles(m.b.Lanes[board.Todo]); len(got) != 2 || got[0] != "Renew passport" || got[1] != "Book dentist" {
		t.Fatalf("x should delete the selected card: %v", got)
	}
	if m.b.Count() != 10 {
		t.Errorf("Count = %d, want 10", m.b.Count())
	}
	if c := m.selectedCard(); c == nil || c.Title != "Book dentist" {
		t.Errorf("selection should stay at the same index: %v", c)
	}
	data, _ := os.ReadFile(st.BoardPath())
	if strings.Contains(string(data), "Tax docs") {
		t.Errorf("delete should be saved:\n%s", data)
	}
	if !strings.HasSuffix(plainLines(m)[39], "10 cards ") {
		t.Errorf("footer count should update: %q", plainLines(m)[39])
	}
}

func TestDeleteKeyClampsSelectionAfterDelete(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = press(m, "k", "x") // last card (Book dentist)
	if m.sel != 1 || m.selectedCard().Title != "Tax docs to accountant" {
		t.Errorf("deleting the last card should select the new last: sel=%d %v", m.sel, m.selectedCard())
	}
	m = press(m, "x", "x")
	if len(m.b.Lanes[board.Todo]) != 0 || m.sel != 0 || m.selectedCard() != nil {
		t.Errorf("emptying the lane: sel=%d card=%v", m.sel, m.selectedCard())
	}
	if !strings.Contains(plainView(m), "· · ·") {
		t.Errorf("empty lane should show the placeholder")
	}
}

func TestDeleteKeyOnEmptyLaneNoop(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m.b.Lanes[board.Todo] = nil
	before := m.b.Count()
	m = press(m, "x")
	if m.b.Count() != before || m.scr != screenBoard {
		t.Errorf("x on an empty lane must do nothing")
	}
}

func TestHelpOverlayListsDeleteAndBoards(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = press(m, "?")
	view := plainView(m)
	for _, want := range []string{"x        delete card", "A        archive (Done)", "B        boards", "?        close help"} {
		if !strings.Contains(view, want) {
			t.Errorf("help overlay missing %q", want)
		}
	}
	// The persistent footer is unchanged (the three spec goldens stay byte-identical).
	m = press(m, "esc")
	if !strings.HasPrefix(plainLines(m)[39], " j/k move  h/l tab lane  a add  enter open  H/L move card  d done  / filter  ? help  q quit") {
		t.Errorf("footer must not change: %q", plainLines(m)[39])
	}
}

// reorderBoard builds a Todo lane of cards with the given titles and tags, so a
// filter can hide a card *between* two visible ones — the case that separates a
// position derived from the visible list from one derived from the lane.
func reorderBoard(t *testing.T, spec ...[2]string) Model {
	t.Helper()
	m := newTestModel(t, 120, 40)
	cards := make([]*board.Card, len(spec))
	for i, s := range spec {
		cards[i] = &board.Card{
			ID: s[0], Title: s[0], Tag: s[1],
			CreatedAt: fixedNow.AddDate(0, 0, -1), MovedAt: fixedNow.AddDate(0, 0, -1),
		}
	}
	m.b.Lanes[board.Todo] = cards
	m.sel, m.first = 0, 0
	return m
}

func TestReorderKeysMoveTheCardWithinItsLane(t *testing.T) {
	m := newTestModel(t, 120, 40) // Todo: Renew passport, Tax docs, Book dentist
	// Keyed by card, not by index: the whole point is that the order changes.
	before := map[*board.Card]time.Time{}
	for _, c := range m.b.Lanes[board.Todo] {
		before[c] = c.MovedAt
	}

	m = press(m, "J")
	if got := titles(m.b.Lanes[board.Todo]); got[0] != "Tax docs to accountant" || got[1] != "Renew passport" {
		t.Fatalf("J moves the card one slot down: %v", got)
	}
	if c := m.selectedCard(); c == nil || c.Title != "Renew passport" {
		t.Errorf("the selection must follow the card, got %v", c)
	}
	if m.sel != 1 {
		t.Errorf("sel should track the card's new position, got %d", m.sel)
	}

	m = press(m, "J")
	if got := titles(m.b.Lanes[board.Todo]); got[2] != "Renew passport" {
		t.Errorf("a second J reaches the bottom: %v", got)
	}

	m = press(m, "K", "K")
	if got := titles(m.b.Lanes[board.Todo]); got[0] != "Renew passport" {
		t.Errorf("K walks it back to the top: %v", got)
	}
	if c := m.selectedCard(); c.Title != "Renew passport" || m.sel != 0 {
		t.Errorf("selection still follows: sel=%d %v", m.sel, c)
	}

	// A reorder is not a lane change (board.go:233-235). This is the assertion
	// that fails if someone routes the helper through Board.Move.
	for _, c := range m.b.Lanes[board.Todo] {
		if !c.MovedAt.Equal(before[c]) || !c.DoneAt.IsZero() {
			t.Errorf("reorder restamped %q: MovedAt=%v want %v, DoneAt=%v", c.Title, c.MovedAt, before[c], c.DoneAt)
		}
	}
}

func TestReorderClampsAtTheLaneEnds(t *testing.T) {
	m := newTestModel(t, 120, 40)
	want := titles(m.b.Lanes[board.Todo])

	m = press(m, "K") // already first
	if got := titles(m.b.Lanes[board.Todo]); !slicesEqual(got, want) {
		t.Errorf("K on the first card must not wrap to the bottom: %v", got)
	}
	if m.sel != 0 {
		t.Errorf("sel = %d, want 0", m.sel)
	}

	m = press(m, "j", "j") // last card
	m = press(m, "J")
	if got := titles(m.b.Lanes[board.Todo]); !slicesEqual(got, want) {
		t.Errorf("J on the last card must not wrap to the top: %v", got)
	}
	if m.sel != 2 {
		t.Errorf("sel = %d, want 2", m.sel)
	}
}

func TestReorderThatChangesNothingWritesNothing(t *testing.T) {
	root := t.TempDir()
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes = sampleBoard(t).Lanes
	m := New(Options{Store: st, Board: b, Styles: testStyles, Now: func() time.Time { return fixedNow }, Width: 120, Height: 40})

	// A real reorder must reach the disk.
	m = press(m, "J")
	data, err := os.ReadFile(st.BoardPath())
	if err != nil {
		t.Fatal(err)
	}
	if i, j := strings.Index(string(data), "Tax docs"), strings.Index(string(data), "Renew passport"); i < 0 || j < 0 || i > j {
		t.Fatalf("J should be saved with Tax docs above Renew passport:\n%s", data)
	}

	// A no-op must not. The store detects change by mtime+size (store.go:123,
	// 454), so rewriting identical bytes still wakes every connected kando web
	// client — which is why comparing file *contents* cannot see this. Removing
	// the file first makes a stray save unmistakable.
	if err := os.Remove(st.BoardPath()); err != nil {
		t.Fatal(err)
	}
	// Select the last card explicitly. Pressing "j" would be ambiguous here:
	// lowercase j WRAPS (TestJKWrap), so a miscount lands back on the top card
	// and turns the press below into a real move.
	m.sel = len(m.visible(board.Todo)) - 1
	m = press(m, "J") // the bottom card, pressed further down: nothing to do
	if _, err := os.Stat(st.BoardPath()); !os.IsNotExist(err) {
		t.Errorf("a no-op reorder must not write board.md (stat err = %v)", err)
	}
}

func TestReorderStepsPastTheVisibleNeighbourUnderAFilter(t *testing.T) {
	// Lane order is a, hidden, b; the filter shows only a and b. Deriving the
	// target from the visible list (a is at visible 0, so "one below" looks
	// like lane index 1) drops the card between "hidden" and b, which leaves
	// the on-screen order unchanged — the move appears to do nothing.
	m := reorderBoard(t, [2]string{"a", "keep"}, [2]string{"hidden", "other"}, [2]string{"b", "keep"})
	m = press(m, "/")
	m = typeText(m, "#keep")
	m = press(m, "enter")

	if got := titles(m.visible(board.Todo)); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("filter should show exactly a and b, got %v", got)
	}

	m = press(m, "J")
	if got := titles(m.b.Lanes[board.Todo]); got[2] != "a" {
		t.Errorf("J must move a past the visible neighbour b, landing last: %v", got)
	}
	if got := titles(m.visible(board.Todo)); got[0] != "b" || got[1] != "a" {
		t.Errorf("on screen a must now follow b: %v", got)
	}
	if c := m.selectedCard(); c == nil || c.Title != "a" {
		t.Errorf("selection follows the card under a filter too: %v", c)
	}

	m = press(m, "K")
	if got := titles(m.visible(board.Todo)); got[0] != "a" || got[1] != "b" {
		t.Errorf("K must move a back above b: %v", got)
	}
	if got := titles(m.b.Lanes[board.Todo]); got[1] != "hidden" && got[0] != "hidden" {
		t.Errorf("the hidden card must still be in the lane: %v", got)
	}
}

func TestReorderInDoneKeepsDoneAt(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = press(m, "l", "l") // Done
	if len(m.b.Lanes[board.Done]) < 2 {
		t.Fatalf("fixture needs at least two Done cards")
	}
	want := map[*board.Card]time.Time{}
	for _, c := range m.b.Lanes[board.Done] {
		want[c] = c.DoneAt
	}
	first := m.b.Lanes[board.Done][0]

	m = press(m, "J")
	if m.b.Lanes[board.Done][1] != first {
		t.Errorf("J should reorder inside Done")
	}
	for _, c := range m.b.Lanes[board.Done] {
		if !c.DoneAt.Equal(want[c]) {
			t.Errorf("reordering inside Done must not restamp %q's DoneAt: %v != %v", c.Title, c.DoneAt, want[c])
		}
	}
	if first.DoneAt.Equal(fixedNow) {
		t.Errorf("DoneAt was stamped to now: the archive buckets cards by it")
	}
}

func TestReorderOnEmptyAndSingleCardLanes(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m.b.Lanes[board.Todo] = nil
	before := m.b.Count()
	m = press(m, "J", "K")
	if m.b.Count() != before || m.scr != screenBoard {
		t.Errorf("J/K on an empty lane must do nothing")
	}

	m = reorderBoard(t, [2]string{"only", "x"})
	m = press(m, "J", "K")
	if got := titles(m.b.Lanes[board.Todo]); len(got) != 1 || got[0] != "only" {
		t.Errorf("J/K on a single-card lane must do nothing: %v", got)
	}
	if m.sel != 0 {
		t.Errorf("sel = %d, want 0", m.sel)
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestReorderAtTheEndOfAFilteredLaneIsANoop(t *testing.T) {
	// "hidden" sits below the last visible card. J on that card must not sink
	// it into the hidden tail: with a filter on, "after the last one I can
	// see" names no real position — the same rule the web states in
	// internal/web/cards.go:217-221.
	m := reorderBoard(t, [2]string{"a", "keep"}, [2]string{"b", "keep"}, [2]string{"hidden", "other"})
	m = press(m, "/")
	m = typeText(m, "#keep")
	m = press(m, "enter")

	m.sel = len(m.visible(board.Todo)) - 1 // "b", the last visible card
	m = press(m, "J")
	if got := titles(m.b.Lanes[board.Todo]); got[0] != "a" || got[1] != "b" || got[2] != "hidden" {
		t.Errorf("J at the end of a filtered lane must change nothing: %v", got)
	}
}

func TestReorderInDoneSurvivesTheDiskRoundTrip(t *testing.T) {
	// Lane order is carried by slice order through store.Marshal, and the
	// archive — not the board — is what sorts by DoneAt. If that sort ever
	// reached the Done lane, a reorder there would look right on screen and
	// silently revert on the next open, which no in-memory test can see.
	root := t.TempDir()
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Lanes = sampleBoard(t).Lanes
	m := New(Options{Store: st, Board: b, Styles: testStyles, Now: func() time.Time { return fixedNow }, Width: 120, Height: 40})

	m = press(m, "l", "l") // Done
	first := m.b.Lanes[board.Done][0].Title
	second := m.b.Lanes[board.Done][1].Title
	m = press(m, "J")

	_, reloaded, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	got := titles(reloaded.Lanes[board.Done])
	if got[0] != second || got[1] != first {
		t.Errorf("the Done reorder must survive a reload: got %v, want %q then %q", got, second, first)
	}
}
