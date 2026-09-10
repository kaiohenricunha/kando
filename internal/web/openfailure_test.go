package web

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/kaiohenricunha/kando/internal/store"
)

// store.Open and (*Store).LoadArchive now repair ids through the checked writer
// and retry a lost race, so they can return ErrConflict. That is a stale write
// like any other and gets commit's 409, not an internal error. The race itself
// cannot be staged from this package, so the mapping is tested directly.
func TestOpenFailureMapsAConflictTo409(t *testing.T) {
	s := &server{errw: io.Discard}
	for _, err := range []error{store.ErrConflict, fmt.Errorf("repair: %w", store.ErrConflict)} {
		if he := s.openFailure("open", "life", err, "cannot open board"); he.status != http.StatusConflict {
			t.Errorf("%v: status %d, want 409", err, he.status)
		}
	}
	if he := s.openFailure("open", "life", errors.New("disk on fire"), "cannot open board"); he.status != http.StatusInternalServerError || he.Error() != "cannot open board" {
		t.Errorf("any other error stays a 500 with the given message: %d %q", he.status, he.Error())
	}
}
