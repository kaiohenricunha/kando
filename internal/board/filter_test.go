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
