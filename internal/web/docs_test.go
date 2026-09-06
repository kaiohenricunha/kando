package web

import (
	"os"
	"strings"
	"testing"
)

// TestMutationRoutesAreDocumented catches route/doc drift directly: every
// path mutationRoutes (cards_test.go) exercises — the same list
// TestEveryMutationRouteIsPostOnlyAndSameOrigin enforces at the HTTP layer —
// must also appear, in its {board}/{id}/{index} pattern form, in the route
// table (spec/5-interfaces-apis.md). A route registered in server.go but
// forgotten in the spec table fails here instead of silently going stale.
//
// parity.md is deliberately not checked the same way: it documents a
// card-scoped route either by its full path or, for one already introduced
// by a fuller row above it, as "…/<action>" — a real, existing, inconsistent
// convention (e.g. "/block" only ever appears as "POST …/block"). An exact
// pattern match against that file would flag already-correct rows as
// missing, which is worse than no check; parity.md coverage is a manual
// review item instead (see the Definition of done checklist).
func TestMutationRoutesAreDocumented(t *testing.T) {
	id, archived := "aaaaaaaa", "bbbbbbbb"
	routes := mutationRoutes(id, archived)

	normalize := func(path string) string {
		path = strings.ReplaceAll(path, "life", "{board}")
		path = strings.ReplaceAll(path, id, "{id}")
		path = strings.ReplaceAll(path, archived, "{id}")
		path = strings.ReplaceAll(path, "/checklist/0", "/checklist/{index}")
		return path
	}

	specData, err := os.ReadFile("../../docs/specs/kando-web/spec/5-interfaces-apis.md")
	if err != nil {
		t.Fatal(err)
	}
	spec := string(specData)

	for _, rt := range routes {
		pattern := normalize(rt.path)
		if !strings.Contains(spec, pattern) {
			t.Errorf("route %q (pattern %q) missing from spec/5-interfaces-apis.md's route table", rt.path, pattern)
		}
	}
}
