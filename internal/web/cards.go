package web

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

// Card mutations (U6). Every route is a plain form POST that mirrors one TUI
// key (§5 of docs/specs/kando-web), applies the same internal/board helper
// the TUI calls, saves, and redirects to a GET (post/redirect/get) so a
// refresh never resubmits. Nothing here decides what an edit means — that
// is internal/board/ops.go's job (BOUND-1).

// maxFormBytes caps a POST body: a form is user input like any other field.
const maxFormBytes = 1 << 20

// form parses the POST body. A body that does not parse, or exceeds the cap,
// is a 400.
func form(w http.ResponseWriter, r *http.Request) (url.Values, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		return nil, &httpError{http.StatusBadRequest, "bad form"}
	}
	return r.PostForm, nil
}

func boardURL(name string) string { return "/b/" + url.PathEscape(name) }

func cardURL(name, id string) string { return boardURL(name) + "/cards/" + url.PathEscape(id) }

// openForWrite opens an existing board for a mutation. A board that does not
// exist is 404: only POST /boards creates one, never a card route.
func (s *server) openForWrite(name string) (*store.Store, *board.Board, error) {
	if !store.ValidBoardName(name) {
		return nil, nil, &httpError{http.StatusBadRequest, "invalid board name"}
	}
	if !store.Exists(s.root, name) {
		return nil, nil, &httpError{http.StatusNotFound, "no such board"}
	}
	st, b, err := store.Open(s.root, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kando web: open %s: %v\n", name, err)
		return nil, nil, &httpError{http.StatusInternalServerError, "cannot open board"}
	}
	return st, b, nil
}

// findCard resolves {id} on b, or 404.
func findCard(b *board.Board, id string) (board.Lane, int, *board.Card, error) {
	lane, i, c := b.Find(id)
	if c == nil {
		return 0, 0, nil, &httpError{http.StatusNotFound, "no such card"}
	}
	return lane, i, c, nil
}

