package store

import (
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

// Watch starts an fsnotify watcher on the board directory. The returned channel
// receives a (coalesced) signal whenever board.md or archive.md changes; the
// caller then runs CheckReload. Call stop to release the watcher.
func (s *Store) Watch() (events <-chan struct{}, stop func(), err error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, err
	}
	if err := w.Add(s.dir); err != nil {
		w.Close()
		return nil, nil, err
	}
	ch := make(chan struct{}, 1)
	go func() {
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				switch filepath.Base(ev.Name) {
				case "board.md", "archive.md":
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
