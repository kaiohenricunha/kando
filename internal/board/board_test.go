package board

import (
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
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

func TestMoveToTheSameLaneIsANoOp(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -5)
	b := &Board{}
	b.Insert(Todo, 0, &Card{ID: "a", Title: "first", MovedAt: old})
	b.Insert(Todo, 1, &Card{ID: "b", Title: "second", MovedAt: old})
	if got := b.Move(Todo, 1, Todo, now); got != 1 {
		t.Errorf("index = %d, want 1", got)
	}
	c := b.Lanes[Todo][1]
	if c.ID != "b" || !c.MovedAt.Equal(old) {
		t.Errorf("a same-lane move must not reorder or restamp: %+v", c)
	}
	done := &Card{ID: "d", DoneAt: old, MovedAt: old}
	b.Insert(Done, 0, done)
	b.Move(Done, 0, Done, now)
	if !done.DoneAt.Equal(old) {
		t.Errorf("a same-lane move must not restamp DoneAt: %v", done.DoneAt)
	}
}

func TestUnarchiveStampsLikeMove(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	for _, to := range Lanes {
		b := &Board{}
		a := &Archive{Cards: []*Card{{ID: "x", DoneAt: now.AddDate(0, 0, -3)}}}
		c := b.Unarchive(a, 0, to, now)
		if c == nil || len(b.Lanes[to]) != 1 || len(a.Cards) != 0 {
			t.Fatalf("unarchive to %v: %+v", to, c)
		}
		wantDone := time.Time{}
		if to == Done {
			wantDone = now
		}
		if !c.MovedAt.Equal(now) || !c.DoneAt.Equal(wantDone) {
			t.Errorf("to %v: moved=%v done=%v want done=%v", to, c.MovedAt, c.DoneAt, wantDone)
		}
	}
}

func TestArchiveViewCapsGroupsAndCounts(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	a := &Archive{}
	for i := 0; i < 60; i++ {
		c := &Card{ID: DeriveID(string(rune('a' + i%26))), Title: "card", DoneAt: now.AddDate(0, 0, -i)}
		if i%2 == 0 {
			c.Tag = "money"
		}
		a.Cards = append(a.Cards, c)
	}
	groups, matched, scanned, total := ArchiveView(a, Parse(""), now)
	if scanned != ArchiveMax || total != 60 {
		t.Errorf("scanned=%d total=%d", scanned, total)
	}
	if matched != ArchiveMax {
		t.Errorf("matched=%d", matched)
	}
	var n int
	for _, g := range groups {
		if len(g.Cards) == 0 {
			t.Errorf("empty group %q should be dropped", g.Label)
		}
		n += len(g.Cards)
	}
	if n != ArchiveMax {
		t.Errorf("groups hold %d cards", n)
	}
	if _, matched, _, _ := ArchiveView(a, Parse("#money"), now); matched != 25 {
		t.Errorf("filtered matched=%d, want 25 (half of the newest 50)", matched)
	}
	if _, _, _, total := ArchiveView(nil, Parse(""), now); total != 0 {
		t.Errorf("nil archive")
	}
}

func TestFieldsAreCapped(t *testing.T) {
	long := strings.Repeat("é", 4000) // 8000 bytes
	c := &Card{}
	c.SetTitle(long)
	c.SetTag(long)
	c.SetBlocked(long)
	c.SetNotes(strings.Repeat("x", 40<<10))
	c.InsertChecklistItem(-1, long)
	if len(c.Title) > 512 || len(c.Tag) > 512 || len(c.BlockedReason) > 512 || len(c.Checklist[0].Text) > 512 {
		t.Errorf("single-line fields not capped: %d %d %d %d", len(c.Title), len(c.Tag), len(c.BlockedReason), len(c.Checklist[0].Text))
	}
	if len(c.Notes) > 16<<10 {
		t.Errorf("notes not capped: %d", len(c.Notes))
	}
	for _, s := range []string{c.Title, c.Tag, c.BlockedReason} {
		if !utf8.ValidString(s) {
			t.Errorf("cap split a rune: %q", s)
		}
	}
}

