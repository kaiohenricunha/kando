package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/kaiohenricunha/kando/internal/board"
)

// cardPalette is the set of styles a card is drawn with (plain or selected).
type cardPalette struct {
	fg, fgBold, muted, accent, accent2, strike, fill lipgloss.Style
}

func (m Model) palette(selected bool) cardPalette {
	s := m.styles
	if selected {
		return cardPalette{fg: s.SelFg, fgBold: s.SelFgBold, muted: s.SelMuted, accent: s.SelAccent, accent2: s.SelAccent2, strike: s.SelStrike, fill: s.SelFill}
	}
	return cardPalette{fg: s.Fg, fgBold: s.Bold, muted: s.Muted, accent: s.Accent, accent2: s.Accent2, strike: s.Strike, fill: s.Plain}
}

// fitWith pads a styled string to w cells using fill for the padding.
func fitWith(s string, w int, fill lipgloss.Style) string {
	s = trunc(s, w)
	return s + render(fill, spaces(w-width(s)))
}

// cardRows draws a 5-row card box of the given outer width.
func (m Model) cardRows(c *board.Card, l board.Lane, outerW int, selected bool) []string {
	p := m.palette(selected)
	tw := outerW - 6
	title := sanitize(c.Title)
	age := board.Age(m.now(), c.AgeSince())
	gap := render(p.fill, "  ")
	done := l == board.Done

	var r0, r2 string
	var meta []string
	if done {
		r0 = p.accent.Render("✓") + render(p.fill, " ") + p.strike.Render(fit(title, tw-2))
		if c.Tag != "" {
			meta = append(meta, p.accent.Render("#"+sanitize(c.Tag)))
		}
		meta = append(meta, p.muted.Render(age))
	} else {
		ageSlot := 3
		if width(age) > ageSlot {
			ageSlot = width(age)
		}
		r0 = p.fg.Render(fit(title, tw-2-ageSlot)) + gap + p.muted.Render(right(age, ageSlot))
		if selected {
			r0 = p.fgBold.Render(fit(title, tw-2-ageSlot)) + gap + p.muted.Render(right(age, ageSlot))
		}
		if c.Tag != "" {
			meta = append(meta, p.accent.Render("#"+sanitize(c.Tag)))
		}
		if pl := c.ProgressLabel(); pl != "" {
			meta = append(meta, p.muted.Render(pl))
		}
		if bl := c.BlockedLabel(); bl != "" {
			meta = append(meta, p.accent2.Render("⊘ "+sanitize(bl)))
		}
	}
	r1 := p.muted.Render(fit(sanitize(c.FirstNoteLine()), tw))
	r2 = fitWith(strings.Join(meta, gap), tw, p.fill)

	border := lipgloss.RoundedBorder()
	if done {
		border = dashedBorder
	}
	borderStyle := m.styles.Border
	if selected {
		borderStyle = m.styles.Accent
	}
	return box([]string{r0, r1, r2}, outerW, border, borderStyle, p.fill, 2)
}

// quickAddRows draws the 5-row input card shown at the top of the active lane.
func (m Model) quickAddRows(outerW int) []string {
	s := m.styles
	tw := outerW - 6
	r0 := fitWith(m.quick.View(), tw, s.SelFill)
	blank := render(s.SelFill, spaces(tw))
	return box([]string{r0, blank, blank}, outerW, lipgloss.RoundedBorder(), s.Accent, s.SelFill, 2)
}
