package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/kaiohenricunha/kando/internal/board"
	"github.com/kaiohenricunha/kando/internal/store"
)

type archiveItem struct {
	ID, Title, Tag, Date string
}

type archiveGroup struct {
	Label string
	Items []archiveItem
}

type archivePage struct {
	Page
	Groups                  []archiveGroup
	Query                   string
	Matched, Scanned, Total int
	Cap                     int
	Path                    string
}

// archive (U8) mirrors `D`: the newest board.ArchiveMax archived cards
// grouped by week, filtered by ?q= with the same parser as the board. The
// grouping, the cap and the filtering all come from board.ArchiveView, so
// this page and the TUI's archive screen cannot drift apart.
func (s *server) archive(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("board")
	// Only the archive is rendered here, so an unparsable board.md must not
	// take this page down with it: check the board exists, do not parse it.
	if err := s.mustExist(name); err != nil {
		s.fail(w, err)
		return
	}
	a, err := store.LoadArchive(s.root, name)
	if err != nil {
		s.logf("archive %s: %v", name, err)
		s.fail(w, &httpError{http.StatusInternalServerError, "cannot read archive"})
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	groups, matched, scanned, total := board.ArchiveView(a, board.Parse(q), s.now())
	p := archivePage{
		Page: s.basePage(name, "archive"), Query: q,
		Matched: matched, Scanned: scanned, Total: total,
		Cap: board.ArchiveMax, Path: store.ArchiveDisplayPath(s.root, name),
	}
	for _, g := range groups {
		gv := archiveGroup{Label: g.Label}
		for _, c := range g.Cards {
			item := archiveItem{ID: c.ID, Title: c.Title, Tag: c.Tag}
			if !c.DoneAt.IsZero() {
				item.Date = board.DayLabel(c.DoneAt)
			}
			gv.Items = append(gv.Items, item)
		}
		p.Groups = append(p.Groups, gv)
	}
	s.render(w, "archive.html", p)
}

// restoreCard mirrors `u` on the archive screen via board.Restore, and
// returns to the archive with the filter the user was reading, as the TUI
// stays on its archive screen. store.SaveRestore owns the write order.
func (s *server) restoreCard(w http.ResponseWriter, r *http.Request) {
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
	a, err := st.LoadArchive()
	if err != nil {
		s.logf("archive %s: %v", name, err)
		s.fail(w, &httpError{http.StatusInternalServerError, "cannot read archive"})
		return
	}
	at, _ := a.Find(id)
	if at < 0 {
		s.fail(w, &httpError{http.StatusNotFound, "no such archived card"})
		return
	}
	// The board already holding this id means a previous restore half
	// failed, or archive.md keeps a copy of a live card. Restoring anyway
	// would put two cards with one id on the board, and Board.Find only ever
	// reaches the first: the second would be uneditable from both surfaces.
	if _, _, c := b.Find(id); c != nil {
		s.fail(w, &httpError{http.StatusConflict, "that card is already on the board"})
		return
	}
	b.Restore(a, at, s.now())
	if err := st.SaveRestore(b, a); err != nil {
		s.logf("restore %s: %v", name, err)
		http.Error(w, "cannot save the restored card", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, withQuery(boardURL(name)+"/archive", f), http.StatusSeeOther)
}

// archiveCard mirrors `A` on a Done card via board.ArchiveDone, the reverse
// of restoreCard: the card leaves board.md for archive.md and the page
// returns to the board, as delete does — the card's own page no longer
// exists. store.SaveArchivalIfUnchanged owns the write order (archive.md
// first) and refuses a stale board or archive.
func (s *server) archiveCard(w http.ResponseWriter, r *http.Request) {
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
	lane, i, _, err := findCard(b, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	// The button only renders on a Done card, so this is a stale page — the
	// same class as a stale drag anchor or checklist witness.
	if lane != board.Done {
		s.fail(w, &httpError{http.StatusConflict, "only a Done card can be archived — reload and try again"})
		return
	}
	a, err := st.LoadArchive()
	if err != nil {
		s.logf("archive %s: %v", name, err)
		s.fail(w, &httpError{http.StatusInternalServerError, "cannot read archive"})
		return
	}
	// The archive already holding this id means a previous archive half
	// failed after archive.md was written: archiving again would file the
	// card twice. One copy has to be deleted first.
	if _, dup := a.Find(id); dup != nil {
		s.fail(w, &httpError{http.StatusConflict, "that card is already archived"})
		return
	}
	b.ArchiveDone(a, i, s.now())
	if err := st.SaveArchivalIfUnchanged(b, a); err != nil {
		if errors.Is(err, store.ErrConflict) {
			s.fail(w, &httpError{http.StatusConflict, "the board changed on disk — reload and try again"})
			return
		}
		s.logf("archive %s: %v", name, err)
		http.Error(w, "cannot save the archived card", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, withQuery(boardURL(name), f), http.StatusSeeOther)
}
