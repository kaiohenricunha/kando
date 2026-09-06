package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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
	if !strings.Contains(body, "4 of the newest 10 match") || strings.Contains(body, "Eye test") {
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
	wantRedirect(t, post(t, h, "/b/life/archive/"+id+"/restore", url.Values{"q": {"#money"}}), "/b/life/archive?q=%23money")
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

func TestArchiveCardRoute(t *testing.T) {
	root := archiveRoot(t)
	h := newHandler(t, root, "life")
	before := reload(t, root, "life")
	lane, _, c := before.Find("c6d7e2f3") // "Cancel gym membership", Done, done: 2026-09-02
	if c == nil || lane != board.Done {
		t.Fatalf("fixture card missing or moved: lane=%v c=%v", lane, c)
	}
	doneAt := c.DoneAt
	wantRedirect(t, post(t, h, "/b/life/cards/c6d7e2f3/archive", nil), "/b/life")
	b := reload(t, root, "life")
	if lane, _, c := b.Find("c6d7e2f3"); c != nil {
		t.Errorf("archived card should be gone from the board: lane=%v", lane)
	}
	a, _ := store.LoadArchive(root, "life")
	if len(a.Cards) != 11 {
		t.Fatalf("archive should have 11 cards, has %d", len(a.Cards))
	}
	got := a.Cards[0]
	if got.ID != "c6d7e2f3" {
		t.Fatalf("archived card should be newest (index 0): %+v", got)
	}
	if !got.DoneAt.Equal(doneAt) {
		t.Errorf("DoneAt must be kept, not restamped to fixedNow: got %v, want %v", got.DoneAt, doneAt)
	}
	if !got.MovedAt.Equal(c.MovedAt) {
		t.Errorf("MovedAt must be untouched: got %v, want %v", got.MovedAt, c.MovedAt)
	}
	if rec := post(t, h, "/b/life/cards/c6d7e2f3/archive", nil); rec.Code != http.StatusNotFound {
		t.Errorf("archiving a card no longer on the board: %d", rec.Code)
	}
}

func TestArchiveCardRefusesACardNotInDone(t *testing.T) {
	root := archiveRoot(t)
	h := newHandler(t, root, "life")
	id := todoCard(t, root)
	before, _ := os.ReadFile(filepath.Join(root, "life", "board.md"))
	beforeArchive, _ := os.ReadFile(filepath.Join(root, "life", "archive.md"))
	rec := post(t, h, "/b/life/cards/"+id+"/archive", nil)
	if rec.Code != http.StatusConflict {
		t.Errorf("archiving a Todo card: %d, want 409", rec.Code)
	}
	after, _ := os.ReadFile(filepath.Join(root, "life", "board.md"))
	afterArchive, _ := os.ReadFile(filepath.Join(root, "life", "archive.md"))
	if string(before) != string(after) || string(beforeArchive) != string(afterArchive) {
		t.Errorf("a refused archive must touch neither file")
	}
}

func TestArchiveCardWithNoDoneAtStampsNow(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "life")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "board.md"), []byte("## Done\n\n### Done by hand\n"), 0o644)
	h := newHandler(t, root, "life")
	_, body := get(t, h, "/b/life")
	id := idRe.FindStringSubmatch(body)[1]
	wantRedirect(t, post(t, h, "/b/life/cards/"+id+"/archive", nil), "/b/life")
	a, _ := store.LoadArchive(root, "life")
	if len(a.Cards) != 1 || !a.Cards[0].DoneAt.Equal(fixedNow) {
		t.Fatalf("a missing done: date should be stamped now: %+v", a.Cards)
	}
	data := string(mustReadWeb(t, filepath.Join(dir, "archive.md")))
	if !strings.Contains(data, "## "+board.ISOWeekKey(fixedNow)) || strings.Contains(data, "undated") {
		t.Errorf("should file under this week, not undated:\n%s", data)
	}
}

