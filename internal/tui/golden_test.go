package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kaiohenricunha/kando/internal/board"
)

// assertFrame compares a rendered frame with a golden file byte for byte and
// reports the first differing line with a caret under the first differing cell.
func assertFrame(t *testing.T, want, got string, w, h int) {
	t.Helper()
	gotLines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	wantLines := strings.Split(strings.TrimSuffix(want, "\n"), "\n")
	if len(gotLines) != h {
		t.Errorf("frame has %d lines, want %d", len(gotLines), h)
	}
	for i, l := range gotLines {
		if width(l) != w {
			t.Errorf("line %d has width %d, want %d: %q", i+1, width(l), w, l)
		}
	}
	if got == want {
		return
	}
	for i := 0; i < len(wantLines) && i < len(gotLines); i++ {
		if gotLines[i] == wantLines[i] {
			continue
		}
		col := 0
		gr, wr := []rune(gotLines[i]), []rune(wantLines[i])
		for col < len(gr) && col < len(wr) && gr[col] == wr[col] {
			col++
		}
		vis := func(s string) string { return strings.ReplaceAll(s, " ", "·") }
		t.Errorf("first difference at line %d, cell %d:\n got: %s\nwant: %s\n      %s^",
			i+1, col+1, vis(gotLines[i]), vis(wantLines[i]), strings.Repeat(" ", col))
		return
	}
	t.Errorf("frames differ in length only: got %d lines, want %d", len(gotLines), len(wantLines))
}

func readGolden(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestGoldenBoard(t *testing.T) {
	cases := []struct {
		name string
		file string
		w, h int
		keys []string
	}{
		{"todo active", "board_120x40.txt", 120, 40, nil},
		{"done active", "board_120x40_done.txt", 120, 40, []string{"l", "l"}},
		{"narrow", "board_80x24.txt", 80, 24, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newTestModel(t, c.w, c.h)
			m = press(m, c.keys...)
			got := ansi.Strip(m.View()) + "\n"
			assertFrame(t, readGolden(t, c.file), got, c.w, c.h)
		})
	}
}

func TestViewHasNoTrailingNewline(t *testing.T) {
	m := newTestModel(t, 120, 40)
	if strings.HasSuffix(m.View(), "\n") {
		t.Errorf("View() must not end with a newline")
	}
}

var invariantSizes = [][2]int{{60, 16}, {80, 24}, {100, 30}, {119, 40}, {120, 40}, {160, 50}}

// boardStates are the board-screen states every size must render exactly.
var boardStates = map[string][]string{
	"todo":         nil,
	"done":         {"l", "l"},
	"backlog":      {"h"},
	"quick add":    {"a", "H", "e", "l", "l", "o"},
	"help":         {"?"},
	"filter typed": {"/", "#", "h", "o", "m", "e"},
	"filter kept":  {"/", "#", "h", "o", "m", "e", "enter"},
	"filter empty": {"/", "z", "z", "z"},
	"scrolled":     {"j", "j", "j", "j"},
}

func TestWidthInvariantsBoard(t *testing.T) {
	for name, keys := range boardStates {
		for _, sz := range invariantSizes {
			w, h := sz[0], sz[1]
			m := newTestModel(t, w, h)
			m = press(m, keys...)
			checkInvariants(t, name, m, w, h)
		}
	}
}

func checkInvariants(t *testing.T, name string, m Model, w, h int) {
	t.Helper()
	rows := m.render()
	if len(rows) != h {
		t.Errorf("%s %dx%d: render() has %d rows, want %d", name, w, h, len(rows), h)
	}
	for i, r := range rows {
		if width(r) != w {
			t.Errorf("%s %dx%d: row %d width %d, want %d: %q", name, w, h, i+1, width(r), w, ansi.Strip(r))
		}
	}
	if v := m.View(); v != strings.Join(rows, "\n") {
		t.Errorf("%s %dx%d: View() differs from render(); frame() had to repair something", name, w, h)
	}
}

func TestTooSmall(t *testing.T) {
	m := newTestModel(t, 59, 16)
	lines := plainLines(m)
	if len(lines) != 16 || strings.TrimRight(lines[0], " ") != "terminal too small (min 60x16)" {
		t.Errorf("too small frame: %q", lines[0])
	}
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) != "" {
			t.Errorf("too small frame must be otherwise blank: %q", l)
		}
	}
	m = newTestModel(t, 60, 15)
	if !strings.HasPrefix(plainLines(m)[0], "terminal too small") {
		t.Errorf("height 15 should be too small")
	}
}

func TestResizeRelayout(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = resize(m, 80, 24)
	got := ansi.Strip(m.View()) + "\n"
	assertFrame(t, readGolden(t, "board_80x24.txt"), got, 80, 24)
	if m.lane != board.Todo {
		t.Errorf("resize changed the active lane")
	}
}
