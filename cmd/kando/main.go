// Command kando opens a personal kanban board in the terminal.
//
//	kando [board]
//
// Boards live under $KANDO_HOME (default ~/.kando) as plain Markdown files.
// KANDO_THEME=paper|ember forces the light or dark palette; NO_COLOR drops colours.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/kaiohenricunha/kando/internal/store"
	"github.com/kaiohenricunha/kando/internal/tui"
)

const version = "0.1.0"

func usage() {
	fmt.Fprintln(os.Stderr, `usage: kando [board]

Opens the board (default "life") from $KANDO_HOME (default ~/.kando).
Environment: KANDO_HOME, KANDO_THEME=paper|ember, NO_COLOR`)
}

func main() {
	name := "life"
	args := os.Args[1:]
	if len(args) > 1 {
		usage()
		os.Exit(2)
	}
	if len(args) == 1 {
		switch args[0] {
		case "-h", "--help", "help":
			usage()
			return
		case "-v", "--version", "version":
			fmt.Println("kando " + version)
			return
		default:
			if strings.HasPrefix(args[0], "-") {
				usage()
				os.Exit(2)
			}
			name = args[0]
		}
	}

	root := os.Getenv("KANDO_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fatal(err)
		}
		root = filepath.Join(home, ".kando")
	}

	st, b, err := store.Open(root, name)
	if err != nil {
		fatal(err)
	}

	// Resolve the theme before Bubble Tea takes over the terminal: background
	// detection talks to the terminal directly.
	styles := resolveStyles()

	var changes <-chan struct{}
	var stop func()
	if ch, st, err := st.Watch(); err != nil {
		fmt.Fprintln(os.Stderr, "kando: file watching disabled:", err)
	} else {
		changes, stop = ch, st
	}

	m := tui.New(tui.Options{Store: st, Board: b, Root: root, Styles: styles, Changes: changes, StopWatch: stop})
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fatal(err)
	}
}

// resolveStyles picks the palette once. NO_COLOR keeps the terminal's real
// colour profile (so bold and strikethrough still render) but never applies a
// colour. Without KANDO_THEME the background is taken from Lip Gloss's global
// detection, which Bubble Tea already ran (and cached) in its package init, so
// no second terminal query is issued.
func resolveStyles() tui.Styles {
	r := lipgloss.NewRenderer(os.Stdout)
	noColor := termenv.EnvNoColor()
	if noColor {
		r.SetColorProfile(termenv.NewOutput(os.Stdout).ColorProfile())
	}
	var dark bool
	switch strings.ToLower(os.Getenv("KANDO_THEME")) {
	case "paper":
		r.SetHasDarkBackground(false)
	case "ember":
		dark = true
		r.SetHasDarkBackground(true)
	default:
		dark = lipgloss.HasDarkBackground()
		r.SetHasDarkBackground(dark)
	}
	return tui.Resolve(r, tui.DefaultTheme, dark, noColor)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "kando:", err)
	os.Exit(1)
}
