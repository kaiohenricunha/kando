package store

import (
	"os"
	"testing"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
)

func recv(t *testing.T, ch <-chan struct{}) (got, ok bool) {
	t.Helper()
	select {
	case _, ok = <-ch:
		return true, ok
	case <-time.After(3 * time.Second):
		return false, false
	}
}

func TestWatchSignalsOnBoardWriteAndClosesOnStop(t *testing.T) {
	root := t.TempDir()
	s, b, err := Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	ch, stop, err := s.Watch()
	if err != nil {
		t.Skipf("fsnotify unavailable: %v", err)
	}
	b.Lanes[board.Todo] = []*board.Card{{ID: "abcdefgh", Title: "x"}}
	if err := os.WriteFile(s.BoardPath(), Marshal(b), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, ok := recv(t, ch); !got || !ok {
		t.Fatalf("expected a change signal after writing board.md (got=%v ok=%v)", got, ok)
	}
	stop()
	// A single write can produce more than one fsnotify event, so one more
	// coalesced signal may still be buffered; the contract is that the channel
	// closes once stopped, so drain until it does.
	for i := 0; i < 3; i++ {
		got, ok := recv(t, ch)
		if !got {
			t.Fatal("stop() must close the channel: receive timed out")
		}
		if !ok {
			return
		}
	}
	t.Fatal("stop() must close the channel: still receiving values after three reads")
}
