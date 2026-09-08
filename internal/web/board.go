package web

import (
	"crypto/sha256"
	"encoding/hex"
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
	Checklist      []checklistItemView
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
		Notes: board.SafeForDisplay(c.NotePreview()), Tag: board.SafeForDisplay(c.Tag),
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
	// De-fanged at entry, not in the view model: the same string is both
	// parsed into the filter and rendered back into the search box, so
	// guarding only the displayed copy would show results computed from one
	// query beside an input holding another.
	q := board.SafeForDisplay(strings.TrimSpace(r.URL.Query().Get("q")))
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

// checklistItemView splits what the page shows from what it submits back.
//
// Text is de-fanged for display. Was is a digest of the verbatim stored text:
// the witness the toggle and edit forms post so checklistIndex can tell a
// stale form from a current one.
//
// A digest rather than the text itself, for two reasons. The comparison is
// against what is on disk, so a de-fanged witness would never match for the
// hand-edited items this guard exists for — every toggle on such a card would
// answer 409 "reload and try again", which no reload could fix. Sending the
// raw text instead would fix the comparison but put the unsafe runes back
// into the page, inside a hidden attribute where nothing displays them but
// nothing strips them either. A digest is opaque, so neither problem arises,
// and it still detects an edit that changed only unsafe runes — which
// comparing de-fanged text would miss.
type checklistItemView struct {
	Text string
	Was  string
	Done bool
}

// checklistWitness fingerprints an item's stored text. Truncated because this
// detects a concurrent edit, it does not defend against one: a writer who can
// forge it can edit the file directly.
func checklistWitness(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:8])
}

// safeChecklist projects items for rendering: visible text de-fanged, witness
// digested. It builds a new slice rather than writing through items[i], which
// would edit the backing array of the loaded board and change what a later
// save writes to the file.
func safeChecklist(items []board.Item) []checklistItemView {
	out := make([]checklistItemView, len(items))
	for i, it := range items {
		out[i] = checklistItemView{
			Text: board.SafeForDisplay(it.Text),
			Was:  checklistWitness(it.Text),
			Done: it.Done,
		}
	}
	return out
}
