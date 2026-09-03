package board

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

type tokKind int

const (
	tokText tokKind = iota
	tokTag
	tokBlocked
	tokAgeGT
	tokAgeLT
)

type token struct {
	kind tokKind
	text string // lowercased text / tag pattern
	n    int
	unit byte // 'd' or 'h'
}

// Filter is a parsed board query: space-separated tokens that must all match.
type Filter struct {
	toks []token
}

var ageRe = regexp.MustCompile(`^age([<>])(\d+)([dh])$`)

// Parse builds a Filter from a query string. Tokens: plain text (title substring),
// "#tag" (substring over "#"+tag), "!blocked", "age>7d" / "age<3d" (also "h").
// Unknown or malformed operators are treated as plain text.
func Parse(q string) Filter {
	var f Filter
	for _, w := range strings.Fields(q) {
		lw := strings.ToLower(w)
		switch {
		case lw == "!blocked":
			f.toks = append(f.toks, token{kind: tokBlocked})
		case strings.HasPrefix(lw, "#"):
			f.toks = append(f.toks, token{kind: tokTag, text: lw})
		default:
			if m := ageRe.FindStringSubmatch(lw); m != nil {
				n, err := strconv.Atoi(m[2])
				if err == nil {
					k := tokAgeGT
					if m[1] == "<" {
						k = tokAgeLT
					}
					f.toks = append(f.toks, token{kind: k, n: n, unit: m[3][0]})
					continue
				}
			}
			f.toks = append(f.toks, token{kind: tokText, text: lw})
		}
	}
	return f
}

// Empty reports whether the filter has no tokens (everything matches).
func (f Filter) Empty() bool { return len(f.toks) == 0 }

// Match reports whether c satisfies every token.
func (f Filter) Match(c *Card, now time.Time) bool {
	for _, t := range f.toks {
		switch t.kind {
		case tokText:
			if !strings.Contains(strings.ToLower(c.Title), t.text) {
				return false
			}
		case tokTag:
			if !strings.Contains(strings.ToLower("#"+c.Tag), t.text) {
				return false
			}
		case tokBlocked:
			if !c.Blocked {
				return false
			}
		case tokAgeGT, tokAgeLT:
			var age int
			if t.unit == 'h' {
				age = Hours(now, c.AgeSince())
			} else {
				age = Days(now, c.AgeSince())
			}
			if t.kind == tokAgeGT && !(age > t.n) {
				return false
			}
			if t.kind == tokAgeLT && !(age < t.n) {
				return false
			}
		}
	}
	return true
}
