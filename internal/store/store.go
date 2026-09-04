package store

import (
	"crypto/sha256"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
)

// Store owns one board directory: <root>/<name>/{board.md,archive.md}.
type Store struct {
	dir     string
	name    string
	mu      sync.Mutex
	board   fileState
	archive fileState
}

// fileState remembers what we last read or wrote so external changes can be told
// apart from our own saves.
type fileState struct {
	valid bool
	mtime time.Time
	size  int64
	hash  [32]byte
}

// Reload carries whatever changed on disk since the last check (nil = unchanged).
type Reload struct {
	Board   *board.Board
	Archive *board.Archive
}

// Open loads (or creates) the board under root/name. Cards missing an id are
// assigned one and the file is rewritten once.
func Open(root, name string) (*Store, *board.Board, error) {
	s := &Store{dir: filepath.Join(root, name), name: name}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return nil, nil, err
	}
	data, err := os.ReadFile(s.BoardPath())
	if errors.Is(err, fs.ErrNotExist) {
		b := &board.Board{Name: name}
		if err := s.SaveBoard(b); err != nil {
			return nil, nil, err
		}
		s.noteArchive()
		return s, b, nil
	}
	if err != nil {
		return nil, nil, err
	}
	b, rewrite, err := Parse(data)
	if err != nil {
		return nil, nil, err
	}
	b.Name = name
	if rewrite {
		if err := s.SaveBoard(b); err != nil {
			return nil, nil, err
		}
	} else {
		s.board = stateOf(s.BoardPath(), data)
	}
	s.noteArchive()
	return s, b, nil
}

// ListBoards returns the names of every board under root — every subdirectory
// that contains a board.md — sorted alphabetically for a stable list/picker UI.
// A missing root is not an error; it simply has no boards yet.
func ListBoards(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, e.Name(), "board.md")); err == nil {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// noteArchive records the archive file's current state so that a pre-existing
// archive is not reported as a change on the first CheckReload.
func (s *Store) noteArchive() {
	if data, err := os.ReadFile(s.ArchivePath()); err == nil {
		s.archive = stateOf(s.ArchivePath(), data)
	}
}

// BoardPath is <dir>/board.md.
func (s *Store) BoardPath() string { return filepath.Join(s.dir, "board.md") }

// ArchivePath is <dir>/archive.md.
func (s *Store) ArchivePath() string { return filepath.Join(s.dir, "archive.md") }

// ArchiveDisplayPath is ArchivePath with the home directory shortened to "~".
func (s *Store) ArchiveDisplayPath() string {
	p := s.ArchivePath()
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
			return "~/" + filepath.ToSlash(rel)
		}
	}
	return p
}

// SaveBoard writes board.md atomically.
func (s *Store) SaveBoard(b *board.Board) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data := Marshal(b)
	if err := writeAtomic(s.BoardPath(), data); err != nil {
		return err
	}
	s.board = stateOf(s.BoardPath(), data)
	return nil
}

// LoadArchive reads archive.md (empty if missing).
func (s *Store) LoadArchive() (*board.Archive, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.ArchivePath())
	if errors.Is(err, fs.ErrNotExist) {
		s.archive = fileState{}
		return &board.Archive{}, nil
	}
	if err != nil {
		return nil, err
	}
	a, rewrite, err := ParseArchive(data)
	if err != nil {
		return nil, err
	}
	if rewrite {
		data = MarshalArchive(a)
		if err := writeAtomic(s.ArchivePath(), data); err != nil {
			return nil, err
		}
	}
	s.archive = stateOf(s.ArchivePath(), data)
	return a, nil
}

// SaveArchive writes archive.md atomically.
func (s *Store) SaveArchive(a *board.Archive) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data := MarshalArchive(a)
	if err := writeAtomic(s.ArchivePath(), data); err != nil {
		return err
	}
	s.archive = stateOf(s.ArchivePath(), data)
	return nil
}

// CheckReload re-reads any file whose stat changed and returns freshly parsed
// content when the bytes differ from what we last wrote or read. Unparsable
// content (a partial write) is skipped and retried on the next call.
func (s *Store) CheckReload() (Reload, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var r Reload
	data, changed, err := changedContent(s.BoardPath(), &s.board)
	if err != nil {
		return r, err
	}
	if changed {
		if b, _, perr := Parse(data); perr == nil {
			b.Name = s.name
			r.Board = b
			s.board = stateOf(s.BoardPath(), data)
		}
	}
	data, changed, err = changedContent(s.ArchivePath(), &s.archive)
	if err != nil {
		return r, err
	}
	if changed {
		if a, _, perr := ParseArchive(data); perr == nil {
			r.Archive = a
			s.archive = stateOf(s.ArchivePath(), data)
		}
	}
	return r, nil
}

// changedContent returns the file's bytes when they differ from the recorded state.
func changedContent(path string, st *fileState) ([]byte, bool, error) {
	fi, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		st.valid = false
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if st.valid && fi.ModTime().Equal(st.mtime) && fi.Size() == st.size {
		return nil, false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	if st.valid && sha256.Sum256(data) == st.hash {
		st.mtime, st.size = fi.ModTime(), fi.Size()
		return nil, false, nil
	}
	return data, true, nil
}

func stateOf(path string, data []byte) fileState {
	st := fileState{valid: true, hash: sha256.Sum256(data), size: int64(len(data))}
	if fi, err := os.Stat(path); err == nil {
		st.mtime, st.size = fi.ModTime(), fi.Size()
	}
	return st
}

// writeAtomic writes data to a temp file in the same directory and renames it over path.
func writeAtomic(path string, data []byte) (err error) {
	dir, base := filepath.Split(path)
	tmp, err := os.CreateTemp(dir, "."+base+".*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
