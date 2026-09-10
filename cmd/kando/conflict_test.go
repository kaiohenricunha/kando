package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/kaiohenricunha/kando/internal/store"
)

// A verb reports a stale board in its own words, whichever store call noticed:
// a save, or now Open and LoadArchive when a race outlasts their retries.
func TestConflictErrUsesTheVerbsWording(t *testing.T) {
	if conflictErr(nil) != nil {
		t.Error("nil must stay nil")
	}
	for _, err := range []error{store.ErrConflict, fmt.Errorf("open: %w", store.ErrConflict)} {
		if got := conflictErr(err); !errors.Is(got, errConflict) {
			t.Errorf("%v: got %v, want errConflict", err, got)
		}
	}
	other := errors.New("disk on fire")
	if got := conflictErr(other); got != other {
		t.Errorf("any other error passes through unchanged: got %v", got)
	}
}