func TestArchivingACardAlreadyArchivedIsRefused(t *testing.T) {
	root := archiveRoot(t)
	h := newHandler(t, root, "life")
	a, _ := store.LoadArchive(root, "life")
	dupID := a.Cards[0].ID
	b := reload(t, root, "life")
	b.Insert(board.Done, 0, &board.Card{ID: dupID, Title: "duplicate of an archived id", DoneAt: fixedNow})
	st, _, _ := store.Open(root, "life")
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	if rec := post(t, h, "/b/life/cards/"+dupID+"/archive", nil); rec.Code != http.StatusConflict {
		t.Errorf("archiving a duplicate id: %d, want 409", rec.Code)
	}
	if a2, _ := store.LoadArchive(root, "life"); len(a2.Cards) != 10 {
		t.Errorf("a refused archive must not touch the archive: %d", len(a2.Cards))
	}
}

func TestCardPageShowsArchiveOnlyInDone(t *testing.T) {
	root := archiveRoot(t)
	h := newHandler(t, root, "life")
	_, doneBody := get(t, h, "/b/life/cards/c6d7e2f3")
	if !strings.Contains(doneBody, `action="/b/life/cards/c6d7e2f3/archive"`) {
		t.Errorf("a Done card's page should offer archive:\n%s", grepLine(doneBody, "archive"))
	}
	_, todoBody := get(t, h, "/b/life/cards/"+todoCard(t, root))
	if strings.Contains(todoBody, "/archive\"") {
		t.Errorf("a Todo card's page must not offer archive")
	}
}

func mustReadWeb(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// mutationRoutes is §5's mutation table in spec order. hasGET marks the two
// paths that are also a page — a GET there renders, it does not mutate.
func mutationRoutes(id, archived string) []struct {
	path   string
	hasGET bool
} {
	card := "/b/life/cards/" + id
	return []struct {
		path   string
		hasGET bool
	}{
		{"/boards", true}, {"/b/life/cards", false}, {card, true},
		{card + "/move", false}, {card + "/block", false},
		{card + "/checklist", false}, {card + "/checklist/0/toggle", false},
		{card + "/checklist/0", false}, {card + "/delete", false},
		{card + "/archive", false},
		{"/b/life/archive/" + archived + "/restore", false},
	}
}

func TestEveryMutationRouteIsPostOnlyAndSameOrigin(t *testing.T) {
	root := archiveRoot(t)
	h := newHandler(t, root, "life")
	a, _ := store.LoadArchive(root, "life")
	routes := mutationRoutes(todoCard(t, root), a.Cards[0].ID)
	before, _ := os.ReadFile(filepath.Join(root, "life", "board.md"))
	for _, rt := range routes {
		path := rt.path
		want := http.StatusMethodNotAllowed
		if rt.hasGET {
			want = http.StatusOK
		}
		if rec, _ := get(t, h, path); rec.Code != want {
			t.Errorf("GET %s = %d, want %d (SEC-3: a mutation is never a GET)", path, rec.Code, want)
		}
		for _, hdr := range [][]string{
			{"Origin", "http://evil.example"},
			{"Host", "kando.evil.example:4242"},
			{"Sec-Fetch-Site", "cross-site"},
		} {
			if rec := post(t, h, path, nil, hdr...); rec.Code != http.StatusForbidden {
				t.Errorf("POST %s with %s: %d, want 403", path, hdr[0], rec.Code)
			}
		}
	}
	if after, _ := os.ReadFile(filepath.Join(root, "life", "board.md")); string(after) != string(before) {
		t.Errorf("rejected requests changed board.md")
	}
}

func TestMutationsWorkOnAHandWrittenBoard(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "hand")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "board.md"), []byte("## Todo\n\n### Typed by hand\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "archive.md"), []byte("## 2026-W36\n\n### Done by hand\ndone: 2026-09-02\n"), 0o644)
	h := newHandler(t, root, "hand")

	// The id a page renders for an id-less card must be the id the POST it
	// produces resolves — otherwise every first mutation 404s.
	_, body := get(t, h, "/b/hand")
	id := idRe.FindStringSubmatch(body)[1]
	if len(id) != 8 {
		t.Fatalf("no card id in the rendered board: %q", grepLine(body, "cards/"))
	}
	wantRedirect(t, post(t, h, "/b/hand/cards/"+id+"/move", url.Values{"lane": {"doing"}}), "/b/hand/cards/"+id)
	if lane, _, c := reload(t, root, "hand").Find(id); c == nil || lane != board.Doing {
		t.Errorf("move on a hand-written board: lane=%v card=%v", lane, c)
	}
	_, body = get(t, h, "/b/hand/archive")
	aid := between(body, `/archive/`, `/restore`)
	if len(aid) != 8 {
		t.Fatalf("no archived id rendered: %q", grepLine(body, "restore"))
	}
	if rec := post(t, h, "/b/hand/archive/"+aid+"/restore", nil); rec.Code != http.StatusSeeOther {
		t.Errorf("restore on a hand-written archive: %d", rec.Code)
	}
}

