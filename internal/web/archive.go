package web

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

// archiveMaxItems caps the list at the most recent entries, as the TUI's
// archive screen does (internal/tui/archive.go).
const archiveMaxItems = 50

type archiveItem struct {
	ID, Title, Tag, Date string
}

type archiveGroup struct {
	Label string
	Items []archiveItem
}

type archivePage struct {
	Page
	Groups         []archiveGroup
	Query          string
	Matched, Total int
	Capped         bool
}

// archive (U8) mirrors `D`: the 50 most recent archived cards grouped by
// week, filtered by ?q= with the same parser as the board.
func (s *server) archive(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("board")
	if _, err := s.load(name); err != nil {
		fail(w, err)
		return
	}
	a, err := store.LoadArchive(s.root, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kando web: archive %s: %v\n", name, err)
		fail(w, &httpError{http.StatusInternalServerError, "cannot read archive"})
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	f := board.Parse(q)
	now := s.now()
	cards := a.Cards
	if len(cards) > archiveMaxItems {
		cards = cards[:archiveMaxItems]
	}
	p := archivePage{Page: s.basePage(name, "archive"), Query: q, Total: len(a.Cards), Capped: len(a.Cards) > archiveMaxItems}
	var groups [3]archiveGroup
	for g := board.ThisWeek; g <= board.Earlier; g++ {
		groups[g].Label = g.Label()
	}
	for _, c := range cards {
		if !f.Empty() && !f.Match(c, now) {
			continue
		}
		g := board.GroupOf(now, c.DoneAt)
		item := archiveItem{ID: c.ID, Title: c.Title, Tag: c.Tag}
		if !c.DoneAt.IsZero() {
			item.Date = board.DayLabel(c.DoneAt)
		}
		groups[g].Items = append(groups[g].Items, item)
		p.Matched++
	}
	for _, g := range groups {
		if len(g.Items) > 0 {
			p.Groups = append(p.Groups, g)
		}
	}
	s.render(w, "archive.html", p)
}

// restoreCard mirrors `u` on the archive screen via board.Restore. The
// board is saved before the archive: if the second write fails the card
// exists in both files (a duplicate the user can see), never in neither.
func (s *server) restoreCard(w http.ResponseWriter, r *http.Request) {
	name, id := r.PathValue("board"), r.PathValue("id")
	st, b, err := s.openForWrite(name)
	if err != nil {
		fail(w, err)
		return
	}
	a, err := st.LoadArchive()
	if err != nil {
		fmt.Fprintf(os.Stderr, "kando web: archive %s: %v\n", name, err)
		fail(w, &httpError{http.StatusInternalServerError, "cannot read archive"})
		return
	}
	at := -1
	for i, c := range a.Cards {
		if c.ID == id {
			at = i
			break
		}
	}
	if at < 0 {
		fail(w, &httpError{http.StatusNotFound, "no such archived card"})
		return
	}
	b.Restore(a, at, s.now())
	if err := st.SaveBoard(b); err != nil {
		fmt.Fprintf(os.Stderr, "kando web: save %s: %v\n", name, err)
		http.Error(w, "cannot save board", http.StatusInternalServerError)
		return
	}
	if err := st.SaveArchive(a); err != nil {
		fmt.Fprintf(os.Stderr, "kando web: save archive %s: %v\n", name, err)
		http.Error(w, "cannot save archive", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, boardURL(name), http.StatusSeeOther)
}
