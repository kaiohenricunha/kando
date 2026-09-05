package web

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

// laneKeys are the lane names as they appear in URLs and forms.
var laneKeys = [4]string{"backlog", "todo", "doing", "done"}

type Page struct {
	Board  string   // board name
	Boards []string // every board under root, for the switcher
	Date   string
	Title  string
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
	Done, Total    int
	Blocked        string
}

type newPage struct {
	Page
	Lane  string
	Lanes [4]string
}

func (s *server) basePage(name, title string) Page {
	boards, _ := store.ListBoards(s.root)
	return Page{Board: name, Boards: boards, Date: board.DayLabel(s.now()), Title: title}
}

func (s *server) cardView(c *board.Card, lane board.Lane) cardView {
	v := cardView{ID: c.ID, Title: c.Title, Notes: c.FirstNoteLine(), Tag: c.Tag, Age: board.Age(s.now(), c.AgeSince()), Done: lane == board.Done}
	if done, total := c.ChecklistProgress(); total > 0 {
		v.Progress = fmt.Sprintf("%d/%d", done, total)
	}
	if c.Blocked {
		v.Blocked = c.BlockedReason
		if v.Blocked == "" {
			v.Blocked = "blocked"
		}
	}
	return v
}

// home redirects to the CLI-given board, or to the boards list.
func (s *server) home(w http.ResponseWriter, r *http.Request) {
	if s.def != "" {
		http.Redirect(w, r, "/b/"+s.def, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/boards", http.StatusSeeOther)
}

// boards lists every board with a form to create one.
func (s *server) boards(w http.ResponseWriter, r *http.Request) {
	s.render(w, "boards.html", s.basePage("", "boards"))
}

// board renders all four lanes, filtered by ?q= with the TUI's own parser.
func (s *server) board(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("board")
	_, b, err := s.load(name)
	if err != nil {
		fail(w, err)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	f := board.Parse(q)
	now := s.now()
	p := boardPage{Page: s.basePage(name, name), Query: q, Total: b.Count()}
	for _, l := range board.Lanes {
		lv := laneView{Key: laneKeys[l], Name: strings.ToUpper(l.String())}
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
	_, b, err := s.load(name)
	if err != nil {
		fail(w, err)
		return
	}
	lane, _, c := b.Find(r.PathValue("id"))
	if c == nil {
		fail(w, &httpError{http.StatusNotFound, "no such card"})
		return
	}
	p := cardPage{Page: s.basePage(name, c.Title), Card: s.cardView(c, lane), Lane: lane.String(), LaneKey: laneKeys[lane]}
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
	p.Checklist = c.Checklist
	p.Done, p.Total = c.ChecklistProgress()
	p.Blocked = p.Card.Blocked
	s.render(w, "card.html", p)
}

// newCard renders the form that POST /b/{board}/cards accepts (U6).
func (s *server) newCard(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("board")
	if _, _, err := s.load(name); err != nil {
		fail(w, err)
		return
	}
	lane := strings.ToLower(r.URL.Query().Get("lane"))
	if _, ok := board.ParseLane(lane); !ok {
		lane = "todo"
	}
	s.render(w, "new.html", newPage{Page: s.basePage(name, "new card"), Lane: lane, Lanes: laneKeys})
}
