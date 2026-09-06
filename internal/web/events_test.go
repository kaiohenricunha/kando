package web

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

// sseClient opens a real SSE connection and reports the events it sees.
type sseClient struct {
	resp   *http.Response
	events chan string
	ids    chan string
	cancel context.CancelFunc
}

// openSSE connects to the stream over real HTTP. A ResponseRecorder would
// satisfy http.Flusher, but it never cancels the request context, so the
// handler's loop would never exit and the test would hang until the package
// timeout — which is why the 400/403/404 cases below are the only ones a
// recorder can drive.
func openSSE(t *testing.T, base, boardName string, lastEventID ...string) *sseClient {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, "GET", base+"/b/"+boardName+"/events", nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if len(lastEventID) > 0 {
		req.Header.Set("Last-Event-ID", lastEventID[0])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		cancel()
		t.Fatalf("stream: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	c := &sseClient{resp: resp, events: make(chan string, 8), ids: make(chan string, 8), cancel: cancel}
	go func() {
		defer close(c.events)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			switch {
			case strings.HasPrefix(line, "event: "):
				c.events <- strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "id: "):
				select {
				case c.ids <- strings.TrimPrefix(line, "id: "):
				default:
				}
			}
		}
	}()
	return c
}

// id waits for the next event id the stream carries.
func (c *sseClient) id(t *testing.T) string {
	t.Helper()
	select {
	case v := <-c.ids:
		return v
	case <-time.After(2 * time.Second):
		t.Fatal("no event id within 2s")
		return ""
	}
}

func (c *sseClient) close() {
	c.cancel()
	c.resp.Body.Close()
}

// want waits for one event name, or fails: PERF-1 gives it 2 seconds.
func (c *sseClient) want(t *testing.T, name string) {
	t.Helper()
	select {
	case got, ok := <-c.events:
		if !ok {
			t.Fatalf("stream closed while waiting for %q", name)
		}
		if got != name {
			t.Fatalf("event %q, want %q", got, name)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no %q within 2s (PERF-1)", name)
	}
}

// liveServer runs the real server on a loopback port, as `kando web` does.
func liveServer(t *testing.T, root, def string) (base string, stop func()) {
	t.Helper()
	ln, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, ln, Options{Root: root, Board: def, Now: func() time.Time { return fixedNow }})
	}()
	return "http://" + ln.Addr().String(), func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Serve: %v", err)
			}
		case <-time.After(6 * time.Second):
			t.Error("Serve did not return after cancel")
		}
	}
}