// commit saves the board atomically (REL-1) and redirects. A failed save is
// a 500 with the detail on stderr (OPS-4), never a success page.
func (s *server) commit(w http.ResponseWriter, r *http.Request, st *store.Store, b *board.Board, to string) {
	if err := st.SaveBoard(b); err != nil {
		fmt.Fprintf(os.Stderr, "kando web: save %s: %v\n", b.Name, err)
		http.Error(w, "cannot save board", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// cardOp is one mutation on an existing card: it edits c (or b, for moves
// and deletes) from the form values and returns the error to answer with.
type cardOp func(b *board.Board, lane board.Lane, i int, c *board.Card, f url.Values) error

// withCard is the shape every card route shares: parse the form, open the
// board for writing, find the card, apply op, save, redirect to `to`.
func (s *server) withCard(to func(name, id string) string, op cardOp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, id := r.PathValue("board"), r.PathValue("id")
		f, err := form(w, r)
		if err != nil {
			fail(w, err)
			return
		}
		st, b, err := s.openForWrite(name)
		if err != nil {
			fail(w, err)
			return
		}
		lane, i, c, err := findCard(b, id)
		if err != nil {
			fail(w, err)
			return
		}
		if err := op(b, lane, i, c, f); err != nil {
			fail(w, err)
			return
		}
		s.commit(w, r, st, b, to(name, id))
	}
}

func toCard(name, id string) string { return cardURL(name, id) }

func toBoard(name, _ string) string { return boardURL(name) }

// createCard mirrors `a`: a new card at the top of the chosen lane.
func (s *server) createCard(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("board")
	f, err := form(w, r)
	if err != nil {
		fail(w, err)
		return
	}
	lane, ok := board.ParseLane(f.Get("lane"))
	if !ok {
		fail(w, &httpError{http.StatusBadRequest, "invalid lane"})
		return
	}
	st, b, err := s.openForWrite(name)
	if err != nil {
		fail(w, err)
		return
	}
	c := board.NewCard(f.Get("title"), lane, s.now())
	if c == nil {
		fail(w, &httpError{http.StatusBadRequest, "title required"})
		return
	}
	b.Insert(lane, 0, c)
	s.commit(w, r, st, b, boardURL(name))
}

// updateCard mirrors the detail screen's `t` and `e` edits. Only the fields
// present in the form change, so a form that omits notes leaves them alone.
func (s *server) updateCard() http.HandlerFunc {
	return s.withCard(toCard, func(_ *board.Board, _ board.Lane, _ int, c *board.Card, f url.Values) error {
		if v, ok := f["title"]; ok && !c.SetTitle(v[0]) {
			return &httpError{http.StatusBadRequest, "title required"}
		}
		if v, ok := f["tag"]; ok {
			c.SetTag(v[0])
		}
		if v, ok := f["notes"]; ok {
			c.SetNotes(v[0])
		}
		return nil
	})
}

// moveCard mirrors H/L, d and the m picker: the card goes to the top of the
// target lane with the same date stamping the TUI applies.
func (s *server) moveCard() http.HandlerFunc {
	return s.withCard(toCard, func(b *board.Board, from board.Lane, i int, _ *board.Card, f url.Values) error {
		to, ok := board.ParseLane(f.Get("lane"))
		if !ok {
			return &httpError{http.StatusBadRequest, "invalid lane"}
		}
		b.Move(from, i, to, s.now())
		return nil
	})
}

// blockCard mirrors `b`: a reason blocks, an empty reason clears.
func (s *server) blockCard() http.HandlerFunc {
	return s.withCard(toCard, func(_ *board.Board, _ board.Lane, _ int, c *board.Card, f url.Values) error {
		c.SetBlocked(f.Get("reason"))
		return nil
	})
}

// addChecklistItem mirrors `o`, appending after the last item.
func (s *server) addChecklistItem() http.HandlerFunc {
	return s.withCard(toCard, func(_ *board.Board, _ board.Lane, _ int, c *board.Card, f url.Values) error {
		if c.InsertChecklistItem(len(c.Checklist)-1, f.Get("text")) < 0 {
			return &httpError{http.StatusBadRequest, "text required"}
		}
		return nil
	})
}

// checklistIndex resolves {index} against the card's checklist, or 404.
func checklistIndex(r *http.Request, c *board.Card) (int, error) {
	i, err := strconv.Atoi(r.PathValue("index"))
	if err != nil || i < 0 || i >= len(c.Checklist) {
		return 0, &httpError{http.StatusNotFound, "no such checklist item"}
	}
	return i, nil
}

// toggleChecklistItem mirrors `x` on an item.
func (s *server) toggleChecklistItem(w http.ResponseWriter, r *http.Request) {
	s.withCard(toCard, func(_ *board.Board, _ board.Lane, _ int, c *board.Card, _ url.Values) error {
		i, err := checklistIndex(r, c)
		if err != nil {
			return err
		}
		c.ToggleChecklistItem(i)
		return nil
	})(w, r)
}

// editChecklistItem mirrors enter on an item.
func (s *server) editChecklistItem(w http.ResponseWriter, r *http.Request) {
	s.withCard(toCard, func(_ *board.Board, _ board.Lane, _ int, c *board.Card, f url.Values) error {
		i, err := checklistIndex(r, c)
		if err != nil {
			return err
		}
		if !c.SetChecklistItemText(i, f.Get("text")) {
			return &httpError{http.StatusBadRequest, "text required"}
		}
		return nil
	})(w, r)
}

// deleteCard mirrors `x` on the board: immediate, no confirmation (§2).
func (s *server) deleteCard() http.HandlerFunc {
	return s.withCard(toBoard, func(b *board.Board, lane board.Lane, i int, _ *board.Card, _ url.Values) error {
		b.DeleteCard(lane, i)
		return nil
	})
}
