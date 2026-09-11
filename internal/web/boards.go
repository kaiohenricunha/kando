package web

import (
	"net/http"
	"strings"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

type boardsPage struct {
	Page
	Boards []boardSummary
}

// boardSummary is one row of the boards list: the board's card count, how
// many are in Doing or blocked, and each lane's size for the spread bar.
type boardSummary struct {
	Name                  string
	Unreadable            bool
	Total, Doing, Blocked int
	Lanes                 [4]int
}

// boards lists every board with a form to create one.
func (s *server) boards(w http.ResponseWriter, r *http.Request) {
	names, err := store.ListBoards(s.root)
	if err != nil {
		s.logf("cannot list boards in %s: %v", s.root, err)
		s.fail(w, &httpError{http.StatusInternalServerError, "cannot list boards"})
		return
	}
	p := boardsPage{Page: s.basePage("", "boards")}
	for _, n := range names {
		p.Boards = append(p.Boards, s.summarize(n))
	}
	s.render(w, "boards.html", p)
}

// summarize counts one board for the list. It reads through store.Load, so
// listing boards never creates or rewrites a file, and a board that cannot be
// read costs its own row the counts rather than the page: the list is how
// every other board is reached.
func (s *server) summarize(name string) boardSummary {
	sum := boardSummary{Name: name}
	b, err := store.Load(s.root, name)
	if err != nil {
		s.logf("boards: %s: %v", name, err)
		sum.Unreadable = true
		return sum
	}
	for _, l := range board.Lanes {
		sum.Lanes[l] = len(b.Lanes[l])
		for _, c := range b.Lanes[l] {
			if c.Blocked {
				sum.Blocked++
			}
		}
	}
	sum.Total = b.Count()
	sum.Doing = sum.Lanes[board.Doing]
	return sum
}

// createBoard (U7) mirrors the TUI picker's `n`: a valid name becomes a
// directory with a canonical board.md, exactly as store.Open does for the
// CLI argument. An existing board is left as is and simply opened.
func (s *server) createBoard(w http.ResponseWriter, r *http.Request) {
	f, err := form(w, r)
	if err != nil {
		s.fail(w, err)
		return
	}
	name := strings.TrimSpace(f.Get("name"))
	if !store.ValidBoardName(name) {
		s.fail(w, &httpError{http.StatusBadRequest, `invalid board name: 1-64 characters, no / \ # ? % & + ; and no leading dot`})
		return
	}
	defer s.writeLock(name)()
	if _, _, err := store.Open(s.root, name); err != nil {
		s.fail(w, s.openFailure("create board", name, err, "cannot create board"))
		return
	}
	http.Redirect(w, r, boardURL(name), http.StatusSeeOther)
}