func TestSSEFansOutToEveryOpenTab(t *testing.T) {
	root := newRoot(t)
	base, stop := liveServer(t, root, "life")
	defer stop()

	first := openSSE(t, base, "life")
	defer first.close()
	second := openSSE(t, base, "life")
	defer second.close()
	// A tab on another board must not hear about this one.
	other := openSSE(t, base, "work")
	defer other.close()

	// An edit from outside the browser entirely: the TUI, or a text editor.
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Insert(board.Todo, 0, &board.Card{ID: "aaaaaaaa", Title: "from elsewhere"})
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	first.want(t, "board-changed")
	second.want(t, "board-changed")
	select {
	case ev := <-other.events:
		t.Errorf("a tab on \"work\" received %q for a change to \"life\"", ev)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestSSEStreamsSeeAMutationMadeThroughTheWeb(t *testing.T) {
	root := newRoot(t)
	base, stop := liveServer(t, root, "life")
	defer stop()
	c := openSSE(t, base, "life")
	defer c.close()

	form := strings.NewReader("lane=todo&title=Typed+in+another+tab")
	req, _ := http.NewRequest("POST", base+"/b/life/cards", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", base)
	// Do not follow the redirect: the status is the assertion.
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create: %d", resp.StatusCode)
	}
	c.want(t, "board-changed")
}

func TestWatchersStopWhenTheLastTabLeaves(t *testing.T) {
	root := newRoot(t)
	s := newServer(Options{Root: root, Board: "life", Port: testPort, Now: func() time.Time { return fixedNow }})
	defer s.Close()
	ch1, drop1, err := s.hub.subscribe("life")
	if err != nil {
		t.Fatal(err)
	}
	_, drop2, err := s.hub.subscribe("life")
	if err != nil {
		t.Fatal(err)
	}
	if subs, watchers := s.hub.counts("life"); subs != 2 || watchers != 1 {
		t.Fatalf("two tabs on one board should share one watcher: subs=%d watchers=%d", subs, watchers)
	}
	drop1()
	if _, watchers := s.hub.counts("life"); watchers != 1 {
		t.Errorf("the watcher must outlive the first tab")
	}
	drop2()
	if subs, watchers := s.hub.counts("life"); subs != 0 || watchers != 0 {
		t.Errorf("the last tab should release the watcher: subs=%d watchers=%d", subs, watchers)
	}
	// The dropped subscriber's channel is no longer written to.
	select {
	case <-ch1:
		t.Errorf("a dropped subscriber received a signal")
	default:
	}
}

func TestFanOutDropsWhenNobodyIsReadingAndDrainsUntilClosed(t *testing.T) {
	root := newRoot(t)
	s := newServer(Options{Root: root, Board: "life", Port: testPort, Now: func() time.Time { return fixedNow }})
	defer s.Close()
	ch := make(chan struct{}, 1)
	s.hub.mu.Lock()
	s.hub.subs["life"] = map[chan struct{}]struct{}{ch: {}}
	s.hub.gen["life"]++
	gen := s.hub.gen["life"]
	s.hub.mu.Unlock()

	events := make(chan struct{})
	done := make(chan struct{})
	go func() { s.hub.fanOut("life", events, gen); close(done) }()
	events <- struct{}{} // fills the subscriber's buffer
	events <- struct{}{} // must be dropped rather than block the watcher
	close(events)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("fanOut did not drain until the watcher channel closed")
	}
	if _, ok := <-ch; !ok {
		t.Fatal("the buffered signal should still be there")
	}
	// A watcher that ended closes its subscribers, so their streams end and
	// the browsers reconnect onto a fresh watcher.
	if _, ok := <-ch; ok {
		t.Errorf("fanOut should close its subscribers on exit")
	}
	if subs, watchers := s.hub.counts("life"); subs != 0 || watchers != 0 {
		t.Errorf("a dead watcher must not leave an entry behind: subs=%d watchers=%d", subs, watchers)
	}
}

func TestReconnectWithAStaleEventIDIsToldToReload(t *testing.T) {
	root := newRoot(t)
	base, stop := liveServer(t, root, "life")
	defer stop()

	first := openSSE(t, base, "life")
	version := first.id(t)
	first.close()

	// The board changes while nothing is subscribed — the gap an EventSource
	// retry leaves open.
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	b.Insert(board.Todo, 0, &board.Card{ID: "aaaaaaaa", Title: "missed while away"})
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}

	// The browser reconnects replaying the id it last saw (REL-4).
	again := openSSE(t, base, "life", version)
	defer again.close()
	again.want(t, "board-changed")

	// A reconnect that missed nothing gets no event.
	current := openSSE(t, base, "life", store.Version(root, "life"))
	defer current.close()
	select {
	case ev := <-current.events:
		t.Errorf("an up-to-date reconnect should be quiet, got %q", ev)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestTooManyStreamsOnOneBoardIsRefused(t *testing.T) {
	root := newRoot(t)
	s := newServer(Options{Root: root, Board: "life", Port: testPort, Now: func() time.Time { return fixedNow }})
	defer s.Close()
	for i := 0; i < maxStreams; i++ {
		if _, _, err := s.hub.subscribe("life"); err != nil {
			t.Fatalf("subscriber %d: %v", i, err)
		}
	}
	if _, _, err := s.hub.subscribe("life"); err == nil {
		t.Errorf("past the cap a subscription should be refused")
	}
	if _, _, err := s.hub.subscribe("work"); err != nil {
		t.Errorf("the cap is per board: %v", err)
	}
}

func TestSubscribeAfterCloseIsRefused(t *testing.T) {
	root := newRoot(t)
	s := newServer(Options{Root: root, Board: "life", Port: testPort, Now: func() time.Time { return fixedNow }})
	s.Close()
	s.Close() // idempotent, and safe to call twice
	if _, _, err := s.hub.subscribe("life"); err == nil {
		t.Errorf("a shutting-down server must not start new watchers")
	}
}

func TestEventsRouteRejectsUnknownBoardsAndCrossSite(t *testing.T) {
	h := newHandler(t, newRoot(t), "life")
	if rec, _ := get(t, h, "/b/nope/events"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown board: %d", rec.Code)
	}
	if rec, _ := get(t, h, "/b/.hidden/events"); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid name: %d", rec.Code)
	}
	if rec, _ := get(t, h, "/b/life/events", "Sec-Fetch-Site", "cross-site"); rec.Code != http.StatusForbidden {
		t.Errorf("cross-site stream: %d", rec.Code)
	}
	// Origin is what a cross-origin EventSource actually sends.
	if rec, _ := get(t, h, "/b/life/events", "Origin", "http://evil.example"); rec.Code != http.StatusForbidden {
		t.Errorf("cross-origin stream: %d", rec.Code)
	}
	if rec, _ := get(t, h, "/b/life/events", "Host", "kando.evil.example:4242"); rec.Code != http.StatusForbidden {
		t.Errorf("foreign Host stream: %d", rec.Code)
	}
	// Regression guard for whoever later widens the embed pattern.
	for _, path := range []string{"/static/", "/static/../templates/layout.html", "/static/../../go.mod"} {
		if rec, _ := get(t, h, path); rec.Code == http.StatusOK {
			t.Errorf("GET %s should not be served: %d", path, rec.Code)
		}
	}
}

func TestBoardPageLoadsTheListenerFromThisOrigin(t *testing.T) {
	h := newHandler(t, newRoot(t), "life")
	_, body := get(t, h, "/b/life")
	if !strings.Contains(body, `<script src="/static/live.js"></script>`) {
		t.Errorf("the page should load the listener: %s", grepLine(body, "script"))
	}
	if !strings.Contains(body, `data-board="life"`) {
		t.Errorf("the listener needs the board name: %s", grepLine(body, "<body"))
	}
	if strings.Contains(body, "new EventSource") {
		t.Errorf("the listener must not be inlined — the CSP forbids inline script")
	}
	rec, js := get(t, h, "/static/live.js")
	if rec.Code != 200 || !strings.Contains(js, "board-changed") {
		t.Errorf("live.js: %d %q", rec.Code, js)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("live.js content type: %q", ct)
	}
}

func TestAReplacedWatcherDoesNotTearDownItsSuccessor(t *testing.T) {
	root := newRoot(t)
	s := newServer(Options{Root: root, Board: "life", Port: testPort, Now: func() time.Time { return fixedNow }})
	defer s.Close()

	// An old watcher's fanOut, still draining after its stop() returned.
	old := make(chan struct{})
	oldDone := make(chan struct{})
	s.hub.mu.Lock()
	s.hub.gen["life"]++
	oldGen := s.hub.gen["life"]
	s.hub.mu.Unlock()
	go func() { s.hub.fanOut("life", old, oldGen); close(oldDone) }()
	// Its last tab has already left, so unsubscribe removed the entry — but
	// store.Watch has not closed the channel yet.

	// The next tab starts a replacement while the old one is still alive.
	ch, drop, err := s.hub.subscribe("life")
	if err != nil {
		t.Fatal(err)
	}
	defer drop()
	close(old)
	select {
	case <-oldDone:
	case <-time.After(2 * time.Second):
		t.Fatal("the old fanOut did not exit")
	}
	if subs, watchers := s.hub.counts("life"); subs != 1 || watchers != 1 {
		t.Fatalf("the replacement was torn down: subs=%d watchers=%d", subs, watchers)
	}
	select {
	case _, ok := <-ch:
		if !ok {
			t.Errorf("the live subscriber's channel was closed by the old watcher")
		}
	default:
	}
}

// The board page's drag-and-drop is a second file beside live.js, pinned by
// its own route for the same reason: a file server over the embed would also
// answer for anything else that lands in static/.
func TestBoardPageLoadsTheDragListener(t *testing.T) {
	h := newHandler(t, newRoot(t), "life")
	_, body := get(t, h, "/b/life")
	if !strings.Contains(body, `<script src="/static/dnd.js"></script>`) {
		t.Errorf("the board page should load the drag listener: %s", grepLine(body, "script"))
	}
	// Neither script defers or asyncs, so tag order is execution order and
	// therefore listener-registration order for the same event on the same
	// target. dnd.js's dragend handler relies on live.js's own dragend
	// handler having already run first (it checks a flag dnd.js has not yet
	// cleared) — reordering the tags would silently break that.
	if i, j := strings.Index(body, "/static/live.js"), strings.Index(body, "/static/dnd.js"); i < 0 || j < 0 || i > j {
		t.Errorf("live.js must load before dnd.js: live at %d, dnd at %d", i, j)
	}
	rec, js := get(t, h, "/static/dnd.js")
	if rec.Code != 200 || !strings.Contains(js, "dragstart") || !strings.Contains(js, "autoScroll") {
		t.Errorf("dnd.js: %d %q", rec.Code, js)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("dnd.js content type: %q", ct)
	}
	// Still no widening of the embed: the guard above must hold with two
	// assets pinned, not one.
	for _, path := range []string{"/static/", "/static/../static/dnd.js", "/static/dnd.js/../live.js", "/static/../../go.mod"} {
		if rec, _ := get(t, h, path); rec.Code == http.StatusOK {
			t.Errorf("GET %s should not be served: %d", path, rec.Code)
		}
	}
}

// The drag script needs an id per card and a drag source it is allowed to
// pick up; the lane key it drops into is the section id the page already has.
func TestBoardCardsAreDragSources(t *testing.T) {
	root := newRoot(t)
	h := newHandler(t, root, "life")
	_, body := get(t, h, "/b/life")
	id := todoCard(t, root)
	if !strings.Contains(body, `draggable="true"`) {
		t.Errorf("cards should be draggable: %s", grepLine(body, "class=\"card"))
	}
	if !strings.Contains(body, `data-id="`+id+`"`) {
		t.Errorf("cards should carry their id: %s", grepLine(body, "class=\"card"))
	}
	if !strings.Contains(body, `data-lane="todo"`) {
		t.Errorf("lanes should name the key a drop posts: %s", grepLine(body, "class=\"lane"))
	}
	// The drop builds its action from the card's own href, which the server
	// escaped; that link must survive the new attributes untouched.
	if !strings.Contains(body, `href="/b/life/cards/`+id+`"`) {
		t.Errorf("the card link should be unchanged: %s", grepLine(body, "class=\"card"))
	}
}
