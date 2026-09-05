package web

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

// Page is the chrome every page shares: the board it belongs to, a title,
// and the date shown top right. Board names and card ids go into URLs only
// through the urlpath template func.
type Page struct {
	Board string
	Title string
	Date  string
}

type cardView struct {
	ID, Title, Notes, Tag, Age, Progress, Blocked string
	Done                                          bool
}

type laneView struct {
	Key, Name string
	Count     int
	Cards     []cardView
}

type boardsPage struct {
	Page
	Boards []string
}

type boardPage struct {
	Page
	Lanes          []laneView
	Query          string
	Matched, Total int
}

type cardPage struct {
	Page
	Card           cardView
	Lane, LaneKey  string
	Created, Since string
	Notes          []string
	Checklist      []board.Item
}

type newPage struct {
	Page
	Lane  string
	Lanes [4]board.Lane
}

func (s *server) basePage(name, title string) Page {
	return Page{Board: name, Title: title, Date: board.DayLabel(s.now())}
}

// cardView projects a card the way the TUI's card_view.go does: the same
// age, first notes line, progress and blocked labels, from the same helpers.
func (s *server) cardView(c *board.Card, lane board.Lane) cardView {
	return cardView{
		ID: c.ID, Title: c.Title, Notes: c.FirstNoteLine(), Tag: c.Tag,
		Age:      board.Age(s.now(), c.AgeSince()),
		Progress: c.ProgressLabel(),
		Blocked:  c.BlockedLabel(),
		Done:     lane == board.Done,
	}
}

// home redirects to the CLI-given board, or to the boards list.
func (s *server) home(w http.ResponseWriter, r *http.Request) {
	if s.def != "" {
		http.Redirect(w, r, "/b/"+url.PathEscape(s.def), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/boards", http.StatusSeeOther)
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

// board renders all four lanes, filtered by ?q= with the TUI's own parser.
func (s *server) board(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("board")
	b, err := s.load(name)
	if err != nil {
		fail(w, err)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	f := board.Parse(q)
	now := s.now()
	p := boardPage{Page: s.basePage(name, name), Query: q, Total: b.Count()}
	for _, l := range board.Lanes {
		lv := laneView{Key: l.Key(), Name: strings.ToUpper(l.String())}
		for _, c := range b.Lanes[l] {
			if f.Empty() || f.Match(c, now) {
				lv.Cards = append(lv.Cards, s.cardView(c, l))
			}
		}
		lv.Count = len(lv.Cards)
		p.Matched += lv.Count
		p.Lanes = append(p.Lanes, lv)
	}
	s.render(w, "board.html", p)
}

// card renders one card in full.
func (s *server) card(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("board")
	b, err := s.load(name)
	if err != nil {
		fail(w, err)
		return
	}
	lane, _, c := b.Find(r.PathValue("id"))
	if c == nil {
		fail(w, &httpError{http.StatusNotFound, "no such card"})
		return
	}
	p := cardPage{Page: s.basePage(name, c.Title), Card: s.cardView(c, lane), Lane: lane.String(), LaneKey: lane.Key(), Checklist: c.Checklist}
	if !c.CreatedAt.IsZero() {
		p.Created = board.DayLabel(c.CreatedAt)
	}
	since := c.MovedAt
	if since.IsZero() {
		since = c.CreatedAt
	}
	if !since.IsZero() {
		p.Since = board.DayLabel(since)
	}
	if strings.TrimSpace(c.Notes) != "" {
		p.Notes = strings.Split(c.Notes, "\n")
	}
	s.render(w, "card.html", p)
}

// newCard renders the form that POST /b/{board}/cards accepts (U6).
func (s *server) newCard(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("board")
	if _, err := s.load(name); err != nil {
		fail(w, err)
		return
	}
	lane := strings.ToLower(r.URL.Query().Get("lane"))
	if _, ok := board.ParseLane(lane); !ok {
		lane = board.Todo.Key()
	}
	s.render(w, "new.html", newPage{Page: s.basePage(name, "new card"), Lane: lane, Lanes: board.Lanes})
}
