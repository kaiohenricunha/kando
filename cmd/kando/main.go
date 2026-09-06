// Command kando opens a personal kanban board in the terminal, or serves it
// as a local web page.
//
//	kando [board]
//	kando web [board] [--port N]
//
// Boards live under $KANDO_HOME (default ~/.kando) as plain Markdown files.
// KANDO_THEME=paper|ember forces the light or dark palette; NO_COLOR drops colours.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/kaiohenricunha/kando/internal/store"
	"github.com/kaiohenricunha/kando/internal/tui"
	"github.com/kaiohenricunha/kando/internal/web"
)

const version = "0.1.0"

// usageText is kando's whole usage message. TestUsageListsEveryVerb checks
// every key of verbs (and any per-verb subcommand map) appears in it, so a
// new verb that forgets to document itself here fails a test instead of
// silently going undocumented.
const usageText = `usage: kando [board]
       kando web [board] [--port N]
       kando move <card> <lane> [board]

Opens the board (default "life") from $KANDO_HOME (default ~/.kando) in the
terminal, or serves it at http://127.0.0.1:<port>/ (default 4242). The board
name may come before or after the flags. kando move moves <card> — an id or
an exact, case-insensitive title — to <lane> (Backlog, Todo, Doing or Done)
on an existing board; it never creates one. A board literally named "web" or
"move" opens in the terminal with: kando -- web or kando -- move
Environment: KANDO_HOME, KANDO_THEME=paper|ember, NO_COLOR, KANDO_WEB_PORT`

func usage() {
	fmt.Fprintln(os.Stderr, usageText)
}

// kandoRoot is $KANDO_HOME, defaulting to ~/.kando.
func kandoRoot() string {
	if root := os.Getenv("KANDO_HOME"); root != "" {
		return root
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fatal(err)
	}
	return filepath.Join(home, ".kando")
}

// webArgs is the board name and port `kando web` was asked for. Parsing is
// separated from serving so the flag and environment rules (OPS-3) can be
// tested without binding a port or taking over the process.
func webArgs(args []string, getenv func(string) string, errOut io.Writer) (name string, port int, err error) {
	fs := flag.NewFlagSet("kando web", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() {}
	p := fs.Int("port", envPort(getenv, errOut, defaultPort), "port to listen on (or KANDO_WEB_PORT)")
	// Accept the board name before or after the flags: `kando web life --port 1234`.
	if err := fs.Parse(args); err != nil {
		return "", 0, err
	}
	if fs.NArg() > 0 {
		name = fs.Arg(0)
		if err := fs.Parse(fs.Args()[1:]); err != nil {
			return "", 0, err
		}
	}
	if fs.NArg() > 0 {
		return "", 0, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if strings.HasPrefix(name, "-") {
		return "", 0, fmt.Errorf("unexpected flag %q", name)
	}
	if *p < 1 || *p > 65535 {
		return "", 0, fmt.Errorf("invalid port %d: must be 1-65535", *p)
	}
	return name, *p, nil
}

// runWeb serves the board over HTTP on the loopback interface until interrupted.
func runWeb(args []string) {
	name, port, err := webArgs(args, os.Getenv, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "kando:", err)
		usage()
		os.Exit(2)
	}
	root := kandoRoot()
	if name != "" {
		// Like the TUI, naming a board on the command line creates it if needed.
		if _, _, err := store.Open(root, name); err != nil {
			fatal(err)
		}
	}
	ln, err := web.Listen(port)
	if err != nil {
		fatal(err)
	}
	bound := ln.Addr().(*net.TCPAddr).Port
	fmt.Fprintf(os.Stderr, "kando web: serving %s at http://127.0.0.1:%d/ (ctrl+c to stop)\n", root, bound)
	// ctrl+c / SIGTERM drain in-flight requests instead of cutting them off.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := web.Serve(ctx, ln, web.Options{Root: root, Board: name}); err != nil {
		fatal(err)
	}
}

// defaultPort is the port `kando web` uses when nothing else says otherwise (OPS-3).
const defaultPort = 4242

// envPort reads KANDO_WEB_PORT, falling back to def and saying so when the
// value is unusable — a silently ignored setting is worse than a noisy one.
func envPort(getenv func(string) string, errOut io.Writer, def int) int {
	if v := getenv("KANDO_WEB_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n < 65536 {
			return n
		}
		fmt.Fprintf(errOut, "kando: ignoring invalid KANDO_WEB_PORT %q\n", v)
	}
	return def
}

func main() {
	name := "life"
	verb, args := dispatch(os.Args[1:])
	if verb != "" {
		verbs[verb](args)
		return
	}
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

	root := kandoRoot()

	st, b, err := store.Open(root, name)
	if err != nil {
		fatal(err)
	}

	// Resolve the theme before Bubble Tea takes over the terminal: background
	// detection talks to the terminal directly.
	styles := resolveStyles()

	// The model owns the file watcher: it starts one on the board, swaps it
	// on a board switch, tears it down on quit, and shows a watch failure in
	// the footer.
	m := tui.New(tui.Options{Store: st, Board: b, Root: root, Styles: styles, Watch: true})
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
