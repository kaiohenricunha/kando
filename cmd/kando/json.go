package main

import (
	"encoding/json"
	"io"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
)

// itemJSON is one checklist entry in --json output.
type itemJSON struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// cardJSON is the stable shape every --json verb emits a card as. Times are
// RFC 3339, omitted when zero. Lane is omitted for an archived card, which
// has none.
//
// The offset is the reading machine's, not the file's: store.formatTime
// writes a bare 2006-01-02 for any midnight stamp, and parseTime reads that
// back with time.ParseInLocation(..., time.Local), so one board.md yields
// 2026-09-01T00:00:00-03:00 here and 2026-09-01T00:00:00+02:00 elsewhere.
// Whether to normalise to UTC or to pass the stamp through date-only is a
// decision for the unit that wires the first --json verb, since it is that
// output's contract to keep.
type cardJSON struct {
	ID            string     `json:"id"`
	Title         string     `json:"title"`
	Lane          string     `json:"lane,omitempty"`
	Tag           string     `json:"tag,omitempty"`
	Notes         string     `json:"notes,omitempty"`
	Blocked       bool       `json:"blocked"`
	BlockedReason string     `json:"blocked_reason,omitempty"`
	Checklist     []itemJSON `json:"checklist"`
	Created       string     `json:"created,omitempty"`
	Moved         string     `json:"moved,omitempty"`
	Done          string     `json:"done,omitempty"`
	Age           string     `json:"age,omitempty"`
}

// cardToJSON projects c as of now. lane < 0 (board.Lane's zero value,
// Backlog, is a real lane, so callers pass -1 explicitly for "no lane" —
// an archived card) omits Lane.
func cardToJSON(lane board.Lane, c *board.Card, now time.Time) cardJSON {
	items := make([]itemJSON, len(c.Checklist))
	for i, it := range c.Checklist {
		items[i] = itemJSON{Text: it.Text, Done: it.Done}
	}
	cj := cardJSON{
		ID: c.ID, Title: c.Title, Tag: c.Tag, Notes: c.Notes,
		Blocked: c.Blocked, BlockedReason: c.BlockedReason,
		Checklist: items,
	}
	if lane >= 0 {
		cj.Lane = lane.String()
	}
	if !c.CreatedAt.IsZero() {
		cj.Created = c.CreatedAt.Format(time.RFC3339)
	}
	if !c.MovedAt.IsZero() {
		cj.Moved = c.MovedAt.Format(time.RFC3339)
	}
	if !c.DoneAt.IsZero() {
		cj.Done = c.DoneAt.Format(time.RFC3339)
	}
	if age := c.AgeSince(); !age.IsZero() {
		cj.Age = board.Age(now, age)
	}
	return cj
}

// laneJSON is one lane's cards in `kando list --json`.
type laneJSON struct {
	Lane  string     `json:"lane"`
	Cards []cardJSON `json:"cards"`
}

// listJSON is `kando list --json`'s whole output. The intent is that all four
// lanes are always present, empty ones carrying an empty Cards slice rather
// than being omitted, so a script never has to special-case a lane with
// nothing in it.
//
// Nothing here enforces that yet: these are plain slices, and a nil one
// encodes as null, not []. cardJSON keeps its equivalent promise because
// cardToJSON builds the checklist with make (pinned by TestCardToJSON); the
// container types have no constructor because they have no caller. The unit
// that adds `kando list --json` owns making the guarantee real — a
// newLaneJSON-style constructor, plus a test that encodes an empty board and
// asserts no null appears.
type listJSON struct {
	Board   string     `json:"board"`
	Filter  string     `json:"filter,omitempty"`
	Matched int        `json:"matched"`
	Total   int        `json:"total"`
	Lanes   []laneJSON `json:"lanes"`
}

// boardJSON is one board's summary in `kando board list --json`. Lanes'
// keys are Lane.Key() (lowercase, e.g. "todo"), matching the web's own
// lane-key vocabulary.
type boardJSON struct {
	Name  string         `json:"name"`
	Cards int            `json:"cards"`
	Lanes map[string]int `json:"lanes"`
}

// archiveGroupJSON is one week bucket in `kando archive list --json`.
type archiveGroupJSON struct {
	Label string     `json:"label"`
	Cards []cardJSON `json:"cards"`
}

// archiveListJSON is `kando archive list --json`'s whole output, mirroring
// board.ArchiveView's own groups/matched/scanned/total shape.
type archiveListJSON struct {
	Board   string             `json:"board"`
	Filter  string             `json:"filter,omitempty"`
	Matched int                `json:"matched"`
	Scanned int                `json:"scanned"`
	Total   int                `json:"total"`
	Groups  []archiveGroupJSON `json:"groups"`
}

// writeJSON encodes v as indented JSON without escaping HTML characters
// (this is a terminal, not a browser) and without a trailing extra blank
// line beyond json.Encoder's own newline.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