// idRe finds a rendered card id (the 8-char base32 the store mints).
var idRe = regexp.MustCompile(`/cards/([a-z2-7]{8})"`)

// between returns the text between the first occurrence of pre and the next end.
func between(s, pre, end string) string {
	i := strings.Index(s, pre)
	if i < 0 {
		return ""
	}
	rest := s[i+len(pre):]
	j := strings.Index(rest, end)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

func TestConcurrentMutationsAllSurvive(t *testing.T) {
	root := newRoot(t)
	h := newHandler(t, root, "life")
	b := reload(t, root, "life")
	var ids []string
	for _, l := range board.Lanes {
		for _, c := range b.Lanes[l] {
			ids = append(ids, c.ID)
		}
	}
	var wg sync.WaitGroup
	codes := make([]int, len(ids))
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = post(t, h, "/b/life/cards/"+id, url.Values{"tag": {"tagged"}}).Code
		}()
	}
	wg.Wait()
	after := reload(t, root, "life")
	for i, id := range ids {
		if codes[i] != http.StatusSeeOther {
			t.Errorf("POST %s: %d", id, codes[i])
			continue
		}
		if _, _, c := after.Find(id); c == nil || c.Tag != "tagged" {
			t.Errorf("edit to %s was lost by a concurrent write", id)
		}
	}
}

func TestChecklistFormRejectsAStaleIndex(t *testing.T) {
	root := newRoot(t)
	h := newHandler(t, root, "life")
	id := todoCard(t, root)
	card := "/b/life/cards/" + id
	// The page was rendered when item 2 read "Book appointment"; by the time
	// the form arrives an insert has shifted it.
	rec := post(t, h, card+"/checklist/2", url.Values{"text": {"Book it"}, "was": {"Something else"}})
	if rec.Code != http.StatusConflict {
		t.Errorf("stale index should be 409, got %d", rec.Code)
	}
	if rec := post(t, h, card+"/checklist/2/toggle", url.Values{"was": {"Something else"}}); rec.Code != http.StatusConflict {
		t.Errorf("stale toggle should be 409, got %d", rec.Code)
	}
	_, _, c := reload(t, root, "life").Find(id)
	if c.Checklist[2].Text != "Book appointment" || c.Checklist[2].Done {
		t.Errorf("a 409 must not have written: %+v", c.Checklist[2])
	}
	// The form the page actually renders carries the current text and works.
	_, body := get(t, h, card)
	if !strings.Contains(body, `<input type="hidden" name="was" value="Book appointment">`) {
		t.Errorf("checklist forms should carry the rendered text: %s", grepLine(body, "was"))
	}
	wantRedirect(t, post(t, h, card+"/checklist/2", url.Values{"text": {"Book it"}, "was": {"Book appointment"}}), card)
}

func TestRestoringACardAlreadyOnTheBoardIsRefused(t *testing.T) {
	root := archiveRoot(t)
	h := newHandler(t, root, "life")
	a, _ := store.LoadArchive(root, "life")
	id := a.Cards[0].ID
	// A duplicate: the same id is on the board and in the archive.
	b := reload(t, root, "life")
	b.Insert(board.Todo, 0, &board.Card{ID: id, Title: "already here"})
	st, _, _ := store.Open(root, "life")
	if err := st.SaveBoard(b); err != nil {
		t.Fatal(err)
	}
	if rec := post(t, h, "/b/life/archive/"+id+"/restore", nil); rec.Code != http.StatusConflict {
		t.Errorf("restoring a duplicate id should be 409, got %d", rec.Code)
	}
	if a2, _ := store.LoadArchive(root, "life"); len(a2.Cards) != 10 {
		t.Errorf("a refused restore must not touch the archive: %d", len(a2.Cards))
	}
}

