// Package tui renders the kando board with Bubble Tea and Lip Gloss.
package tui

import "github.com/charmbracelet/lipgloss"

// Theme holds the design tokens (Paper = light, Ember = dark) from docs/design/README.md.
type Theme struct {
	Bg, Fg, Muted, Border, Accent, Accent2, SelectionBg lipgloss.AdaptiveColor
}

// DefaultTheme is the handoff palette.
var DefaultTheme = Theme{
	Bg:          lipgloss.AdaptiveColor{Light: "#f6f1e8", Dark: "#1a1512"},
	Fg:          lipgloss.AdaptiveColor{Light: "#2b2622", Dark: "#ead9c8"},
	Muted:       lipgloss.AdaptiveColor{Light: "#8f8579", Dark: "#7d6b5c"},
	Border:      lipgloss.AdaptiveColor{Light: "#d9cfc0", Dark: "#33291f"},
	Accent:      lipgloss.AdaptiveColor{Light: "#1f7a6d", Dark: "#e0a458"},
	Accent2:     lipgloss.AdaptiveColor{Light: "#b5532b", Dark: "#8fb98a"},
	SelectionBg: lipgloss.AdaptiveColor{Light: "#ece4d6", Dark: "#2a211a"},
}

// Styles are the concrete Lip Gloss styles for one resolved theme. They are built
// once at startup; rendering never consults the terminal again.
type Styles struct {
	Dark    bool
	NoColor bool

	Plain, Fg, Muted, Border, Accent, Accent2 lipgloss.Style
	Brand                                     lipgloss.Style // "kando" in the header
	Bold                                      lipgloss.Style // fg bold: footer keys, titles
	AccentBold                                lipgloss.Style // active lane header
	Strike                                    lipgloss.Style // done titles: muted + strikethrough
	Cursor                                    lipgloss.Style // reverse-video text cursor
	TabActive                                 lipgloss.Style // narrow-mode active tab

	// Selected-card variants: the selection background on every cell.
	SelFill, SelFg, SelFgBold, SelMuted, SelAccent, SelAccent2, SelStrike lipgloss.Style
}

// Resolve picks the light or dark value of every token and builds the styles on r.
// With noColor set, no foreground or background colour is ever applied, but bold,
// strikethrough and reverse video are kept.
func Resolve(r *lipgloss.Renderer, th Theme, dark, noColor bool) Styles {
	pick := func(c lipgloss.AdaptiveColor) lipgloss.Color {
		if dark {
			return lipgloss.Color(c.Dark)
		}
		return lipgloss.Color(c.Light)
	}
	fg, muted, border := pick(th.Fg), pick(th.Muted), pick(th.Border)
	accent, accent2, selBg, bg := pick(th.Accent), pick(th.Accent2), pick(th.SelectionBg), pick(th.Bg)
	col := func(st lipgloss.Style, c lipgloss.Color) lipgloss.Style {
		if noColor {
			return st
		}
		return st.Foreground(c)
	}
	onBg := func(st lipgloss.Style, c lipgloss.Color) lipgloss.Style {
		if noColor {
			return st
		}
		return st.Background(c)
	}
	base := r.NewStyle()
	s := Styles{Dark: dark, NoColor: noColor, Plain: base}
	s.Fg = col(base, fg)
	s.Muted = col(base, muted)
	s.Border = col(base, border)
	s.Accent = col(base, accent)
	s.Accent2 = col(base, accent2)
	brand := fg
	if dark {
		brand = accent
	}
	s.Brand = col(base, brand).Bold(true)
	s.Bold = s.Fg.Bold(true)
	s.AccentBold = s.Accent.Bold(true)
	s.Strike = s.Muted.Strikethrough(true).StrikethroughSpaces(false)
	s.Cursor = s.Fg.Reverse(true)
	if noColor {
		s.TabActive = base.Reverse(true)
	} else {
		s.TabActive = onBg(col(base, bg), accent)
	}
	s.SelFill = onBg(base, selBg)
	s.SelFg = onBg(s.Fg, selBg)
	s.SelFgBold = s.SelFg.Bold(true)
	s.SelMuted = onBg(s.Muted, selBg)
	s.SelAccent = onBg(s.Accent, selBg)
	s.SelAccent2 = onBg(s.Accent2, selBg)
	s.SelStrike = onBg(s.Strike, selBg)
	return s
}
