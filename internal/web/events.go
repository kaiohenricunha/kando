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
// answers it by reloading the page it is already on. The stream is a pure
// latency optimisation — a reload is correct on its own (PERF-2), so a
// missed or dropped event can never leave a page permanently stale (REL-4).

// keepalive is how often an idle stream emits a comment frame, so a proxy or
// a sleeping laptop cannot mistake a quiet board for a dead connection.
const keepalive = 25 * time.Second

// hub owns one watcher per watched board and the set of subscribers on it.
type hub struct {
	root string
	mu   sync.Mutex
	subs map[string]map[chan struct{}]struct{} // board → subscribers
	stop map[string]func()                     // board → watcher teardown
	logf func(string, ...any)
}

func newHub(root string, logf func(string, ...any)) *hub {
	return &hub{
		root: root,
		subs: map[string]map[chan struct{}]struct{}{},
		stop: map[string]func(){},
		logf: logf,
	}
}

// subscribe returns a channel that receives one signal per change to the
// board, and the function that unsubscribes it. The first subscriber to a
// board starts its watcher; the last to leave stops it, so a server nobody
// is watching holds no fsnotify handles.
func (h *hub) subscribe(name string) (<-chan struct{}, func(), error) {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[name] == nil {
		events, stop, err := store.WatchBoard(h.root, name)
		if err != nil {
			return nil, nil, err
		}
		h.subs[name] = map[chan struct{}]struct{}{}
		h.stop[name] = stop
		go h.fanOut(name, events)
	}
	h.subs[name][ch] = struct{}{}
	return ch, func() { h.unsubscribe(name, ch) }, nil
}

func (h *hub) unsubscribe(name string, ch chan struct{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	subs := h.subs[name]
	if subs == nil {
		return
	}
	delete(subs, ch)
	if len(subs) == 0 {
		if stop := h.stop[name]; stop != nil {
			stop()
		}
		delete(h.subs, name)
		delete(h.stop, name)
	}
}

// fanOut relays the watcher's signals to every subscriber until the watcher
// channel closes. It drains until the close rather than stopping at the
// first receive after stop(), which is store.Watch's documented contract.
// A send that would block is dropped: the subscriber already has an unread
// signal, and "changed again" means the same thing as "changed".
func (h *hub) fanOut(name string, events <-chan struct{}) {
	for range events {
		h.mu.Lock()
		for ch := range h.subs[name] {
			select {
			case ch <- struct{}{}:
			default:
			}
		}
		h.mu.Unlock()
	}
}

// close stops every watcher. The server calls it on shutdown so ctrl+c does
// not wait on watchers nobody will read again.
func (h *hub) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for name, stop := range h.stop {
		stop()
		delete(h.stop, name)
		delete(h.subs, name)
	}
}

// events streams board changes as Server-Sent Events. It is the one route
// that does not answer and finish: it holds the connection open and writes a
// "board-changed" event per signal, which the page's listener turns into a
// reload (PERF-1: within 2s of the file write).
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
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	tick := time.NewTicker(keepalive)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done(): // tab closed, or the server is shutting down
			return
		case <-s.done:
			return
		case <-ch:
			fmt.Fprint(w, "event: board-changed\ndata: reload\n\n")
			flusher.Flush()
		case <-tick.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