// laneIDs is the ids of lane l in order — the readable form for a
// reordering assertion.
func laneIDs(b *Board, l Lane) string {
	ids := make([]string, len(b.Lanes[l]))
	for i, c := range b.Lanes[l] {
		ids[i] = c.ID
	}
	return strings.Join(ids, " ")
}

// MoveAt's `at` is insert-before against the lane as it stood before the
// move, so a same-lane move down has to account for the removal shifting
// everything below it up one. These are the cases that pin that arithmetic,
// including both ways of dropping a card on itself.
func TestMoveAtReordersWithinALane(t *testing.T) {
	old := now.AddDate(0, 0, -5)
	cases := []struct {
		i, at, wantIdx int
		want           string
	}{
		{0, 2, 1, "b a c"}, // a in front of c
		{2, 0, 0, "c a b"}, // c in front of a
		{0, 3, 2, "b c a"}, // a appended
		{1, 1, 1, "a b c"}, // b in front of itself
		{1, 2, 1, "a b c"}, // b in front of the card below it
		{0, 0, 0, "a b c"}, // a in front of itself
		{2, 3, 2, "a b c"}, // c appended, already last
	}
	for _, tc := range cases {
		b := &Board{}
		for _, id := range []string{"a", "b", "c"} {
			b.Lanes[Todo] = append(b.Lanes[Todo], &Card{ID: id, MovedAt: old})
		}
		idx := b.MoveAt(Todo, tc.i, Todo, tc.at, now)
		if got := laneIDs(b, Todo); got != tc.want || idx != tc.wantIdx {
			t.Errorf("MoveAt(Todo, %d, Todo, %d) = %d %q, want %d %q", tc.i, tc.at, idx, got, tc.wantIdx, tc.want)
		}
		for _, c := range b.Lanes[Todo] {
			if !c.MovedAt.Equal(old) {
				t.Errorf("MoveAt(Todo, %d, Todo, %d) restamped %q: a reorder is not a lane change", tc.i, tc.at, c.ID)
			}
		}
	}
}

// A cross-lane MoveAt is Move with a chosen position: the same stamping
// rule, a different insert index.
func TestMoveAtPlacesACardAcrossLanes(t *testing.T) {
	b := &Board{}
	for _, id := range []string{"x", "y", "z"} {
		b.Lanes[Doing] = append(b.Lanes[Doing], &Card{ID: id})
	}
	a := &Card{ID: "a", MovedAt: now.AddDate(0, 0, -5)}
	b.Lanes[Todo] = []*Card{a}

	if idx := b.MoveAt(Todo, 0, Doing, 2, now); idx != 2 || laneIDs(b, Doing) != "x y a z" {
		t.Fatalf("into the middle of a lane: idx=%d doing=%q", idx, laneIDs(b, Doing))
	}
	if len(b.Lanes[Todo]) != 0 {
		t.Errorf("the card should have left Todo: %q", laneIDs(b, Todo))
	}
	if !a.MovedAt.Equal(now) {
		t.Errorf("a cross-lane move stamps MovedAt: %v", a.MovedAt)
	}
	later := now.Add(time.Hour)
	b.MoveAt(Doing, 2, Done, 0, later)
	if !a.DoneAt.Equal(later) || !a.MovedAt.Equal(later) {
		t.Errorf("entering Done: doneAt=%v movedAt=%v", a.DoneAt, a.MovedAt)
	}
	b.MoveAt(Done, 0, Backlog, 0, later)
	if !a.DoneAt.IsZero() {
		t.Errorf("DoneAt should clear leaving Done: %v", a.DoneAt)
	}
	if idx := b.MoveAt(Backlog, 0, Todo, 0, now); idx != 0 || laneIDs(b, Todo) != "a" {
		t.Errorf("an empty lane takes the card at its only position: idx=%d todo=%q", idx, laneIDs(b, Todo))
	}
}

