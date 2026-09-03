package tui

import (
	"flag"
	"os"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// update regenerates the goldens this test file authors. The three board
// goldens are user-provided and are never rewritten.
var update = flag.Bool("update", false, "rewrite authored golden files")

// authoredGoldens are frames rendered from the sample data at 120x40.
var authoredGoldens = []struct {
	name string
	file string
	keys []string
}{
	{"filter #home typed", "filter_home_120x40.txt", []string{"/", "#", "h", "o", "m", "e"}},
	{"help overlay", "help_120x40.txt", []string{"?"}},
	{"detail renew passport", "detail_120x40.txt", []string{"enter"}},
}

func TestGoldenScreens(t *testing.T) {
	for _, c := range authoredGoldens {
		t.Run(c.name, func(t *testing.T) {
			m := newTestModel(t, 120, 40)
			m = press(m, c.keys...)
			got := ansi.Strip(m.View()) + "\n"
			path := "testdata/" + c.file
			if *update {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run with -update to create it)", err)
			}
			assertFrame(t, string(want), got, 120, 40)
		})
	}
}
