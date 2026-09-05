package store

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

// WatchBoard is Watch without a Store: it watches root/name's directory
// read-only, creating and rewriting nothing, which is what a GET handler
// (the web server's SSE stream) needs. The channel behaves exactly as
// Watch's, including the drain-until-closed contract.
func WatchBoard(root, name string) (events <-chan struct{}, stop func(), err error) {
	if !ValidBoardName(name) {
		return nil, nil, fmt.Errorf("invalid board name %q", name)
	}
	dir := filepath.Join(root, name)
	if _, err := os.Stat(filepath.Join(dir, boardFile)); err != nil {
		return nil, nil, err
	}
	return watchDir(dir)
}

// Watch starts an fsnotify watcher on the board directory. The returned channel
// receives a (coalesced) signal whenever board.md or archive.md changes; the
// caller then runs CheckReload. Call stop to release the watcher. The channel
// is closed once the watcher has drained, so a consumer must keep receiving
// until the close: at most one buffered signal may still arrive after stop
// returns.
func (s *Store) Watch() (events <-chan struct{}, stop func(), err error) {
	return watchDir(s.dir)
}

// watchDir is the shared body of Watch and WatchBoard.
func watchDir(dir string) (events <-chan struct{}, stop func(), err error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, err
	}
	if err := w.Add(dir); err != nil {
		w.Close()
		return nil, nil, err
	}
	ch := make(chan struct{}, 1)
	go func() {
		defer close(ch) // wakes any listener when stop() closes the watcher
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				switch filepath.Base(ev.Name) {
				case boardFile, archiveFile:
					select {
					case ch <- struct{}{}:
					default:
					}
				}
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	return ch, func() { w.Close() }, nil
}
