package tui

import (
	"strings"
	"unicode/utf8"

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
// This is the TUI's render-time guard for text read off the board: every
// read-only pane puts its strings through it, which is why board.md content
// that never passed a write-time sanitizer — a hand-edited file, or a note
// stored by a build from before those sanitizers existed — is safe on screen.
// The inline editors render their own buffer and do not call this; they are
// covered instead at their seed sites in detail.go, because bubbles' own
// sanitizer drops only unicode.IsControl and lets the bidi controls through.
//
// The fast path must test for non-ASCII too. It is a byte-wise scan, and the
// unsafe runes that are not C0 (the C1 block, the bidi overrides) are all
// multi-byte UTF-8, so a byte-wise search for "< 0x20" cannot see them: a
// string carrying U+009B (CSI — "ESC [" as one rune, with no ESC byte to
// find) would otherwise take the early return and reach the terminal intact.
// Any byte >= 0x80 therefore falls through to the rune loop, which decides
// with board.UnsafeRune.
//
// The cost of that bail-out is measured, not assumed — BenchmarkSanitize and
// BenchmarkBoardFrame exist to answer it. One call is 6.6 ns and no allocation
// on the ASCII path, 140-220 ns and one allocation off it. A whole 120x40
// frame is ~0.6 ms and ~1690 allocations, and prefixing every card title with
// a non-ASCII rune — so that not one of them takes the fast path — moves that
// to ~1700 allocations and leaves the time inside run-to-run noise (both
// variants span 0.59-0.71 ms over five runs).
//
// So it is not worth narrowing this test to the three lead bytes that can
// begin an unsafe rune (0xC2, 0xD8, 0xE2). That would buy roughly eleven
// allocations per keystroke out of seventeen hundred, and it would cost a
// second, hand-maintained encoding of which runes are unsafe — exactly the
// duplication board.UnsafeRune was introduced to remove.
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

// maxSlotBytes bounds the bytes one cell of a row may carry.
//
// A cell is normally at most 4 bytes of UTF-8. The headroom is for legitimate
// grapheme clusters, and it is generous on purpose: the densest this repo
// knows of is the England flag tag sequence at 28 bytes over 2 cells, so 64
// leaves about four times the margin. Nothing real approaches it.
//
// What does approach it is a value that measures far fewer cells than it has
// bytes, because ansi.StringWidth clusters correctly: a base character with
// hundreds of combining marks is one cluster of width 1, and FirstNoteLine can
// hand a whole 16 KiB notes body to a one-cell slot. The cell arithmetic was
// never wrong; only the bytes were unbounded, and they were emitted on every
// frame.
const maxSlotBytes = 64

// trunc keeps the first w-1 cells and appends "…" when s is wider than w, and
// bounds the bytes it emits either way.
func trunc(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if width(s) > w {
		s = ansi.Truncate(s, w, ellipsis)
	}
	// The byte ceiling applies even when the cell count already fits: that is
	// the case ansi.Truncate never sees. Cut on a rune boundary; fit() and
	// hsplit() both re-measure afterwards, so the exact-cell contract holds.
	if n := w * maxSlotBytes; len(s) > n {
		for n > 0 && !utf8.RuneStart(s[n]) {
			n--
		}
		s = s[:n]
	}
	return s
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
