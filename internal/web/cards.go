package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

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
// exist is 404: only POST /boards creates one, never a card route. The
// caller must already hold the board's write lock.
func (s *server) openForWrite(name string) (*store.Store, *board.Board, error) {
	if err := s.mustExist(name); err != nil {
		return nil, nil, err
	}
	st, b, err := store.Open(s.root, name)
	if err != nil {
		s.logf("open %s: %v", name, err)
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

// commit saves the board atomically (REL-1) and redirects. A save refused
// because the file changed underneath is a 409, not a silent overwrite; any
// other failure is a 500 with the detail logged (OPS-4). Neither redirects,
// so no response ever claims success for an edit that is not on disk.
func (s *server) commit(w http.ResponseWriter, r *http.Request, st *store.Store, b *board.Board, to string) {
	err := st.SaveBoardIfUnchanged(b)
	if errors.Is(err, store.ErrConflict) {
		http.Error(w, "the board changed on disk — reload and try again", http.StatusConflict)
		return
	}
	if err != nil {
		s.logf("save %s: %v", b.Name, err)
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
func (s *server) withCard(to func(name, id string, f url.Values) string, op cardOp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, id := r.PathValue("board"), r.PathValue("id")
		f, err := form(w, r)
		if err != nil {
			s.fail(w, err)
			return
		}
		defer s.writeLock(name)()
		st, b, err := s.openForWrite(name)
		if err != nil {
			s.fail(w, err)
			return
		}
		lane, i, c, err := findCard(b, id)
		if err != nil {
			s.fail(w, err)
			return
		}
		if err := op(b, lane, i, c, f); err != nil {
			s.fail(w, err)
			return
		}
		s.commit(w, r, st, b, to(name, id, f))
	}
}

func toCard(name, id string, _ url.Values) string { return cardURL(name, id) }

func toBoard(name, _ string, _ url.Values) string { return boardURL(name) }

// withQuery puts back the ?q= the page carried, so a redirect lands the user
// on the view they acted from rather than an unfiltered one.
func withQuery(base string, f url.Values) string {
	if q := strings.TrimSpace(f.Get("q")); q != "" {
		return base + "?q=" + url.QueryEscape(q)
	}
	return base
}

// afterMove sends a move back where it came from: a positioned move is a drag
// on the board page, so it returns to the board with the filter intact; a
// move with no position came from the card detail page's lane picker, so it
// returns to the card.
func afterMove(name, id string, f url.Values) string {
	if f.Get("pos") == "" {
		return cardURL(name, id)
	}
	return withQuery(boardURL(name), f)
}

// createCard mirrors `a`: a new card at the top of the chosen lane.
func (s *server) createCard(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("board")
	f, err := form(w, r)
	if err != nil {
		s.fail(w, err)
		return
	}
	lane, ok := board.ParseLane(f.Get("lane"))
	if !ok {
		s.fail(w, &httpError{http.StatusBadRequest, "invalid lane"})
		return
	}
	defer s.writeLock(name)()
	st, b, err := s.openForWrite(name)
	if err != nil {
		s.fail(w, err)
		return
	}
	c := board.NewCard(f.Get("title"), lane, s.now())
	if c == nil {
		s.fail(w, &httpError{http.StatusBadRequest, "title required"})
		return
	}
	b.Insert(lane, 0, c)
	s.commit(w, r, st, b, boardURL(name))
}

// updateCard mirrors the detail screen's `T`, `t` and `e` edits (title, tag
// and notes). Only the fields present in the form change, so a form that
// omits notes leaves them alone.
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

// moveCard mirrors H/L, d and the m picker, plus the board page's drag: the
// card goes to the top of the target lane with the same date stamping the
// TUI applies, or to a named position in it when the form asks for one
// (BOUND-1b, the one thing here the TUI cannot do).
func (s *server) moveCard() http.HandlerFunc {
	return s.withCard(afterMove, func(b *board.Board, from board.Lane, i int, _ *board.Card, f url.Values) error {
		to, ok := board.ParseLane(f.Get("lane"))
		if !ok {
			return &httpError{http.StatusBadRequest, "invalid lane"}
		}
		at, positioned, err := movePos(b, to, f)
		if err != nil {
			return err
		}
		if !positioned {
			// No position asked for: the lane picker's move, unchanged —
			// including its same-lane no-op, which MoveAt would otherwise
			// turn into a jump to the top of the lane the card is in.
			b.Move(from, i, to, s.now())
			return nil
		}
		b.MoveAt(from, i, to, at, s.now())
		return nil
	})
}

// movePos resolves where in the destination lane a move lands. It names a
// position relative to a card the user was looking at, never an index:
//
//	(no pos)             not positioned — Board.Move, the top of the lane
//	pos=start            index 0
//	pos=before, anchor=X immediately above X
//	pos=after,  anchor=X immediately below X
//
// An index would be wrong twice over. The board page filters with ?q=
// (board.go), so the nth card on screen is not the nth card in the lane —
// which is also why there is no "append": the last card the user can see is
// not the last card in the lane, and "after the last one I can see" is the
// only reading of that gesture that is true on a filtered board. And the TUI
// or a text editor can move a card between the render and the drop, so an
// anchor that has left this lane means the page is stale: that is a 409,
// the same answer a stale checklist index gets, rather than a card landing
// somewhere it was not dropped.
//
// pos and anchor are separate fields so no id-shaped value is ever reserved;
// a hand-edited board.md may give a card any id at all, "start" included.
func movePos(b *board.Board, to board.Lane, f url.Values) (at int, positioned bool, err error) {
	switch pos := f.Get("pos"); pos {
	case "":
		return 0, false, nil
	case "start":
		return 0, true, nil
	case "before", "after":
		anchor := f.Get("anchor")
		if anchor == "" {
			return 0, false, &httpError{http.StatusBadRequest, "a position needs an anchor card"}
		}
		// A card dropped against itself resolves to a position it already
		// holds, which MoveAt treats as the no-op it is.
		for i, c := range b.Lanes[to] {
			if c.ID == anchor {
				if pos == "after" {
					return i + 1, true, nil
				}
				return i, true, nil
			}
		}
		return 0, false, &httpError{http.StatusConflict, "that card moved — reload and try again"}
	default:
		return 0, false, &httpError{http.StatusBadRequest, "invalid position"}
	}
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

// checklistIndex resolves {index} against the card's checklist. The index is
// a position, and positions move: the TUI can insert an item between the
// moment this page was rendered and the moment its form arrives, which would
// silently retarget the write to a different line. So the form also carries
// the text it was rendered with ("was"), and a mismatch is a 409 rather than
// an edit of the wrong item.
func checklistIndex(r *http.Request, c *board.Card, f url.Values) (int, error) {
	i, err := strconv.Atoi(r.PathValue("index"))
	if err != nil || i < 0 || i >= len(c.Checklist) {
		return 0, &httpError{http.StatusNotFound, "no such checklist item"}
	}
	if was, ok := f["was"]; ok && was[0] != checklistWitness(c.Checklist[i].Text) {
		return 0, &httpError{http.StatusConflict, "this card changed — reload and try again"}
	}
	return i, nil
}

// toggleChecklistItem mirrors `x` on an item.
func (s *server) toggleChecklistItem(w http.ResponseWriter, r *http.Request) {
	s.withCard(toCard, func(_ *board.Board, _ board.Lane, _ int, c *board.Card, f url.Values) error {
		i, err := checklistIndex(r, c, f)
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
		i, err := checklistIndex(r, c, f)
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
