package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
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
// valid UTF-8, no unsafe runes, no path separators, none of the URL
// delimiters "#?%&+;", and no leading dot (which also excludes "." and
// ".."). A valid name always resolves to a directory directly under root;
// what a symlink placed under root points at is the user's own business.
//
// "Unsafe" is board.UnsafeRune, the same predicate the field sanitizers use,
// rather than unicode.IsControl. A name is a stored value that every surface
// renders — the web breadcrumb and boards list, kando board list, the TUI
// header — so a bidi override in a directory name would display a board as a
// name it does not have. Checking here rather than at each of those render
// sites keeps the name clean at the source.
func ValidBoardName(name string) bool {
	if name == "" || len(name) > maxBoardName || !utf8.ValidString(name) {
		return false
	}
	if strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\#?%&+;`) {
		return false
	}
	return strings.IndexFunc(name, board.UnsafeRune) < 0
}

// Load reads root/name read-only: no directory is created, nothing is
// written, and cards missing or sharing an id get one in memory only (the next writer
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

// Version is a short token that changes whenever root/name's board.md or
// archive.md changes on disk, and stays the same while they do not. The web
// server sends it as the SSE event id, so a browser that reconnects after a
// dropped connection can be told whether it missed anything.
func Version(root, name string) string {
	dir := filepath.Join(root, name)
	h := sha256.New()
	for _, f := range []string{boardFile, archiveFile} {
		fmt.Fprintf(h, "%s\x00", f)
		if fi, err := os.Stat(filepath.Join(dir, f)); err == nil {
			fmt.Fprintf(h, "%d\x00%d\x00", fi.ModTime().UnixNano(), fi.Size())
		}
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// testHook, when non-nil, is called at a named point inside a read-then-write
// sequence, so a test can land a concurrent write exactly there. Only tests set
// it. The points:
//
//	"read"   after a file's bytes were read, before they are parsed
//	"repair" after the baseline is recorded, before the checked write
//	"wrote"  after this Store's own write, before its baseline is recorded
//	"reload" after CheckReload read a changed file, before it is parsed
var testHook func(point string)

// hook calls testHook at point, if a test has set it.
func hook(point string) {
	if f := testHook; f != nil {
		f(point)
	}
}

// maxOpenAttempts bounds how many times Open and (*Store).LoadArchive re-read a
// file whose repair was refused. A refusal means another writer landed between
// the read and the repair, and the next read sees that write, so one retry
// normally settles it; past the bound the ErrConflict is returned rather than
// spun on.
const maxOpenAttempts = 3

// Open loads (or creates) the board under root/name. Cards missing an id, or
// sharing one, are assigned one and the file is rewritten through the checked
// writer, against a baseline whose stat was taken before the read. A write the
// checked writer can detect — anything landing between Open's read and its
// repair — makes the repair refuse, and Open re-reads and repairs what it then
// finds, up to maxOpenAttempts times, returning ErrConflict past that. The repair
// is a pure function of the bytes on disk, so repairing the winner's bytes loses
// nothing.
//
// Two gaps remain, shared by every checked write in this package rather than
// introduced here: the check and the rename are separate steps, so a write
// landing between them is overwritten; and a same-size write within one mtime
// tick passes the fast path. This narrows the window for the repair. It does not
// close REL-3, which concerns the TUI's deliberately unchecked saves.
func Open(root, name string) (*Store, *board.Board, error) {
	if !ValidBoardName(name) {
		return nil, nil, fmt.Errorf("invalid board name %q", name)
	}
	s := &Store{dir: filepath.Join(root, name), name: name}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return nil, nil, err
	}
	for attempt := 1; ; attempt++ {
		// b is rebound on every attempt. After a lost race the board to return is
		// the one repaired from the winner's bytes: handing back the first read
		// would leave the caller holding a board older than the Store's baseline,
		// and its next checked save would overwrite the winner without noticing.
		b, err := s.openBoard()
		if err == nil {
			s.noteArchive()
			return s, b, nil
		}
		if !errors.Is(err, ErrConflict) || attempt == maxOpenAttempts {
			return nil, nil, err
		}
	}
}

// openBoard is one attempt at Open's read-and-repair. It records what it read as
// the Store's baseline before deciding whether to write, so the checked writer
// has something to compare against: a fresh Store's zero fileState would
// otherwise make every repair look like a conflict.
func (s *Store) openBoard() (*board.Board, error) {
	data, seen, err := readState(s.BoardPath())
	if errors.Is(err, fs.ErrNotExist) {
		// The baseline is "no file". changedContent reports a still-missing file
		// as unchanged and one that has since appeared as changed, so this creates
		// the board only if nobody else has in the meantime.
		s.board = fileState{}
		b := &board.Board{Name: s.name}
		hook("repair")
		if err := s.SaveBoardIfUnchanged(b); err != nil {
			return nil, err
		}
		return b, nil
	}
	if err != nil {
		return nil, err
	}
	b, rewrite, err := Parse(data)
	if err != nil {
		return nil, err
	}
	b.Name = s.name
	s.board = seen
	if !rewrite {
		return b, nil
	}
	hook("repair")
	if err := s.SaveBoardIfUnchanged(b); err != nil {
		return nil, err
	}
	return b, nil
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
	if _, seen, err := readState(s.ArchivePath()); err == nil {
		s.archive = seen
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

// SaveRestoreIfUnchanged is SaveRestore for a caller that loaded, edited and
// is writing back both files — kando archive restore: it refuses with
// ErrConflict, touching neither file, when board.md or archive.md no longer
// holds the bytes this Store last read or wrote.
//
// SaveRestore itself stays unchecked because the TUI wants it that way (it is
// the surface that wins on purpose) and the web serialises its own restores
// behind a write lock. A one-shot CLI process has neither, so it needs this:
// without it a concurrent edit landing between the open and the save is
// silently overwritten, which is the one thing every other CLI write path
// refuses to do.
func (s *Store) SaveRestoreIfUnchanged(b *board.Board, a *board.Archive) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, changed, err := changedContent(s.BoardPath(), &s.board); err != nil {
		return err
	} else if changed {
		return ErrConflict
	}
	if _, changed, err := changedContent(s.ArchivePath(), &s.archive); err != nil {
		return err
	} else if changed {
		return ErrConflict
	}
	if err := s.saveBoard(b); err != nil {
		return err
	}
	return s.saveArchive(a)
}

// SaveArchival writes both files of an archive move in the order that fails
// safely — the inverse of SaveRestore: archive.md first, so a failure leaves
// the card in both files (a duplicate the restore and archive guards both
// refuse until one copy is deleted) and never in neither. The TUI calls it:
// the unchecked writer that wins on purpose.
func (s *Store) SaveArchival(b *board.Board, a *board.Archive) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveArchival(b, a)
}

// SaveArchivalIfUnchanged is SaveArchival for a caller that loaded, edited
// and is writing back both files — the web's archive route: it refuses with
// ErrConflict, touching neither file, when board.md or archive.md no longer
// holds the bytes this Store last read or wrote. Load the archive through
// (*Store).LoadArchive first so its state is on record.
//
// A missing archive.md is not a conflict, and that covers deleted as well as
// never created: changedContent reports fs.ErrNotExist as unchanged, so an
// archive.md removed between the load and this call is re-created from the
// copy held in memory rather than refused. The asymmetry is deliberate and
// matches the write order — an archived card can come back, but it is never
// silently dropped.
func (s *Store) SaveArchivalIfUnchanged(b *board.Board, a *board.Archive) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, changed, err := changedContent(s.BoardPath(), &s.board); err != nil {
		return err
	} else if changed {
		return ErrConflict
	}
	if _, changed, err := changedContent(s.ArchivePath(), &s.archive); err != nil {
		return err
	} else if changed {
		return ErrConflict
	}
	return s.saveArchival(b, a)
}

