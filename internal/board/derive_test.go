package board

import (
	"regexp"
	"testing"
)

func TestDeriveFreeIDMatchesDeriveIDWhenUntaken(t *testing.T) {
	// Every id kando derives today must stay byte-identical, or links and
	// scripts holding one would stop resolving.
	for _, seed := range []string{"", "a", "0\x00Todo\x00Buy milk\x00\x00"} {
		if got, want := DeriveFreeID(seed, func(string) bool { return false }), DeriveID(seed); got != want {
			t.Errorf("seed %q: DeriveFreeID = %q, DeriveID = %q", seed, got, want)
		}
	}
}

func TestDeriveFreeIDSkipsTakenIDs(t *testing.T) {
	shape := regexp.MustCompile(`^[a-z2-7]{8}$`)
	taken := map[string]bool{}
	for i := 0; i < 64; i++ {
		id := DeriveFreeID("seed", func(s string) bool { return taken[s] })
		if taken[id] {
			t.Fatalf("iteration %d returned a taken id %q", i, id)
		}
		if !shape.MatchString(id) {
			t.Fatalf("id %q has the wrong shape", id)
		}
		taken[id] = true
	}
	// Deterministic: the same seed and the same taken set give the same answer.
	a := DeriveFreeID("seed", func(s string) bool { return taken[s] })
	b := DeriveFreeID("seed", func(s string) bool { return taken[s] })
	if a != b {
		t.Errorf("not deterministic: %q then %q", a, b)
	}
}
