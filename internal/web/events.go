package web

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/kaiohenricunha/kando/internal/store"
)

// Live updates (U9). One store.WatchBoard watcher per board with an open
// connection, fanned out to every tab on that board (KD-2): a watcher signal
// says "something changed, re-read", never how many times, so the browser
// answers it by reloading the page it is already on. A reload is correct on
// its own (PERF-2), which is what makes the stream a latency optimisation
// rather than a correctness requirement — and what lets every failure path
// here fall back to "the page still works, it just stops refreshing itself".

const (
	// keepalive is how often an idle stream emits a comment frame, so a
	// proxy or a sleeping laptop cannot mistake a quiet board for a dead
	// connection.
	keepalive = 25 * time.Second
	// maxStreams caps concurrent streams per board. A browser opens at most
	// ~6 connections per origin, so this is invisible in use; it exists so a
	// local process cannot pin unbounded goroutines and file descriptors on
	// a server that trusts every local caller (SEC-1).
	maxStreams = 64
)

// hub owns one watcher per watched board and the set of subscribers on it.
type hub struct {
	root string
	mu   sync.Mutex
	subs map[string]map[chan struct{}]struct{} // board → subscribers
	stop map[string]func()                     // board → watcher teardown
	// gen counts the watchers started for a board. store.Watch closes its
	// channel only after fsnotify has drained, which can be after the next
	// tab has already started a replacement watcher, so a fanOut checks its
	// generation before tearing anything down — otherwise a departing
	// watcher would close the live one's subscribers.
	gen    map[string]int
	closed bool
	logf   func(string, ...any)
}

func newHub(root string, logf func(string, ...any)) *hub {
	return &hub{
		root: root,
		subs: map[string]map[chan struct{}]struct{}{},
		stop: map[string]func(){},
		gen:  map[string]int{},
		logf: logf,
	}
}

// errNoWatch is returned when a stream cannot be opened: the hub is shutting
// down, the board has too many already, or the watcher would not start.
type errNoWatch struct{ err error }

func (e *errNoWatch) Error() string { return e.err.Error() }

// subscribe returns a channel that receives one signal per change to the
// board, and the function that unsubscribes it. The first subscriber to a
// board starts its watcher; the last to leave stops it, so a server nobody
// is watching holds no fsnotify handles. The channel is closed if the
// watcher dies, which ends the stream and lets the browser reconnect onto a
// fresh one.
func (h *hub) subscribe(name string) (<-chan struct{}, func(), error) {
	// Build the watcher before taking the lock: WatchBoard stats the
	// directory and registers with the kernel, and h.mu is the same lock
	// fanOut needs to deliver an event to every other board.
	var (
		events <-chan struct{}
		stop   func()
		err    error
	)
	h.mu.Lock()
	needsWatcher := !h.closed && h.subs[name] == nil
	h.mu.Unlock()
	if needsWatcher {
		if events, stop, err = store.WatchBoard(h.root, name); err != nil {
			return nil, nil, &errNoWatch{err}
		}
	}

	ch := make(chan struct{}, 1)
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case h.closed:
		h.stopLater(stop)
		return nil, nil, &errNoWatch{fmt.Errorf("server is shutting down")}
	case len(h.subs[name]) >= maxStreams:
		h.stopLater(stop)
		return nil, nil, &errNoWatch{fmt.Errorf("too many open streams for %q", name)}
	case h.subs[name] == nil && events == nil:
		// Another goroutine took the entry away between the two locks.
		return nil, nil, &errNoWatch{fmt.Errorf("watcher for %q went away", name)}
	case h.subs[name] == nil:
		h.subs[name] = map[chan struct{}]struct{}{}
		h.stop[name] = stop
		h.gen[name]++
		go h.fanOut(name, events, h.gen[name])
	default:
		h.stopLater(stop) // lost the race; another subscriber's watcher won
	}
	h.subs[name][ch] = struct{}{}
	return ch, func() { h.unsubscribe(name, ch) }, nil
}

// stopLater releases a watcher this call built but does not need, outside
// the lock: fsnotify's Close waits for its reader goroutine to exit.
func (h *hub) stopLater(stop func()) {
	if stop != nil {
		go stop()
	}
}