// saveArchival writes archive.md, then board.md; the caller holds s.mu.
func (s *Store) saveArchival(b *board.Board, a *board.Archive) error {
	if err := s.saveArchive(a); err != nil {
		return err
	}
	return s.saveBoard(b)
}

// saveBoard writes board.md; the caller holds s.mu.
func (s *Store) saveBoard(b *board.Board) error {
	data := Marshal(b)
	if err := writeAtomic(s.BoardPath(), data); err != nil {
		return err
	}
	hook("wrote")
	s.board = stateOf(s.BoardPath(), data)
	return nil
}

// LoadArchive reads archive.md (empty if missing). Cards missing or sharing an
// id are assigned one and archive.md is rewritten with them, through the same
// checked, retried path as Open's repair and with the same remaining gaps; it
// returns ErrConflict past maxOpenAttempts.
func (s *Store) LoadArchive() (*board.Archive, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for attempt := 1; ; attempt++ {
		a, err := s.loadArchive()
		if err == nil {
			return a, nil
		}
		if !errors.Is(err, ErrConflict) || attempt == maxOpenAttempts {
			return nil, err
		}
	}
}

// loadArchive is one attempt at LoadArchive, openBoard's archive twin. The caller
// holds s.mu, so it checks and writes inline rather than through the locking
// SaveArchivalIfUnchanged.
func (s *Store) loadArchive() (*board.Archive, error) {
	data, seen, err := readState(s.ArchivePath())
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
	s.archive = seen
	if !rewrite {
		return a, nil
	}
	hook("repair")
	if _, changed, err := changedContent(s.ArchivePath(), &s.archive); err != nil {
		return nil, err
	} else if changed {
		return nil, ErrConflict
	}
	if err := s.saveArchive(a); err != nil {
		return nil, err
	}
	return a, nil
}

