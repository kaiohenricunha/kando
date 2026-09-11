package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A board card carries title, age, tag, checklist and blocked state; the notes
// stay on the card's own page.
func TestBoardCardsLeaveNotesToTheCardPage(t *testing.T) {
	h := newHandler(t, newRoot(t), "life")
	_, body := get(t, h, "/b/life")
	if strings.Contains(body, "Expires 14 Nov.") {
		t.Errorf("the board should not show a notes preview: %s", grepLine(body, "Expires"))
	}
	if !strings.Contains(body, `class="meter"`) {
		t.Errorf("a card with a checklist should draw its progress meter")
	}
	_, card := get(t, h, "/b/life/cards/k7q2m9ab")
	if !strings.Contains(card, "Expires 14 Nov.") {
		t.Errorf("the card page should still show the notes")
	}
}

// A narrow screen shows one lane at a time: a tab per lane naming the section
// it scrolls to, and an add button that lanes.js points at the lane in view.
func TestBoardPageHasLaneTabsAndAnAddDock(t *testing.T) {
	_, body := get(t, newHandler(t, newRoot(t), "life"), "/b/life")
	for _, want := range []string{`href="#lane-todo" data-tab="todo"`, `id="lane-todo"`, `data-label="Todo"`,
		`data-dock-add href="/b/life/cards/new?lane=todo"`, `<script src="/static/lanes.js"></script>`} {
		if !strings.Contains(body, want) {
			t.Errorf("board page missing %q", want)
		}
	}
}

// The card page mirrors the TUI detail screen: its lane's cards down the left
// with this one current, and d's move to Done beside the lane picker.
func TestCardPageListsItsLaneAndOffersDone(t *testing.T) {
	h := newHandler(t, newRoot(t), "life")
	_, body := get(t, h, "/b/life/cards/k7q2m9ab")
	for _, want := range []string{`href="/b/life/cards/k7q2m9ab" aria-current="page"`, "Book dentist",
		`<input type="hidden" name="lane" value="done">`, `href="/b/life#lane-todo"`, "data-card-page",
		`<script src="/static/edit.js"></script>`} {
		if !strings.Contains(body, want) {
			t.Errorf("card page missing %q", want)
		}
	}
	_, done := get(t, h, "/b/life/cards/c6d7e2f3")
	if strings.Contains(done, `name="lane" value="done"`) {
		t.Errorf("a Done card should not offer a move to Done")
	}
}

func TestBoardsListSummarisesEachBoard(t *testing.T) {
	_, body := get(t, newHandler(t, newRoot(t), "life"), "/boards")
	if !strings.Contains(body, "11 cards · 2 doing · 1 blocked") {
		t.Errorf("the boards list should count life's cards: %s", grepLine(body, "cards"))
	}
}

// A board that cannot be read keeps its row, without counts, and does not take
// the list of every other board down with it.
func TestBoardsListSurvivesAnUnreadableBoard(t *testing.T) {
	root := newRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "broken", "board.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	rec, body := get(t, newHandler(t, root, "life"), "/boards")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(body, `href="/b/broken"`) || !strings.Contains(body, "cannot read this board") {
		t.Errorf("the unreadable board should keep its row: %s", grepLine(body, "broken"))
	}
	if !strings.Contains(body, "11 cards") {
		t.Errorf("the other boards should keep their counts")
	}
}

// Both new scripts are pinned routes like live.js and dnd.js, not a widened
// file server.
func TestRedesignScriptsAreServedFromThisOrigin(t *testing.T) {
	h := newHandler(t, newRoot(t), "life")
	for path, marker := range map[string]string{"/static/lanes.js": "scrollLeft", "/static/edit.js": "data-autosave"} {
		rec, js := get(t, h, path)
		if rec.Code != http.StatusOK || !strings.Contains(js, marker) {
			t.Errorf("%s: %d", path, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
			t.Errorf("%s content type: %q", path, ct)
		}
	}
	for _, path := range []string{"/static/", "/static/../static/edit.js", "/static/edit.js/../lanes.js"} {
		if rec, _ := get(t, h, path); rec.Code == http.StatusOK {
			t.Errorf("GET %s should not be served: %d", path, rec.Code)
		}
	}
}
