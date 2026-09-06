package board

import (
	"testing"
	"time"
)

func card(title, tag string, ageDays int, blocked bool) *Card {
	return &Card{Title: title, Tag: tag, CreatedAt: now.Add(-time.Duration(ageDays) * 24 * time.Hour), Blocked: blocked}
}

func TestFilterMatch(t *testing.T) {
	sourdough := card("Learn to make sourdough", "home", 12, false)
	brake := card("Fix bike brake", "home", 4, true)
	dentist := card("Book dentist", "health", 1, false)
	fresh := &Card{Title: "Fresh", CreatedAt: now.Add(-36 * time.Hour)}
	untagged := card("Birthday card for mom", "", 1, false)

	cases := []struct {
		q    string
		c    *Card
		want bool
	}{
		{"", sourdough, true},
		{"   ", sourdough, true},
		{"sour", sourdough, true},
		{"SOUR", sourdough, true},
		{"make sour", sourdough, true}, // tokens are AND-ed substrings
		{"make xyz", sourdough, false},
		{"home", sourdough, false}, // plain text does not search the tag
		{"#home", sourdough, true},
		{"#HOME", sourdough, true},
		{"#ho", sourdough, true}, // substring over #tag
		{"#home", dentist, false},
		{"#", untagged, true}, // "#" alone matches "#" + "" ... treated as substring of "#"
		{"!blocked", brake, true},
		{"!blocked", sourdough, false},
		{"#home !blocked", brake, true},
		{"#home !blocked", sourdough, false},
		{"age>7d", sourdough, true},
		{"age>7d", brake, false},
		{"age>12d", sourdough, false},
		{"age<3d", dentist, true},
		{"age<3d", brake, false},
		{"age<1d", dentist, false},
		{"age>36h", fresh, false},
		{"age>35h", fresh, true},
		{"age<37h", fresh, true},
		{"age>>7d", sourdough, false}, // malformed → plain text, no title contains it
		{"age", sourdough, false},
		{"bike age<7d", brake, true},
	}
	for _, c := range cases {
		f := Parse(c.q)
		if got := f.Match(c.c, now); got != c.want {
			t.Errorf("Parse(%q).Match(%q) = %v, want %v", c.q, c.c.Title, got, c.want)
		}
	}
	if !Parse("  ").Empty() || Parse("x").Empty() {
		t.Errorf("Empty() wrong")
	}
}

func TestFilterDoneCardUsesDoneAt(t *testing.T) {
	c := &Card{Title: "Pay electric bill", CreatedAt: now.Add(-30 * 24 * time.Hour), DoneAt: now.Add(-3 * 24 * time.Hour)}
	if !Parse("age<5d").Match(c, now) {
		t.Errorf("done card age should be measured from DoneAt")
	}
}

func TestBoardMatching(t *testing.T) {
	sourdough := card("Learn to make sourdough", "home", 12, false)
	brake := card("Fix bike brake", "home", 4, true)
	dentist := card("Book dentist", "health", 1, false)

	b := &Board{}
	b.Lanes[Backlog] = []*Card{sourdough}
	b.Lanes[Todo] = []*Card{brake, dentist}

	t.Run("empty filter matches everything, lane order preserved", func(t *testing.T) {
		lanes, matched, total := b.Matching(Parse(""), now)
		if matched != 3 || total != 3 {
			t.Fatalf("matched=%d total=%d", matched, total)
		}
		if len(lanes[Backlog]) != 1 || lanes[Backlog][0] != sourdough {
			t.Errorf("Backlog: %v", lanes[Backlog])
		}
		if len(lanes[Todo]) != 2 || lanes[Todo][0] != brake || lanes[Todo][1] != dentist {
			t.Errorf("Todo: %v", lanes[Todo])
		}
	})

	t.Run("filter narrows matched but not total", func(t *testing.T) {
		lanes, matched, total := b.Matching(Parse("#home"), now)
		if matched != 2 || total != 3 {
			t.Fatalf("matched=%d total=%d", matched, total)
		}
		if len(lanes[Backlog]) != 1 || len(lanes[Todo]) != 1 || lanes[Todo][0] != brake {
			t.Errorf("lanes: backlog=%v todo=%v", lanes[Backlog], lanes[Todo])
		}
	})

	t.Run("a lane with no matches is an empty, non-nil-checked slice", func(t *testing.T) {
		lanes, matched, _ := b.Matching(Parse("#health"), now)
		if matched != 1 || len(lanes[Backlog]) != 0 || len(lanes[Doing]) != 0 {
			t.Errorf("matched=%d backlog=%v doing=%v", matched, lanes[Backlog], lanes[Doing])
		}
	})

	t.Run("empty board", func(t *testing.T) {
		lanes, matched, total := (&Board{}).Matching(Parse(""), now)
		if matched != 0 || total != 0 {
			t.Errorf("matched=%d total=%d", matched, total)
		}
		for _, l := range lanes {
			if len(l) != 0 {
				t.Errorf("expected every lane empty, got %v", l)
			}
		}
	})
}
