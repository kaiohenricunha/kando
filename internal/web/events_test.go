package web

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

// sseClient opens a real SSE connection and reports the events it sees.
type sseClient struct {
	resp   *http.Response
	events chan string
	cancel context.CancelFunc
}

// openSSE connects to the stream over real HTTP: httptest.ResponseRecorder
// cannot flush, and the point of this test is the streaming.
func openSSE(t *testing.T, base, boardName string) *sseClient {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, "GET", base+"/b/"+boardName+"/events", nil)
	if err != nil {
		cancel()
		t.Fatal(err)
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
	c := &sseClient{resp: resp, events: make(chan string, 8), cancel: cancel}
	go func() {
		defer close(c.events)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if line := strings.TrimSpace(sc.Text()); strings.HasPrefix(line, "event: ") {
				c.events <- strings.TrimPrefix(line, "event: ")
			}
		}
	}()
	return c
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
	if n := len(s.hub.subs["life"]); n != 2 {
		t.Fatalf("subscribers = %d", n)
	}
	if len(s.hub.stop) != 1 {
		t.Errorf("two tabs on one board should share one watcher: %d", len(s.hub.stop))
	}
	drop1()
	if len(s.hub.stop) != 1 {
		t.Errorf("the watcher must outlive the first tab")
	}
	drop2()
	if len(s.hub.subs) != 0 || len(s.hub.stop) != 0 {
		t.Errorf("the last tab should release the watcher: subs=%d stop=%d", len(s.hub.subs), len(s.hub.stop))
	}
	// The dropped subscriber's channel is no longer written to.
	select {
	case <-ch1:
		t.Errorf("a dropped subscriber received a signal")
	default:
	}
}

func TestSlowTabNeverBlocksTheWatcher(t *testing.T) {
	root := newRoot(t)
	s := newServer(Options{Root: root, Board: "life", Port: testPort, Now: func() time.Time { return fixedNow }})
	defer s.Close()
	ch, drop, err := s.hub.subscribe("life")
	if err != nil {
		t.Fatal(err)
	}
	defer drop()
	// Nobody reads ch; the fan-out must drop signals rather than block, and
	// a coalesced signal still means exactly "something changed".
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			s.hub.mu.Lock()
			for sub := range s.hub.subs["life"] {
				select {
				case sub <- struct{}{}:
				default:
				}
			}
			s.hub.mu.Unlock()
		}
	}()
	wg.Wait()
	select {
	case <-ch:
	default:
		t.Errorf("the subscriber should hold one coalesced signal")
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
	if strings.Contains(body, "EventSource") {
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
