package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

// fixedNow is the clock every golden frame is rendered against.
var fixedNow = time.Date(2026, 9, 3, 12, 0, 0, 0, time.FixedZone("-03", -3*3600))

func sampleBoard(t *testing.T) *board.Board {
	t.Helper()
	data, err := os.ReadFile("../store/testdata/sample_board.md")
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := store.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	b.Name = "life"
	return b
}

func sampleArchive(t *testing.T) *board.Archive {
	t.Helper()
	data, err := os.ReadFile("../store/testdata/sample_archive.md")
	if err != nil {
		t.Fatal(err)
	}
	a, _, err := store.ParseArchive(data)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func newTestModel(t *testing.T, w, h int) Model {
	t.Helper()
	m := New(Options{
		Board:   sampleBoard(t),
		Archive: sampleArchive(t),
		Styles:  testStyles,
		Now:     func() time.Time { return fixedNow },
		Width:   w,
		Height:  h,
	})
	return m
}

// press feeds key names ("j", "enter", "esc", "shift+tab", "ctrl+s", "H", ...) to Update.
func press(m Model, keys ...string) Model {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case "shift+tab":
			msg = tea.KeyMsg{Type: tea.KeyShiftTab}
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "left":
			msg = tea.KeyMsg{Type: tea.KeyLeft}
		case "right":
			msg = tea.KeyMsg{Type: tea.KeyRight}
		case "backspace":
			msg = tea.KeyMsg{Type: tea.KeyBackspace}
		case "ctrl+s":
			msg = tea.KeyMsg{Type: tea.KeyCtrlS}
		case "ctrl+c":
			msg = tea.KeyMsg{Type: tea.KeyCtrlC}
		case "space":
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

// typeText feeds every rune of s as a separate key press.
func typeText(m Model, s string) Model {
	for _, r := range s {
		if r == ' ' {
			m = press(m, "space")
		} else {
			m = press(m, string(r))
		}
	}
	return m
}

func resize(m Model, w, h int) Model {
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(Model)
}

func plainView(m Model) string { return ansi.Strip(m.View()) }

func plainLines(m Model) []string { return strings.Split(plainView(m), "\n") }
