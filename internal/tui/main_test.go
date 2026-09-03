package tui

import (
	"io"
	"os"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// testStyles renders real truecolor SGR sequences (dark theme) so width bugs caused
// by escape codes are caught; tests strip them before comparing.
var testStyles Styles

func TestMain(m *testing.M) {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)
	r.SetHasDarkBackground(true)
	testStyles = Resolve(r, DefaultTheme, true, false)
	os.Exit(m.Run())
}
