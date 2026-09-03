package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestFit(t *testing.T) {
	cases := []struct {
		in   string
		w    int
		want string
	}{
		{"abcdefgh", 5, "abcd…"},
		{"abc", 5, "abc  "},
		{"", 3, "   "},
		{"abc", 0, ""},
		{"abcde", 5, "abcde"},
		{"日本語", 4, "日… "},
		{"日本語", 5, "日本…"},
		{"日本語", 3, "日…"},
		{"•✓⊘…", 3, "•✓…"},
	}
	for _, c := range cases {
		got := fit(c.in, c.w)
		if got != c.want || width(got) != c.w {
			t.Errorf("fit(%q,%d) = %q (width %d), want %q", c.in, c.w, got, width(got), c.want)
		}
	}
	for _, in := range []string{"日本語", "🙂🙂🙂", "a🙂b"} {
		for w := 0; w < 8; w++ {
			if got := fit(in, w); width(got) != w {
				t.Errorf("fit(%q,%d) has width %d: %q", in, w, width(got), got)
			}
		}
	}
	styled := testStyles.Accent.Render("abcdefgh")
	got := fit(styled, 5)
	if width(got) != 5 || ansi.Strip(got) != "abcd…" {
		t.Errorf("styled fit = %q", got)
	}
}

func TestSanitize(t *testing.T) {
	if got := sanitize("a\tb\rc\x1bd\x7fe"); got != "a bc d e" {
		t.Errorf("sanitize = %q", got)
	}
	if got := sanitize("plain"); got != "plain" {
		t.Errorf("sanitize = %q", got)
	}
}

func TestRightAndHsplit(t *testing.T) {
	if got := right("3d", 3); got != " 3d" {
		t.Errorf("right = %q", got)
	}
	if got := right("12d", 3); got != "12d" {
		t.Errorf("right = %q", got)
	}
	if got := right("100d", 3); got != "100d" {
		t.Errorf("right must never truncate: %q", got)
	}
	got := hsplit("kando  life", "Thu 3 Sep", 118)
	if width(got) != 118 || !strings.HasPrefix(got, "kando  life ") || !strings.HasSuffix(got, " Thu 3 Sep") {
		t.Errorf("hsplit = %q", got)
	}
	if got := hsplit("x", "right", 8); got != "x  right" {
		t.Errorf("hsplit = %q", got)
	}
	if got := hsplit("abcdef", "right", 8); got != "…  right" {
		t.Errorf("hsplit shrinks the left item: %q", got)
	}
	if got := hsplit("abc", "", 5); got != "abc  " {
		t.Errorf("hsplit no right = %q", got)
	}
	styled := hsplit(testStyles.Brand.Render("kando"), testStyles.Muted.Render("Thu 3 Sep"), 30)
	if width(styled) != 30 || ansi.Strip(styled) != "kando                Thu 3 Sep" {
		t.Errorf("styled hsplit = %q", ansi.Strip(styled))
	}
}

func TestSplice(t *testing.T) {
	row := strings.Repeat("a", 20)
	if got := splice(row, 5, "XYZ"); got != "aaaaaXYZaaaaaaaaaaaa" {
		t.Errorf("splice = %q", got)
	}
	styled := testStyles.Accent.Render(strings.Repeat("a", 10)) + testStyles.Muted.Render(strings.Repeat("b", 10))
	got := splice(styled, 8, testStyles.Border.Render("XYZW"))
	if width(got) != 20 || ansi.Strip(got) != "aaaaaaaaXYZWbbbbbbbb" {
		t.Errorf("styled splice = %q (%d)", ansi.Strip(got), width(got))
	}
	if got := splice(row, 18, "XYZ"); ansi.Strip(got) != "aaaaaaaaaaaaaaaaaaXYZ" {
		t.Errorf("splice past end = %q", got)
	}
}

func TestBox(t *testing.T) {
	plain := lipgloss.NewStyle()
	got := box([]string{"ab", "cd"}, 6, lipgloss.RoundedBorder(), plain, plain, 1)
	want := []string{"╭────╮", "│ ab │", "│ cd │", "╰────╯"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("box =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	got = box([]string{"ab"}, 8, dashedBorder, plain, plain, 2)
	want = []string{"╭╌╌╌╌╌╌╮", "┆  ab  ┆", "╰╌╌╌╌╌╌╯"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("dashed box =\n%s", strings.Join(got, "\n"))
	}
	styled := box([]string{testStyles.SelFg.Render("ab")}, 6, lipgloss.RoundedBorder(), testStyles.Accent, testStyles.SelFill, 1)
	for _, l := range styled {
		if width(l) != 6 {
			t.Errorf("styled box row width %d: %q", width(l), l)
		}
	}
}

func TestFrame(t *testing.T) {
	got := frame([]string{"abc"}, 5, 3)
	want := []string{"abc  ", "     ", "     "}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("frame = %q", got)
	}
	got = frame([]string{"abcdefg", "x", "y", "z"}, 3, 2)
	want = []string{"ab…", "x  "}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("frame clip = %q", got)
	}
}