func TestArchiveCapsAtFifty(t *testing.T) {
	root := newRoot(t)
	a := &board.Archive{}
	for i := 0; i < 60; i++ {
		a.Cards = append(a.Cards, &board.Card{
			ID: board.DeriveID(strconv.Itoa(i)), Title: "archived " + strconv.Itoa(i),
			DoneAt: fixedNow.AddDate(0, 0, -i),
		})
	}
	st, _, err := store.Open(root, "life")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveArchive(a); err != nil {
		t.Fatal(err)
	}
	_, body := get(t, newHandler(t, root, "life"), "/b/life/archive")
	if n := strings.Count(body, "restore</button>"); n != board.ArchiveMax {
		t.Errorf("rendered %d rows, want %d", n, board.ArchiveMax)
	}
	if !strings.Contains(body, "showing the 50 most recent entries") || !strings.Contains(body, "archive.md") {
		t.Errorf("capped footer: %s", grepLine(body, "showing"))
	}
	if strings.Contains(body, "archived 55") {
		t.Errorf("cards past the cap should not render")
	}
}

func TestFailedSaveIsNotReportedAsSuccess(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	root := newRoot(t)
	var logs strings.Builder
	h := New(Options{Root: root, Board: "life", Port: testPort, ErrorLog: &logs, Now: func() time.Time { return fixedNow }})
	id := todoCard(t, root)
	dir := filepath.Join(root, "life")
	before, _ := os.ReadFile(filepath.Join(dir, "board.md"))
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755)
	rec := post(t, h, "/b/life/cards/"+id, url.Values{"tag": {"nope"}})
	if rec.Code != http.StatusInternalServerError || rec.Header().Get("Location") != "" {
		t.Errorf("a failed save must be a 500 with no redirect: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if !strings.Contains(logs.String(), "save life") {
		t.Errorf("the failure should reach the error log (OPS-4): %q", logs.String())
	}
	if strings.Contains(rec.Body.String(), root) {
		t.Errorf("the response leaks a path: %q", rec.Body.String())
	}
	os.Chmod(dir, 0o755)
	if after, _ := os.ReadFile(filepath.Join(dir, "board.md")); string(after) != string(before) {
		t.Errorf("board.md changed despite the failure")
	}
}

func TestNotesCannotBreakTheBoardFile(t *testing.T) {
	root := newRoot(t)
	h := newHandler(t, root, "life")
	id := todoCard(t, root)
	notes := "## Nope\n### Ghost\n- [ ] not an item\nplain line"
	wantRedirect(t, post(t, h, "/b/life/cards/"+id, url.Values{"notes": {notes}}), "/b/life/cards/"+id)
	b := reload(t, root, "life") // parses, or the structural lines took over
	if b.Count() != 11 {
		t.Fatalf("card count changed: %d", b.Count())
	}
	_, _, c := b.Find(id)
	if c == nil || c.Notes != notes {
		t.Errorf("notes round trip:\n got %q\nwant %q", c.Notes, notes)
	}
	if len(c.Checklist) != 4 {
		t.Errorf("a note line became a checklist item: %+v", c.Checklist)
	}
	if rec, _ := get(t, h, "/b/life"); rec.Code != 200 {
		t.Errorf("board page after structural notes: %d", rec.Code)
	}
}

// laneIDs is the ids of one lane as it is on disk, in order — the readable
// form for a drop-position assertion.
func laneIDs(t *testing.T, root, name string, l board.Lane) string {
	t.Helper()
	var ids []string
	for _, c := range reload(t, root, name).Lanes[l] {
		ids = append(ids, c.ID)
	}
	return strings.Join(ids, " ")
}