func (h *hub) unsubscribe(name string, ch chan struct{}) {
	h.mu.Lock()
	subs := h.subs[name]
	if subs == nil {
		h.mu.Unlock()
		return
	}
	delete(subs, ch)
	var stop func()
	if len(subs) == 0 {
		stop = h.stop[name]
		delete(h.subs, name)
		delete(h.stop, name)
	}
	h.mu.Unlock()
	if stop != nil {
		stop() // outside the lock: this blocks until fsnotify has drained
	}
}

// fanOut relays the watcher's signals to every subscriber until the watcher
// channel closes, draining until the close rather than stopping at the first
// receive after stop() — store.Watch's documented contract. A send that
// would block is dropped: the subscriber already has an unread signal, and
// "changed again" means the same thing as "changed".
//
// The channel also closes when the watcher dies on its own (an inotify
// failure, or the board directory being removed), which store.Watch reports
// the same way as a clean stop. So on exit the entry is torn down and every
// subscriber channel closed: the streams end, the browsers reconnect, and
// the reconnect builds a fresh watcher instead of joining a dead one.
func (h *hub) fanOut(name string, events <-chan struct{}, gen int) {
	for range events {
		h.mu.Lock()
		if h.gen[name] != gen {
			h.mu.Unlock()
			return // a newer watcher owns this board now
		}
		for ch := range h.subs[name] {
			select {
			case ch <- struct{}{}:
			default:
			}
		}
		h.mu.Unlock()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.gen[name] != gen {
		return // this watcher was already replaced; its subscribers moved on
	}
	if orphans := len(h.subs[name]); orphans > 0 {
		h.logf("watcher for %s stopped with %d stream(s) open; they will reconnect", name, orphans)
	}
	for ch := range h.subs[name] {
		close(ch)
	}
	delete(h.subs, name)
	delete(h.stop, name)
}

// close stops every watcher and refuses new ones. The server calls it on
// shutdown so ctrl+c does not wait on watchers nobody will read again.
func (h *hub) close() {
	h.mu.Lock()
	h.closed = true
	stops := make([]func(), 0, len(h.stop))
	for name, stop := range h.stop {
		stops = append(stops, stop)
		delete(h.stop, name)
	}
	h.mu.Unlock()
	for _, stop := range stops {
		stop()
	}
}

// counts reports the subscriber and watcher counts for a board, under the
// lock. Tests use it; nothing else should need it.
func (h *hub) counts(name string) (subs, watchers int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs[name]), len(h.stop)
}

// events streams board changes as Server-Sent Events. It is the one route
// that does not answer and finish: it holds the connection open and writes a
// "board-changed" event per signal, which the page's listener turns into a
// reload (PERF-1: within 2s of the file write).
//
// Every frame carries the board's current version as its event id. A browser
// reconnecting after a dropped connection replays that id in Last-Event-ID,
// so a change made while nothing was subscribed is reported immediately
// instead of being missed until the next unrelated edit (REL-4).
func (s *server) events(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("board")
	if err := s.mustExist(name); err != nil {
		s.fail(w, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.fail(w, &httpError{http.StatusInternalServerError, "streaming unsupported"})
		return
	}
	ch, unsubscribe, err := s.hub.subscribe(name)
	if err != nil {
		s.logf("watch %s: %v", name, err)
		// A board nobody can watch still renders and still saves: the page
		// falls back to manual reloads rather than the request failing.
		s.fail(w, &httpError{http.StatusServiceUnavailable, "live updates unavailable"})
		return
	}
	defer unsubscribe()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no") // no proxy here, but a stream must not be buffered
	w.WriteHeader(http.StatusOK)

	version := store.Version(s.root, name)
	fmt.Fprintf(w, ": connected\nid: %s\n\n", version)
	if seen := r.Header.Get("Last-Event-ID"); seen != "" && seen != version {
		// The board changed while this client was away; tell it now.
		fmt.Fprintf(w, "event: board-changed\nid: %s\ndata: reload\n\n", version)
	}
	flusher.Flush()

	tick := time.NewTicker(keepalive)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done(): // tab closed, or the server is shutting down
			return
		case <-s.done:
			return
		case _, ok := <-ch:
			if !ok {
				return // the watcher died; ending the stream makes the browser reconnect
			}
			fmt.Fprintf(w, "event: board-changed\nid: %s\ndata: reload\n\n", store.Version(s.root, name))
			flusher.Flush()
		case <-tick.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