// SaveArchive writes archive.md atomically.
func (s *Store) SaveArchive(a *board.Archive) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveArchive(a)
}

// saveArchive writes archive.md; the caller holds s.mu.
func (s *Store) saveArchive(a *board.Archive) error {
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
	data, seen, changed, err := changedContentState(s.BoardPath(), &s.board)
	if err != nil {
		return r, err
	}
	if changed {
		hook("reload")
		if b, _, perr := Parse(data); perr == nil {
			b.Name = s.name
			r.Board = b
			s.board = seen
		}
	}
	data, seen, changed, err = changedContentState(s.ArchivePath(), &s.archive)
	if err != nil {
		return r, err
	}
	if changed {
		if a, _, perr := ParseArchive(data); perr == nil {
			r.Archive = a
			s.archive = seen
		}
	}
	return r, nil
}

// changedContent returns the file's bytes when they differ from the recorded state.
func changedContent(path string, st *fileState) ([]byte, bool, error) {
	data, _, changed, err := changedContentState(path, st)
	return data, changed, err
}

// changedContentState is changedContent that also returns, for a changed file,
// the baseline to record for the bytes it read: their hash with the stat taken
// before the read, for the reason readState gives.
func changedContentState(path string, st *fileState) ([]byte, fileState, bool, error) {
	fi, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		st.valid = false
		return nil, fileState{}, false, nil
	}
	if err != nil {
		return nil, fileState{}, false, err
	}
	if st.valid && fi.ModTime().Equal(st.mtime) && fi.Size() == st.size {
		return nil, fileState{}, false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fileState{}, false, err
	}
	sum := sha256.Sum256(data)
	if st.valid && sum == st.hash {
		st.mtime, st.size = fi.ModTime(), fi.Size()
		return nil, fileState{}, false, nil
	}
	return data, fileState{valid: true, hash: sum, mtime: fi.ModTime(), size: fi.Size()}, true, nil
}

// readState reads path and returns its bytes with the baseline to record for
// them. The stat is taken BEFORE the read. A write landing after that stat shows
// up at the next check as a changed stat, which forces a hash comparison, and
// the hash then tells the truth either way. Taken after the read instead, a
// write landing in between pairs the old bytes' hash with the new file's stat,
// and changedContent's fast path calls the file unchanged.
func readState(path string) ([]byte, fileState, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fileState{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fileState{}, err
	}
	hook("read")
	return data, fileState{valid: true, hash: sha256.Sum256(data), mtime: fi.ModTime(), size: fi.Size()}, nil
}

// stateOf records the baseline for bytes this Store has just written to path.
// Its stat can only follow the write, so a write by someone else landing in
// between would pair our hash with their stat. When the sizes disagree that is
// detectable: the mtime is left zero, changedContent never takes its fast path on
// it, and the next check compares hashes instead. A same-size write in that gap
// is not detectable this way; see Open's doc for the gaps that remain.
func stateOf(path string, data []byte) fileState {
	st := fileState{valid: true, hash: sha256.Sum256(data), size: int64(len(data))}
	if fi, err := os.Stat(path); err == nil && fi.Size() == st.size {
		st.mtime = fi.ModTime()
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
