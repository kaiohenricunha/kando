package web

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/kaiohenricunha/kando/internal/store"
)

type boardsPage struct {
	Page
	Boards []string
}

// boards lists every board with a form to create one.
func (s *server) boards(w http.ResponseWriter, r *http.Request) {
	names, err := store.ListBoards(s.root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kando web: cannot list boards in %s: %v\n", s.root, err)
		fail(w, &httpError{http.StatusInternalServerError, "cannot list boards"})
		return
	}
	s.render(w, "boards.html", boardsPage{Page: s.basePage("", "boards"), Boards: names})
}

// createBoard (U7) mirrors the TUI picker's `n`: a valid name becomes a
// directory with a canonical board.md, exactly as store.Open does for the
// CLI argument. An existing board is left as is and simply opened.
func (s *server) createBoard(w http.ResponseWriter, r *http.Request) {
	f, err := form(w, r)
	if err != nil {
		fail(w, err)
		return
	}
	name := strings.TrimSpace(f.Get("name"))
	if !store.ValidBoardName(name) {
		fail(w, &httpError{http.StatusBadRequest, `invalid board name: 1-64 characters, no / \ # ? % & + ; and no leading dot`})
		return
	}
	if _, _, err := store.Open(s.root, name); err != nil {
		fmt.Fprintf(os.Stderr, "kando web: create board %s: %v\n", name, err)
		fail(w, &httpError{http.StatusInternalServerError, "cannot create board"})
		return
	}
	http.Redirect(w, r, boardURL(name), http.StatusSeeOther)
}
