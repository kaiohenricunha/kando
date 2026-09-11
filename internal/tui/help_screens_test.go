package tui

import (
	"strings"
	"testing"
)

// The help overlay used to be one table drawn over every screen. On the detail
// screen it said x deletes a card and J/K reorder the lane, when there x toggles
// a checklist item and J/K open the next card: the key a user looked up did
// something else.
func TestHelpOverlayShowsTheCurrentScreensKeys(t *testing.T) {
	sample := func(t *testing.T) Model { return newTestModel(t, 120, 40) }
	root := func(t *testing.T) Model { m, _ := newRootModel(t, 120, 40); return m }
	cases := []struct {
		name    string
		model   func(t *testing.T) Model
		keys    []string
		want    []string
		mustNot []string
	}{
		{"board", sample, []string{"?"},
			[]string{"KEYS", "tab      next lane", "⇧tab     previous lane", "H/L      move card"},
			[]string{"H/L      move card ±lane"}},
		{"detail", sample, []string{"enter", "?"},
			[]string{"KEYS", "j/k ↓↑   select item", "J/K      next/prev card", "x        toggle item", "enter    edit item",
				"o        new item", "e        edit notes", "A        archive (Done)", "T        title", "t        tag",
				"b        block", "m        move to lane", "esc      back", "ctrl+s   save notes", "?        close help"},
			[]string{"x        delete card", "J/K      reorder in lane", "q        quit"}},
		{"archive", sample, []string{"D", "?"},
			[]string{"KEYS", "j/k ↓↑   select card", "u        back to Doing", "enter    open card", "/        filter",
				"esc      board", "?        close help"},
			[]string{"x        delete card", "a        quick add", "q        quit"}},
		{"boards", root, []string{"B", "?"},
			[]string{"KEYS", "j/k ↓↑   select board", "enter    open board", "n        new board", "esc      back",
				"?        close help"},
			[]string{"x        delete card", "a        quick add", "q        quit"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := press(c.model(t), c.keys...)
			if !m.help {
				t.Fatal("? must open help")
			}
			view := plainView(m)
			for _, w := range c.want {
				if !strings.Contains(view, w) {
					t.Errorf("help on the %s screen is missing %q", c.name, w)
				}
			}
			for _, w := range c.mustNot {
				if strings.Contains(view, w) {
					t.Errorf("help on the %s screen lists a key that does something else there: %q", c.name, w)
				}
			}
		})
	}
}

// The help tables, the footers and the key handlers are three lists of the same
// keys. This pins what a test can see: every key a screen's footer shows is in
// that screen's help table, and every label fits its column, since fit and
// spaces truncate or pad without complaint.
func TestHelpTablesCoverTheFooterAndFitTheBox(t *testing.T) {
	screens := []struct {
		name   string
		m      Model
		footer []keyGroup
	}{
		{"board", Model{scr: screenBoard}, boardFooterFull},
		{"detail", Model{scr: screenDetail}, detailFooterGroups},
		{"archive", Model{scr: screenArchive}, archiveFooterGroups},
		{"boards", Model{scr: screenBoards}, boardsFooterGroups},
	}
	tokens := func(key string) []string {
		if key == "/" {
			return []string{"/"}
		}
		return strings.FieldsFunc(key, func(r rune) bool { return r == ' ' || r == '/' })
	}
	canvas := helpWidth - 4
	for _, sc := range screens {
		t.Run(sc.name, func(t *testing.T) {
			table := sc.m.helpFor()
			have := map[string]bool{}
			for _, g := range append(append([]keyGroup{}, table.left...), table.right...) {
				for _, tok := range tokens(g.key) {
					have[tok] = true
				}
			}
			for _, g := range sc.footer {
				for _, tok := range tokens(g.key) {
					if !have[tok] {
						t.Errorf("the %s footer shows %q, but its help table does not list it", sc.name, tok)
					}
				}
			}
			for _, g := range table.left {
				if width(g.key) > 8 || width(g.label) > 19 {
					t.Errorf("left entry %q %q does not fit its column", g.key, g.label)
				}
			}
			for _, g := range table.right {
				if width(g.key) > 8 || 29+9+width(g.label) > canvas {
					t.Errorf("right entry %q %q does not fit the box", g.key, g.label)
				}
			}
		})
	}
}
