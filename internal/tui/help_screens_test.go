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
		{"detail", sample, []string{"enter", "?"},
			[]string{"KEYS", "j/k ↓↑   select item", "J/K      next/prev card", "x        toggle item", "enter    edit item",
				"o        new item", "e        edit notes", "A        archive (Done)", "T        title", "t        tag",
				"b        block", "m        move to lane", "esc      back", "?        close help"},
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
