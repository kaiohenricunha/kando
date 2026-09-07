package main

import (
	"strings"
	"testing"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
)

func TestCardToJSON(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

	t.Run("empty fields are omitted, checklist is never null", func(t *testing.T) {
		c := &board.Card{ID: "aaaaaaaa", Title: "Bare card"}
		cj := cardToJSON(board.Todo, c, now)
		if cj.Tag != "" || cj.Notes != "" || cj.BlockedReason != "" || cj.Created != "" || cj.Moved != "" || cj.Done != "" || cj.Age != "" {
			t.Errorf("expected empty fields, got %+v", cj)
		}
		if cj.Checklist == nil || len(cj.Checklist) != 0 {
			t.Errorf("Checklist should be an empty slice, not nil: %#v", cj.Checklist)
		}
		if cj.Lane != "Todo" {
			t.Errorf("Lane = %q, want %q", cj.Lane, "Todo")
		}
	})

	t.Run("negative lane omits Lane", func(t *testing.T) {
		c := &board.Card{ID: "aaaaaaaa", Title: "Archived"}
		cj := cardToJSON(-1, c, now)
		if cj.Lane != "" {
			t.Errorf("Lane = %q, want empty for an archived card", cj.Lane)
		}
	})

	t.Run("times are RFC3339, age is set", func(t *testing.T) {
		created := now.AddDate(0, 0, -5)
		c := &board.Card{ID: "aaaaaaaa", Title: "Aged", CreatedAt: created, MovedAt: created}
		cj := cardToJSON(board.Todo, c, now)
		if cj.Created != created.Format(time.RFC3339) {
			t.Errorf("Created = %q, want %q", cj.Created, created.Format(time.RFC3339))
		}
		if cj.Age != "5d" {
			t.Errorf("Age = %q, want %q", cj.Age, "5d")
		}
	})

	t.Run("checklist and blocked round-trip", func(t *testing.T) {
		c := &board.Card{
			ID: "aaaaaaaa", Title: "Full", Tag: "errand", Notes: "line1\nline2",
			Blocked: true, BlockedReason: "waiting",
			Checklist: []board.Item{{Text: "a", Done: true}, {Text: "b", Done: false}},
		}
		cj := cardToJSON(board.Doing, c, now)
		if cj.Tag != "errand" || cj.Notes != "line1\nline2" || !cj.Blocked || cj.BlockedReason != "waiting" {
			t.Errorf("got %+v", cj)
		}
		if len(cj.Checklist) != 2 || cj.Checklist[0] != (itemJSON{"a", true}) || cj.Checklist[1] != (itemJSON{"b", false}) {
			t.Errorf("checklist: %+v", cj.Checklist)
		}
	})

	t.Run("no HTML escaping, no null checklist in the encoded output", func(t *testing.T) {
		c := &board.Card{ID: "aaaaaaaa", Title: "<b>&Title</b>"}
		cj := cardToJSON(board.Todo, c, now)
		var sb strings.Builder
		if err := writeJSON(&sb, cj); err != nil {
			t.Fatal(err)
		}
		out := sb.String()
		// SetEscapeHTML(false) means these characters pass through raw; with
		// the default encoder this substring would be <-escaped instead
		// and this check would fail.
		if !strings.Contains(out, "<b>&Title</b>") {
			t.Errorf("HTML characters should pass through unescaped, got: %s", out)
		}
		if strings.Contains(out, `"checklist": null`) {
			t.Errorf("checklist must never encode as null: %s", out)
		}
	})
}
