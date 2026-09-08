package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

// These are the repo's first benchmarks, and they exist to answer one question
// that was deferred rather than measured: sanitize's fast path bails out on any
// byte >= 0x80, so every accented, CJK or emoji string takes the allocating
// rune loop to produce output identical to its input. That is either a real
// cost on a per-keystroke render path or it is noise, and nobody knew which.
//
// BenchmarkBoardFrame is the number that decides it. sanitize runs on the order
// of a hundred times per frame, and bubbletea calls View once per message, so
// one frame is what a single keypress costs.

// benchModel builds the same fixture the golden tests use, without going
// through newTestModel, which takes a *testing.T.
func benchModel(b *testing.B, w, h int) Model {
	b.Helper()
	data, err := os.ReadFile("../store/testdata/sample_board.md")
	if err != nil {
		b.Fatal(err)
	}
	bd, _, err := store.Parse(data)
	if err != nil {
		b.Fatal(err)
	}
	bd.Name = "life"
	adata, err := os.ReadFile("../store/testdata/sample_archive.md")
	if err != nil {
		b.Fatal(err)
	}
	a, _, err := store.ParseArchive(adata)
	if err != nil {
		b.Fatal(err)
	}
	return New(Options{
		Board:   bd,
		Archive: a,
		Styles:  testStyles,
		Now:     func() time.Time { return fixedNow },
		Width:   w,
		Height:  h,
	})
}

func BenchmarkSanitize(b *testing.B) {
	for _, tc := range []struct{ name, in string }{
		// Takes the fast path and returns the input unchanged.
		{"ascii", "Renew passport"},
		// All of these miss the fast path on the >= 0x80 test and allocate,
		// even though nothing in them is unsafe.
		{"accented", "Café résumé for the tax office"},
		{"cjk", "日本語のタイトルをここに書く"},
		{"emoji", "Ship it 🚀 before Friday"},
		// The one case that genuinely has work to do.
		{"bidi to strip", "safe\u202egnp.exe and more text"},
		// The largest single string sanitize ever sees: a full notes body,
		// via wrapNotes.
		{"notes 16KiB ascii", strings.Repeat("lorem ipsum dolor ", 910)},
		{"notes 16KiB accented", strings.Repeat("lorem ipsúm dolor ", 910)},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sinkString = sanitize(tc.in)
			}
		})
	}
}

// BenchmarkBoardFrame measures a whole rendered frame — every sanitize call a
// keystroke actually pays for, plus the layout around them.
//
// The "accented" variants are the A/B that settles the fast-path question:
// identical board, every title pushed off sanitize's ASCII fast path by one
// non-ASCII rune. The difference between the two is the entire cost of the
// >= 0x80 bail-out, measured rather than reasoned about.
func BenchmarkBoardFrame(b *testing.B) {
	for _, tc := range []struct {
		name     string
		w, h     int
		accented bool
	}{
		{"120x40", 120, 40, false},
		{"120x40 accented", 120, 40, true},
		{"200x60", 200, 60, false},
		{"200x60 accented", 200, 60, true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			m := benchModel(b, tc.w, tc.h)
			if tc.accented {
				for _, l := range board.Lanes {
					for _, c := range m.b.Lanes[l] {
						c.Title = "é" + c.Title
					}
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				sinkString = m.View()
			}
		})
	}
}

// sinkString keeps the compiler from optimising the call away.
var sinkString string
