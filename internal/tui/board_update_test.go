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
