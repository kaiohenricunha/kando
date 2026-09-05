package board

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestLaneNames(t *testing.T) {
	for _, l := range Lanes {
		got, ok := ParseLane(l.String())
		if !ok || got != l {
			t.Errorf("ParseLane(%q) = %v,%v", l.String(), got, ok)
		}
	}
	if _, ok := ParseLane("Nope"); ok {
		t.Errorf("ParseLane accepted unknown lane")
	}
	if Backlog.String() != "Backlog" || Done.String() != "Done" {
		t.Errorf("names: %q %q", Backlog.String(), Done.String())
	}
}

func TestMove(t *testing.T) {
	b := &Board{}
	a := &Card{ID: "a", Title: "A", CreatedAt: now.Add(-48 * time.Hour)}
	c := &Card{ID: "c", Title: "C"}
	d := &Card{ID: "d", Title: "D"}
	b.Lanes[Todo] = []*Card{a}
	b.Lanes[Doing] = []*Card{c}
	b.Lanes[Done] = []*Card{d}

	idx := b.Move(Todo, 0, Doing, now)
	if idx != 0 || len(b.Lanes[Todo]) != 0 || len(b.Lanes[Doing]) != 2 || b.Lanes[Doing][0] != a {
		t.Fatalf("move to top failed: idx=%d todo=%d doing=%v", idx, len(b.Lanes[Todo]), b.Lanes[Doing])
	}
	if !a.MovedAt.Equal(now) || !a.DoneAt.IsZero() {
		t.Errorf("MovedAt/DoneAt after move: %v %v", a.MovedAt, a.DoneAt)
	}
	later := now.Add(time.Hour)
	b.Move(Doing, 0, Done, later)
	if !a.DoneAt.Equal(later) || !a.MovedAt.Equal(later) || b.Lanes[Done][0] != a || b.Lanes[Done][1] != d {
		t.Errorf("move to Done: doneAt=%v movedAt=%v done=%v", a.DoneAt, a.MovedAt, b.Lanes[Done])
	}
	b.Move(Done, 0, Doing, later)
	if !a.DoneAt.IsZero() {
		t.Errorf("DoneAt should clear when leaving Done")
	}
	if b.Count() != 3 {
		t.Errorf("Count = %d", b.Count())
	}
	l, i, got := b.Find("d")
	if got != d || l != Done || i != 0 {
		t.Errorf("Find = %v %d %v", l, i, got)
	}
	if _, _, got := b.Find("zzz"); got != nil {
		t.Errorf("Find unknown should be nil")
	}
}

func TestCardHelpers(t *testing.T) {
	c := &Card{Notes: "first line\nsecond", Checklist: []Item{{"a", true}, {"b", false}, {"c", false}}}
	if c.FirstNoteLine() != "first line" {
		t.Errorf("FirstNoteLine = %q", c.FirstNoteLine())
	}
	d, n := c.ChecklistProgress()
	if d != 1 || n != 3 {
		t.Errorf("progress = %d/%d", d, n)
	}
	if (&Card{}).FirstNoteLine() != "" {
		t.Errorf("empty notes")
	}
	done := &Card{CreatedAt: now.Add(-10 * 24 * time.Hour), DoneAt: now.Add(-24 * time.Hour)}
	if !done.AgeSince().Equal(done.DoneAt) {
		t.Errorf("AgeSince should use DoneAt when set")
	}
}

func TestNewID(t *testing.T) {
	re := regexp.MustCompile(`^[a-z2-7]{8}$`)
	seen := map[string]bool{}
	for i := 0; i < 2000; i++ {
		id := NewID()
		if !re.MatchString(id) {
			t.Fatalf("bad id %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestLaneKey(t *testing.T) {
	for _, l := range Lanes {
		got, ok := ParseLane(l.Key())
		if !ok || got != l || l.Key() != strings.ToLower(l.String()) {
			t.Errorf("Key(%v) = %q, ParseLane → %v,%v", l, l.Key(), got, ok)
		}
	}
}

func TestCardLabels(t *testing.T) {
	c := &Card{}
	if c.ProgressLabel() != "" || c.BlockedLabel() != "" {
		t.Errorf("empty card: %q %q", c.ProgressLabel(), c.BlockedLabel())
	}
	c.Checklist = []Item{{Done: true}, {}, {}, {}}
	c.Blocked = true
	if c.ProgressLabel() != "1/4" || c.BlockedLabel() != "blocked" {
		t.Errorf("labels: %q %q", c.ProgressLabel(), c.BlockedLabel())
	}
	c.BlockedReason = "waiting on pads"
	if c.BlockedLabel() != "waiting on pads" {
		t.Errorf("reason: %q", c.BlockedLabel())
	}
}

func TestRestoreMovesArchivedCardToTopOfDoing(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	b := &Board{}
	b.Insert(Doing, 0, &Card{ID: "old", Title: "already doing"})
	a := &Archive{Cards: []*Card{{ID: "x", DoneAt: now.Add(-24 * time.Hour)}, {ID: "y", DoneAt: now.Add(-48 * time.Hour)}}}
	if got := b.Restore(a, 5, now); got != nil {
		t.Fatalf("out of range should be nil, got %v", got)
	}
	c := b.Restore(a, 1, now)
	if c == nil || c.ID != "y" || !c.DoneAt.IsZero() || !c.MovedAt.Equal(now) {
		t.Fatalf("restored: %+v", c)
	}
	if len(a.Cards) != 1 || a.Cards[0].ID != "x" {
		t.Errorf("archive after restore: %v", a.Cards)
	}
	if len(b.Lanes[Doing]) != 2 || b.Lanes[Doing][0] != c {
		t.Errorf("restored card should be at the top of Doing")
	}
}
