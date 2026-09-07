package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kaiohenricunha/kando/internal/board"
)

// maxNotesInputBytes caps how much a --file or stdin source is allowed to
// provide before SetNotes' own 16 KiB clip runs — large enough that the
// clip, not this cap, is what a normal file trips, small enough that a
// misdirected `cat /dev/urandom` cannot hang the process reading it all in.
const maxNotesInputBytes = 1 << 20

// notesSource is which of --set/--file/- kando notes was given, deferring
// the actual read (I/O) out of the pure argument parser.
type notesSource struct {
	kind  string // "set", "file", or "stdin"
	value string // the --set text, or the --file path; unused for stdin
}

// notesArgs parses kando notes' arguments: a card, an optional board, and
// exactly one of --set TEXT, --file PATH, or a bare "-" positional for
// stdin (Go's flag package never treats a lone "-" as a flag, so it always
// survives parseMixed as a positional).
func notesArgs(args []string, errOut io.Writer) (card, name string, src notesSource, err error) {
	fs := flag.NewFlagSet("notes", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() {}
	setFlag := fs.String("set", "", "replace notes with this text")
	fileFlag := fs.String("file", "", "replace notes with this file's contents")
	pos, err := parseMixed(fs, args)
	if err != nil {
		return "", "", notesSource{}, err
	}

	stdin := false
	if len(pos) > 0 && pos[len(pos)-1] == "-" {
		stdin = true
		pos = pos[:len(pos)-1]
	}

	vals, name, err := splitBoard("notes", "a card", pos, 1)
	if err != nil {
		return "", "", notesSource{}, err
	}

	setGiven, fileGiven := false, false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "set":
			setGiven = true
		case "file":
			fileGiven = true
		}
	})

	n := 0
	for _, given := range []bool{setGiven, fileGiven, stdin} {
		if given {
			n++
		}
	}
	switch {
	case n == 0:
		return "", "", notesSource{}, fmt.Errorf("kando notes needs --set, --file or -")
	case n > 1:
		return "", "", notesSource{}, fmt.Errorf("use only one of --set, --file or -")
	}

	switch {
	case setGiven:
		src = notesSource{kind: "set", value: *setFlag}
	case fileGiven:
		src = notesSource{kind: "file", value: *fileFlag}
	default:
		src = notesSource{kind: "stdin"}
	}
	if card, err = required(vals[0], "card id or title"); err != nil {
		return "", "", notesSource{}, err
	}
	return card, name, src, nil
}

// readNotes resolves src to the text kando notes will set — the one place
// this verb touches a file or stdin, kept separate from notesArgs so
// argument parsing stays pure.
func readNotes(src notesSource, stdin io.Reader) (string, error) {
	switch src.kind {
	case "set":
		return src.value, nil
	case "file":
		// Open and LimitReader rather than os.ReadFile: ReadFile sizes its
		// buffer from Stat, and a character device or FIFO reports 0, so it
		// grows until EOF — which /dev/zero never reaches. The cap has to
		// bound the read, not just reject the result, or the constant's own
		// promise about `cat /dev/urandom` is only true of the stdin branch.
		f, err := os.Open(src.value)
		if err != nil {
			return "", err
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, maxNotesInputBytes+1))
		if err != nil {
			return "", err
		}
		if len(data) > maxNotesInputBytes {
			return "", fmt.Errorf("--file is larger than %d bytes", maxNotesInputBytes)
		}
		return string(data), nil
	case "stdin":
		data, err := io.ReadAll(io.LimitReader(stdin, maxNotesInputBytes+1))
		if err != nil {
			return "", err
		}
		if len(data) > maxNotesInputBytes {
			return "", fmt.Errorf("stdin is larger than %d bytes", maxNotesInputBytes)
		}
		return string(data), nil
	default:
		return "", fmt.Errorf("no notes source")
	}
}

// setNotes is kando notes' testable core. SetNotes itself normalizes line
// endings, clips to 16 KiB, and trims trailing blank lines — stored is that
// final value, not the raw input, so a caller reporting on it (e.g. a line
// count) describes what actually landed on disk.
func setNotes(root, name, cardArg, text string) (title, stored string, err error) {
	_, c, err := withCard(root, name, cardArg, func(b *board.Board, lane board.Lane, i int, c *board.Card) (bool, error) {
		before := c.Notes
		c.SetNotes(text)
		return c.Notes != before, nil
	})
	if err != nil {
		return "", "", err
	}
	return c.Title, c.Notes, nil
}

func runNotes(args []string) {
	if helpWanted(args) {
		usage()
		return
	}
	cardArg, name, src, err := notesArgs(args, os.Stderr)
	if err != nil {
		usageErr(err)
	}
	text, err := readNotes(src, os.Stdin)
	if err != nil {
		fatal(err)
	}
	title, stored, err := setNotes(kandoRoot(), name, cardArg, text)
	if err != nil {
		fatal(err)
	}
	if stored == "" {
		fmt.Printf("cleared notes on %q\n", title)
		return
	}
	lines := strings.Count(stored, "\n") + 1
	unit := "lines"
	if lines == 1 {
		unit = "line"
	}
	fmt.Printf("set notes on %q (%d %s)\n", title, lines, unit)
}