func TestMoveAtClampsThePositionAndRejectsABadSource(t *testing.T) {
	b := &Board{}
	b.Lanes[Todo] = []*Card{{ID: "a"}, {ID: "b"}}
	b.Lanes[Doing] = []*Card{{ID: "x"}}

	if idx := b.MoveAt(Todo, 0, Doing, 99, now); idx != 1 || laneIDs(b, Doing) != "x a" {
		t.Errorf("an at past the end should clamp to the end: idx=%d doing=%q", idx, laneIDs(b, Doing))
	}
	if idx := b.MoveAt(Todo, 0, Doing, -3, now); idx != 0 || laneIDs(b, Doing) != "b x a" {
		t.Errorf("a negative at should clamp to the top: idx=%d doing=%q", idx, laneIDs(b, Doing))
	}
	if idx := b.MoveAt(Todo, 5, Doing, 0, now); idx != -1 {
		t.Errorf("an out-of-range source index should be -1, got %d", idx)
	}
	if idx := b.MoveAt(Todo, -1, Doing, 0, now); idx != -1 {
		t.Errorf("a negative source index should be -1, got %d", idx)
	}
	if got := laneIDs(b, Doing); got != "b x a" {
		t.Errorf("a rejected move must not touch the board: %q", got)
	}
}

// MoveAt at the top of a lane must be Move: the two are one rule for what a
// lane change does, and only the position differs. Cheap insurance against
// them drifting apart.
func TestMoveAtAtTheTopMatchesMove(t *testing.T) {
	build := func() *Board {
		b := &Board{}
		old := now.AddDate(0, 0, -5)
		b.Lanes[Todo] = []*Card{{ID: "a", MovedAt: old}, {ID: "b", MovedAt: old}}
		b.Lanes[Done] = []*Card{{ID: "d", MovedAt: old, DoneAt: old}}
		return b
	}
	for _, to := range Lanes {
		byMove, byMoveAt := build(), build()
		from, i := Todo, 1
		if to == Todo {
			from, i = Done, 0
		}
		byMove.Move(from, i, to, now)
		byMoveAt.MoveAt(from, i, to, 0, now)
		for _, l := range Lanes {
			if laneIDs(byMove, l) != laneIDs(byMoveAt, l) {
				t.Fatalf("into %v: Move gave %q, MoveAt gave %q", to, laneIDs(byMove, l), laneIDs(byMoveAt, l))
			}
			for j, c := range byMove.Lanes[l] {
				if got := byMoveAt.Lanes[l][j]; !got.MovedAt.Equal(c.MovedAt) || !got.DoneAt.Equal(c.DoneAt) {
					t.Errorf("into %v, card %q: Move stamped %v/%v, MoveAt %v/%v",
						to, c.ID, c.MovedAt, c.DoneAt, got.MovedAt, got.DoneAt)
				}
			}
		}
	}
}

// Tidying the order inside Done must not touch DoneAt: it is what the age
// both surfaces show is measured from, and what the archive groups by week.
func TestMoveAtWithinDoneKeepsDoneAt(t *testing.T) {
	old := now.AddDate(0, 0, -5)
	b := &Board{}
	b.Lanes[Done] = []*Card{{ID: "d", MovedAt: old, DoneAt: old}, {ID: "e", MovedAt: old, DoneAt: old}}
	b.MoveAt(Done, 0, Done, 2, now)
	if laneIDs(b, Done) != "e d" {
		t.Fatalf("done = %q, want %q", laneIDs(b, Done), "e d")
	}
	for _, c := range b.Lanes[Done] {
		if !c.DoneAt.Equal(old) || !c.MovedAt.Equal(old) {
			t.Errorf("%q restamped: moved=%v done=%v", c.ID, c.MovedAt, c.DoneAt)
		}
	}
}
