package tui

import (
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func newTestRenderer() *lipgloss.Renderer {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)
	return r
}

// seq is the SGR parameter string termenv emits for a truecolor hex value.
func seq(hex string, bg bool) string { return termenv.TrueColor.Color(hex).Sequence(bg) }

func TestResolveDarkLight(t *testing.T) {
	dark := Resolve(newTestRenderer(), DefaultTheme, true, false)
	if s := dark.Accent.Render("x"); !strings.Contains(s, seq("#e0a458", false)) {
		t.Errorf("dark accent = %q", s)
	}
	if s := dark.Brand.Render("kando"); !strings.Contains(s, seq("#e0a458", false)) || !strings.Contains(s, "\x1b[1;") && !strings.Contains(s, "\x1b[1m") {
		t.Errorf("dark brand should be bold accent: %q", s)
	}
	light := Resolve(newTestRenderer(), DefaultTheme, false, false)
	if s := light.Accent.Render("x"); !strings.Contains(s, seq("#1f7a6d", false)) {
		t.Errorf("light accent = %q", s)
	}
	if s := light.Brand.Render("kando"); !strings.Contains(s, seq("#2b2622", false)) {
		t.Errorf("light brand should be fg: %q", s)
	}
	if s := dark.SelFg.Render("x"); !strings.Contains(s, seq("#2a211a", true)) {
		t.Errorf("selection bg missing: %q", s)
	}
}

func TestResolveNoColor(t *testing.T) {
	s := Resolve(newTestRenderer(), DefaultTheme, true, true)
	for name, st := range map[string]lipgloss.Style{"Accent": s.Accent, "SelFg": s.SelFg, "Brand": s.Brand, "Strike": s.Strike, "Muted": s.Muted} {
		out := st.Render("x")
		if strings.Contains(out, "38;") || strings.Contains(out, "48;") {
			t.Errorf("%s emits colour under NO_COLOR: %q", name, out)
		}
	}
	if out := s.Brand.Render("x"); !strings.Contains(out, "\x1b[1m") {
		t.Errorf("bold should survive NO_COLOR: %q", out)
	}
	if out := s.Strike.Render("x"); !strings.Contains(out, "\x1b[9m") {
		t.Errorf("strikethrough should survive NO_COLOR: %q", out)
	}
}

func TestStrikeDoesNotCrossSpaces(t *testing.T) {
	out := testStyles.Strike.Render("ab  ")
	// The padding spaces must not carry the strikethrough attribute.
	if strings.Contains(out, "\x1b[9m ") || strings.Contains(out, "9m  ") {
		t.Errorf("padding spaces are struck through: %q", out)
	}
}
