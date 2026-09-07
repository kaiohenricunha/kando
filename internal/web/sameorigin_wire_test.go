package web

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kaiohenricunha/kando/internal/board"
)

// TestSameOriginOverARealConnection drives the guard through a real
// net/http.Server and a real TCP connection, which TestSameOriginMiddleware
// does not: that test builds requests with httptest.NewRequest and sets
// req.Host directly, so it covers the guard's decision table but never the
// server's own parsing of the Host header and the request line.
//
// That distinction is the point. sameOrigin is the whole authorization story
// for kando web — there is no login, so Host, Origin and Sec-Fetch-Site are
// the entire boundary — and it is decided from values net/http parses off the
// wire. A toolchain bump can change that parsing underneath an in-process
// test that never exercises it, which is exactly what happened to prompt this
// (the Go 1.25 -> 1.27 move in #16 shifts program-wide GODEBUG defaults, and
// net/http request-parsing changes have historically shipped that way).
//
// The two Host cases are what this test uniquely covers: httptest.NewRequest
// assigns req.Host on the struct and skips net/http's own readRequest parse,
// so nothing else in the suite exercises it. The POST cases are here because
// sameOrigin is a CSRF guard and CSRF only has teeth on a state-changing
// request — a GET carrying form-post headers would assert the outcome on a
// path the attack cannot use. The allowed POST doubles as the control that
// proves the refusals are the guard talking and not a broken fixture.
func TestSameOriginOverARealConnection(t *testing.T) {
	root := newRoot(t)
	ln, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, ln, Options{Root: root, Board: "life", Now: func() time.Time { return fixedNow }})
	}()

	// A client that never follows redirects, so the 303 a successful mutation
	// returns is not mistaken for the guard having allowed something it did
	// not — and so the allowed POST runs exactly once.
	client := &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	// The form a browser submits to create a card; the lane and title are the
	// two fields createCard requires.
	newCard := url.Values{"lane": {"todo"}, "title": {"wire test card"}}

	cases := []struct {
		name    string
		method  string // defaults to GET
		path    string // defaults to the board page
		form    url.Values
		host    string // overrides the Host header when set
		headers []string
		want    int
	}{
		{
			name: "plain request from the bound address",
			want: 200,
		},
		{
			// The shape a real browser form post takes against this app:
			// secureHeaders sets Referrer-Policy: no-referrer, so Chrome sends
			// Origin: null, and Sec-Fetch-Site says it is same-origin. A 303
			// means the card was actually created.
			name:    "browser form post with a withheld origin",
			method:  http.MethodPost,
			path:    "/b/life/cards",
			form:    newCard,
			headers: []string{"Origin", "null", "Sec-Fetch-Site", "same-origin"},
			want:    http.StatusSeeOther,
		},
		{
			// The attack sameOrigin exists to stop: a page on another origin
			// submits a form at the loopback port. This must die at the guard,
			// before createCard runs — asserted on the card count below.
			name:    "cross-origin form post to a mutation route",
			method:  http.MethodPost,
			path:    "/b/life/cards",
			form:    newCard,
			headers: []string{"Origin", "http://evil.example"},
			want:    http.StatusForbidden,
		},
		{
			name:    "forged origin",
			headers: []string{"Origin", "http://evil.example"},
			want:    http.StatusForbidden,
		},
		{
			name:    "cross-site fetch",
			headers: []string{"Sec-Fetch-Site", "cross-site"},
			want:    http.StatusForbidden,
		},
		{
			// DNS rebinding: the request reaches the loopback listener, but
			// the Host names somewhere else. This is the case that depends
			// most directly on how net/http parses Host off the wire.
			name: "foreign host header",
			host: "evil.example",
			want: http.StatusForbidden,
		},
		{
			// A Host that is loopback but on the wrong port is still not this
			// origin, and the port comes from the same parse.
			name: "right host, wrong port",
			host: "127.0.0.1:1",
			want: http.StatusForbidden,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			method := tc.method
			if method == "" {
				method = http.MethodGet
			}
			path := tc.path
			if path == "" {
				path = "/b/life"
			}
			var body io.Reader
			if tc.form != nil {
				body = strings.NewReader(tc.form.Encode())
			}
			req, err := http.NewRequest(method, "http://"+addr+path, body)
			if err != nil {
				t.Fatal(err)
			}
			if tc.form != nil {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			// req.Host overrides the Host header on the wire while the
			// connection still goes to the real listener.
			if tc.host != "" {
				req.Host = tc.host
			}
			for i := 0; i+1 < len(tc.headers); i += 2 {
				req.Header.Set(tc.headers[i], tc.headers[i+1])
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}

	// A status code alone would not prove the cross-origin POST was refused
	// rather than merely redirected somewhere: check the board itself. Exactly
	// one card must have been added — the allowed form post — so the refused
	// one never reached createCard.
	var got int
	for _, c := range reload(t, root, "life").Lanes[board.Todo] {
		if c.Title == "wire test card" {
			got++
		}
	}
	if got != 1 {
		t.Errorf("cards created = %d, want 1 (the allowed form post only)", got)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve returned %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
}
