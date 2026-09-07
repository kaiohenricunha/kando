package web

import (
	"net/http"
	"strings"

	"github.com/kaiohenricunha/kando/internal/board"
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

type boardPage struct {
	Page
	Lanes          []laneView
	Query          string
	Matched, Total int
}

type cardPage struct {
	Page
	Card           cardView
	URL            string // the card's own route; forms post to URL/<action>
	Lane, LaneKey  string
	Lanes          [4]board.Lane
	Created, Since string
	Notes, Reason  string
	Checklist      []board.Item
}

type newPage struct {
	Page
	Lane  string
	Lanes [4]board.Lane
}

func (s *server) basePage(name, title string) Page {
	// title is a card title on the detail page, so it is user-controlled and
	// reaches both the <title> element and the breadcrumb. Defanging it here
	// covers every page that passes one. The board name is not defanged: it
	// comes from a directory name that ValidBoardName already constrains.
	return Page{Board: name, Title: board.SafeForDisplay(title), Date: board.DayLabel(s.now())}
}

// cardView projects a card the way the TUI's card_view.go does: the same
// age, first notes line, progress and blocked labels, from the same helpers.
func (s *server) cardView(c *board.Card, lane board.Lane) cardView {
	// Every user-controlled field goes through SafeForDisplay. board.md is
	// parsed verbatim, so these values may never have met a write-time
	// sanitizer, and html/template escapes HTML metacharacters only — it does
	// nothing about a bidi override, which would reorder what the page shows.
	return cardView{
		ID: c.ID, Title: board.SafeForDisplay(c.Title),
		Notes: board.SafeForDisplay(c.FirstNoteLine()), Tag: board.SafeForDisplay(c.Tag),
		Age:      board.Age(s.now(), c.AgeSince()),
		Progress: c.ProgressLabel(),
		Blocked:  board.SafeForDisplay(c.BlockedLabel()),
		Done:     lane == board.Done,
	}
}

// home redirects to the CLI-given board, or to the boards list.
func (s *server) home(w http.ResponseWriter, r *http.Request) {
	if s.def != "" {
		http.Redirect(w, r, boardURL(s.def), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/boards", http.StatusSeeOther)
}

// board renders all four lanes, filtered by ?q= with the TUI's own parser.
func (s *server) board(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("board")
	b, err := s.load(name)
	if err != nil {
		s.fail(w, err)
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
		s.fail(w, err)
		return
	}
	lane, _, c := b.Find(r.PathValue("id"))
	if c == nil {
		s.fail(w, &httpError{http.StatusNotFound, "no such card"})
		return
	}
	p := cardPage{
		Page: s.basePage(name, c.Title), Card: s.cardView(c, lane), URL: cardURL(name, c.ID),
		Lane: lane.String(), LaneKey: lane.Key(), Lanes: board.Lanes,
		Notes: board.SafeForDisplay(c.Notes), Reason: board.SafeForDisplay(c.BlockedReason),
		Checklist: safeChecklist(c.Checklist),
	}
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
	s.render(w, "card.html", p)
}

// newCard renders the form that POST /b/{board}/cards accepts (U6).
func (s *server) newCard(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("board")
	if _, err := s.load(name); err != nil {
		s.fail(w, err)
		return
	}
	lane := strings.ToLower(r.URL.Query().Get("lane"))
	if _, ok := board.ParseLane(lane); !ok {
		lane = board.Todo.Key()
	}
	s.render(w, "new.html", newPage{Page: s.basePage(name, "new card"), Lane: lane, Lanes: board.Lanes})
}

// safeChecklist copies items with their text put through SafeForDisplay. The
// copy matters: these items are pointers into the loaded board, and mutating
// them here would change what a later save writes back to the file.
func safeChecklist(items []board.Item) []board.Item {
	if items == nil {
		return nil
	}
	out := make([]board.Item, len(items))
	for i, it := range items {
		it.Text = board.SafeForDisplay(it.Text)
		out[i] = it
	}
	return out
}
