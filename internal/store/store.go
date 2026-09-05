package store

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kaiohenricunha/kando/internal/board"
)

const (
	boardFile   = "board.md"
	archiveFile = "archive.md"
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

// maxBoardName caps a board name's length in bytes.
const maxBoardName = 64

// ValidBoardName reports whether name is acceptable as a board directory
// name and as a URL path segment: non-empty, at most maxBoardName bytes,
// valid UTF-8, no control runes, no path separators, none of the URL
// delimiters "#?%&+;", and no leading dot (which also excludes "." and
// ".."). A valid name always resolves to a directory directly under root;
// what a symlink placed under root points at is the user's own business.
func ValidBoardName(name string) bool {
	if name == "" || len(name) > maxBoardName || !utf8.ValidString(name) {
		return false
	}
	if strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\#?%&+;`) {
		return false
	}
	return strings.IndexFunc(name, unicode.IsControl) < 0
}

// Load reads root/name read-only: no directory is created, nothing is
// written, and cards missing an id get one in memory only (the next writer
// persists them). A missing board is fs.ErrNotExist. This is the read path
// for GET handlers; Open is the create-or-repair path for writers.
func Load(root, name string) (*board.Board, error) {
	if !ValidBoardName(name) {
		return nil, fmt.Errorf("invalid board name %q", name)
	}
	data, err := os.ReadFile(filepath.Join(root, name, boardFile))
	if err != nil {
		return nil, err
	}
	b, _, err := Parse(data)
	if err != nil {
		return nil, err
	}
	b.Name = name
	return b, nil
}

// LoadArchive reads root/name/archive.md read-only, the archive counterpart
// of Load: empty when the file is missing, never rewritten, ids assigned in
// memory only. Unlike (*Store).LoadArchive it is safe on a GET.
func LoadArchive(root, name string) (*board.Archive, error) {
	if !ValidBoardName(name) {
		return nil, fmt.Errorf("invalid board name %q", name)
	}
	data, err := os.ReadFile(filepath.Join(root, name, archiveFile))
	if errors.Is(err, fs.ErrNotExist) {
		return &board.Archive{}, nil
	}
	if err != nil {
		return nil, err
	}
	a, _, err := ParseArchive(data)
	return a, err
}

// Open loads (or creates) the board under root/name. Cards missing an id are
// assigned one and the file is rewritten once.
func Open(root, name string) (*Store, *board.Board, error) {
	if !ValidBoardName(name) {
		return nil, nil, fmt.Errorf("invalid board name %q", name)
	}
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

// Exists reports whether root/name is an openable board: a valid name whose
// board.md is present. It never creates anything, unlike Open.
func Exists(root, name string) bool {
	if !ValidBoardName(name) {
		return false
	}
	_, err := os.Stat(filepath.Join(root, name, boardFile))
	return err == nil
}

// ListBoards returns the names of every board under root — every subdirectory
// with a valid board name (see ValidBoardName) that contains a board.md —
// sorted alphabetically for a stable list/picker UI, so the listed set is
// exactly the set Open accepts. A missing root is not an error; it simply
// has no boards yet. Names only:
// callers wanting more than a name (card counts, previews) must not call Open
// per listed board — Open creates or rewrites board.md, turning a read into a
// write.
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
		if !ValidBoardName(e.Name()) {
			continue
		}
		// os.Stat follows symlinks, so a symlinked board directory is listed too
		// — the same directories Open can open. A plain file at root has no
		// <name>/board.md and is naturally skipped; no IsDir check is needed.
		if _, err := os.Stat(filepath.Join(root, e.Name(), boardFile)); err == nil {
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
func (s *Store) BoardPath() string { return filepath.Join(s.dir, boardFile) }

// ArchivePath is <dir>/archive.md.
func (s *Store) ArchivePath() string { return filepath.Join(s.dir, archiveFile) }

// ArchiveDisplayPath is root/name/archive.md with the home directory
// shortened to "~", for a surface that wants to tell the user where the
// entries it is not showing live.
func ArchiveDisplayPath(root, name string) string {
	return displayPath(filepath.Join(root, name, archiveFile))
}

// ArchiveDisplayPath is ArchivePath with the home directory shortened to "~".
func (s *Store) ArchiveDisplayPath() string { return displayPath(s.ArchivePath()) }

func displayPath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
			return "~/" + filepath.ToSlash(rel)
		}
	}
	return p
}

// ErrConflict says the file changed underneath a read-modify-write: the
// board on disk is no longer the one the caller loaded, so saving it would
// silently drop whoever wrote in between.
var ErrConflict = errors.New("the board changed on disk since it was loaded")

// SaveBoard writes board.md atomically.
func (s *Store) SaveBoard(b *board.Board) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveBoard(b)
}

// SaveBoardIfUnchanged is SaveBoard for a caller that loaded, edited and is
// now writing back the whole file: it refuses with ErrConflict when
// board.md no longer holds the bytes this Store last read or wrote. The web
// server needs it because each request opens its own Store, so nothing else
// stops two overlapping requests from each writing a whole board built
// before the other's edit.
func (s *Store) SaveBoardIfUnchanged(b *board.Board) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, changed, err := changedContent(s.BoardPath(), &s.board); err != nil {
		return err
	} else if changed {
		return ErrConflict
	}
	return s.saveBoard(b)
}

// SaveRestore writes both files of a restore in the order that fails safely:
// board.md first, so a failure leaves the card in both files (a duplicate
// the user can see and delete) and never in neither. Both surfaces call it,
// so the same action has the same failure mode in the terminal and the
// browser.
func (s *Store) SaveRestore(b *board.Board, a *board.Archive) error {
	if err := s.SaveBoard(b); err != nil {
		return err
	}
	return s.SaveArchive(a)
}

// saveBoard writes board.md; the caller holds s.mu.
func (s *Store) saveBoard(b *board.Board) error {
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
