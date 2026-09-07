package web

import (
	"os"
	"strings"
	"testing"
)

// TestMutationRoutesAreDocumented catches route/doc drift: every path
// mutationRoutes (cards_test.go) exercises — the same list
// TestEveryMutationRouteIsPostOnlyAndSameOrigin enforces at the HTTP layer —
// must also appear, in its {board}/{id}/{index} pattern form, in the route
// table (spec/5-interfaces-apis.md).
//
// What this does not do, so the next reader does not over-trust it: the
// assertion is mutationRoutes → §5, not server.go → §5. It never reads the
// mux, so a route added to server.go and forgotten in BOTH mutationRoutes and
// §5 still passes — mutationRoutes is hand-maintained, and adding a route to
// it is the step being relied on. It also covers no GET route (none are in
// mutationRoutes), does not check the method column, and matches by substring,
// so a row whose path is a prefix of another's is held up by that other row.
// Closing those needs the route list to come from the mux itself, which
// net/http does not expose.
//
// parity.md is deliberately not checked the same way: it documents a
// card-scoped route either by its full path or, for one already introduced
// by a fuller row above it, as "…/<action>" — a real, existing, inconsistent
// convention (e.g. "/block" only ever appears as "POST …/block"). An exact
// pattern match against that file would flag already-correct rows as
// missing, which is worse than no check, so parity.md stays a review item.
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
