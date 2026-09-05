// Package web serves the kando board as server-rendered HTML on localhost.
// It is a second renderer over the same internal/board model and
// internal/store files the TUI uses — never a second model (KD-1, §4 of
// docs/specs/kando-web). Every request loads the board from disk read-only
// (store.Load), acts, and forgets (KD-3): no board lives in memory between
// requests, so handlers need no locking and a page is never stale.
package web

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
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
}

//go:embed templates/*.html
var templateFS embed.FS

type server struct {
	root string
	def  string
	now  func() time.Time
	tpl  *template.Template
}

// New builds the HTTP handler: the routes of §5 behind the security headers
// and the same-origin guard.
func New(o Options) http.Handler {
	if o.Now == nil {
		o.Now = time.Now
	}
	s := &server{root: o.Root, def: o.Board, now: o.Now}
	funcs := template.FuncMap{"urlpath": url.PathEscape}
	s.tpl = template.Must(template.New("").Funcs(funcs).ParseFS(templateFS, "templates/*.html"))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /boards", s.boards)
	mux.HandleFunc("GET /b/{board}", s.board)
	mux.HandleFunc("GET /b/{board}/cards/new", s.newCard)
	mux.HandleFunc("GET /b/{board}/cards/{id}", s.card)
	// The board page links here already (the §5 route table is the U6–U8
	// contract, frozen now); the real handler lands in U8.
	mux.HandleFunc("GET /b/{board}/archive", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not implemented yet (U8)", http.StatusNotImplemented)
	})
	return secureHeaders(sameOrigin(o.Port, mux))
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
	srv := &http.Server{
		Handler:           New(o),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
			http.Error(w, "forbidden: cross-origin request", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// secureHeaders is set on every response, errors and redirects included. The
// page has no scripts yet, so the policy allows none; the inline stylesheet
// in layout.html needs 'unsafe-inline' for styles only. frame-ancestors
// 'none' keeps the board out of other sites' frames, form-action 'self'
// keeps an injected form from posting elsewhere, and no-store keeps personal
// board content out of the browser cache (PERF-2).
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
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
		fmt.Fprintf(os.Stderr, "kando web: load %s: %v\n", name, err)
		return nil, &httpError{http.StatusInternalServerError, "cannot read board"}
	}
	return b, nil
}

// fail writes an error response; unexpected errors are logged and become
// a generic 500 so no path or file detail reaches the page.
func fail(w http.ResponseWriter, err error) {
	var he *httpError
	if errors.As(err, &he) {
		http.Error(w, he.msg, he.status)
		return
	}
	fmt.Fprintf(os.Stderr, "kando web: %v\n", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

// render executes a template into a buffer first, so a failure can still
// become a clean 500 instead of a truncated 200 with the error spliced in.
func (s *server) render(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if err := s.tpl.ExecuteTemplate(&buf, name, data); err != nil {
		fmt.Fprintf(os.Stderr, "kando web: render %s: %v\n", name, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}
