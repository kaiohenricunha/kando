package web

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

var fixedNow = time.Date(2026, 9, 3, 12, 0, 0, 0, time.FixedZone("-03", -3*3600))

const testPort = 4242

// newRoot builds a KANDO_HOME with "life" (the sample data) and "work" (empty).
func newRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	data, err := os.ReadFile("../store/testdata/sample_board.md")
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(root, "life"), 0o755)
	if err := os.WriteFile(filepath.Join(root, "life", "board.md"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Open(root, "work"); err != nil {
		t.Fatal(err)
	}
	return root
}

func newHandler(t *testing.T, root, defaultBoard string) http.Handler {
	t.Helper()
	return New(Options{Root: root, Board: defaultBoard, Port: testPort, Now: func() time.Time { return fixedNow }})
}

// post performs a same-origin form POST the way a browser on localhost would:
// every browser sends Origin on a form submission.
func post(t *testing.T, h http.Handler, path string, form url.Values, hdr ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Host = "127.0.0.1:4242"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://127.0.0.1:4242")
	for i := 0; i+1 < len(hdr); i += 2 {
		if hdr[i] == "Host" {
			req.Host = hdr[i+1]
		} else {
			req.Header.Set(hdr[i], hdr[i+1])
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// reload reads the board back from disk, the way the TUI's detail tests do.
func reload(t *testing.T, root, name string) *board.Board {
	t.Helper()
	b, err := store.Load(root, name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// wantRedirect asserts a post/redirect/get response.
func wantRedirect(t *testing.T, rec *httptest.ResponseRecorder, to string) {
	t.Helper()
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != to {
		t.Fatalf("want 303 → %s, got %d → %s: %s", to, rec.Code, rec.Header().Get("Location"), strings.TrimSpace(rec.Body.String()))
	}
}

// get performs a same-origin GET the way a browser on localhost would.
func get(t *testing.T, h http.Handler, path string, hdr ...string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req.Host = "127.0.0.1:4242"
	for i := 0; i+1 < len(hdr); i += 2 {
		if hdr[i] == "Host" {
			req.Host = hdr[i+1]
		} else {
			req.Header.Set(hdr[i], hdr[i+1])
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body, _ := io.ReadAll(rec.Result().Body)
	return rec, string(body)
}

func TestRootRedirectsToDefaultBoardOrBoardsList(t *testing.T) {
	root := newRoot(t)
	rec, _ := get(t, newHandler(t, root, "life"), "/")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/b/life" {
		t.Errorf("with a default board: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	rec, _ = get(t, newHandler(t, root, ""), "/")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/boards" {
		t.Errorf("without a default board: %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestBoardsListShowsExistingBoards(t *testing.T) {
	rec, body := get(t, newHandler(t, newRoot(t), "life"), "/boards")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	for _, want := range []string{`href="/b/life"`, `href="/b/work"`, `action="/boards"`, `method="post"`, `name="name"`} {
		if !strings.Contains(body, want) {
			t.Errorf("boards page missing %q", want)
		}
	}
}

func TestBoardViewRendersAllLanes(t *testing.T) {
	rec, body := get(t, newHandler(t, newRoot(t), "life"), "/b/life")
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("status %d type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	for _, want := range []string{"BACKLOG", "TODO", "DOING", "DONE", "Renew passport", "3d", "#errand", "1/4",
		"⊘ waiting on pads", "Cancel gym membership", "11 cards", `href="/b/life/cards/k7q2m9ab"`,
		`href="/b/life/cards/new?lane=todo"`, `href="/b/life/archive"`} {
		if !strings.Contains(body, want) {
			t.Errorf("board page missing %q", want)
		}
	}
}

func TestBoardViewFilterQueryParam(t *testing.T) {
	_, body := get(t, newHandler(t, newRoot(t), "life"), "/b/life?q=%23home")
	for _, want := range []string{"Learn to make sourdough", "Fix bike brake", "3 of 11 match", "3 cards", `value="#home"`} {
		if !strings.Contains(body, want) {
			t.Errorf("filtered page missing %q", want)
		}
	}
	if strings.Contains(body, "Renew passport") {
		t.Errorf("filtered-out card still rendered")
	}
	_, body = get(t, newHandler(t, newRoot(t), "life"), "/b/life?q=%21blocked+age%3E3d")
	if !strings.Contains(body, "Fix bike brake") || !strings.Contains(body, "1 of 11 match") {
		t.Errorf("operators should combine: %s", grepLine(body, "match"))
	}
}

func TestCardDetailPageRenders(t *testing.T) {
	rec, body := get(t, newHandler(t, newRoot(t), "life"), "/b/life/cards/k7q2m9ab")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	for _, want := range []string{"Renew passport", "#errand", "created Mon 31 Aug", "in Todo since Tue 1 Sep", "NOTES",
		"Appointment slots open on Mondays.", "CHECKLIST", "1/4", "Photos from the pharmacy", "Fill in the form", "BLOCKED", "not blocked"} {
		if !strings.Contains(body, want) {
			t.Errorf("detail page missing %q", want)
		}
	}
	rec, _ = get(t, newHandler(t, newRoot(t), "life"), "/b/life/cards/nope1234")
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown card: %d", rec.Code)
	}
}

func TestNewCardFormRenders(t *testing.T) {
	rec, body := get(t, newHandler(t, newRoot(t), "life"), "/b/life/cards/new?lane=doing")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	for _, want := range []string{`action="/b/life/cards"`, `method="post"`, `name="title"`, `value="doing"`, `selected`} {
		if !strings.Contains(body, want) {
			t.Errorf("new-card form missing %q", want)
		}
	}
}

func TestUnknownAndInvalidBoards(t *testing.T) {
	h := newHandler(t, newRoot(t), "life")
	if rec, _ := get(t, h, "/b/nope"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown board: %d", rec.Code)
	}
	if rec, _ := get(t, h, "/b/.hidden"); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid board name: %d", rec.Code)
	}
}

func TestGetNeverCreatesABoard(t *testing.T) {
	root := newRoot(t)
	get(t, newHandler(t, root, "life"), "/b/brandnew")
	if _, err := os.Stat(filepath.Join(root, "brandnew")); err == nil {
		t.Errorf("GET /b/brandnew created a directory")
	}
}

func TestHTMLEscapesUserText(t *testing.T) {
	root := newRoot(t)
	st, b, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	c := board.NewCard("<script>alert(1)</script>", board.Todo, fixedNow)
	c.SetNotes("<b>bold</b> & <i>")
	c.SetTag("<img>")
	b.Insert(board.Todo, 0, c)
	st.SaveBoard(b)
	h := newHandler(t, root, "life")
	for _, path := range []string{"/b/life", "/b/life/cards/" + c.ID, "/b/life?q=%3Cscript%3E"} {
		_, body := get(t, h, path)
		if strings.Contains(body, "<script>alert") || strings.Contains(body, "<b>bold") || strings.Contains(body, "#<img>") {
			t.Errorf("%s: user text rendered unescaped", path)
		}
		if !strings.Contains(body, "&lt;script&gt;") {
			t.Errorf("%s: escaped title missing", path)
		}
	}
}

func TestSameOriginMiddleware(t *testing.T) {
	h := newHandler(t, newRoot(t), "life")
	cases := []struct {
		name string
		hdr  []string
		want int
	}{
		{"loopback host", nil, 200},
		{"localhost host", []string{"Host", "localhost:4242"}, 200},
		{"ipv6 loopback", []string{"Host", "[::1]:4242"}, 200},
		{"matching origin", []string{"Origin", "http://127.0.0.1:4242"}, 200},
		{"foreign host (DNS rebinding)", []string{"Host", "evil.example:4242"}, http.StatusForbidden},
		{"wrong port", []string{"Host", "127.0.0.1:9999"}, http.StatusForbidden},
		{"foreign origin (CSRF)", []string{"Origin", "http://evil.example"}, http.StatusForbidden},
		{"null origin", []string{"Origin", "null"}, http.StatusForbidden},
		// Chrome withholds the origin on a form POST whenever the page's
		// Referrer-Policy is no-referrer, which secureHeaders sets — so
		// every mutation form in this app arrives like this. Withheld is
		// not cross-origin, and Sec-Fetch-Site has already said which it is.
		{"origin withheld, browser says same-origin", []string{"Origin", "null", "Sec-Fetch-Site", "same-origin"}, 200},
		{"origin withheld, browser says cross-site", []string{"Origin", "null", "Sec-Fetch-Site", "cross-site"}, http.StatusForbidden},
		{"foreign origin, browser says same-origin", []string{"Origin", "http://evil.example", "Sec-Fetch-Site", "same-origin"}, http.StatusForbidden},
		{"cross-site GET without Origin (img/iframe)", []string{"Sec-Fetch-Site", "cross-site"}, http.StatusForbidden},
		{"same-site subdomain fetch", []string{"Sec-Fetch-Site", "same-site"}, http.StatusForbidden},
		{"same-origin fetch", []string{"Sec-Fetch-Site", "same-origin"}, 200},
		{"typed into the address bar", []string{"Sec-Fetch-Site", "none"}, 200},
	}
	for _, c := range cases {
		if rec, _ := get(t, h, "/b/life", c.hdr...); rec.Code != c.want {
			t.Errorf("%s: got %d, want %d", c.name, rec.Code, c.want)
		}
	}
}

func grepLine(body, needle string) string {
	for _, l := range strings.Split(body, "\n") {
		if strings.Contains(l, needle) {
			return strings.TrimSpace(l)
		}
	}
	return ""
}

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	h := newHandler(t, newRoot(t), "life")
	for _, path := range []string{"/b/life", "/b/nope", "/"} {
		rec, _ := get(t, h, path)
		for k, want := range map[string]string{
			"Content-Security-Policy": "default-src 'none'; script-src 'self'; connect-src 'self'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'",
			"X-Frame-Options":         "DENY",
			"X-Content-Type-Options":  "nosniff",
			"Referrer-Policy":         "no-referrer",
			"Cache-Control":           "no-store",
		} {
			if got := rec.Header().Get(k); got != want {
				t.Errorf("%s: %s = %q, want %q", path, k, got, want)
			}
		}
	}
}

func TestGetDoesNotRewriteIdlessBoard(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "hand")
	os.MkdirAll(dir, 0o755)
	idless := []byte("## Todo\n\n### Typed by hand\n")
	os.WriteFile(filepath.Join(dir, "board.md"), idless, 0o644)
	h := newHandler(t, root, "hand")
	rec, body := get(t, h, "/b/hand")
	if rec.Code != 200 || !strings.Contains(body, "Typed by hand") {
		t.Fatalf("status %d", rec.Code)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "board.md")); string(got) != string(idless) {
		t.Errorf("a GET rewrote board.md:\n%s", got)
	}
	names, _ := store.ListBoards(root)
	if len(names) != 1 {
		t.Errorf("a GET created something: %v", names)
	}
}

func TestBoardNameIsEscapedInLinks(t *testing.T) {
	root := newRoot(t)
	if _, _, err := store.Open(root, "My Board"); err != nil {
		t.Fatal(err)
	}
	h := newHandler(t, root, "My Board")
	rec, _ := get(t, h, "/")
	if loc := rec.Header().Get("Location"); loc != "/b/My%20Board" {
		t.Errorf("redirect = %q", loc)
	}
	_, body := get(t, h, "/boards")
	if !strings.Contains(body, `href="/b/My%20Board"`) {
		t.Errorf("boards list should escape the name: %s", grepLine(body, "My"))
	}
	if rec, _ := get(t, h, "/b/My%20Board"); rec.Code != 200 {
		t.Errorf("escaped link must resolve: %d", rec.Code)
	}
}

func TestListenBindsLoopbackOnly(t *testing.T) {
	ln, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if host, _, _ := net.SplitHostPort(ln.Addr().String()); host != "127.0.0.1" {
		t.Errorf("bound %s, want 127.0.0.1 (OPS-1)", ln.Addr())
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_, err = Listen(port)
	if err == nil {
		t.Fatal("a busy port must be an error, never a fallback (OPS-3)")
	}
	for _, want := range []string{strconv.Itoa(port), "--port", "KANDO_WEB_PORT"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should name %q: %v", want, err)
		}
	}
}

func TestServeDerivesOriginFromListenerAndShutsDown(t *testing.T) {
	root := newRoot(t)
	ln, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, ln, Options{Root: root, Board: "life", Now: func() time.Time { return fixedNow }})
	}()
	// Options.Port was not set; the guard must still accept the real port.
	resp, err := http.Get("http://" + ln.Addr().String() + "/b/life")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("GET via the bound port: %d", resp.StatusCode)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve returned %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
}
