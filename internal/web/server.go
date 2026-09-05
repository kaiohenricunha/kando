// Package web serves the kando board as server-rendered HTML on localhost.
// It is a second renderer over the same internal/board model and
// internal/store files the TUI uses — never a second model (KD-1, §4 of
// docs/specs/kando-web). Every request loads the board from disk, acts, and
// forgets (KD-3): no board lives in memory between requests, so handlers
// need no locking and a page is never stale.
package web

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

// Options configures the server. Board is the board "/" redirects to; when
// empty "/" goes to the boards list. Port is the port the listener is bound
// to; the same-origin check (SEC-3, §7) compares Host and Origin against it.
type Options struct {
	Root  string
	Board string
	Port  int
	Now   func() time.Time
}

//go:embed templates/*.html
var templateFS embed.FS

type server struct {
	root string
	def  string
	now  func() time.Time
	tpl  *template.Template
}

// New builds the HTTP handler: the routes of §5 behind the same-origin guard.
func New(o Options) http.Handler {
	if o.Now == nil {
		o.Now = time.Now
	}
	s := &server{root: o.Root, def: o.Board, now: o.Now}
	s.tpl = template.Must(template.New("").ParseFS(templateFS, "templates/*.html"))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /boards", s.boards)
	mux.HandleFunc("GET /b/{board}", s.board)
	mux.HandleFunc("GET /b/{board}/cards/new", s.newCard)
	mux.HandleFunc("GET /b/{board}/cards/{id}", s.card)
	return sameOrigin(o.Port, mux)
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

// sameOrigin is the whole security boundary of a server with no login
// (SEC-1/SEC-3, §7): the Host must be this server's own loopback origin, which
// defeats DNS rebinding, and an Origin header, when a browser sends one, must
// name the same origin, which defeats a cross-site form POST. Anything else is
// refused before any handler runs.
func sameOrigin(port int, next http.Handler) http.Handler {
	hosts := map[string]bool{
		fmt.Sprintf("127.0.0.1:%d", port): true,
		fmt.Sprintf("localhost:%d", port): true,
		fmt.Sprintf("[::1]:%d", port):     true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hosts[r.Host] {
			http.Error(w, "forbidden: not a local origin", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
			http.Error(w, "forbidden: cross-origin request", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// httpError pairs a status with a message for the error page.
type httpError struct {
	status int
	msg    string
}

func (e *httpError) Error() string { return e.msg }

// load opens the named board fresh from disk for this request. A GET never
// creates a board: an unknown one is 404, an invalid name 400.
func (s *server) load(name string) (*store.Store, *board.Board, error) {
	if !store.ValidBoardName(name) {
		return nil, nil, &httpError{http.StatusBadRequest, "invalid board name"}
	}
	if !store.Exists(s.root, name) {
		return nil, nil, &httpError{http.StatusNotFound, "no such board"}
	}
	st, b, err := store.Open(s.root, name)
	if err != nil {
		return nil, nil, &httpError{http.StatusInternalServerError, err.Error()}
	}
	return st, b, nil
}

// fail writes an error response; unexpected errors become 500s.
func fail(w http.ResponseWriter, err error) {
	var he *httpError
	if errors.As(err, &he) {
		http.Error(w, he.msg, he.status)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func (s *server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
