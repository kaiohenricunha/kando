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
