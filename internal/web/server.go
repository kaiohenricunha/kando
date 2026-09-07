// Package web serves the kando board as server-rendered HTML on localhost.
// It is a second renderer over the same internal/board model and
// internal/store files the TUI uses — never a second model (KD-1, §4 of
// docs/specs/kando-web). A GET loads the board read-only (store.Load) and
// renders it; a POST loads it (store.Open), applies one internal/board
// helper, saves and forgets (KD-3). No board lives in memory between
// requests, so a page is never stale — but a write is a whole-file
// read-modify-write, so writers to one board are serialised by the server
// (see writeLock) and the save refuses outright if the file changed
// underneath (store.ErrConflict → 409).
package web

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

// Options configures the server. Board is the board "/" redirects to; when
// empty "/" goes to the boards list. Port is the port the listener is bound
// to; the same-origin check (SEC-3, §7) compares Host and Origin against it.
// Serve fills Port in from the listener so the two can never disagree.
type Options struct {
	Root  string
	Board string
	Port  int
	Now   func() time.Time
	// ErrorLog receives operational errors (OPS-4); os.Stderr when nil.
	ErrorLog io.Writer
}

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/live.js static/dnd.js
var staticFS embed.FS

type server struct {
	root      string
	def       string
	now       func() time.Time
	tpl       *template.Template
	errw      io.Writer // operational errors (OPS-4); os.Stderr outside tests
	locks     sync.Map  // board name → *sync.Mutex, held across load-mutate-save
	hub       *hub      // one file watcher per board with an open SSE stream
	port      int
	mux       *http.ServeMux
	done      chan struct{}
	closeOnce sync.Once
}

// Close releases the file watchers and ends every open SSE stream. Serve
// calls it on shutdown. It is idempotent and safe from several goroutines
// at once, which matters because Serve calls it from two.
func (s *server) Close() {
	s.closeOnce.Do(func() {
		close(s.done)
		s.hub.close()
	})
}

// writeLock serialises the read-modify-write of one board against other
// requests in this process. It says nothing about the TUI or a text editor
// writing the same file — SaveBoardIfUnchanged covers those.
func (s *server) writeLock(name string) func() {
	v, _ := s.locks.LoadOrStore(name, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// logf reports an operational error (OPS-4): never to the user, always to
// the error writer.
func (s *server) logf(format string, a ...any) {
	fmt.Fprintf(s.errw, "kando web: "+format+"\n", a...)
}

// New builds the HTTP handler: the routes of §5 behind the security headers
// and the same-origin guard. The watchers it allocates are released only by
// Serve, so a handler built here and served by something else keeps its SSE
// streams open until each client disconnects — fine for tests, which is what
// this constructor is for now; production goes through Serve.
func New(o Options) http.Handler { return newServer(o).handler() }

// newServer is New with the concrete type, so a caller that must release the
// watchers (Serve, and the tests) can reach Close.
func newServer(o Options) *server {
	if o.Now == nil {
		o.Now = time.Now
	}
	s := &server{root: o.Root, def: o.Board, now: o.Now, errw: o.ErrorLog, port: o.Port, done: make(chan struct{})}
	if s.errw == nil {
		s.errw = os.Stderr
	}
	s.hub = newHub(o.Root, s.logf)
	funcs := template.FuncMap{"urlpath": url.PathEscape}
	s.tpl = template.Must(template.New("").Funcs(funcs).ParseFS(templateFS, "templates/*.html"))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /boards", s.boards)
	mux.HandleFunc("GET /b/{board}", s.board)
	mux.HandleFunc("GET /b/{board}/cards/new", s.newCard)
	mux.HandleFunc("GET /b/{board}/cards/{id}", s.card)
	mux.HandleFunc("GET /b/{board}/archive", s.archive)
	mux.HandleFunc("GET /b/{board}/events", s.events)
	// The assets, each pinned by path: a file server would also answer
	// GET /static/ with a directory listing and would depend on the embed
	// layout for its prefix. This list is the pin — widening it to a
	// pattern is what events_test.go guards against.
	for _, name := range []string{"static/live.js", "static/dnd.js"} {
		mux.HandleFunc("GET /"+name, func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, staticFS, name)
		})
	}
	// Mutations (§5): one form POST per TUI key; see cards.go, boards.go,
	// archive.go. The same-origin guard is what makes a bare POST safe.
	mux.HandleFunc("POST /boards", s.createBoard)
	mux.HandleFunc("POST /b/{board}/cards", s.createCard)
	mux.HandleFunc("POST /b/{board}/cards/{id}", s.updateCard())
	mux.HandleFunc("POST /b/{board}/cards/{id}/move", s.moveCard())
	mux.HandleFunc("POST /b/{board}/cards/{id}/block", s.blockCard())
	mux.HandleFunc("POST /b/{board}/cards/{id}/checklist", s.addChecklistItem())
	mux.HandleFunc("POST /b/{board}/cards/{id}/checklist/{index}/toggle", s.toggleChecklistItem)
	mux.HandleFunc("POST /b/{board}/cards/{id}/checklist/{index}", s.editChecklistItem)
	mux.HandleFunc("POST /b/{board}/cards/{id}/delete", s.deleteCard())
	mux.HandleFunc("POST /b/{board}/cards/{id}/archive", s.archiveCard)
	mux.HandleFunc("POST /b/{board}/archive/{id}/restore", s.restoreCard)
	s.mux = mux
	return s
}

// handler is the mux behind the middlewares. Both New and Serve go through
// it, so a middleware added here cannot miss one of them.
func (s *server) handler() http.Handler {
	return secureHeaders(sameOrigin(s.port, s.mux))
}