// A drop names the card it landed against, so the position survives a
// filtered page and a board that changed underneath, and returns to the
// board it was dragged on rather than to the card's own page.
func TestMoveCardToAPosition(t *testing.T) {
	root := newRoot(t)
	h := newHandler(t, root, "life")
	if got := laneIDs(t, root, "life", board.Doing); got != "u4v5w6x7 y2z3a4b5" {
		t.Fatalf("fixture changed: doing=%q", got)
	}
	id := todoCard(t, root) // k7q2m9ab, the first Todo card
	wantRedirect(t, post(t, h, "/b/life/cards/"+id+"/move",
		url.Values{"lane": {"doing"}, "pos": {"after"}, "anchor": {"u4v5w6x7"}}), "/b/life")
	if got := laneIDs(t, root, "life", board.Doing); got != "u4v5w6x7 "+id+" y2z3a4b5" {
		t.Errorf("after u4v5w6x7: doing=%q", got)
	}

	next := reload(t, root, "life").Lanes[board.Todo][0].ID // m2n3o4p5
	wantRedirect(t, post(t, h, "/b/life/cards/"+next+"/move",
		url.Values{"lane": {"doing"}, "pos": {"before"}, "anchor": {"u4v5w6x7"}}), "/b/life")
	if got := laneIDs(t, root, "life", board.Doing); got != next+" u4v5w6x7 "+id+" y2z3a4b5" {
		t.Errorf("before u4v5w6x7: doing=%q", got)
	}

	// A lane rendering no cards has one position, and it is the top — the
	// same place `+ add` and a plain lane move put a card.
	last := reload(t, root, "life").Lanes[board.Todo][0].ID
	wantRedirect(t, post(t, h, "/b/life/cards/"+last+"/move",
		url.Values{"lane": {"backlog"}, "pos": {"start"}}), "/b/life")
	if got := laneIDs(t, root, "life", board.Backlog); got != last+" a2b3c4d5 e6f7g2h3 i4j5k6l7" {
		t.Errorf("start of a lane: backlog=%q", got)
	}
}

// The board page a card was dragged on may be filtered, so the redirect has
// to carry the filter back or the drag silently clears the view.
func TestMoveToAPositionKeepsTheFilter(t *testing.T) {
	root := newRoot(t)
	h := newHandler(t, root, "life")
	id := todoCard(t, root)
	wantRedirect(t, post(t, h, "/b/life/cards/"+id+"/move",
		url.Values{"lane": {"doing"}, "pos": {"start"}, "q": {"#home"}}), "/b/life?q=%23home")
}

// Reordering inside a lane is not a lane change, so the dates must survive
// it: MovedAt drives nothing visible, but DoneAt drives the age both
// surfaces show and the week the archive groups a card under.
func TestMoveCardWithinALaneReordersWithoutRestamping(t *testing.T) {
	root := newRoot(t)
	h := newHandler(t, root, "life")
	first := todoCard(t, root)
	last := "q6r7s2t3"
	_, _, was := reload(t, root, "life").Find(last)
	moved := was.MovedAt

	wantRedirect(t, post(t, h, "/b/life/cards/"+last+"/move",
		url.Values{"lane": {"todo"}, "pos": {"before"}, "anchor": {first}}), "/b/life")
	if got := laneIDs(t, root, "life", board.Todo); got != last+" "+first+" m2n3o4p5" {
		t.Errorf("reorder to the top: todo=%q", got)
	}
	_, _, now := reload(t, root, "life").Find(last)
	if !now.MovedAt.Equal(moved) {
		t.Errorf("a reorder must not restamp MovedAt: %v → %v", moved, now.MovedAt)
	}

	done := reload(t, root, "life").Lanes[board.Done][0]
	wantRedirect(t, post(t, h, "/b/life/cards/"+done.ID+"/move",
		url.Values{"lane": {"done"}, "pos": {"after"}, "anchor": {"k2l3m4n5"}}), "/b/life")
	after := reload(t, root, "life")
	if got := laneIDs(t, root, "life", board.Done); got != "g4h5i6j7 k2l3m4n5 "+done.ID {
		t.Errorf("reorder inside Done: done=%q", got)
	}
	if _, _, c := after.Find(done.ID); !c.DoneAt.Equal(done.DoneAt) || !c.MovedAt.Equal(done.MovedAt) {
		t.Errorf("a reorder must not restamp: moved %v → %v, done %v → %v", done.MovedAt, c.MovedAt, done.DoneAt, c.DoneAt)
	}
}

// The guard on the dispatch: a move with no position is the lane picker's,
// which has always been a top-insert and a same-lane no-op. Routing it
// through MoveAt instead would turn a stray click on the picker — it
// pre-selects the lane the card is already in — into a jump to the top,
// restamping the dates on the way.
func TestMoveWithoutAPositionIsUnchanged(t *testing.T) {
	root := newRoot(t)
	h := newHandler(t, root, "life")
	mid := reload(t, root, "life").Lanes[board.Todo][1] // m2n3o4p5
	before := laneIDs(t, root, "life", board.Todo)

	wantRedirect(t, post(t, h, "/b/life/cards/"+mid.ID+"/move",
		url.Values{"lane": {"todo"}}), "/b/life/cards/"+mid.ID)
	if got := laneIDs(t, root, "life", board.Todo); got != before {
		t.Errorf("a same-lane move with no position must not reorder: %q → %q", before, got)
	}
	_, _, now := reload(t, root, "life").Find(mid.ID)
	if !now.MovedAt.Equal(mid.MovedAt) {
		t.Errorf("nor restamp: %v → %v", mid.MovedAt, now.MovedAt)
	}
}

