package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		"⊘ waiting on pads", "Cancel gym membership", "11 cards", "Expires 14 Nov.", `href="/b/life/cards/k7q2m9ab"`,
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
	if _, err := os.Stat(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Errorf("a GET must never create a board directory")
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