// Listen binds the loopback interface only (OPS-1, §7). A busy port is an
// error the caller reports, never a silent fallback to another port (OPS-3).
func Listen(port int) (net.Listener, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, fmt.Errorf("cannot listen on 127.0.0.1:%d (choose another with --port or KANDO_WEB_PORT): %w", port, err)
	}
	return ln, nil
}

// Serve runs the server on ln until ctx is cancelled, then shuts it down
// gracefully. The origin the same-origin guard trusts is taken from ln, so a
// requested and a bound port can never disagree. Read and idle timeouts keep
// a stuck local client from pinning connections; there is no write timeout,
// so long-lived streams (U9's SSE) are not cut off.
func Serve(ctx context.Context, ln net.Listener, o Options) error {
	if addr, ok := ln.Addr().(*net.TCPAddr); ok {
		o.Port = addr.Port
	}
	h := newServer(o)
	defer h.Close()
	srv := &http.Server{
		Handler:           h.handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	// Shutdown closes the listener, which makes Serve return straight away,
	// so the drain has to be waited for explicitly or the process exits
	// mid-request.
	ctx, cancelAll := context.WithCancel(ctx)
	defer cancelAll()
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		<-ctx.Done()
		// End the streams first: an open SSE connection would otherwise hold
		// Shutdown until its grace period expires.
		h.Close()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()
	err := srv.Serve(ln)
	cancelAll() // Serve may have returned for its own reasons
	<-drained
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// sameOrigin is the whole security boundary of a server with no login
// (SEC-1/SEC-3, §7). The Host must be this server's own loopback origin, which
// defeats DNS rebinding; an Origin header, when a browser sends one (every
// form POST, cross-site or not), must name the same origin, which defeats a
// cross-site form POST; and Sec-Fetch-Site, which browsers send on every
// request including the Origin-less cross-site GETs an <img> or <iframe>
// makes, must not say cross-site. Non-browser clients send neither header
// and are accepted on Host alone.
func sameOrigin(port int, next http.Handler) http.Handler {
	hosts := map[string]bool{
		fmt.Sprintf("127.0.0.1:%d", port): true,
		fmt.Sprintf("localhost:%d", port): true,
		fmt.Sprintf("[::1]:%d", port):     true,
	}
	if port == 80 { // browsers omit the default port from Host
		hosts["127.0.0.1"], hosts["localhost"], hosts["[::1]"] = true, true, true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hosts[r.Host] {
			http.Error(w, "forbidden: not a local origin", http.StatusForbidden)
			return
		}
		if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
			http.Error(w, "forbidden: cross-site request", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
			// "null" is an origin withheld, not a foreign one, and withheld
			// is not the same as cross-origin. Chrome sends it on every form
			// POST from a page whose Referrer-Policy is no-referrer — which
			// is the policy secureHeaders sets below, so this is how this
			// app's own forms arrive. Sec-Fetch-Site has already said which
			// kind of request it is, the browser sets it and script cannot
			// forge it, and a cross-site value was refused just above. An
			// Origin naming some other host is still refused outright.
			if o != "null" || r.Header.Get("Sec-Fetch-Site") != "same-origin" {
				http.Error(w, "forbidden: cross-origin request", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// secureHeaders is set on every response, errors and redirects included. The
// scripts are /static/live.js and /static/dnd.js, both served from this
// origin; live.js is what opens the SSE stream connect-src allows. The
// inline stylesheet in layout.html needs 'unsafe-inline' for styles only,
// and no inline script is ever allowed. frame-ancestors
// 'none' keeps the board out of other sites' frames, form-action 'self'
// keeps an injected form from posting elsewhere, and no-store keeps personal
// board content out of the browser cache (PERF-2).
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; connect-src 'self'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// httpError pairs a status with a message for the error response.
type httpError struct {
	status int
	msg    string
}

func (e *httpError) Error() string { return e.msg }

// mustExist is the 400/404 gate every {board} route shares: a name the
// store would reject is 400, a board that is not there is 404.
func (s *server) mustExist(name string) error {
	if !store.ValidBoardName(name) {
		return &httpError{http.StatusBadRequest, "invalid board name"}
	}
	if !store.Exists(s.root, name) {
		return &httpError{http.StatusNotFound, "no such board"}
	}
	return nil
}

// load reads the named board for this request, read-only: an unknown board
// is 404, an invalid name 400, and nothing on disk is ever created or
// rewritten by a GET.
func (s *server) load(name string) (*board.Board, error) {
	if !store.ValidBoardName(name) {
		return nil, &httpError{http.StatusBadRequest, "invalid board name"}
	}
	b, err := store.Load(s.root, name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, &httpError{http.StatusNotFound, "no such board"}
	}
	if err != nil {
		s.logf("load %s: %v", name, err)
		return nil, &httpError{http.StatusInternalServerError, "cannot read board"}
	}
	return b, nil
}

// fail writes an error response; unexpected errors are logged and become
// a generic 500 so no path or file detail reaches the page.
func (s *server) fail(w http.ResponseWriter, err error) {
	var he *httpError
	if errors.As(err, &he) {
		http.Error(w, he.msg, he.status)
		return
	}
	s.logf("%v", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

// render executes a template into a buffer first, so a failure can still
// become a clean 500 instead of a truncated 200 with the error spliced in.
func (s *server) render(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if err := s.tpl.ExecuteTemplate(&buf, name, data); err != nil {
		s.logf("render %s: %v", name, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}