// The anchor is a card that was on the page when it rendered. If it has left
// the destination lane the page is stale, and the write is refused rather
// than landing the card somewhere it was not dropped.
func TestMoveRejectsABadOrStalePosition(t *testing.T) {
	root := newRoot(t)
	h := newHandler(t, root, "life")
	id := todoCard(t, root)
	before, err := os.ReadFile(filepath.Join(root, "life", "board.md"))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		form   url.Values
		status int
	}{
		{"an anchor in another lane", url.Values{"lane": {"todo"}, "pos": {"before"}, "anchor": {"u4v5w6x7"}}, http.StatusConflict},
		{"an anchor that is gone", url.Values{"lane": {"todo"}, "pos": {"after"}, "anchor": {"zzzzzzzz"}}, http.StatusConflict},
		{"a position with no anchor", url.Values{"lane": {"todo"}, "pos": {"before"}}, http.StatusBadRequest},
		{"an empty anchor", url.Values{"lane": {"todo"}, "pos": {"after"}, "anchor": {""}}, http.StatusBadRequest},
		{"a position nobody defines", url.Values{"lane": {"todo"}, "pos": {"sideways"}}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		if rec := post(t, h, "/b/life/cards/"+id+"/move", tc.form); rec.Code != tc.status {
			t.Errorf("%s: status = %d, want %d", tc.name, rec.Code, tc.status)
		}
	}
	after, err := os.ReadFile(filepath.Join(root, "life", "board.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("a refused move must not write to board.md")
	}
}

// Dropping a card on its own edge is a real gesture, not an error: both
// halves of its own box name a position it already holds.
func TestDroppingACardOnItselfChangesNothing(t *testing.T) {
	root := newRoot(t)
	h := newHandler(t, root, "life")
	id := todoCard(t, root)
	before, err := os.ReadFile(filepath.Join(root, "life", "board.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, pos := range []string{"before", "after"} {
		wantRedirect(t, post(t, h, "/b/life/cards/"+id+"/move",
			url.Values{"lane": {"todo"}, "pos": {pos}, "anchor": {id}}), "/b/life")
		after, err := os.ReadFile(filepath.Join(root, "life", "board.md"))
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != string(before) {
			t.Errorf("pos=%s on itself rewrote the board", pos)
		}
	}
}

// data-id is the first place a card id reaches a plain HTML attribute —
// everywhere else it goes through urlpath. A hand-edited board.md assigns
// ids verbatim (internal/store/markdown.go), so one can carry quotes.
func TestHandWrittenCardIdIsSafeInADragAttribute(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "hand"), 0o755); err != nil {
		t.Fatal(err)
	}
	md := "## Todo\n\n### Ordinary\nid: start\n\n### Sneaky\nid: x\" onmouseover=\"alert(1)\n"
	if err := os.WriteFile(filepath.Join(root, "hand", "board.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	h := newHandler(t, root, "hand")
	rec, body := get(t, h, "/b/hand")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /b/hand: %d", rec.Code)
	}
	if strings.Contains(body, `onmouseover="alert(1)"`) {
		t.Errorf("a hand-written id must not escape its attribute: %s", grepLine(body, "Sneaky"))
	}
	if !strings.Contains(body, `data-id="x&#34; onmouseover=&#34;alert(1)"`) {
		t.Errorf("the id should be there, escaped: %s", grepLine(body, "Sneaky"))
	}
	// An id that spells a position word is still just an id: pos and anchor
	// are separate fields, so nothing about it is reserved.
	wantRedirect(t, post(t, h, "/b/hand/cards/start/move",
		url.Values{"lane": {"doing"}, "pos": {"start"}}), "/b/hand")
	if got := laneIDs(t, root, "hand", board.Doing); got != "start" {
		t.Errorf("a card named \"start\": doing=%q", got)
	}
}
