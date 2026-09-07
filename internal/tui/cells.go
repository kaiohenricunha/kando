package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/kaiohenricunha/kando/internal/board"
)

// Every row of every screen is composed here as a string of an exact number of
// terminal cells. Lip Gloss only colours segments; it never lays anything out.

const ellipsis = "…"

// dashedBorder is the Done-card border: dashed edges with rounded corners.
var dashedBorder = lipgloss.Border{
	Top: "╌", Bottom: "╌", Left: "┆", Right: "┆",
	TopLeft: "╭", TopRight: "╮", BottomLeft: "╰", BottomRight: "╯",
}

// width measures a string in terminal cells, ignoring ANSI escape sequences.
func width(s string) int { return ansi.StringWidth(s) }

// sanitize makes user text safe to measure and safe to print. Tabs and C0
// control characters become spaces (they would measure 0 cells but render
// wider), carriage returns vanish, and every other unsafe rune is dropped.
//
// This is the TUI's single render-time guard: every visible string in every
// view goes through it, which is why board.md content that never passed a
// write-time sanitizer — a hand-edited file, or a note stored by a build from
// before those sanitizers existed — is still safe on screen.
//
// The fast path must test for non-ASCII too. It is a byte-wise scan, and the
// unsafe runes that are not C0 (the C1 block, the bidi overrides) are all
// multi-byte UTF-8, so a byte-wise search for "< 0x20" cannot see them: a
// string carrying U+009B (CSI — "ESC [" as one rune, with no ESC byte to
// find) would otherwise take the early return and reach the terminal intact.
// Any byte >= 0x80 therefore falls through to the rune loop, which decides
// with board.UnsafeRune.
func sanitize(s string) string {
	clean := true
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f || s[i] >= 0x80 {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\r':
		case r < 0x20 || r == 0x7f:
			// C0 keeps its column so table layout does not shift.
			b.WriteByte(' ')
		case board.UnsafeRune(r):
			// C1 and the bidi controls measure zero cells; dropping them
			// changes no layout and leaves no visible artefact.
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}

// trunc keeps the first w-1 cells and appends "…" when s is wider than w.
func trunc(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if width(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, ellipsis)
}

// fit truncates or pads s to exactly w cells.
func fit(s string, w int) string {
	s = trunc(s, w)
	return s + spaces(w-width(s))
}

// right right-aligns s in w cells. It never truncates: a wider s overflows.
func right(s string, w int) string { return spaces(w-width(s)) + s }

// hsplit places left at the start and right at the end of a w-cell row with at
// least two spaces between them, shrinking left (never right) when needed.
func hsplit(left, right string, w int) string {
	rw := width(right)
	if rw == 0 {
		return fit(left, w)
	}
	lw := w - 2 - rw
	if lw < 0 {
		return fit(right, w)
	}
	left = trunc(left, lw)
	return left + spaces(w-width(left)-rw) + right
}

// render styles s, emitting nothing for an empty string.
func render(st lipgloss.Style, s string) string {
	if s == "" {
		return ""
	}
	return st.Render(s)
}

// splice overwrites the cells [x, x+width(overlay)) of row with overlay.
func splice(row string, x int, overlay string) string {
	head := ansi.Truncate(row, x, "")
	head += spaces(x - width(head))
	if strings.Contains(head, "\x1b") {
		head += "\x1b[0m"
	}
	tail := ansi.TruncateLeft(row, x+width(overlay), "")
	return head + overlay + tail
}

// box draws a border around inner rows. Each inner row must already be exactly
// outerW-2-2*pad cells wide; the padding cells are rendered with fill.
func box(inner []string, outerW int, b lipgloss.Border, borderStyle, fill lipgloss.Style, pad int) []string {
	iw := outerW - 2
	rows := make([]string, 0, len(inner)+2)
	rows = append(rows, borderStyle.Render(b.TopLeft+strings.Repeat(b.Top, iw)+b.TopRight))
	left, rightB := borderStyle.Render(b.Left), borderStyle.Render(b.Right)
	p := render(fill, spaces(pad))
	for _, r := range inner {
		rows = append(rows, left+p+r+p+rightB)
	}
	rows = append(rows, borderStyle.Render(b.BottomLeft+strings.Repeat(b.Bottom, iw)+b.BottomRight))
	return rows
}

// frame normalises a screen to exactly h rows of exactly w cells.
func frame(rows []string, w, h int) []string {
	out := make([]string, h)
	for i := range out {
		if i < len(rows) {
			out[i] = fit(rows[i], w)
		} else {
			out[i] = spaces(w)
		}
	}
	return out
}
