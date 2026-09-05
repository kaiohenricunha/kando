package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

// todoCard is the id of the first Todo card in the sample board.
func todoCard(t *testing.T, root string) string {
	t.Helper()
	return reload(t, root, "life").Lanes[board.Todo][0].ID
}

func TestCreateCardRoute(t *testing.T) {
	root := newRoot(t)
	h := newHandler(t, root, "life")
	before := reload(t, root, "life").Count()
	rec := post(t, h, "/b/life/cards", url.Values{"lane": {"doing"}, "title": {"  Water the plants \n"}})
	wantRedirect(t, rec, "/b/life")
	b := reload(t, root, "life")
	c := b.Lanes[board.Doing][0]
	if c.Title != "Water the plants" || !c.CreatedAt.Equal(fixedNow) || !c.MovedAt.Equal(fixedNow) || len(c.ID) != 8 {
		t.Errorf("created card: %+v", c)
	}
	if b.Count() != before+1 {
		t.Errorf("count %d, want %d", b.Count(), before+1)
	}
	for name, form := range map[string]url.Values{
		"empty title":  {"lane": {"todo"}, "title": {"   "}},
		"invalid lane": {"lane": {"soon"}, "title": {"x"}},
	} {
		if rec := post(t, h, "/b/life/cards", form); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	if rec := post(t, h, "/b/nope/cards", url.Values{"lane": {"todo"}, "title": {"x"}}); rec.Code != http.StatusNotFound {
		t.Errorf("unknown board: %d", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(root, "nope")); err == nil {
		t.Errorf("a card POST must never create a board")
	}
	if reload(t, root, "life").Count() != before+1 {
		t.Errorf("rejected posts must not change the board")
	}
}

func TestUpdateCardRoute(t *testing.T) {
	root := newRoot(t)
	h := newHandler(t, root, "life")
	id := todoCard(t, root)
	rec := post(t, h, "/b/life/cards/"+id, url.Values{"title": {" Renew passport now "}, "tag": {"#travel"}, "notes": {"line one\r\nline two\r\n\r\n"}})
	wantRedirect(t, rec, "/b/life/cards/"+id)
	_, _, c := reload(t, root, "life").Find(id)
	if c.Title != "Renew passport now" || c.Tag != "travel" || c.Notes != "line one\nline two" {
		t.Errorf("updated: %q %q %q", c.Title, c.Tag, c.Notes)
	}
	// Only the fields present change: a tag-only form keeps title and notes.
	post(t, h, "/b/life/cards/"+id, url.Values{"tag": {""}})
	_, _, c = reload(t, root, "life").Find(id)
	if c.Tag != "" || c.Title != "Renew passport now" || c.Notes != "line one\nline two" {
		t.Errorf("partial update: %+v", c)
	}
	if rec := post(t, h, "/b/life/cards/"+id, url.Values{"title": {""}}); rec.Code != http.StatusBadRequest {
		t.Errorf("empty title: %d", rec.Code)
	}
	if rec := post(t, h, "/b/life/cards/zzzzzzzz", url.Values{"title": {"x"}}); rec.Code != http.StatusNotFound {
		t.Errorf("unknown card: %d", rec.Code)
	}
}

func TestMoveCardRoute(t *testing.T) {
	root := newRoot(t)
	h := newHandler(t, root, "life")
	id := todoCard(t, root)
	wantRedirect(t, post(t, h, "/b/life/cards/"+id+"/move", url.Values{"lane": {"done"}}), "/b/life/cards/"+id)
	b := reload(t, root, "life")
	lane, i, c := b.Find(id)
	if lane != board.Done || i != 0 || !c.DoneAt.Equal(fixedNow) || !c.MovedAt.Equal(fixedNow) {
		t.Errorf("after move: lane=%v i=%d done=%v moved=%v", lane, i, c.DoneAt, c.MovedAt)
	}
	wantRedirect(t, post(t, h, "/b/life/cards/"+id+"/move", url.Values{"lane": {"Doing"}}), "/b/life/cards/"+id)
	lane, _, c = reload(t, root, "life").Find(id)
	if lane != board.Doing || !c.DoneAt.IsZero() {
		t.Errorf("undo: lane=%v done=%v", lane, c.DoneAt)
	}
	if rec := post(t, h, "/b/life/cards/"+id+"/move", url.Values{"lane": {"later"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid lane: %d", rec.Code)
	}
}

func TestBlockCardRoute(t *testing.T) {
	root := newRoot(t)
	h := newHandler(t, root, "life")
	id := todoCard(t, root)
	wantRedirect(t, post(t, h, "/b/life/cards/"+id+"/block", url.Values{"reason": {" waiting on photos "}}), "/b/life/cards/"+id)
	_, _, c := reload(t, root, "life").Find(id)
	if !c.Blocked || c.BlockedReason != "waiting on photos" {
		t.Errorf("block: %+v", c)
	}
	_, body := get(t, h, "/b/life/cards/"+id)
	if !strings.Contains(body, "⊘ waiting on photos") || !strings.Contains(body, `value="waiting on photos"`) {
		t.Errorf("detail should show and prefill the reason: %s", grepLine(body, "waiting"))
	}
	post(t, h, "/b/life/cards/"+id+"/block", url.Values{"reason": {""}})
	if _, _, c := reload(t, root, "life").Find(id); c.Blocked || c.BlockedReason != "" {
		t.Errorf("clear: %+v", c)
	}
}

func TestChecklistRoutes(t *testing.T) {
	root := newRoot(t)
	h := newHandler(t, root, "life")
	id := todoCard(t, root) // Renew passport: 4 items, first done
	card := "/b/life/cards/" + id
	wantRedirect(t, post(t, h, card+"/checklist", url.Values{"text": {" Collect it "}}), card)
	_, _, c := reload(t, root, "life").Find(id)
	if n := len(c.Checklist); n != 5 || c.Checklist[4].Text != "Collect it" || c.Checklist[4].Done {
		t.Fatalf("add: %+v", c.Checklist)
	}
	wantRedirect(t, post(t, h, card+"/checklist/1/toggle", nil), card)
	wantRedirect(t, post(t, h, card+"/checklist/0/toggle", nil), card)
	_, _, c = reload(t, root, "life").Find(id)
	if !c.Checklist[1].Done || c.Checklist[0].Done {
		t.Errorf("toggle: %+v", c.Checklist)
	}
	wantRedirect(t, post(t, h, card+"/checklist/2", url.Values{"text": {"Book the appointment"}}), card)
	_, _, c = reload(t, root, "life").Find(id)
	if c.Checklist[2].Text != "Book the appointment" {
		t.Errorf("edit: %+v", c.Checklist[2])
	}
	for name, path := range map[string]string{"toggle out of range": card + "/checklist/9/toggle", "toggle non-numeric": card + "/checklist/x/toggle"} {
		if rec := post(t, h, path, nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	if rec := post(t, h, card+"/checklist/7", url.Values{"text": {"x"}}); rec.Code != http.StatusNotFound {
		t.Errorf("edit out of range: %d", rec.Code)
	}
	for name, path := range map[string]string{"add empty": card + "/checklist", "edit empty": card + "/checklist/0"} {
		if rec := post(t, h, path, url.Values{"text": {"  "}}); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	_, body := get(t, h, card)
	if !strings.Contains(body, `action="/b/life/cards/`+id+`/checklist/1/toggle"`) || !strings.Contains(body, "1/5") {
		t.Errorf("detail should render toggle forms and progress: %s", grepLine(body, "CHECKLIST"))
	}
}

func TestDeleteCardRoute(t *testing.T) {
	root := newRoot(t)
	h := newHandler(t, root, "life")
	id := todoCard(t, root)
	before := reload(t, root, "life").Count()
	wantRedirect(t, post(t, h, "/b/life/cards/"+id+"/delete", nil), "/b/life")
	b := reload(t, root, "life")
	if _, _, c := b.Find(id); c != nil || b.Count() != before-1 {
		t.Errorf("delete: found=%v count=%d", c != nil, b.Count())
	}
	if rec := post(t, h, "/b/life/cards/"+id+"/delete", nil); rec.Code != http.StatusNotFound {
		t.Errorf("deleting twice: %d", rec.Code)
	}
}

func TestMutationsRequireSameOriginAndPost(t *testing.T) {
	root := newRoot(t)
	h := newHandler(t, root, "life")
	id := todoCard(t, root)
	before, _ := os.ReadFile(filepath.Join(root, "life", "board.md"))
	cases := []struct {
		name string
		hdr  []string
		code int
	}{
		{"foreign Origin", []string{"Origin", "http://evil.example"}, http.StatusForbidden},
		{"foreign Host", []string{"Host", "kando.evil.example:4242"}, http.StatusForbidden},
		{"cross-site fetch", []string{"Sec-Fetch-Site", "cross-site"}, http.StatusForbidden},
	}
	for _, tc := range cases {
		if rec := post(t, h, "/b/life/cards/"+id+"/delete", nil, tc.hdr...); rec.Code != tc.code {
			t.Errorf("%s: %d", tc.name, rec.Code)
		}
	}
	if rec, _ := get(t, h, "/b/life/cards/"+id+"/delete"); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET on a mutation route: %d (SEC-3)", rec.Code)
	}
	if after, _ := os.ReadFile(filepath.Join(root, "life", "board.md")); string(after) != string(before) {
		t.Errorf("rejected requests changed board.md")
	}
}

func TestCreateBoardRoute(t *testing.T) {
	root := newRoot(t)
	h := newHandler(t, root, "")
	wantRedirect(t, post(t, h, "/boards", url.Values{"name": {" garden "}}), "/b/garden")
	data, err := os.ReadFile(filepath.Join(root, "garden", "board.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(store.Marshal(&board.Board{Name: "garden"})) {
		t.Errorf("board.md should be canonical:\n%s", data)
	}
	_, body := get(t, h, "/boards")
	if !strings.Contains(body, `href="/b/garden"`) {
		t.Errorf("new board should be listed: %s", grepLine(body, "garden"))
	}
	// Creating an existing board is a no-op redirect; the file is untouched.
	before, _ := os.ReadFile(filepath.Join(root, "life", "board.md"))
	wantRedirect(t, post(t, h, "/boards", url.Values{"name": {"life"}}), "/b/life")
	if after, _ := os.ReadFile(filepath.Join(root, "life", "board.md")); string(after) != string(before) {
		t.Errorf("re-creating rewrote board.md")
	}
	for _, bad := range []string{"", ".hidden", "a/b", "q#1", strings.Repeat("x", 65)} {
		if rec := post(t, h, "/boards", url.Values{"name": {bad}}); rec.Code != http.StatusBadRequest {
			t.Errorf("name %q: %d", bad, rec.Code)
		}
	}
	names, _ := store.ListBoards(root)
	if len(names) != 3 {
		t.Errorf("boards after invalid names: %v", names)
	}
}

// archiveRoot is newRoot plus the sample archive on "life".
func archiveRoot(t *testing.T) string {
	t.Helper()
	root := newRoot(t)
	data, err := os.ReadFile("../store/testdata/sample_archive.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "life", "archive.md"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestArchiveViewGroupsByWeek(t *testing.T) {
	root := archiveRoot(t)
	h := newHandler(t, root, "life")
	rec, body := get(t, h, "/b/life/archive")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	for _, want := range []string{"THIS WEEK 3", "LAST WEEK 4", "EARLIER 3", "10 done", "Cancel gym membership", "Eye test", "Wed 2 Sep", `action="/b/life/archive/`} {
		if !strings.Contains(body, want) {
			t.Errorf("archive page should contain %q", want)
		}
	}
	if i, j := strings.Index(body, "THIS WEEK"), strings.Index(body, "EARLIER"); i > j {
		t.Errorf("groups out of order")
	}
	_, body = get(t, h, "/b/life/archive?q=%23money")
	if !strings.Contains(body, "4 of 10 match") || strings.Contains(body, "Eye test") {
		t.Errorf("filter: %s", grepLine(body, "match"))
	}
	if rec, body := get(t, h, "/b/work/archive"); rec.Code != 200 || !strings.Contains(body, "nothing archived") {
		t.Errorf("empty archive: %d %s", rec.Code, grepLine(body, "archived"))
	}
	if rec, _ := get(t, h, "/b/nope/archive"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown board: %d", rec.Code)
	}
}

func TestArchiveRestoreRoute(t *testing.T) {
	root := archiveRoot(t)
	h := newHandler(t, root, "life")
	a, _ := store.LoadArchive(root, "life")
	id := a.Cards[0].ID
	wantRedirect(t, post(t, h, "/b/life/archive/"+id+"/restore", nil), "/b/life")
	b := reload(t, root, "life")
	lane, i, c := b.Find(id)
	if c == nil || lane != board.Doing || i != 0 || !c.DoneAt.IsZero() || !c.MovedAt.Equal(fixedNow) {
		t.Fatalf("restored: lane=%v i=%d card=%+v", lane, i, c)
	}
	a, _ = store.LoadArchive(root, "life")
	if len(a.Cards) != 9 {
		t.Errorf("archive should have 9 left, has %d", len(a.Cards))
	}
	for _, x := range a.Cards {
		if x.ID == id {
			t.Errorf("restored card still archived")
		}
	}
	if rec := post(t, h, "/b/life/archive/"+id+"/restore", nil); rec.Code != http.StatusNotFound {
		t.Errorf("restoring twice: %d", rec.Code)
	}
}
